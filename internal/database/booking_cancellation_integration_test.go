package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
)

func TestOwnerCancellationRetainsBookingReleasesIntervalAndPreservesReplay(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 2, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create booking to cancel: %v", err)
	}
	original := created.Booking
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })

	result, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, original.ID)
	if err != nil {
		t.Fatalf("cancel owner's future booking: %v", err)
	}
	if !result.Changed || result.Booking.ID != original.ID || result.Booking.ResourceID != original.ResourceID ||
		result.Booking.OwnerAccountID != original.OwnerAccountID || result.Booking.RequestID != original.RequestID ||
		!result.Booking.StartAt.Equal(original.StartAt) || !result.Booking.EndAt.Equal(original.EndAt) ||
		result.Booking.Purpose != original.Purpose || !result.Booking.CreatedAt.Equal(original.CreatedAt) ||
		result.Booking.State != bookings.StateCancelled || result.Booking.CancelledByAccountID == nil ||
		*result.Booking.CancelledByAccountID != fixtures.accounts[0] || result.Booking.CancelledAt == nil ||
		!result.Booking.CancelledAt.Equal(now) {
		t.Fatalf("cancellation did not retain the original Booking and set metadata: %+v", result)
	}

	var resourceID, ownerID, requestID, purpose, state, cancelledBy string
	var startAt, endAt, createdAt, cancelledAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT resource_id::text, owner_account_id::text, request_id::text,
		       start_at, end_at, purpose, state, created_at,
		       cancelled_by_account_id::text, cancelled_at
		FROM bookings WHERE id = $1`, original.ID).Scan(
		&resourceID, &ownerID, &requestID, &startAt, &endAt, &purpose, &state,
		&createdAt, &cancelledBy, &cancelledAt); err != nil {
		t.Fatalf("read retained cancelled booking: %v", err)
	}
	if resourceID != original.ResourceID || ownerID != original.OwnerAccountID || requestID != original.RequestID ||
		!startAt.Equal(original.StartAt) || !endAt.Equal(original.EndAt) || purpose != original.Purpose ||
		state != string(bookings.StateCancelled) || !createdAt.Equal(original.CreatedAt) ||
		cancelledBy != fixtures.accounts[0] || !cancelledAt.Equal(now) {
		t.Fatalf("persisted cancellation changed immutable booking data: resource=%s owner=%s request=%s interval=%v..%v purpose=%q state=%s created=%v cancelled=%s at=%v",
			resourceID, ownerID, requestID, startAt, endAt, purpose, state, createdAt, cancelledBy, cancelledAt)
	}

	var creationEvents, cancellationEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'),
		       count(*) FILTER (WHERE action = 'booking.cancelled')
		FROM activity_events WHERE booking_id = $1`, original.ID).Scan(&creationEvents, &cancellationEvents); err != nil {
		t.Fatalf("count retained booking events: %v", err)
	}
	if creationEvents != 1 || cancellationEvents != 1 {
		t.Fatalf("booking events created/cancelled = %d/%d, want exactly one each", creationEvents, cancellationEvents)
	}
	var eventActor, eventAction, eventResource string
	var eventAt, eventStart, eventEnd time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT actor_account_id::text, action, occurred_at,
		       details->>'resource_id', (details->>'start_at')::timestamptz,
		       (details->>'end_at')::timestamptz
		FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled'`, original.ID).Scan(
		&eventActor, &eventAction, &eventAt, &eventResource, &eventStart, &eventEnd); err != nil {
		t.Fatalf("read cancellation event: %v", err)
	}
	if eventActor != fixtures.accounts[0] || eventAction != "booking.cancelled" || !eventAt.Equal(now) ||
		eventResource != original.ResourceID || !eventStart.Equal(original.StartAt) || !eventEnd.Equal(original.EndAt) {
		t.Fatalf("cancellation event does not describe the committed operation: actor=%s action=%s at=%v resource=%s interval=%v..%v",
			eventActor, eventAction, eventAt, eventResource, eventStart, eventEnd)
	}

	replacement := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	replacement.Purpose = "replacement booking"
	if _, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[1]}, replacement); err != nil {
		t.Fatalf("create overlapping booking after cancellation commit: %v", err)
	}
	replay, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil || !replay.Replayed || replay.Booking.ID != original.ID || replay.Booking.State != bookings.StateCancelled ||
		replay.Booking.CancelledByAccountID == nil || *replay.Booking.CancelledByAccountID != fixtures.accounts[0] ||
		replay.Booking.CancelledAt == nil || !replay.Booking.CancelledAt.Equal(now) {
		t.Fatalf("original creation replay after cancellation = %+v, %v; want retained cancelled booking", replay, err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 1, 1)
}

