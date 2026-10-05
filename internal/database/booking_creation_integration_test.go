package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
)

func TestBookingCreationPersistsCanonicalBookingAndActivity(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	request.Purpose = "\u2003run network test\u00a0"

	result, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	if result.Replayed || result.Booking.ID == "" || result.Booking.OwnerAccountID != fixtures.accounts[0] {
		t.Fatalf("creation result = %+v, want a new booking owned by authenticated account", result)
	}
	if result.Booking.Purpose != "run network test" || result.Booking.State != bookings.StateConfirmed ||
		!result.Booking.CreatedAt.Equal(now) || !result.Booking.StartAt.Equal(request.StartAt) {
		t.Fatalf("persisted booking is not canonical: %+v", result.Booking)
	}

	var eventActor, action, targetID, detailResource, detailRequest, purpose string
	var occurredAt, detailStart, detailEnd time.Time
	err = adminPool.QueryRow(t.Context(), `
		SELECT actor_account_id::text, action, booking_id::text, occurred_at,
		       details->>'resource_id', details->>'request_id', details->>'purpose',
		       (details->>'start_at')::timestamptz, (details->>'end_at')::timestamptz
		FROM activity_events WHERE booking_id = $1`, result.Booking.ID).Scan(
		&eventActor, &action, &targetID, &occurredAt, &detailResource, &detailRequest,
		&purpose, &detailStart, &detailEnd)
	if err != nil {
		t.Fatalf("read booking creation event: %v", err)
	}
	if eventActor != fixtures.accounts[0] || action != "booking.created" || targetID != result.Booking.ID ||
		detailResource != request.ResourceID || detailRequest != request.RequestID || purpose != "run network test" ||
		!occurredAt.Equal(now) || !detailStart.Equal(result.Booking.StartAt) || !detailEnd.Equal(result.Booking.EndAt) {
		t.Fatalf("booking activity event is incomplete or inconsistent: actor=%s action=%s target=%s resource=%s request=%s purpose=%q occurred=%v start=%v end=%v",
			eventActor, action, targetID, detailResource, detailRequest, purpose, occurredAt, detailStart, detailEnd)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 1, 1)
}

func TestBookingCreationValidationAndEligibilityOutcomes(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 3, func() time.Time { return now })
	validCases := []struct {
		name       string
		resource   int
		startAfter time.Duration
		duration   time.Duration
	}{
		{name: "minimum start and duration inclusive", resource: 0, startAfter: 5 * time.Minute, duration: 30 * time.Minute},
		{name: "maximum start horizon inclusive", resource: 1, startAfter: 30 * 24 * time.Hour, duration: 30 * time.Minute},
		{name: "maximum duration inclusive", resource: 2, startAfter: time.Hour, duration: 8 * time.Hour},
	}
	for _, test := range validCases {
		t.Run(test.name, func(t *testing.T) {
			request := bookingCreationRequest(fixtures.resources[test.resource], testDatabaseUUID(t, adminPool), now, test.startAfter, test.duration)
			if _, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request); err != nil {
				t.Fatalf("valid inclusive boundary rejected: %v", err)
			}
		})
	}

	invalidCases := []struct {
		name       string
		startAfter time.Duration
		duration   time.Duration
	}{
		{name: "start below minimum", startAfter: 5*time.Minute - time.Microsecond, duration: time.Hour},
		{name: "start beyond maximum", startAfter: 30*24*time.Hour + time.Microsecond, duration: 30 * time.Minute},
		{name: "duration below minimum", startAfter: time.Hour, duration: 30*time.Minute - time.Microsecond},
		{name: "duration above maximum", startAfter: time.Hour, duration: 8*time.Hour + time.Microsecond},
	}
	for _, test := range invalidCases {
		t.Run(test.name, func(t *testing.T) {
			request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, test.startAfter, test.duration)
			_, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
			if !errors.Is(err, bookings.ErrInvalidInput) {
				t.Fatalf("invalid booking error = %v, want invalid-input outcome", err)
			}
			assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 0, 0)
		})
	}

	t.Run("unknown resource", func(t *testing.T) {
		request := bookingCreationRequest(testDatabaseUUID(t, adminPool), testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
		_, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
		if !errors.Is(err, bookings.ErrUnknownResource) {
			t.Fatalf("unknown resource error = %v", err)
		}
	})
	t.Run("inactive resource", func(t *testing.T) {
		if _, err := adminPool.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", fixtures.resources[0]); err != nil {
			t.Fatalf("prepare inactive resource: %v", err)
		}
		request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
		_, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
		if !errors.Is(err, bookings.ErrInactiveResource) {
			t.Fatalf("inactive resource error = %v", err)
		}
		assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 0, 0)
	})
}

