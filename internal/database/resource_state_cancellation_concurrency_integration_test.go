package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceStateCancellationRaceCancellationCommitsBeforeDeactivation(t *testing.T) {
	for _, test := range []struct {
		name             string
		remainingBlocker bool
	}{
		{name: "no remaining blocker permits deactivation"},
		{name: "remaining blocker still rejects deactivation", remainingBlocker: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runCancellationBeforeDeactivation(t, test.remainingBlocker)
		})
	}
}

func runCancellationBeforeDeactivation(t *testing.T, remainingBlocker bool) {
	t.Helper()
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	creationClock := newAtomicBookingClockAt(now)
	first, firstRequest := createCancellationRaceBooking(t, admin, app, fixtures, bookingIDs, resourceID, fixtures.engineer, creationClock, now, time.Hour)

	var remaining bookings.Booking
	var remainingRequest bookings.CreateRequest
	if remainingBlocker {
		remaining, remainingRequest = createCancellationRaceBooking(
			t, admin, app, fixtures, bookingIDs, resourceID, fixtures.secondEngineer, creationClock, now, 3*time.Hour)
	}

	group := newBookingRaceGroup(t)
	cancellationGate := group.newGate()
	cancellationStarted := make(chan int32, 1)
	cancellationClock := newAtomicBookingClockAt(now)
	cancellationService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: cancellationStarted, beforeCommit: cancellationGate,
	}, cancellationClock.Now)
	cancellationCtx, cancelCancellation := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCancellation()
	cancellation := startCancellationRaceCall(group, cancellationCtx, cancellationService, fixtures.engineer, first.ID)
	cancellationPID := awaitBookingRacePID(t, cancellationCtx, cancellationStarted, "owner cancellation transaction")
	if gatePID := cancellationGate.await(t, cancellationCtx, "cancellation after Booking update and Activity insert"); gatePID != cancellationPID {
		t.Fatalf("cancellation gate backend PID=%d, transaction PID=%d", gatePID, cancellationPID)
	}

	stateStarted := make(chan int32, 1)
	stateClock := newAtomicBookingClockAt(now)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation transaction")
	waitCtx, cancelWait := context.WithTimeout(stateCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, statePID, cancellationPID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe deactivation waiting for uncommitted owner cancellation: %v", err)
	}
	logBookingRaceWait(t, "deactivation waits for cancellation commit", wait)

	cancellationGate.open()
	cancellationOutcome := awaitCancellationRaceCall(t, cancellationCtx, cancellation)
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	if cancellationOutcome.err != nil || !cancellationOutcome.result.Changed || cancellationOutcome.result.Booking.ID != first.ID {
		t.Fatalf("leading owner cancellation result=%+v error=%v", cancellationOutcome.result, cancellationOutcome.err)
	}
	if cancellationClock.calls.Load() != 1 || stateClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: cancellation=%d deactivation=%d, want one each", cancellationClock.calls.Load(), stateClock.calls.Load())
	}
	assertCancellationRaceBooking(t, admin, first, bookings.StateCancelled, fixtures.engineer, &now)
	assertBookingRequestCounts(t, admin, fixtures.engineer, firstRequest.RequestID, 1, 1)

	if remainingBlocker {
		if !errors.Is(stateOutcome.err, resources.ErrResourceOccupied) || stateOutcome.result != (resources.StateResult{}) {
			t.Fatalf("deactivation after cancellation with another blocker result=%+v error=%v, want occupied", stateOutcome.result, stateOutcome.err)
		}
		assertCancellationRaceBooking(t, admin, remaining, bookings.StateConfirmed, "", nil)
		assertBookingRequestCounts(t, admin, fixtures.secondEngineer, remainingRequest.RequestID, 1, 1)
		if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
			t.Fatalf("remaining Booking did not preserve Active Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
		}
		if events := readResourceCapabilityEvents(t, admin, resourceID); len(events) != 0 {
			t.Fatalf("rejected deactivation persisted Resource events: %+v", events)
		}
		assertBookingRaceResource(t, admin, resourceID, true, createdAt, createdAt)
		return
	}

	if stateOutcome.err != nil || !stateOutcome.result.Changed || stateOutcome.result.Resource.Active {
		t.Fatalf("deactivation after cancellation commit result=%+v error=%v", stateOutcome.result, stateOutcome.err)
	}
	assertBookingRaceResource(t, admin, resourceID, false, createdAt, now)
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", now, true, false)
}