func TestOwnerCancellationAllowsOnlyEngineerAndCoordinatorOwnersAndInactiveResource(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 3, 2, func() time.Time { return now })
	if _, err := adminPool.Exec(t.Context(), "UPDATE accounts SET role = 'coordinator' WHERE id = $1", fixtures.accounts[2]); err != nil {
		t.Fatalf("prepare coordinator account: %v", err)
	}
	engineerRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	engineerBooking, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, engineerRequest)
	if err != nil {
		t.Fatalf("create Engineer booking: %v", err)
	}
	coordinatorRequest := bookingCreationRequest(fixtures.resources[1], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	coordinatorBooking, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[2]}, coordinatorRequest)
	if err != nil {
		t.Fatalf("create Coordinator's own booking: %v", err)
	}
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })

	for _, actorID := range []string{fixtures.accounts[1], fixtures.accounts[2]} {
		if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: actorID}, engineerBooking.Booking.ID); !errors.Is(err, bookings.ErrBookingNotAccessible) {
			t.Fatalf("cross-owner cancellation by %s = %v, want inaccessible", actorID, err)
		}
	}
	var rejectedCancellationEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM activity_events
		WHERE booking_id IN ($1, $2) AND action = 'booking.cancelled'`,
		engineerBooking.Booking.ID, coordinatorBooking.Booking.ID).Scan(&rejectedCancellationEvents); err != nil {
		t.Fatalf("count rejected cancellation events: %v", err)
	}
	if rejectedCancellationEvents != 0 {
		t.Fatalf("rejected cross-owner requests created %d cancellation events", rejectedCancellationEvents)
	}
	var engineerBookingState string
	if err := adminPool.QueryRow(t.Context(), "SELECT state FROM bookings WHERE id = $1", engineerBooking.Booking.ID).Scan(&engineerBookingState); err != nil {
		t.Fatalf("read booking after rejected cross-owner requests: %v", err)
	}
	if engineerBookingState != "confirmed" {
		t.Fatalf("cross-owner requests changed Engineer booking state to %s", engineerBookingState)
	}

	if _, err := adminPool.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", fixtures.resources[1]); err != nil {
		t.Fatalf("prepare inactive Resource: %v", err)
	}
	result, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[2]}, coordinatorBooking.Booking.ID)
	if err != nil || !result.Changed || result.Booking.OwnerAccountID != fixtures.accounts[2] {
		t.Fatalf("Coordinator could not cancel own booking on inactive Resource: %+v, %v", result, err)
	}
	if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[1]}, coordinatorBooking.Booking.ID); !errors.Is(err, bookings.ErrBookingNotAccessible) {
		t.Fatalf("non-owner retry of cancelled Coordinator booking = %v, want inaccessible", err)
	}
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM activity_events
		WHERE booking_id = $1 AND action = 'booking.cancelled'`, coordinatorBooking.Booking.ID).Scan(&rejectedCancellationEvents); err != nil {
		t.Fatalf("count Coordinator cancellation events: %v", err)
	}
	if rejectedCancellationEvents != 1 {
		t.Fatalf("Coordinator self-cancellation events = %d, want exactly one", rejectedCancellationEvents)
	}
}

func TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp(t *testing.T) {
	adminPool, appPool, fixtures, _ := bookingCreationSetup(t, 2, 1, func() time.Time { return bookingCreationNow() })
	start := bookingTestStart().Add(time.Hour)
	cases := []struct {
		name   string
		offset time.Duration
		ok     bool
	}{
		{name: "one microsecond before", offset: -time.Microsecond, ok: true},
		{name: "exact start", offset: 0},
		{name: "after start", offset: time.Microsecond},
		{name: "after end", offset: time.Hour + time.Microsecond},
	}
	for index, test := range cases {
		booking := fixtures.booking(t, 0, 0, start.Add(time.Duration(index*2)*time.Hour))
		if err := fixtures.insertBooking(t.Context(), booking); err != nil {
			t.Fatalf("insert %s fixture: %v", test.name, err)
		}
		cancellationTime := booking.startAt.Add(test.offset)
		canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return cancellationTime })
		result, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, bookingIDForRequest(t, adminPool, fixtures.accounts[0], booking.requestID))
		if test.ok {
			if err != nil || !result.Changed || result.Booking.State != bookings.StateCancelled {
				t.Fatalf("cancellation %s = %+v, %v; want committed transition", test.name, result, err)
			}
		} else if !errors.Is(err, bookings.ErrCancellationStarted) {
			t.Fatalf("cancellation %s error = %v, want started outcome", test.name, err)
		}
	}

	cancelled := fixtures.booking(t, 0, 0, start.Add(10*time.Hour))
	cancelled.state = "cancelled"
	cancelledBy := fixtures.accounts[0]
	cancelledAt := cancelled.startAt.Add(-time.Hour)
	cancelled.cancelledByAccountID = &cancelledBy
	cancelled.cancelledAt = &cancelledAt
	if err := fixtures.insertBooking(t.Context(), cancelled); err != nil {
		t.Fatalf("insert already-cancelled booking: %v", err)
	}
	cancelledID := bookingIDForRequest(t, adminPool, fixtures.accounts[0], cancelled.requestID)
	if err := insertCancellationTestEvent(t.Context(), adminPool, fixtures.accounts[0], cancelledID); err != nil {
		t.Fatalf("insert original cancellation event: %v", err)
	}
	var clockReads int
	retryTime := cancelled.startAt.Add(-time.Hour)
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time {
		clockReads++
		return retryTime
	})
	for _, retryOffset := range []time.Duration{-time.Hour, 0, time.Hour} {
		retryTime = cancelled.startAt.Add(retryOffset)
		if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, cancelledID); err != nil {
			t.Fatalf("authorized cancelled-state retry at %v: %v", retryTime, err)
		}
	}
	if clockReads != 0 {
		t.Fatalf("cancelled-state no-op read authoritative time %d times", clockReads)
	}
	if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[1]}, cancelledID); !errors.Is(err, bookings.ErrBookingNotAccessible) {
		t.Fatalf("non-owner cancelled-state retry = %v, want inaccessible", err)
	}
	var storedActor string
	var storedAt time.Time
	var eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT cancelled_by_account_id::text, cancelled_at,
		       (SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled')
		FROM bookings WHERE id = $1`, cancelledID).Scan(&storedActor, &storedAt, &eventCount); err != nil {
		t.Fatalf("read no-op retained state: %v", err)
	}
	if storedActor != cancelledBy || !storedAt.Equal(cancelledAt) || eventCount != 1 {
		t.Fatalf("cancelled retry changed metadata/history: actor=%s at=%v events=%d", storedActor, storedAt, eventCount)
	}
}

func TestOwnerCancellationWaitsForResourceBeforeBookingLock(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create lock-order fixture: %v", err)
	}
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })
	resourceLock := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	result := startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], created.Booking.ID)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, resourceLock.pid, 1); err != nil {
		_ = resourceLock.release()
		t.Fatalf("cancellation did not wait for Resource before booking lock: %v", err)
	}

	bookingLock := holdBookingRowLock(t, adminPool,
		"SELECT id FROM bookings WHERE id = $1 FOR UPDATE NOWAIT", created.Booking.ID)
	if err := resourceLock.release(); err != nil {
		t.Fatalf("release Resource lock: %v", err)
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelWait()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, bookingLock.pid, 1); err != nil {
		_ = bookingLock.release()
		t.Fatalf("cancellation did not acquire Resource before waiting on Booking: %v", err)
	}
	if err := bookingLock.release(); err != nil {
		t.Fatalf("release Booking lock: %v", err)
	}
	outcome := receiveBookingCancellation(t, result)
	if outcome.err != nil || !outcome.result.Changed {
		t.Fatalf("cancellation after ordered locks = %+v, %v", outcome.result, outcome.err)
	}
}

func TestConcurrentOwnerCancellationsCommitOneTransitionAndEvent(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create concurrent-cancellation fixture: %v", err)
	}
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	first := startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], created.Booking.ID)
	second := startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], created.Booking.ID)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		_ = held.release()
		t.Fatalf("both cancellation transactions did not wait on the shared Resource lock: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release concurrent-cancellation Resource lock: %v", err)
	}
	firstResult := receiveBookingCancellation(t, first)
	secondResult := receiveBookingCancellation(t, second)
	if firstResult.err != nil || secondResult.err != nil || firstResult.result.Changed == secondResult.result.Changed {
		t.Fatalf("concurrent cancellation results = (%+v, %v), (%+v, %v); want both success and one transition",
			firstResult.result, firstResult.err, secondResult.result, secondResult.err)
	}
	var state string
	var eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT state, (SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled')
		FROM bookings WHERE id = $1`, created.Booking.ID).Scan(&state, &eventCount); err != nil {
		t.Fatalf("read concurrent cancellation result: %v", err)
	}
	if state != "cancelled" || eventCount != 1 {
		t.Fatalf("concurrent cancellation persisted state/events %s/%d, want cancelled/1", state, eventCount)
	}
}