func TestBookingCreationReplaySurvivesPastAndCancelledInactiveRecords(t *testing.T) {
	now := bookingCreationNow()
	adminPool, appPool, fixtures, service := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	confirmedRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	confirmed, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, confirmedRequest)
	if err != nil {
		t.Fatalf("create initial confirmed booking: %v", err)
	}

	cancelledFixture := fixtures.booking(t, 0, 0, bookingTestStart())
	cancelledFixture.state = "cancelled"
	cancelledFixture.cancelledByAccountID = stringPointer(fixtures.accounts[0])
	cancelledAt := cancelledFixture.startAt.Add(-time.Minute)
	cancelledFixture.cancelledAt = &cancelledAt
	if err := fixtures.insertBooking(t.Context(), cancelledFixture); err != nil {
		t.Fatalf("prepare retained cancelled booking: %v", err)
	}
	var cancelledBookingID string
	if err := adminPool.QueryRow(t.Context(), `
		SELECT id::text FROM bookings WHERE owner_account_id = $1 AND request_id = $2`,
		fixtures.accounts[0], cancelledFixture.requestID).Scan(&cancelledBookingID); err != nil {
		t.Fatalf("read retained cancelled booking identifier: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", fixtures.resources[0]); err != nil {
		t.Fatalf("prepare inactive resource: %v", err)
	}

	replayNow := cancelledFixture.endAt.Add(time.Hour)
	service = bookings.NewService(NewStore(appPool), func() time.Time { return replayNow })
	replayWithoutResourceLock := func(request bookings.CreateRequest, expectedID string, expectedState bookings.State) {
		t.Helper()
		held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
		result := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request)
		var outcome bookingCreationOutcome
		select {
		case outcome = <-result:
		case <-time.After(5 * time.Second):
			_ = held.release()
			t.Fatal("identical replay waited on its Resource; it must return before eligibility locks")
		}
		if err := held.release(); err != nil {
			t.Fatalf("release replay Resource lock: %v", err)
		}
		if outcome.err != nil {
			t.Fatalf("replay retained booking: %v", outcome.err)
		}
		if !outcome.result.Replayed || outcome.result.Booking.ID != expectedID || outcome.result.Booking.State != expectedState {
			t.Fatalf("retained replay = %+v, want %s booking %s", outcome.result, expectedState, expectedID)
		}
	}

	replayWithoutResourceLock(bookings.CreateRequest{
		ResourceID: confirmedRequest.ResourceID, RequestID: confirmedRequest.RequestID,
		StartAt: confirmedRequest.StartAt.In(time.FixedZone("UTC+7", 7*60*60)).Add(999 * time.Nanosecond),
		EndAt:   confirmedRequest.EndAt.In(time.FixedZone("UTC+7", 7*60*60)).Add(999 * time.Nanosecond),
		Purpose: "\u2003" + confirmedRequest.Purpose + "\u00a0",
	}, confirmed.Booking.ID, bookings.StateConfirmed)
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], confirmedRequest.RequestID, 1, 1)

	cancelledRequest := bookings.CreateRequest{
		ResourceID: cancelledFixture.resourceID, RequestID: cancelledFixture.requestID,
		StartAt: cancelledFixture.startAt, EndAt: cancelledFixture.endAt,
		Purpose: cancelledFixture.purpose,
	}
	replayWithoutResourceLock(cancelledRequest, cancelledBookingID, bookings.StateCancelled)
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], cancelledFixture.requestID, 1, 0)
}