func TestResourceStateCancellationRaceDeactivationRejectsBeforeCancellationAndRequiresResubmission(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	creationClock := newAtomicBookingClockAt(now)
	booking, request := createCancellationRaceBooking(t, admin, app, fixtures, bookingIDs,
		resourceID, fixtures.engineer, creationClock, now, time.Hour)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	group := newBookingRaceGroup(t)
	occupancyGate := group.newGate()
	stateStarted := make(chan int32, 1)
	stateClock := newAtomicBookingClockAt(now)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted, afterOccupied: occupancyGate,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation transaction")
	if gatePID := occupancyGate.await(t, stateCtx, "real occupied query while Resource lock is held"); gatePID != statePID {
		t.Fatalf("occupancy gate backend PID=%d, transaction PID=%d", gatePID, statePID)
	}

	cancellationStarted := make(chan int32, 1)
	cancellationClock := newAtomicBookingClockAt(now)
	cancellationService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: cancellationStarted,
	}, cancellationClock.Now)
	cancellationCtx, cancelCancellation := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCancellation()
	cancellation := startCancellationRaceCall(group, cancellationCtx, cancellationService, fixtures.engineer, booking.ID)
	cancellationPID := awaitBookingRacePID(t, cancellationCtx, cancellationStarted, "waiting owner cancellation transaction")
	waitCtx, cancelWait := context.WithTimeout(cancellationCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, cancellationPID, statePID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe owner cancellation waiting for occupied deactivation decision: %v", err)
	}
	logBookingRaceWait(t, "owner cancellation waits for occupied deactivation", wait)

	occupancyGate.open()
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	cancellationOutcome := awaitCancellationRaceCall(t, cancellationCtx, cancellation)
	if !errors.Is(stateOutcome.err, resources.ErrResourceOccupied) || stateOutcome.result != (resources.StateResult{}) {
		t.Fatalf("leading occupied deactivation result=%+v error=%v, want rejection before cancellation", stateOutcome.result, stateOutcome.err)
	}
	if cancellationOutcome.err != nil || !cancellationOutcome.result.Changed || cancellationOutcome.result.Booking.ID != booking.ID {
		t.Fatalf("owner cancellation after deactivation rejection result=%+v error=%v", cancellationOutcome.result, cancellationOutcome.err)
	}
	if stateClock.calls.Load() != 1 || cancellationClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: rejected deactivation=%d cancellation=%d, want one each", stateClock.calls.Load(), cancellationClock.calls.Load())
	}
	assertCancellationRaceBooking(t, admin, booking, bookings.StateCancelled, fixtures.engineer, &now)
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 1, 1)
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("rejected first deactivation changed Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
	if events := readResourceCapabilityEvents(t, admin, resourceID); len(events) != 0 {
		t.Fatalf("rejected first deactivation persisted Resource events: %+v", events)
	}
	assertBookingRaceResource(t, admin, resourceID, true, createdAt, createdAt)

	// This is a separate explicit submission after observing both prior outcomes.
	retryAt := now.Add(time.Minute)
	retryClock := newAtomicBookingClockAt(retryAt)
	retryService := resources.NewStateService(&resourceStateRaceRepository{base: NewStore(app)}, retryClock.Now)
	retry, err := retryService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator},
		resources.StateRequest{ResourceID: resourceID, Active: false})
	if err != nil || !retry.Changed || retry.Resource.Active {
		t.Fatalf("explicit deactivation resubmission result=%+v error=%v", retry, err)
	}
	if retryClock.calls.Load() != 1 {
		t.Fatalf("explicit deactivation resubmission clock reads=%d, want one", retryClock.calls.Load())
	}
	assertBookingRaceResource(t, admin, resourceID, false, createdAt, retryAt)
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", retryAt, true, false)
}