func TestOwnerCancellationSerializesWithConflictingCreation(t *testing.T) {
	for _, firstOperation := range []string{"cancellation", "creation"} {
		t.Run(firstOperation+" wins Resource lock", func(t *testing.T) {
			now := bookingCreationNow()
			adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 2, 1, func() time.Time { return now })
			originalRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
			original, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, originalRequest)
			if err != nil {
				t.Fatalf("create occupancy fixture: %v", err)
			}
			canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })
			newRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)

			held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
			var cancellation <-chan bookingCancellationOutcome
			var creation <-chan bookingCreationOutcome
			if firstOperation == "cancellation" {
				cancellation = startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], original.Booking.ID)
			} else {
				creation = startBookingCreation(t.Context(), creator, bookings.Actor{AccountID: fixtures.accounts[1]}, newRequest)
			}
			waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
				cancel()
				_ = held.release()
				t.Fatalf("first operation did not wait for Resource lock: %v", err)
			}
			cancel()
			if firstOperation == "cancellation" {
				creation = startBookingCreation(t.Context(), creator, bookings.Actor{AccountID: fixtures.accounts[1]}, newRequest)
			} else {
				cancellation = startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], original.Booking.ID)
			}
			waitCtx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
			if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
				cancel()
				_ = held.release()
				t.Fatalf("both operations did not wait on Resource lock: %v", err)
			}
			cancel()
			if err := held.release(); err != nil {
				t.Fatalf("release Resource lock: %v", err)
			}
			cancelled := receiveBookingCancellation(t, cancellation)
			created := receiveBookingCreation(t, creation)
			if cancelled.err != nil || !cancelled.result.Changed {
				t.Fatalf("owner cancellation outcome = %+v, %v", cancelled.result, cancelled.err)
			}
			if firstOperation == "cancellation" {
				if created.err != nil || created.result.Replayed {
					t.Fatalf("creation after cancellation commit = %+v, %v; want a new booking", created.result, created.err)
				}
			} else {
				if !errors.Is(created.err, bookings.ErrConflict) {
					t.Fatalf("creation before cancellation = %+v, %v; want existing conflict", created.result, created.err)
				}
				assertBookingRequestCounts(t, adminPool, fixtures.accounts[1], newRequest.RequestID, 0, 0)
			}
			var confirmedCount int
			if err := adminPool.QueryRow(t.Context(), `
				SELECT count(*) FROM bookings WHERE resource_id = $1 AND state = 'confirmed'
				  AND start_at < $3 AND end_at > $2`, fixtures.resources[0], originalRequest.StartAt, originalRequest.EndAt).Scan(&confirmedCount); err != nil {
				t.Fatalf("check confirmed overlaps after ordered race: %v", err)
			}
			if firstOperation == "cancellation" && confirmedCount != 1 {
				t.Fatalf("cancellation-first race has %d overlapping confirmed bookings, want one replacement", confirmedCount)
			}
			if firstOperation == "creation" && confirmedCount != 0 {
				t.Fatalf("creation-first conflict left %d confirmed bookings after cancellation, want none", confirmedCount)
			}
		})
	}
}