func TestBookingCreationChangedDataWinsBeforeResourceEligibility(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 2, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	_, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil {
		t.Fatalf("create original request: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), "UPDATE resources SET active = FALSE WHERE id = $1", fixtures.resources[1]); err != nil {
		t.Fatalf("prepare inactive changed target: %v", err)
	}
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[1])
	changedResource := request
	changedResource.ResourceID = fixtures.resources[1]
	result := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, changedResource)
	select {
	case outcome := <-result:
		if !errors.Is(outcome.err, bookings.ErrRequestIDReuse) {
			t.Fatalf("changed-resource result = %+v, want request-id reuse before resource eligibility", outcome)
		}
	case <-time.After(5 * time.Second):
		_ = held.release()
		t.Fatal("changed request waited on its target Resource; replay decision must precede Resource locking")
	}
	if err := held.release(); err != nil {
		t.Fatalf("release held Resource: %v", err)
	}

	for _, mutation := range []struct {
		name   string
		change func(*bookings.CreateRequest)
	}{
		{name: "start", change: func(input *bookings.CreateRequest) { input.StartAt = input.StartAt.Add(time.Microsecond) }},
		{name: "end", change: func(input *bookings.CreateRequest) { input.EndAt = input.EndAt.Add(time.Microsecond) }},
		{name: "purpose", change: func(input *bookings.CreateRequest) { input.Purpose = "different" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			changed := request
			mutation.change(&changed)
			_, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, changed)
			if !errors.Is(err, bookings.ErrRequestIDReuse) {
				t.Fatalf("changed %s error = %v, want request-id reuse", mutation.name, err)
			}
		})
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 1, 1)
}

func TestUnexpectedRequestUniqueViolationUsesFreshReplayLookup(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 2, func() time.Time { return now })
	requestID := testDatabaseUUID(t, adminPool)
	request := bookingCreationRequest(fixtures.resources[0], requestID, now, time.Hour, time.Hour)
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	creation := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		_ = held.release()
		t.Fatalf("booking creator did not reach Resource lock after replay lookup: %v", err)
	}

	// A writer that bypasses Account coordination commits the unique request
	// key while the normal creator is blocked before its insert. Its foreign
	// key KEY SHARE lock must remain compatible with the creator's
	// FOR NO KEY UPDATE lock.
	winner := fixtures.booking(t, 1, 0, request.StartAt.Add(time.Hour))
	winner.requestID = requestID
	winner.purpose = "out-of-protocol winner"
	insertResult := make(chan error, 1)
	go func() { insertResult <- fixtures.insertBooking(t.Context(), winner) }()
	select {
	case err := <-insertResult:
		if err != nil {
			_ = held.release()
			t.Fatalf("direct request-key winner was blocked/rejected under Account row lock: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = held.release()
		t.Fatal("direct booking insert blocked on the Account lock; FOR NO KEY UPDATE must allow FK KEY SHARE")
	}

	if err := held.release(); err != nil {
		t.Fatalf("release Resource lock: %v", err)
	}
	outcome := receiveBookingCreation(t, creation)
	if !errors.Is(outcome.err, bookings.ErrRequestIDReuse) {
		t.Fatalf("creation after request-key uniqueness race = %+v, want fresh lookup and changed-data reuse", outcome)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], requestID, 1, 0)
}

func TestConcurrentIdenticalBookingRequestsCreateOneBookingAndEvent(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	held := holdBookingAccountLock(t, adminPool, fixtures.accounts[0])
	first := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	second := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		_ = held.release()
		t.Fatalf("both creators did not wait on Account serialization: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release Account lock: %v", err)
	}
	one, two := receiveBookingCreation(t, first), receiveBookingCreation(t, second)
	if one.err != nil || two.err != nil || one.result.Booking.ID != two.result.Booking.ID || one.result.Replayed == two.result.Replayed {
		t.Fatalf("concurrent identical results = (%+v, %v), (%+v, %v); want one creation and one replay", one.result, one.err, two.result, two.err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 1, 1)
}

