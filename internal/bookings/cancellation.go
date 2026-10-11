package bookings

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBookingNotAccessible = errors.New("booking not found or not accessible")
	ErrCancellationStarted  = errors.New("booking is no longer cancellable")
)

type BookingCancelledEvent struct {
	ActorAccountID string
	Booking        Booking
	OccurredAt     time.Time
}

type CancellationResult struct {
	Booking Booking
	Changed bool
}

type CancellationRepository interface {
	BeginBookingCancellation(context.Context) (CancellationTransaction, error)
}

type CancellationTransaction interface {
	FindBookingResource(context.Context, string, string) (string, bool, error)
	LockResource(context.Context, string) (bool, error)
	LockBooking(context.Context, string) (Booking, bool, error)
	UpdateBookingCancellation(context.Context, string, string, time.Time) error
	AppendBookingCancelledEvent(context.Context, BookingCancelledEvent) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type CancellationService struct {
	repository CancellationRepository
	now        func() time.Time
}

func NewCancellationService(repository CancellationRepository, now func() time.Time) *CancellationService {
	if now == nil {
		now = time.Now
	}
	return &CancellationService{repository: repository, now: now}
}

// Cancel permits only the authenticated booking owner to cancel a future
// Confirmed booking. The actor is trusted application context, never request data.
func (s *CancellationService) Cancel(ctx context.Context, actor Actor, bookingID string) (CancellationResult, error) {
	if s == nil || s.repository == nil {
		return CancellationResult{}, operationalError("initialize cancellation service")
	}
	ownerID, ownerOK := canonicalUUID(actor.AccountID)
	bookingID, bookingOK := canonicalUUID(bookingID)
	if !ownerOK || !bookingOK {
		return CancellationResult{}, ErrBookingNotAccessible
	}

	tx, err := s.repository.BeginBookingCancellation(ctx)
	if err != nil {
		return CancellationResult{}, operationalError("begin cancellation transaction")
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	resourceID, accessible, err := tx.FindBookingResource(ctx, bookingID, ownerID)
	if err != nil {
		return CancellationResult{}, operationalError("resolve owned booking resource")
	}
	if !accessible {
		return CancellationResult{}, ErrBookingNotAccessible
	}

	locked, err := tx.LockResource(ctx, resourceID)
	if err != nil {
		return CancellationResult{}, operationalError("lock cancellation resource")
	}
	if !locked {
		return CancellationResult{}, ErrBookingNotAccessible
	}

	booking, found, err := tx.LockBooking(ctx, bookingID)
	if err != nil {
		return CancellationResult{}, operationalError("lock cancellation booking")
	}
	if !found || booking.OwnerAccountID != ownerID {
		return CancellationResult{}, ErrBookingNotAccessible
	}
	if booking.State == StateCancelled {
		if err := tx.Commit(ctx); err != nil {
			return CancellationResult{}, operationalError("commit cancellation no-op")
		}
		return CancellationResult{Booking: booking}, nil
	}
	if booking.State != StateConfirmed {
		return CancellationResult{}, operationalError("read cancellation booking state")
	}

	// Read fresh authoritative time only after both Resource and Booking locks.
	now := canonicalInstant(s.now())
	if !now.Before(canonicalInstant(booking.StartAt)) {
		return CancellationResult{}, ErrCancellationStarted
	}
	if err := tx.UpdateBookingCancellation(ctx, bookingID, ownerID, now); err != nil {
		return CancellationResult{}, operationalError("update cancellation state")
	}
	booking.State = StateCancelled
	booking.CancelledByAccountID = &ownerID
	booking.CancelledAt = &now
	if err := tx.AppendBookingCancelledEvent(ctx, BookingCancelledEvent{
		ActorAccountID: ownerID, Booking: booking, OccurredAt: now,
	}); err != nil {
		return CancellationResult{}, operationalError("append cancellation activity event")
	}
	if err := tx.Commit(ctx); err != nil {
		return CancellationResult{}, operationalError("commit cancellation transaction")
	}
	return CancellationResult{Booking: booking, Changed: true}, nil
}