func TestOwnerCancellationUsesFreshTimeAfterResourceLockWait(t *testing.T) {
	initial := bookingCreationNow()
	clock := &atomicBookingClock{}
	clock.Set(initial)
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 1, 1, clock.Now)
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), initial, 5*time.Minute, time.Hour)
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create lock-wait cancellation fixture: %v", err)
	}
	callsAfterCreate := clock.calls.Load()
	canceller := bookings.NewCancellationService(NewStore(appPool), clock.Now)
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	result := startBookingCancellation(t.Context(), canceller, fixtures.accounts[0], created.Booking.ID)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		_ = held.release()
		t.Fatalf("cancellation did not wait for Resource lock: %v", err)
	}
	if clock.calls.Load() != callsAfterCreate {
		_ = held.release()
		t.Fatalf("authoritative clock read %d times while Resource lock was held", clock.calls.Load()-callsAfterCreate)
	}
	clock.Set(created.Booking.StartAt)
	if err := held.release(); err != nil {
		t.Fatalf("release Resource lock: %v", err)
	}
	outcome := receiveBookingCancellation(t, result)
	if !errors.Is(outcome.err, bookings.ErrCancellationStarted) || clock.calls.Load() != callsAfterCreate+1 {
		t.Fatalf("fresh post-lock cancellation = %+v, clock calls=%d; want started rejection and one fresh read", outcome, clock.calls.Load()-callsAfterCreate)
	}
	var state string
	var eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT state, (SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled')
		FROM bookings WHERE id = $1`, created.Booking.ID).Scan(&state, &eventCount); err != nil {
		t.Fatalf("read fresh-time rejection: %v", err)
	}
	if state != "confirmed" || eventCount != 0 {
		t.Fatalf("post-start lock-wait rejection persisted state/events %s/%d", state, eventCount)
	}
}

func TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 2, 1, func() time.Time { return now })
	originalRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	original, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, originalRequest)
	if err != nil {
		t.Fatalf("create rollback fixture: %v", err)
	}
	functionName := "cancel_event_failure_" + strings.ReplaceAll(original.Booking.ID, "-", "")
	triggerName := functionName
	functionIdentifier := pgx.Identifier{"public", functionName}.Sanitize()
	triggerIdentifier := pgx.Identifier{triggerName}.Sanitize()
	if _, err := adminPool.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.action = 'booking.cancelled' THEN
				RAISE EXCEPTION 'injected booking cancellation event failure';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionIdentifier)); err != nil {
		t.Fatalf("create cancellation event failure trigger function: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON activity_events
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerIdentifier, functionIdentifier)); err != nil {
		_, _ = adminPool.Exec(t.Context(), "DROP FUNCTION "+functionIdentifier+"()")
		t.Fatalf("create cancellation event failure trigger: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP TRIGGER IF EXISTS "+triggerIdentifier+" ON activity_events"); err != nil {
			t.Errorf("drop cancellation event failure trigger: %v", err)
		}
		if _, err := adminPool.Exec(cleanupCtx, "DROP FUNCTION IF EXISTS "+functionIdentifier+"()"); err != nil {
			t.Errorf("drop cancellation event failure function: %v", err)
		}
	})

	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now })
	if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, original.Booking.ID); !errors.Is(err, bookings.ErrOperational) {
		t.Fatalf("event insertion failure = %v, want operational error", err)
	}
	var state, cancelledBy string
	var cancellationMetadataPresent bool
	var cancellationEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT state, COALESCE(cancelled_by_account_id::text, ''),
		       cancelled_by_account_id IS NOT NULL AND cancelled_at IS NOT NULL,
		       (SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled')
		FROM bookings WHERE id = $1`, original.Booking.ID).Scan(
		&state, &cancelledBy, &cancellationMetadataPresent, &cancellationEvents); err != nil {
		t.Fatalf("read event-failure rollback state: %v", err)
	}
	if state != "confirmed" || cancelledBy != "" || cancellationMetadataPresent || cancellationEvents != 0 {
		t.Fatalf("cancellation event failure partially committed state=%s actor=%q metadata=%v events=%d",
			state, cancelledBy, cancellationMetadataPresent, cancellationEvents)
	}

	conflictingRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	if _, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[1]}, conflictingRequest); !errors.Is(err, bookings.ErrConflict) {
		t.Fatalf("overlapping creation after cancellation rollback = %v, want conflict", err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[1], conflictingRequest.RequestID, 0, 0)
	if _, err := adminPool.Exec(t.Context(), "DROP TRIGGER "+triggerIdentifier+" ON activity_events"); err != nil {
		t.Fatalf("remove cancellation failure trigger: %v", err)
	}
	result, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, original.Booking.ID)
	if err != nil || !result.Changed {
		t.Fatalf("retry cancellation after removing failure = %+v, %v", result, err)
	}
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled'`, original.Booking.ID).Scan(&cancellationEvents); err != nil {
		t.Fatalf("count cancellation event after retry: %v", err)
	}
	if cancellationEvents != 1 {
		t.Fatalf("successful retry committed %d cancellation events, want one", cancellationEvents)
	}
}