func TestConcurrentSameKeyDifferentResourceHasOneBinding(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 2, func() time.Time { return now })
	requestID := testDatabaseUUID(t, adminPool)
	firstRequest := bookingCreationRequest(fixtures.resources[0], requestID, now, time.Hour, time.Hour)
	secondRequest := bookingCreationRequest(fixtures.resources[1], requestID, now, 2*time.Hour, time.Hour)
	held := holdBookingAccountLock(t, adminPool, fixtures.accounts[0])
	first := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, firstRequest)
	second := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, secondRequest)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		_ = held.release()
		t.Fatalf("same-key creators did not wait on Account serialization: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release Account lock: %v", err)
	}
	one, two := receiveBookingCreation(t, first), receiveBookingCreation(t, second)
	if (one.err == nil) == (two.err == nil) ||
		(one.err != nil && !errors.Is(one.err, bookings.ErrRequestIDReuse)) ||
		(two.err != nil && !errors.Is(two.err, bookings.ErrRequestIDReuse)) {
		t.Fatalf("same-key different-data results = (%+v, %v), (%+v, %v); want one binding and one reuse error", one.result, one.err, two.result, two.err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], requestID, 1, 1)
}

func TestSameOwnerDifferentRequestIDsAndSameRequestIDDifferentOwners(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 2, 4, func() time.Time { return now })
	firstRequest := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	secondRequest := bookingCreationRequest(fixtures.resources[1], testDatabaseUUID(t, adminPool), now, 2*time.Hour, time.Hour)
	held := holdBookingAccountLock(t, adminPool, fixtures.accounts[0])
	first := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, firstRequest)
	second := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, secondRequest)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		_ = held.release()
		t.Fatalf("different-key creators did not serialize on Account: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release Account lock: %v", err)
	}
	if one, two := receiveBookingCreation(t, first), receiveBookingCreation(t, second); one.err != nil || two.err != nil {
		t.Fatalf("same-owner distinct keys failed: (%+v, %v), (%+v, %v)", one.result, one.err, two.result, two.err)
	}

	sharedRequestID := testDatabaseUUID(t, adminPool)
	thirdRequest := bookingCreationRequest(fixtures.resources[2], sharedRequestID, now, 3*time.Hour, time.Hour)
	fourthRequest := bookingCreationRequest(fixtures.resources[3], sharedRequestID, now, 4*time.Hour, time.Hour)
	if _, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, thirdRequest); err != nil {
		t.Fatalf("create first owner's shared request identity: %v", err)
	}
	if _, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[1]}, fourthRequest); err != nil {
		t.Fatalf("same request identity for independent owner: %v", err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], sharedRequestID, 1, 1)
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[1], sharedRequestID, 1, 1)
}