func TestResourceStateCancellationRaceWaitCrossesExactStart(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	creationClock := newAtomicBookingClockAt(now)
	booking, request := createCancellationRaceBooking(t, admin, app, fixtures, bookingIDs,
		resourceID, fixtures.engineer, creationClock, now, time.Hour)
	beforeBooking := resourceStateBookingSnapshot(t, admin, booking.ID)
	beforeRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	callsAfterCreate := creationClock.calls.Load()

	group := newBookingRaceGroup(t)
	held := holdBookingResourceLock(t, admin, resourceID)
	cancellationStarted := make(chan int32, 1)
	cancellationService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: cancellationStarted,
	}, creationClock.Now)
	cancellationCtx, cancelCancellation := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCancellation()
	cancellation := startCancellationRaceCall(group, cancellationCtx, cancellationService, fixtures.engineer, booking.ID)
	cancellationPID := awaitBookingRacePID(t, cancellationCtx, cancellationStarted, "Resource-lock-waiting cancellation transaction")
	waitCtx, cancelWait := context.WithTimeout(cancellationCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, cancellationPID, held.pid)
	cancelWait()
	if err != nil {
		t.Fatalf("observe owner cancellation waiting on independent Resource holder: %v", err)
	}
	logBookingRaceWait(t, "owner cancellation waits for independent Resource lock", wait)
	if creationClock.calls.Load() != callsAfterCreate {
		t.Fatalf("cancellation read its clock before Resource lock release: calls=%d before=%d", creationClock.calls.Load(), callsAfterCreate)
	}
	creationClock.Set(booking.StartAt)
	if err := held.release(); err != nil {
		t.Fatalf("release independent Resource lock: %v", err)
	}
	cancellationOutcome := awaitCancellationRaceCall(t, cancellationCtx, cancellation)
	if !errors.Is(cancellationOutcome.err, bookings.ErrCancellationStarted) || cancellationOutcome.result.Changed || cancellationOutcome.result.Booking.ID != "" {
		t.Fatalf("cancellation at exact start after lock wait result=%+v error=%v, want started rejection", cancellationOutcome.result, cancellationOutcome.err)
	}
	if creationClock.calls.Load() != callsAfterCreate+1 {
		t.Fatalf("cancellation post-wait clock reads=%d, want exactly one", creationClock.calls.Load()-callsAfterCreate)
	}
	assertCancellationRaceBooking(t, admin, booking, bookings.StateConfirmed, "", nil)
	if afterBooking := resourceStateBookingSnapshot(t, admin, booking.ID); afterBooking != beforeBooking {
		t.Fatalf("exact-start rejection changed retained Booking: before=%s after=%s", beforeBooking, afterBooking)
	}
	if afterRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); afterRequest != beforeRequest {
		t.Fatalf("exact-start rejection changed Booking request/creation history: before=%s after=%s", beforeRequest, afterRequest)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("exact-start rejection changed Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}

	stateClock := newAtomicBookingClockAt(booking.StartAt)
	stateStarted := make(chan int32, 1)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation check for retained exact-start Booking")
	_ = statePID
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	if !errors.Is(stateOutcome.err, resources.ErrResourceOccupied) || stateOutcome.result != (resources.StateResult{}) {
		t.Fatalf("deactivation after cancellation boundary rejection result=%+v error=%v, want retained Booking blocker", stateOutcome.result, stateOutcome.err)
	}
	if stateClock.calls.Load() != 1 {
		t.Fatalf("retained-Booking deactivation clock reads=%d, want one", stateClock.calls.Load())
	}
	assertCancellationRaceBooking(t, admin, booking, bookings.StateConfirmed, "", nil)
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 1, 1)
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("retained Booking rejection changed Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
}

func TestResourceStateCancellationRaceActivityFailureRollsBackBeforeDeactivation(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	creationClock := newAtomicBookingClockAt(now)
	booking, request := createCancellationRaceBooking(t, admin, app, fixtures, bookingIDs,
		resourceID, fixtures.engineer, creationClock, now, time.Hour)
	beforeBooking := resourceStateBookingSnapshot(t, admin, booking.ID)
	beforeRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	removeFailure := installCancellationRaceActivityFailure(t, admin, fixtures.engineer, booking.ID)

	group := newBookingRaceGroup(t)
	appendGate := group.newGate()
	cancellationStarted := make(chan int32, 1)
	cancellationClock := newAtomicBookingClockAt(now)
	cancellationService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: cancellationStarted, beforeAppend: appendGate,
	}, cancellationClock.Now)
	cancellationCtx, cancelCancellation := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCancellation()
	cancellation := startCancellationRaceCall(group, cancellationCtx, cancellationService, fixtures.engineer, booking.ID)
	cancellationPID := awaitBookingRacePID(t, cancellationCtx, cancellationStarted, "Activity-failing cancellation transaction")
	if gatePID := appendGate.await(t, cancellationCtx, "Booking updated before cancellation Activity append"); gatePID != cancellationPID {
		t.Fatalf("pre-append gate backend PID=%d, transaction PID=%d", gatePID, cancellationPID)
	}

	stateStarted := make(chan int32, 1)
	stateClock := newAtomicBookingClockAt(now)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation waiting on Activity-failing cancellation")
	waitCtx, cancelWait := context.WithTimeout(stateCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, statePID, cancellationPID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe deactivation waiting for cancellation that will fail Activity: %v", err)
	}
	logBookingRaceWait(t, "deactivation waits for uncommitted cancellation before Activity failure", wait)

	appendGate.open()
	cancellationOutcome := awaitCancellationRaceCall(t, cancellationCtx, cancellation)
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	removeFailure()
	if !errors.Is(cancellationOutcome.err, bookings.ErrOperational) || strings.Contains(cancellationOutcome.err.Error(), "injected owner cancellation Activity failure") {
		t.Fatalf("Activity-failing cancellation error=%v, want safe operational failure", cancellationOutcome.err)
	}
	if !errors.Is(stateOutcome.err, resources.ErrResourceOccupied) || stateOutcome.result != (resources.StateResult{}) {
		t.Fatalf("deactivation after cancellation rollback result=%+v error=%v, want occupied", stateOutcome.result, stateOutcome.err)
	}
	if cancellationClock.calls.Load() != 1 || stateClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: failed cancellation=%d waiting deactivation=%d, want one each", cancellationClock.calls.Load(), stateClock.calls.Load())
	}
	assertCancellationRaceBooking(t, admin, booking, bookings.StateConfirmed, "", nil)
	if afterBooking := resourceStateBookingSnapshot(t, admin, booking.ID); afterBooking != beforeBooking {
		t.Fatalf("failed cancellation did not restore exact Booking snapshot: before=%s after=%s", beforeBooking, afterBooking)
	}
	if afterRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); afterRequest != beforeRequest {
		t.Fatalf("failed cancellation changed request/creation Activity: before=%s after=%s", beforeRequest, afterRequest)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("failed cancellation/deactivation changed Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
	if events := readResourceCapabilityEvents(t, admin, resourceID); len(events) != 0 {
		t.Fatalf("rollback schedule persisted Resource state events: %+v", events)
	}
}

func TestResourceStateCancellationRaceCoordinatorCannotCancelForeignConfirmedOrCancelledBooking(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	creationClock := newAtomicBookingClockAt(now)
	booking, request := createCancellationRaceBooking(t, admin, app, fixtures, bookingIDs,
		resourceID, fixtures.engineer, creationClock, now, time.Hour)
	beforeConfirmed := resourceStateBookingSnapshot(t, admin, booking.ID)
	beforeRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID)
	coordinatorStarted := make(chan int32, 3)
	coordinatorClock := newAtomicBookingClockAt(now)
	coordinatorService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: coordinatorStarted,
	}, coordinatorClock.Now)

	if result, err := coordinatorService.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, booking.ID); !errors.Is(err, bookings.ErrBookingNotAccessible) || result.Changed {
		t.Fatalf("Coordinator cancellation of another owner's Confirmed Booking result=%+v error=%v", result, err)
	}
	<-coordinatorStarted
	if afterConfirmed := resourceStateBookingSnapshot(t, admin, booking.ID); afterConfirmed != beforeConfirmed {
		t.Fatalf("Coordinator changed another owner's Confirmed Booking: before=%s after=%s", beforeConfirmed, afterConfirmed)
	}
	if afterRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); afterRequest != beforeRequest {
		t.Fatalf("Coordinator Confirmed attempt changed request/history: before=%s after=%s", beforeRequest, afterRequest)
	}

	ownerClock := newAtomicBookingClockAt(now)
	ownerService := bookings.NewCancellationService(&cancellationRaceRepository{
		base: NewStore(app), started: make(chan int32, 1),
	}, ownerClock.Now)
	if result, err := ownerService.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, booking.ID); err != nil || !result.Changed {
		t.Fatalf("owner cancellation before Coordinator retry result=%+v error=%v", result, err)
	}
	assertCancellationRaceBooking(t, admin, booking, bookings.StateCancelled, fixtures.engineer, &now)
	cancelledSnapshot := resourceStateBookingSnapshot(t, admin, booking.ID)
	cancelledRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID)

	if result, err := coordinatorService.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, booking.ID); !errors.Is(err, bookings.ErrBookingNotAccessible) || result.Changed {
		t.Fatalf("Coordinator cancellation of another owner's Cancelled Booking result=%+v error=%v", result, err)
	}
	<-coordinatorStarted
	if afterCancelled := resourceStateBookingSnapshot(t, admin, booking.ID); afterCancelled != cancelledSnapshot {
		t.Fatalf("Coordinator used another owner's Cancelled no-op: before=%s after=%s", cancelledSnapshot, afterCancelled)
	}
	if afterRequest := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); afterRequest != cancelledRequest {
		t.Fatalf("Coordinator Cancelled retry changed request/history: before=%s after=%s", cancelledRequest, afterRequest)
	}
	if coordinatorClock.calls.Load() != 0 {
		t.Fatalf("unauthorized Coordinator attempts sampled time %d times, want zero", coordinatorClock.calls.Load())
	}
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 1, 1)
}