func TestOwnerCancellationRuntimePrivilegeIsColumnScoped(t *testing.T) {
	adminPool, appPool, fixtures, creator := bookingCreationSetup(t, 1, 1, func() time.Time { return bookingCreationNow() })
	var tableUpdate, deletePrivilege bool
	var stateUpdate, actorUpdate, timeUpdate bool
	var ownerUpdate, resourceUpdate, requestUpdate, startUpdate, endUpdate, purposeUpdate, createdUpdate bool
	if err := appPool.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'bookings', 'UPDATE'),
		       has_table_privilege(current_user, 'bookings', 'DELETE'),
		       has_column_privilege(current_user, 'bookings', 'state', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'cancelled_by_account_id', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'cancelled_at', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'owner_account_id', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'resource_id', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'request_id', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'start_at', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'end_at', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'purpose', 'UPDATE'),
		       has_column_privilege(current_user, 'bookings', 'created_at', 'UPDATE')`).Scan(
		&tableUpdate, &deletePrivilege, &stateUpdate, &actorUpdate, &timeUpdate,
		&ownerUpdate, &resourceUpdate, &requestUpdate, &startUpdate, &endUpdate, &purposeUpdate, &createdUpdate); err != nil {
		t.Fatalf("inspect runtime booking update privileges: %v", err)
	}
	if tableUpdate || deletePrivilege || !stateUpdate || !actorUpdate || !timeUpdate || ownerUpdate || resourceUpdate ||
		requestUpdate || startUpdate || endUpdate || purposeUpdate || createdUpdate {
		t.Fatalf("booking privilege boundary is too broad/narrow: table update/delete=%v/%v allowed cancellation=%v/%v/%v immutable=%v",
			tableUpdate, deletePrivilege, stateUpdate, actorUpdate, timeUpdate,
			[]bool{ownerUpdate, resourceUpdate, requestUpdate, startUpdate, endUpdate, purposeUpdate, createdUpdate})
	}
	if _, err := appPool.Exec(t.Context(), "UPDATE bookings SET purpose = purpose"); err == nil {
		t.Fatal("runtime role unexpectedly updated immutable booking purpose")
	}
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, bookingCreationRequest(
		fixtures.resources[0], testDatabaseUUID(t, adminPool), bookingCreationNow(), time.Hour, time.Hour))
	if err != nil {
		t.Fatalf("create privilege test booking: %v", err)
	}
	canceller := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return bookingCreationNow() })
	if _, err := canceller.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, created.Booking.ID); err != nil {
		t.Fatalf("column-scoped runtime role could not perform owner cancellation: %v", err)
	}
}

func TestOwnerCancellationRestartFixture(t *testing.T) {
	if os.Getenv("LABRESERVE_TEST_CANCELLATION_RESTART_FIXTURE") != "1" {
		t.Skip("run by scripts/verify.sh to check cancellation across a PostgreSQL restart")
	}
	adminPool := bookingTestPool(t)
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	const login = "task-3b-restart@example.test"
	const resourceCode = "TASK-3B-RESTART"
	if _, err := adminPool.Exec(t.Context(), `
		DELETE FROM activity_events WHERE booking_id IN (
			SELECT id FROM bookings WHERE resource_id IN (SELECT id FROM resources WHERE code = $1)
		) OR actor_account_id IN (SELECT id FROM accounts WHERE login = $2)`, resourceCode, login); err != nil {
		t.Fatalf("remove prior isolated restart fixture events: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), `
		DELETE FROM bookings WHERE resource_id IN (SELECT id FROM resources WHERE code = $1)
		   OR owner_account_id IN (SELECT id FROM accounts WHERE login = $2)`, resourceCode, login); err != nil {
		t.Fatalf("remove prior isolated restart fixture booking: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), "DELETE FROM resources WHERE code = $1", resourceCode); err != nil {
		t.Fatalf("remove prior isolated restart fixture Resource: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), "DELETE FROM accounts WHERE login = $1", login); err != nil {
		t.Fatalf("remove prior isolated restart fixture account: %v", err)
	}

	var ownerID, resourceID string
	if err := adminPool.QueryRow(t.Context(), `
		INSERT INTO accounts (id, login, display_name, password_hash, role)
		VALUES (gen_random_uuid(), $1, 'Cancellation restart fixture', 'unused', 'engineer')
		RETURNING id::text`, login).Scan(&ownerID); err != nil {
		t.Fatalf("create restart fixture account: %v", err)
	}
	if err := adminPool.QueryRow(t.Context(), `
		INSERT INTO resources (id, code, name, description)
		VALUES (gen_random_uuid(), $1, 'Cancellation restart fixture', '')
		RETURNING id::text`, resourceCode).Scan(&resourceID); err != nil {
		t.Fatalf("create restart fixture Resource: %v", err)
	}
	now := bookingCreationNow()
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	created, err := bookings.NewService(NewStore(appPool), func() time.Time { return now }).Create(
		t.Context(), bookings.Actor{AccountID: ownerID}, request)
	if err != nil {
		t.Fatalf("create restart fixture Booking: %v", err)
	}
	result, err := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now }).Cancel(
		t.Context(), bookings.Actor{AccountID: ownerID}, created.Booking.ID)
	if err != nil || !result.Changed {
		t.Fatalf("cancel restart fixture Booking: %+v, %v", result, err)
	}
	var state string
	var eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT state, (SELECT count(*) FROM activity_events WHERE booking_id = $1 AND action = 'booking.cancelled')
		FROM bookings WHERE id = $1`, created.Booking.ID).Scan(&state, &eventCount); err != nil {
		t.Fatalf("verify restart fixture before database restart: %v", err)
	}
	if state != "cancelled" || eventCount != 1 {
		t.Fatalf("restart fixture state/events before restart = %s/%d", state, eventCount)
	}
}