func TestConcurrentConflictingBookingCreationsReturnOneConflict(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 2, 1, func() time.Time { return now })
	request1 := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	request2 := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour+30*time.Minute, time.Hour)
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	first := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request1)
	second := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[1]}, request2)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		_ = held.release()
		t.Fatalf("both conflicting creators did not wait on Resource serialization: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release Resource lock: %v", err)
	}
	one, two := receiveBookingCreation(t, first), receiveBookingCreation(t, second)
	if (one.err == nil) == (two.err == nil) ||
		(one.err != nil && !errors.Is(one.err, bookings.ErrConflict)) ||
		(two.err != nil && !errors.Is(two.err, bookings.ErrConflict)) {
		t.Fatalf("conflicting results = (%+v, %v), (%+v, %v); want one success and one booking conflict", one.result, one.err, two.result, two.err)
	}
	var bookingCount, eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*), (SELECT count(*) FROM activity_events e JOIN bookings b ON b.id = e.booking_id
		                  WHERE b.resource_id = $1 AND e.action = 'booking.created')
		FROM bookings WHERE resource_id = $1 AND state = 'confirmed'`, fixtures.resources[0]).Scan(&bookingCount, &eventCount); err != nil {
		t.Fatal(err)
	}
	if bookingCount != 1 || eventCount != 1 {
		t.Fatalf("conflicting creation retained bookings/events = %d/%d, want one each", bookingCount, eventCount)
	}
}

func TestBookingCreationValidatesTimeAfterResourceLockWait(t *testing.T) {
	initial := bookingCreationNow()
	clock := &atomicBookingClock{}
	clock.Set(initial)
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 1, clock.Now)
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), initial, 5*time.Minute, time.Hour)
	held := holdBookingResourceLock(t, adminPool, fixtures.resources[0])
	result := startBookingCreation(t.Context(), service, bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		_ = held.release()
		t.Fatalf("creation did not wait on the locked Resource: %v", err)
	}
	if clock.calls.Load() != 0 {
		_ = held.release()
		t.Fatalf("authoritative clock was read %d times before Resource lock acquisition", clock.calls.Load())
	}
	clock.Set(initial.Add(time.Microsecond))
	if err := held.release(); err != nil {
		t.Fatalf("release Resource lock: %v", err)
	}
	outcome := receiveBookingCreation(t, result)
	if !errors.Is(outcome.err, bookings.ErrInvalidInput) || clock.calls.Load() != 1 {
		t.Fatalf("post-lock validation = %+v, clock calls=%d; want invalid input from fresh time", outcome, clock.calls.Load())
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 0, 0)
}

func TestActivityFailureRollsBackBookingAndLeavesRequestIdentityAvailable(t *testing.T) {
	now := bookingCreationNow()
	adminPool, _, fixtures, service := bookingCreationSetup(t, 1, 1, func() time.Time { return now })
	request := bookingCreationRequest(fixtures.resources[0], testDatabaseUUID(t, adminPool), now, time.Hour, time.Hour)
	functionName := "booking_event_failure_" + strings.ReplaceAll(request.RequestID, "-", "")
	triggerName := "booking_event_failure_" + strings.ReplaceAll(request.RequestID, "-", "")
	functionIdentifier := pgx.Identifier{"public", functionName}.Sanitize()
	triggerIdentifier := pgx.Identifier{triggerName}.Sanitize()
	functionDDL := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.action = 'booking.created' AND NEW.details->>'request_id' = '%s' THEN
				RAISE EXCEPTION 'injected booking event failure';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionIdentifier, request.RequestID)
	if _, err := adminPool.Exec(t.Context(), functionDDL); err != nil {
		t.Fatalf("create event failure trigger function: %v", err)
	}
	if _, err := adminPool.Exec(t.Context(), fmt.Sprintf(`
			CREATE TRIGGER %s BEFORE INSERT ON activity_events
			FOR EACH ROW EXECUTE FUNCTION %s()`, triggerIdentifier, functionIdentifier)); err != nil {
		_, _ = adminPool.Exec(t.Context(), "DROP FUNCTION "+functionIdentifier+"()")
		t.Fatalf("create event failure trigger: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP TRIGGER IF EXISTS "+triggerIdentifier+" ON activity_events"); err != nil {
			t.Errorf("drop event failure trigger: %v", err)
		}
		if _, err := adminPool.Exec(cleanupCtx, "DROP FUNCTION IF EXISTS "+functionIdentifier+"()"); err != nil {
			t.Errorf("drop event failure function: %v", err)
		}
	})

	if _, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request); !errors.Is(err, bookings.ErrOperational) {
		t.Fatalf("activity append error = %v, want operational failure", err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 0, 0)
	if _, err := adminPool.Exec(t.Context(), "DROP TRIGGER "+triggerIdentifier+" ON activity_events"); err != nil {
		t.Fatalf("remove event failure trigger for retry: %v", err)
	}
	result, err := service.Create(t.Context(), bookings.Actor{AccountID: fixtures.accounts[0]}, request)
	if err != nil || result.Replayed {
		t.Fatalf("same-key retry after event rollback = %+v, %v; want new creation", result, err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.accounts[0], request.RequestID, 1, 1)
}

func TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability(t *testing.T) {
	adminPool := bookingTestPool(t)
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var ownerExecute, resourceExecute, ownerUpdate, resourceUpdate bool
	if err := runtimePool.QueryRow(t.Context(), `
		SELECT has_function_privilege(current_user, 'public.labreserve_lock_booking_owner(uuid)', 'EXECUTE'),
		       has_function_privilege(current_user, 'public.labreserve_lock_booking_resource(uuid)', 'EXECUTE'),
		       has_table_privilege(current_user, 'accounts', 'UPDATE'),
		       has_table_privilege(current_user, 'resources', 'UPDATE')`).Scan(
		&ownerExecute, &resourceExecute, &ownerUpdate, &resourceUpdate); err != nil {
		t.Fatal(err)
	}
	if !ownerExecute || !resourceExecute || ownerUpdate || resourceUpdate {
		t.Fatalf("runtime lock capability is not minimal: execute owner/resource=%v/%v, update accounts/resources=%v/%v",
			ownerExecute, resourceExecute, ownerUpdate, resourceUpdate)
	}
	var ownerSecurityDefiner, resourceSecurityDefiner bool
	if err := adminPool.QueryRow(t.Context(), `
		SELECT owner.prosecdef, resource.prosecdef
		FROM pg_proc owner, pg_proc resource
		WHERE owner.oid = 'public.labreserve_lock_booking_owner(uuid)'::regprocedure
		  AND resource.oid = 'public.labreserve_lock_booking_resource(uuid)'::regprocedure`).Scan(
		&ownerSecurityDefiner, &resourceSecurityDefiner); err != nil {
		t.Fatal(err)
	}
	if !ownerSecurityDefiner || !resourceSecurityDefiner {
		t.Fatalf("row-lock functions are not SECURITY DEFINER: %v/%v", ownerSecurityDefiner, resourceSecurityDefiner)
	}
}

func bookingCreationSetup(t *testing.T, accountCount, resourceCount int, now func() time.Time) (*pgxpool.Pool, *pgxpool.Pool, *bookingTestFixtures, *bookings.Service) {
	t.Helper()
	adminPool := bookingTestPool(t)
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newBookingTestFixtures(t, adminPool, accountCount, resourceCount)
	service := bookings.NewService(NewStore(appPool), now)
	return adminPool, appPool, fixtures, service
}

func bookingCreationNow() time.Time {
	return time.Date(2040, time.January, 1, 10, 0, 0, 0, time.UTC)
}

func bookingCreationRequest(resourceID, requestID string, now time.Time, startAfter, duration time.Duration) bookings.CreateRequest {
	start := now.Add(startAfter)
	return bookings.CreateRequest{
		ResourceID: resourceID, RequestID: requestID,
		StartAt: start, EndAt: start.Add(duration), Purpose: "integration booking",
	}
}

func assertBookingRequestCounts(t *testing.T, pool *pgxpool.Pool, ownerID, requestID string, expectedBookings, expectedEvents int) {
	t.Helper()
	var bookingCount, eventCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*),
		       (SELECT count(*) FROM activity_events e JOIN bookings b ON b.id = e.booking_id
		        WHERE b.owner_account_id = $1 AND b.request_id = $2 AND e.action = 'booking.created')
		FROM bookings WHERE owner_account_id = $1 AND request_id = $2`, ownerID, requestID).Scan(&bookingCount, &eventCount); err != nil {
		t.Fatal(err)
	}
	if bookingCount != expectedBookings || eventCount != expectedEvents {
		t.Fatalf("request %s retained bookings/events %d/%d, want %d/%d", requestID, bookingCount, eventCount, expectedBookings, expectedEvents)
	}
}