func createCancellationRaceBooking(
	t *testing.T,
	admin, app *pgxpool.Pool,
	fixtures *resourceCapabilityFixtures,
	bookingIDs *[]string,
	resourceID, ownerID string,
	clock *atomicBookingClock,
	now time.Time,
	startAfter time.Duration,
) (bookings.Booking, bookings.CreateRequest) {
	t.Helper()
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, startAfter, time.Hour)
	creator := bookings.NewService(NewStore(app), clock.Now)
	created, err := creator.Create(t.Context(), bookings.Actor{AccountID: ownerID}, request)
	if err != nil {
		t.Fatalf("create cancellation/deactivation Booking fixture: %v", err)
	}
	*bookingIDs = append(*bookingIDs, created.Booking.ID)
	return created.Booking, request
}

func assertCancellationRaceBooking(
	t *testing.T,
	admin *pgxpool.Pool,
	original bookings.Booking,
	wantState bookings.State,
	wantCancelledBy string,
	wantCancelledAt *time.Time,
) {
	t.Helper()
	var id, resourceID, ownerID, requestID, purpose, state, cancelledBy string
	var startAt, endAt, createdAt, cancelledAt time.Time
	var cancelledAtNull bool
	err := admin.QueryRow(t.Context(), `
		SELECT id::text, resource_id::text, owner_account_id::text, request_id::text,
		       start_at, end_at, purpose, state, created_at,
		       COALESCE(cancelled_by_account_id::text, ''), cancelled_at IS NULL,
		       COALESCE(cancelled_at, 'epoch'::timestamptz)
		FROM public.bookings WHERE id = $1::uuid`, original.ID).Scan(
		&id, &resourceID, &ownerID, &requestID, &startAt, &endAt, &purpose, &state, &createdAt,
		&cancelledBy, &cancelledAtNull, &cancelledAt)
	if err != nil {
		t.Fatalf("read retained cancellation-race Booking: %v", err)
	}
	if id != original.ID || resourceID != original.ResourceID || ownerID != original.OwnerAccountID ||
		requestID != original.RequestID || !startAt.Equal(original.StartAt) || !endAt.Equal(original.EndAt) ||
		purpose != original.Purpose || !createdAt.Equal(original.CreatedAt) || state != string(wantState) {
		t.Fatalf("Booking identity/state changed: id=%s resource=%s owner=%s request=%s interval=%s..%s purpose=%q created=%s state=%s",
			id, resourceID, ownerID, requestID, startAt, endAt, purpose, createdAt, state)
	}
	if wantCancelledAt == nil {
		if cancelledBy != "" || !cancelledAtNull {
			t.Fatalf("unmodified Booking cancellation metadata = %q/%v, want NULL/NULL", cancelledBy, !cancelledAtNull)
		}
	} else if cancelledBy != wantCancelledBy || cancelledAtNull || !cancelledAt.Equal(*wantCancelledAt) {
		t.Fatalf("cancellation metadata = %q/%s null=%v, want %q/%s", cancelledBy, cancelledAt, cancelledAtNull, wantCancelledBy, *wantCancelledAt)
	}

	var creationEvents, cancellationEvents, totalEvents int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'),
		       count(*) FILTER (WHERE action = 'booking.cancelled'), count(*)
		FROM public.activity_events WHERE booking_id = $1::uuid`, original.ID).Scan(
		&creationEvents, &cancellationEvents, &totalEvents); err != nil {
		t.Fatalf("count retained Booking Activity: %v", err)
	}
	wantCancellationEvents := 0
	if wantCancelledAt != nil {
		wantCancellationEvents = 1
	}
	if creationEvents != 1 || cancellationEvents != wantCancellationEvents || totalEvents != 1+wantCancellationEvents {
		t.Fatalf("Booking Activity counts created/cancelled/total=%d/%d/%d, want 1/%d/%d",
			creationEvents, cancellationEvents, totalEvents, wantCancellationEvents, 1+wantCancellationEvents)
	}
	var createdActor, createdAction, createdResource, createdRequest, createdPurpose string
	var createdAtEvent time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT actor_account_id::text, action, occurred_at,
		       details->>'resource_id', details->>'request_id', details->>'purpose'
		FROM public.activity_events WHERE booking_id = $1::uuid AND action = 'booking.created'`, original.ID).Scan(
		&createdActor, &createdAction, &createdAtEvent, &createdResource, &createdRequest, &createdPurpose); err != nil {
		t.Fatalf("read retained Booking creation Activity: %v", err)
	}
	if createdActor != original.OwnerAccountID || createdAction != "booking.created" ||
		!createdAtEvent.Equal(original.CreatedAt) || createdResource != original.ResourceID ||
		createdRequest != original.RequestID || createdPurpose != original.Purpose {
		t.Fatalf("retained creation Activity changed: actor=%s action=%s at=%s resource=%s request=%s purpose=%q",
			createdActor, createdAction, createdAtEvent, createdResource, createdRequest, createdPurpose)
	}
	if wantCancelledAt != nil {
		var actor, action, eventResource string
		var occurredAt, eventStart, eventEnd time.Time
		if err := admin.QueryRow(t.Context(), `
			SELECT actor_account_id::text, action, occurred_at, details->>'resource_id',
			       (details->>'start_at')::timestamptz, (details->>'end_at')::timestamptz
			FROM public.activity_events WHERE booking_id = $1::uuid AND action = 'booking.cancelled'`, original.ID).Scan(
			&actor, &action, &occurredAt, &eventResource, &eventStart, &eventEnd); err != nil {
			t.Fatalf("read retained Booking cancellation Activity: %v", err)
		}
		if actor != wantCancelledBy || action != "booking.cancelled" || !occurredAt.Equal(*wantCancelledAt) ||
			eventResource != original.ResourceID || !eventStart.Equal(original.StartAt) || !eventEnd.Equal(original.EndAt) {
			t.Fatalf("cancellation Activity mismatch: actor=%s action=%s at=%s resource=%s interval=%s..%s",
				actor, action, occurredAt, eventResource, eventStart, eventEnd)
		}
	}
}