func TestOwnerCancellationRestartReplay(t *testing.T) {
	if os.Getenv("LABRESERVE_TEST_CANCELLATION_RESTART_REPLAY") != "1" {
		t.Skip("run by scripts/verify.sh after PostgreSQL restart")
	}
	adminPool := bookingTestPool(t)
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var bookingID, ownerID, state string
	var cancelledAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT b.id::text, b.owner_account_id::text, b.state, b.cancelled_at
		FROM bookings b JOIN resources r ON r.id = b.resource_id
		WHERE r.code = 'TASK-3B-RESTART'`).Scan(&bookingID, &ownerID, &state, &cancelledAt); err != nil {
		t.Fatalf("load persisted restart fixture: %v", err)
	}
	if state != "cancelled" {
		t.Fatalf("restart fixture state = %s, want cancelled", state)
	}
	now := bookingCreationNow().Add(2 * time.Hour)
	result, err := bookings.NewCancellationService(NewStore(appPool), func() time.Time { return now }).Cancel(
		t.Context(), bookings.Actor{AccountID: ownerID}, bookingID)
	if err != nil || result.Changed || result.Booking.State != bookings.StateCancelled ||
		result.Booking.CancelledByAccountID == nil || *result.Booking.CancelledByAccountID != ownerID ||
		result.Booking.CancelledAt == nil || !result.Booking.CancelledAt.Equal(cancelledAt) {
		t.Fatalf("post-restart authorized cancellation replay = %+v, %v; want unchanged no-op", result, err)
	}
	var creationEvents, cancellationEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'),
		       count(*) FILTER (WHERE action = 'booking.cancelled')
		FROM activity_events WHERE booking_id = $1`, bookingID).Scan(&creationEvents, &cancellationEvents); err != nil {
		t.Fatalf("count restart fixture events after no-op: %v", err)
	}
	if creationEvents != 1 || cancellationEvents != 1 {
		t.Fatalf("post-restart no-op changed event counts to %d/%d", creationEvents, cancellationEvents)
	}
}

