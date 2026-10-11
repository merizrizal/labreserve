package bookings

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	cancellationOwnerID = "11111111-1111-4111-8111-111111111111"
	cancellationOtherID = "44444444-4444-4444-8444-444444444444"
	cancellationBookID  = "55555555-5555-4555-8555-555555555555"
)

func TestCancelTransitionsOwnedBookingAndAppendsOneEvent(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 123456789, time.UTC)
	repository := newFakeCancellationRepository(testCancellationBooking(now.Add(time.Hour)))
	service := NewCancellationService(repository, func() time.Time {
		repository.transactions[0].trace = append(repository.transactions[0].trace, "clock")
		return now
	})

	result, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
	if err != nil {
		t.Fatalf("cancel owned booking: %v", err)
	}
	if !result.Changed || result.Booking.State != StateCancelled || result.Booking.CancelledByAccountID == nil ||
		*result.Booking.CancelledByAccountID != cancellationOwnerID || result.Booking.CancelledAt == nil ||
		!result.Booking.CancelledAt.Equal(now.Truncate(time.Microsecond)) {
		t.Fatalf("cancellation result = %+v, want a retained booking with canonical cancellation metadata", result)
	}
	if len(repository.events) != 1 || repository.events[0].ActorAccountID != cancellationOwnerID ||
		repository.events[0].Booking.ID != cancellationBookID || !repository.events[0].OccurredAt.Equal(*result.Booking.CancelledAt) {
		t.Fatalf("cancellation events = %+v, want one event for the owner and retained booking", repository.events)
	}
	wantTrace := []string{"begin", "lookup", "resource", "booking", "clock", "update", "event", "commit"}
	if !reflect.DeepEqual(repository.transactions[0].trace, wantTrace) {
		t.Fatalf("cancellation transaction order = %v, want %v", repository.transactions[0].trace, wantTrace)
	}
}

func TestCancelUsesLockedStateAndOwnerBeforeNoOp(t *testing.T) {
	now := time.Date(2040, time.January, 2, 12, 0, 0, 0, time.UTC)
	cancelledAt := now.Add(-time.Hour)
	cancelledBy := cancellationOwnerID
	booking := testCancellationBooking(now.Add(-time.Hour))
	booking.State = StateCancelled
	booking.CancelledByAccountID = &cancelledBy
	booking.CancelledAt = &cancelledAt

	t.Run("authorized repeat after original start is a no-op", func(t *testing.T) {
		repository := newFakeCancellationRepository(booking)
		clockCalls := 0
		service := NewCancellationService(repository, func() time.Time {
			clockCalls++
			return now
		})
		result, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
		if err != nil || result.Changed || !reflect.DeepEqual(result.Booking, booking) {
			t.Fatalf("cancelled replay = %+v, %v; want unchanged successful no-op", result, err)
		}
		if clockCalls != 0 || len(repository.events) != 0 || !reflect.DeepEqual(repository.booking, booking) {
			t.Fatalf("no-op changed state/history or read time: clock=%d events=%d booking=%+v", clockCalls, len(repository.events), repository.booking)
		}
	})

	t.Run("owner is rechecked after row lock", func(t *testing.T) {
		repository := newFakeCancellationRepository(booking)
		repository.lookupAccessible = true
		repository.lookupOwnerID = cancellationOwnerID
		repository.booking.OwnerAccountID = cancellationOtherID
		service := NewCancellationService(repository, func() time.Time { return now })
		_, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
		if !errors.Is(err, ErrBookingNotAccessible) {
			t.Fatalf("locked owner mismatch error = %v, want inaccessible", err)
		}
		if len(repository.events) != 0 || repository.booking.State != StateCancelled {
			t.Fatalf("owner mismatch changed booking or history: %+v, events=%d", repository.booking, len(repository.events))
		}
	})
}