type cancellationRaceOutcome struct {
	result bookings.CancellationResult
	err    error
}

type cancellationRaceCall struct {
	result chan cancellationRaceOutcome
	done   chan struct{}
}

func startCancellationRaceCall(
	group *bookingRaceGroup,
	ctx context.Context,
	service *bookings.CancellationService,
	actorID, bookingID string,
) *cancellationRaceCall {
	call := &cancellationRaceCall{result: make(chan cancellationRaceOutcome, 1), done: make(chan struct{})}
	group.track(call.done)
	go func() {
		defer close(call.done)
		result, err := service.Cancel(ctx, bookings.Actor{AccountID: actorID}, bookingID)
		call.result <- cancellationRaceOutcome{result: result, err: err}
	}()
	return call
}

func awaitCancellationRaceCall(t *testing.T, ctx context.Context, call *cancellationRaceCall) cancellationRaceOutcome {
	t.Helper()
	select {
	case <-call.done:
		return <-call.result
	case <-ctx.Done():
		t.Fatalf("owner cancellation did not finish: %v", ctx.Err())
		return cancellationRaceOutcome{}
	}
}

type cancellationRaceRepository struct {
	base         bookings.CancellationRepository
	started      chan<- int32
	beforeAppend *bookingRaceGate
	beforeCommit *bookingRaceGate
}