type bookingCreationOutcome struct {
	result bookings.CreateResult
	err    error
}

func startBookingCreation(ctx context.Context, service *bookings.Service, actor bookings.Actor, request bookings.CreateRequest) <-chan bookingCreationOutcome {
	result := make(chan bookingCreationOutcome, 1)
	go func() {
		created, err := service.Create(ctx, actor, request)
		result <- bookingCreationOutcome{result: created, err: err}
	}()
	return result
}

func receiveBookingCreation(t *testing.T, result <-chan bookingCreationOutcome) bookingCreationOutcome {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("booking creation did not finish")
		return bookingCreationOutcome{}
	}
}

func waitForBlockedBookingCalls(ctx context.Context, pool *pgxpool.Pool, blockerPID int32, minimum int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := pool.QueryRow(ctx, `
			WITH RECURSIVE lock_waits(waiter, blocker, path) AS (
				SELECT waiting.pid, blocker.pid, ARRAY[waiting.pid, blocker.pid]
				FROM pg_locks AS waiting
				CROSS JOIN LATERAL unnest(pg_blocking_pids(waiting.pid)) AS blocker(pid)
				WHERE NOT waiting.granted
				UNION ALL
				SELECT lock_waits.waiter, blocker.pid, lock_waits.path || blocker.pid
				FROM lock_waits
				CROSS JOIN LATERAL unnest(pg_blocking_pids(lock_waits.blocker)) AS blocker(pid)
				WHERE NOT blocker.pid = ANY(lock_waits.path)
			)
			SELECT count(DISTINCT waiter) FROM lock_waits WHERE blocker = $1`, blockerPID).Scan(&count)
		if err != nil {
			if ctx.Err() != nil {
				diagnosticCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				var pending string
				_ = pool.QueryRow(diagnosticCtx, `
					SELECT COALESCE(string_agg(format('pid=%s type=%s mode=%s relation=%s blockers=%s',
						pid, locktype, mode, relation, pg_blocking_pids(pid)), '; '), 'none')
					FROM pg_locks WHERE NOT granted`).Scan(&pending)
				cancel()
				return fmt.Errorf("observe booking lock waits (pending %s): %w", pending, err)
			}
			return fmt.Errorf("observe booking lock waits: %w", err)
		}
		if count >= minimum {
			return nil
		}
		select {
		case <-ctx.Done():
			diagnosticCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var pending string
			_ = pool.QueryRow(diagnosticCtx, `
				SELECT COALESCE(string_agg(format('pid=%s type=%s mode=%s relation=%s blockers=%s',
					pid, locktype, mode, relation, pg_blocking_pids(pid)), '; '), 'none')
				FROM pg_locks WHERE NOT granted`).Scan(&pending)
			return fmt.Errorf("%w; pending PostgreSQL locks: %s", ctx.Err(), pending)
		case <-ticker.C:
		}
	}
}