func TestCancelDoesNotDiscloseUnknownOrOtherOwnersBookings(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		bookingID string
		actorID   string
	}{
		{name: "unknown booking", bookingID: "66666666-6666-4666-8666-666666666666", actorID: cancellationOwnerID},
		{name: "another owner's booking", bookingID: cancellationBookID, actorID: cancellationOtherID},
		{name: "malformed target", bookingID: "not-a-booking-id", actorID: cancellationOwnerID},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newFakeCancellationRepository(testCancellationBooking(now.Add(time.Hour)))
			service := NewCancellationService(repository, func() time.Time { return now })
			_, err := service.Cancel(context.Background(), Actor{AccountID: test.actorID}, test.bookingID)
			if !errors.Is(err, ErrBookingNotAccessible) {
				t.Fatalf("inaccessible booking error = %v, want indistinguishable not-found outcome", err)
			}
			if len(repository.events) != 0 || repository.booking.State != StateConfirmed {
				t.Fatalf("inaccessible attempt changed booking/history: %+v, events=%d", repository.booking, len(repository.events))
			}
		})
	}
}

func TestCancelStrictStartBoundaryUsesMicrosecondPrecision(t *testing.T) {
	start := time.Date(2040, time.January, 2, 11, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		now     time.Time
		wantErr error
	}{
		{name: "one microsecond before", now: start.Add(-time.Microsecond)},
		{name: "exactly at start", now: start, wantErr: ErrCancellationStarted},
		{name: "after start", now: start.Add(time.Microsecond), wantErr: ErrCancellationStarted},
		{name: "after end", now: start.Add(time.Hour + time.Microsecond), wantErr: ErrCancellationStarted},
	} {
		t.Run(test.name, func(t *testing.T) {
			booking := testCancellationBooking(start)
			repository := newFakeCancellationRepository(booking)
			service := NewCancellationService(repository, func() time.Time { return test.now })
			result, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || repository.booking.State != StateConfirmed || len(repository.events) != 0 {
					t.Fatalf("boundary cancellation = %+v, %v; booking=%+v events=%d", result, err, repository.booking, len(repository.events))
				}
				return
			}
			if err != nil || !result.Changed || repository.booking.State != StateCancelled || len(repository.events) != 1 {
				t.Fatalf("one-microsecond-before cancellation = %+v, %v; want one transition/event", result, err)
			}
		})
	}
}

func TestCancelCapturesClockAfterBookingLock(t *testing.T) {
	start := time.Date(2040, time.January, 2, 11, 0, 0, 0, time.UTC)
	now := start.Add(-time.Microsecond)
	repository := newFakeCancellationRepository(testCancellationBooking(start))
	repository.onBookingLock = func() { now = start }
	service := NewCancellationService(repository, func() time.Time {
		repository.transactions[0].trace = append(repository.transactions[0].trace, "clock")
		return now
	})

	_, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
	if !errors.Is(err, ErrCancellationStarted) || repository.booking.State != StateConfirmed || len(repository.events) != 0 {
		t.Fatalf("post-lock boundary result = %v; booking=%+v events=%d", err, repository.booking, len(repository.events))
	}
	wantTrace := []string{"begin", "lookup", "resource", "booking", "clock", "rollback"}
	if !reflect.DeepEqual(repository.transactions[0].trace, wantTrace) {
		t.Fatalf("post-lock time was not read after both locks: %v, want %v", repository.transactions[0].trace, wantTrace)
	}
}

func TestCancelRollsBackWhenActivityAppendFails(t *testing.T) {
	now := time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
	original := testCancellationBooking(now.Add(time.Hour))
	repository := newFakeCancellationRepository(original)
	repository.eventError = errors.New("injected activity failure")
	service := NewCancellationService(repository, func() time.Time { return now })

	_, err := service.Cancel(context.Background(), Actor{AccountID: cancellationOwnerID}, cancellationBookID)
	if !errors.Is(err, ErrOperational) || !reflect.DeepEqual(repository.booking, original) || len(repository.events) != 0 {
		t.Fatalf("failed event append = %v; booking=%+v events=%d, want rollback", err, repository.booking, len(repository.events))
	}
}