func (repository *cancellationRaceRepository) BeginBookingCancellation(ctx context.Context) (bookings.CancellationTransaction, error) {
	transaction, err := repository.base.BeginBookingCancellation(ctx)
	if err != nil {
		return nil, err
	}
	concrete, ok := transaction.(*bookingCancellationTransaction)
	if !ok {
		_ = transaction.Rollback(context.Background())
		return nil, fmt.Errorf("unexpected cancellation transaction type %T", transaction)
	}
	pid, err := requireRuntimeTransaction(ctx, concrete.tx)
	if err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	if err := sendBookingRacePID(ctx, repository.started, pid); err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	return &cancellationRaceTransaction{
		CancellationTransaction: transaction, raw: concrete, pid: pid,
		beforeAppend: repository.beforeAppend, beforeCommit: repository.beforeCommit,
	}, nil
}

type cancellationRaceTransaction struct {
	bookings.CancellationTransaction
	raw          *bookingCancellationTransaction
	pid          int32
	bookingID    string
	actorID      string
	occurredAt   time.Time
	beforeAppend *bookingRaceGate
	beforeCommit *bookingRaceGate
}

func (tx *cancellationRaceTransaction) AppendBookingCancelledEvent(ctx context.Context, event bookings.BookingCancelledEvent) error {
	tx.bookingID = event.Booking.ID
	tx.actorID = event.ActorAccountID
	tx.occurredAt = event.OccurredAt
	if tx.beforeAppend != nil {
		if err := tx.assertPendingCancellation(ctx, event.Booking.ID, event.ActorAccountID, event.OccurredAt, false); err != nil {
			return err
		}
		if err := tx.beforeAppend.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.CancellationTransaction.AppendBookingCancelledEvent(ctx, event)
}

func (tx *cancellationRaceTransaction) Commit(ctx context.Context) error {
	if tx.beforeCommit != nil {
		if tx.bookingID == "" {
			return fmt.Errorf("cancellation transaction reached commit barrier before Activity append")
		}
		if err := tx.assertPendingCancellation(ctx, tx.bookingID, tx.actorID, tx.occurredAt, true); err != nil {
			return err
		}
		if err := tx.beforeCommit.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.CancellationTransaction.Commit(ctx)
}

func (tx *cancellationRaceTransaction) assertPendingCancellation(
	ctx context.Context,
	bookingID, actorID string,
	occurredAt time.Time,
	wantEvent bool,
) error {
	var state, cancelledBy string
	var cancelledAt time.Time
	var eventCount int
	err := tx.raw.tx.QueryRow(ctx, `
		SELECT booking.state, booking.cancelled_by_account_id::text, booking.cancelled_at,
		       (SELECT count(*) FROM public.activity_events AS event
		        WHERE event.booking_id = booking.id AND event.action = 'booking.cancelled'
		          AND event.actor_account_id = $2::uuid AND event.occurred_at = $3::timestamptz)
		FROM public.bookings AS booking WHERE booking.id = $1::uuid`,
		bookingID, actorID, occurredAt).Scan(&state, &cancelledBy, &cancelledAt, &eventCount)
	if err != nil {
		return fmt.Errorf("inspect cancellation transaction before test barrier: %w", err)
	}
	wantEventCount := 0
	if wantEvent {
		wantEventCount = 1
	}
	if state != "cancelled" || cancelledBy != actorID || !cancelledAt.Equal(occurredAt) || eventCount != wantEventCount {
		return fmt.Errorf("pending cancellation state/metadata/event mismatch: state=%s actor=%s at=%s events=%d want=%d",
			state, cancelledBy, cancelledAt, eventCount, wantEventCount)
	}
	return nil
}

func installCancellationRaceActivityFailure(t *testing.T, admin *pgxpool.Pool, actorID, bookingID string) func() {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionID := pgx.Identifier{"public", "labreserve_c7_cancel_event_fail_" + suffix}.Sanitize()
	triggerID := pgx.Identifier{"labreserve_c7_cancel_event_fail_" + suffix}.Sanitize()
	sqlLiteral := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.actor_account_id = %s::uuid AND NEW.booking_id = %s::uuid
			   AND NEW.action = 'booking.cancelled' THEN
				RAISE EXCEPTION 'injected owner cancellation Activity failure';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionID, sqlLiteral(actorID), sqlLiteral(bookingID))); err != nil {
		t.Fatalf("create scoped owner cancellation Activity failure trigger function: %v", err)
	}
	cleanup := installBookingRaceTriggerCleanup(t, admin, triggerID, functionID)
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON public.activity_events
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerID, functionID)); err != nil {
		t.Fatalf("install scoped owner cancellation Activity failure trigger: %v", err)
	}
	return cleanup
}