type heldBookingLock struct {
	conn     *pgxpool.Conn
	tx       pgx.Tx
	pid      int32
	released bool
}

func holdBookingAccountLock(t *testing.T, pool *pgxpool.Pool, accountID string) *heldBookingLock {
	t.Helper()
	return holdBookingRowLock(t, pool, "SELECT id FROM accounts WHERE id = $1 FOR NO KEY UPDATE", accountID)
}

func holdBookingResourceLock(t *testing.T, pool *pgxpool.Pool, resourceID string) *heldBookingLock {
	t.Helper()
	return holdBookingRowLock(t, pool, "SELECT id FROM resources WHERE id = $1 FOR UPDATE", resourceID)
}

func holdBookingRowLock(t *testing.T, pool *pgxpool.Pool, query, id string) *heldBookingLock {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire lock-test connection: %v", err)
	}
	tx, err := conn.Begin(t.Context())
	if err != nil {
		conn.Release()
		t.Fatalf("begin lock-test transaction: %v", err)
	}
	var pid int32
	if err := tx.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		_ = tx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("read lock-test backend pid: %v", err)
	}
	var lockedID string
	if err := tx.QueryRow(t.Context(), query, id).Scan(&lockedID); err != nil {
		_ = tx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("lock test row: %v", err)
	}
	held := &heldBookingLock{conn: conn, tx: tx, pid: pid}
	t.Cleanup(func() {
		if !held.released {
			_ = held.release()
		}
	})
	return held
}

func (held *heldBookingLock) release() error {
	if held.released {
		return nil
	}
	held.released = true
	defer held.conn.Release()
	return held.tx.Commit(context.Background())
}

type atomicBookingClock struct {
	micros atomic.Int64
	calls  atomic.Int64
}

func (clock *atomicBookingClock) Set(value time.Time) {
	clock.micros.Store(value.UnixMicro())
}

func (clock *atomicBookingClock) Now() time.Time {
	clock.calls.Add(1)
	return time.UnixMicro(clock.micros.Load()).UTC()
}