func testCancellationBooking(start time.Time) Booking {
	return Booking{
		ID: cancellationBookID, ResourceID: testResourceID, OwnerAccountID: cancellationOwnerID,
		RequestID: testRequestID, StartAt: start, EndAt: start.Add(time.Hour), Purpose: "retained purpose",
		State: StateConfirmed, CreatedAt: start.Add(-24 * time.Hour),
	}
}

type fakeCancellationRepository struct {
	booking          Booking
	lookupOwnerID    string
	lookupAccessible bool
	resourceFound    bool
	bookingFound     bool
	transactions     []*fakeCancellationTransaction
	events           []BookingCancelledEvent
	eventError       error
	onBookingLock    func()
}

func newFakeCancellationRepository(booking Booking) *fakeCancellationRepository {
	return &fakeCancellationRepository{
		booking: booking, lookupOwnerID: booking.OwnerAccountID,
		lookupAccessible: true, resourceFound: true, bookingFound: true,
	}
}

func (r *fakeCancellationRepository) BeginBookingCancellation(context.Context) (CancellationTransaction, error) {
	tx := &fakeCancellationTransaction{repository: r}
	r.transactions = append(r.transactions, tx)
	tx.trace = append(tx.trace, "begin")
	return tx, nil
}

type fakeCancellationTransaction struct {
	repository     *fakeCancellationRepository
	trace          []string
	pendingBooking *Booking
	pendingEvent   *BookingCancelledEvent
	committed      bool
}

func (tx *fakeCancellationTransaction) FindBookingResource(_ context.Context, bookingID, ownerID string) (string, bool, error) {
	tx.trace = append(tx.trace, "lookup")
	if bookingID != tx.repository.booking.ID || ownerID != tx.repository.lookupOwnerID || !tx.repository.lookupAccessible {
		return "", false, nil
	}
	return tx.repository.booking.ResourceID, true, nil
}

func (tx *fakeCancellationTransaction) LockResource(context.Context, string) (bool, error) {
	tx.trace = append(tx.trace, "resource")
	return tx.repository.resourceFound, nil
}

func (tx *fakeCancellationTransaction) LockBooking(_ context.Context, _ string) (Booking, bool, error) {
	tx.trace = append(tx.trace, "booking")
	if tx.repository.onBookingLock != nil {
		tx.repository.onBookingLock()
	}
	return tx.repository.booking, tx.repository.bookingFound, nil
}

func (tx *fakeCancellationTransaction) UpdateBookingCancellation(_ context.Context, _, actorID string, at time.Time) error {
	tx.trace = append(tx.trace, "update")
	booking := tx.repository.booking
	booking.State = StateCancelled
	booking.CancelledByAccountID = &actorID
	booking.CancelledAt = &at
	tx.pendingBooking = &booking
	return nil
}

func (tx *fakeCancellationTransaction) AppendBookingCancelledEvent(_ context.Context, event BookingCancelledEvent) error {
	tx.trace = append(tx.trace, "event")
	if tx.repository.eventError != nil {
		return tx.repository.eventError
	}
	tx.pendingEvent = &event
	return nil
}

func (tx *fakeCancellationTransaction) Commit(context.Context) error {
	tx.trace = append(tx.trace, "commit")
	if tx.pendingBooking != nil {
		tx.repository.booking = *tx.pendingBooking
	}
	if tx.pendingEvent != nil {
		tx.repository.events = append(tx.repository.events, *tx.pendingEvent)
	}
	tx.committed = true
	return nil
}

func (tx *fakeCancellationTransaction) Rollback(context.Context) error {
	if tx.committed {
		return nil
	}
	tx.trace = append(tx.trace, "rollback")
	return nil
}