func bookingIDForRequest(t *testing.T, pool *pgxpool.Pool, ownerID, requestID string) string {
	t.Helper()
	var bookingID string
	if err := pool.QueryRow(t.Context(), `
		SELECT id::text FROM bookings WHERE owner_account_id = $1 AND request_id = $2`, ownerID, requestID).Scan(&bookingID); err != nil {
		t.Fatalf("resolve booking fixture ID: %v", err)
	}
	return bookingID
}

func insertCancellationTestEvent(ctx context.Context, pool *pgxpool.Pool, actorID, bookingID string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
		VALUES (gen_random_uuid(), $1, 'booking.cancelled', $2, $3, '{}'::jsonb)`, actorID, bookingTestStart(), bookingID)
	return err
}

func startBookingCancellation(ctx context.Context, service *bookings.CancellationService, actorID, bookingID string) <-chan bookingCancellationOutcome {
	result := make(chan bookingCancellationOutcome, 1)
	go func() {
		cancelled, err := service.Cancel(ctx, bookings.Actor{AccountID: actorID}, bookingID)
		result <- bookingCancellationOutcome{result: cancelled, err: err}
	}()
	return result
}

func receiveBookingCancellation(t *testing.T, result <-chan bookingCancellationOutcome) bookingCancellationOutcome {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("booking cancellation did not finish")
		return bookingCancellationOutcome{}
	}
}

type bookingCancellationOutcome struct {
	result bookings.CancellationResult
	err    error
}
