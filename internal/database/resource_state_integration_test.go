package database

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceStateActivationServiceEligibilityAndRetention(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456789, time.UTC).Truncate(time.Microsecond)
	createdAt := operationAt.Add(-24 * time.Hour)
	resourceCode := "State-Service-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	resourceID := insertResourceCapabilityFixture(t, admin, resourceCode,
		"  Legacy Resource Name  ", "<b>legacy description</b> ", false, createdAt)
	fixtures.trackResource(resourceID)
	insertResourceStateCreationHistory(t, admin, fixtures.coordinator, resourceID, createdAt)

	confirmedID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	cancelledID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
		operationAt.Add(30*time.Minute), operationAt.Add(90*time.Minute), createdAt, "cancelled", fixtures.secondEngineer)
	*bookingIDs = append(*bookingIDs, confirmedID, cancelledID)
	confirmedBefore := resourceStateBookingSnapshot(t, admin, confirmedID)
	cancelledBefore := resourceStateBookingSnapshot(t, admin, cancelledID)

	clock := &atomicBookingClock{}
	clock.Set(operationAt)
	service := resources.NewStateService(NewStore(app), clock.Now)
	result, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	})
	if err != nil || !result.Changed || !result.Resource.Active || result.Resource.ID != resourceID ||
		result.Resource.Code != resourceCode || result.Resource.Name != "  Legacy Resource Name  " ||
		result.Resource.Description != "<b>legacy description</b> " || !result.Resource.CreatedAt.Equal(createdAt) ||
		!result.Resource.UpdatedAt.Equal(operationAt) || clock.calls.Load() != 1 {
		t.Fatalf("activate retained Resource through runtime service: result=%+v clock calls=%d error=%v", result, clock.calls.Load(), err)
	}
	if got := resourceStateBookingSnapshot(t, admin, confirmedID); got != confirmedBefore {
		t.Fatalf("activation changed Confirmed Booking: before=%s after=%s", confirmedBefore, got)
	}
	if got := resourceStateBookingSnapshot(t, admin, cancelledID); got != cancelledBefore {
		t.Fatalf("activation changed Cancelled Booking: before=%s after=%s", cancelledBefore, got)
	}

	var active bool
	var persistedCode, persistedName, persistedDescription string
	var persistedCreatedAt, persistedUpdatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(
		&persistedCode, &persistedName, &persistedDescription, &active, &persistedCreatedAt, &persistedUpdatedAt); err != nil {
		t.Fatalf("read activated Resource: %v", err)
	}
	if !active || persistedCode != resourceCode || persistedName != "  Legacy Resource Name  " ||
		persistedDescription != "<b>legacy description</b> " || !persistedCreatedAt.Equal(createdAt) ||
		!persistedUpdatedAt.Equal(operationAt) {
		t.Fatalf("activation did not preserve Resource fields: code=%q name=%q description=%q active=%v created=%s updated=%s",
			persistedCode, persistedName, persistedDescription, active, persistedCreatedAt, persistedUpdatedAt)
	}
	assertResourceStateActivationHistory(t, admin, resourceID, fixtures.coordinator, createdAt, operationAt)

	view, found, err := NewStore(app).ResourceByID(t.Context(), resourceID)
	if err != nil || !found || !view.Active {
		t.Fatalf("existing Resource read seam after activation: found=%v Resource=%+v error=%v", found, view, err)
	}
	listed, err := NewStore(app).ListResources(t.Context())
	if err != nil || !containsResource(listed, resourceID) {
		t.Fatalf("existing Resource list after activation: found=%v error=%v", containsResource(listed, resourceID), err)
	}

	bookingService := bookings.NewService(NewStore(app), clock.Now)
	newRequest := bookings.CreateRequest{
		ResourceID: resourceID, RequestID: testDatabaseUUID(t, admin),
		StartAt: operationAt.Add(3 * time.Hour), EndAt: operationAt.Add(4 * time.Hour),
		Purpose: "valid booking after activation",
	}
	createdBooking, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, newRequest)
	if err != nil || createdBooking.Replayed || createdBooking.Booking.ID == "" ||
		createdBooking.Booking.State != bookings.StateConfirmed || createdBooking.Booking.ResourceID != resourceID {
		t.Fatalf("ordinary Engineer booking after activation: result=%+v error=%v", createdBooking, err)
	}
	*bookingIDs = append(*bookingIDs, createdBooking.Booking.ID)

	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	for index, engineerID := range []string{fixtures.engineer, fixtures.secondEngineer} {
		if _, err := service.SetState(t.Context(), resources.Actor{AccountID: engineerID}, resources.StateRequest{
			ResourceID: resourceID, Active: true,
		}); !errors.Is(err, resources.ErrForbidden) {
			t.Fatalf("Engineer %d matching-state request error = %v, want forbidden", index+1, err)
		}
	}
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.unknown}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	}); !errors.Is(err, resources.ErrUnauthenticated) {
		t.Fatalf("unknown actor activation error = %v, want unauthenticated", err)
	}
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: "not-a-uuid"}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	}); !errors.Is(err, resources.ErrUnauthenticated) {
		t.Fatalf("malformed actor activation error = %v, want unauthenticated", err)
	}
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: "not-a-uuid", Active: true,
	}); !errors.Is(err, resources.ErrInvalidInput) {
		t.Fatalf("malformed Resource ID error = %v, want invalid input", err)
	}
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: testDatabaseUUID(t, admin), Active: true,
	}); !errors.Is(err, resources.ErrUnknownResource) {
		t.Fatalf("unknown Resource activation error = %v, want unknown Resource", err)
	}
	afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	if afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("unauthorized/rejected state submissions changed Resource/history: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
}

func TestResourceStateActivationFailureRollsBackAndNoOpDoesNoWrites(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	operationAt := time.Date(2040, time.February, 3, 4, 5, 6, 123456000, time.UTC)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Failure-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Failure Lab", "Retained text", false, operationAt.Add(-time.Hour))
	fixtures.trackResource(resourceID)
	insertResourceStateCreationHistory(t, admin, fixtures.coordinator, resourceID, operationAt.Add(-time.Hour))
	service := resources.NewStateService(NewStore(app), func() time.Time { return operationAt })
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	removeFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.activated")
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	}); !errors.Is(err, resources.ErrOperational) {
		t.Fatalf("activation with required Activity failure = %v, want operational failure", err)
	}
	removeFailure()
	afterFailureResource, afterFailureEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	if afterFailureResource != beforeResource || afterFailureEvents != beforeEvents {
		t.Fatalf("Activity failure left partial activation: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterFailureResource, afterFailureEvents)
	}

	retry, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	})
	if err != nil || !retry.Changed || !retry.Resource.Active || !retry.Resource.UpdatedAt.Equal(operationAt) {
		t.Fatalf("explicit activation retry: result=%+v error=%v", retry, err)
	}
	assertResourceStateActivationHistory(t, admin, resourceID, fixtures.coordinator,
		operationAt.Add(-time.Hour), operationAt)
	beforeNoOpResource, beforeNoOpEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	installResourceUpdateFailureTrigger(t, admin, resourceID)
	removeNoOpEventFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.activated")
	noOpClockCalls := atomic.Int32{}
	noOpService := resources.NewStateService(NewStore(app), func() time.Time {
		noOpClockCalls.Add(1)
		return operationAt.Add(time.Hour)
	})
	noOp, err := noOpService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	})
	removeNoOpEventFailure()
	if err != nil || noOp.Changed || !noOp.Resource.Active || !noOp.Resource.UpdatedAt.Equal(operationAt) || noOpClockCalls.Load() != 0 {
		t.Fatalf("active no-op with UPDATE/event failure detectors: result=%+v clock calls=%d error=%v",
			noOp, noOpClockCalls.Load(), err)
	}
	afterNoOpResource, afterNoOpEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	if beforeNoOpResource != afterNoOpResource || beforeNoOpEvents != afterNoOpEvents {
		t.Fatalf("active no-op attempted a write or event: before=%s %s after=%s %s",
			beforeNoOpResource, beforeNoOpEvents, afterNoOpResource, afterNoOpEvents)
	}
}

func TestResourceStateActivationUsesFreshTimeAfterResourceLockWait(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	initialTime := time.Date(2040, time.March, 4, 5, 6, 7, 123456000, time.UTC)
	postWaitTime := initialTime.Add(2 * time.Hour).Truncate(time.Microsecond)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Wait-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Wait Lab", "Preserve after wait", false, initialTime.Add(-time.Hour))
	fixtures.trackResource(resourceID)
	clock := &atomicBookingClock{}
	clock.Set(initialTime)
	service := resources.NewStateService(NewStore(app), clock.Now)

	held := holdBookingResourceLock(t, admin, resourceID)
	callCtx, cancelCall := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelCall()
	type activationOutcome struct {
		result resources.StateResult
		err    error
	}
	resultCh := make(chan activationOutcome, 1)
	go func() {
		result, err := service.SetState(callCtx, resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
			ResourceID: resourceID, Active: true,
		})
		resultCh <- activationOutcome{result: result, err: err}
	}()
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 10*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, admin, held.pid, 1); err != nil {
		cancelWait()
		_ = held.release()
		t.Fatalf("observe activation waiting for Resource FOR UPDATE: %v", err)
	}
	cancelWait()
	if calls := clock.calls.Load(); calls != 0 {
		_ = held.release()
		t.Fatalf("activation clock read %d times before Resource lock release", calls)
	}
	clock.Set(postWaitTime)
	if err := held.release(); err != nil {
		t.Fatalf("release Resource lock after observed activation wait: %v", err)
	}

	var outcome activationOutcome
	select {
	case outcome = <-resultCh:
	case <-callCtx.Done():
		t.Fatalf("activation did not finish after Resource lock release: %v", callCtx.Err())
	}
	if outcome.err != nil || !outcome.result.Changed || !outcome.result.Resource.Active ||
		!outcome.result.Resource.UpdatedAt.Equal(postWaitTime) || clock.calls.Load() != 1 {
		t.Fatalf("post-wait activation result=%+v clock calls=%d error=%v", outcome.result, clock.calls.Load(), outcome.err)
	}
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 1 || events[0].ActorID != fixtures.coordinator || events[0].Action != "resource.activated" ||
		!events[0].OccurredAt.Equal(postWaitTime) || !events[0].BookingIsNil {
		t.Fatalf("post-wait activation Activity = %+v", events)
	}
}

func insertResourceStateCreationHistory(t *testing.T, admin *pgxpool.Pool, actorID, resourceID string, occurredAt time.Time) {
	t.Helper()
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'resource.created', $2, $3::uuid,
		        jsonb_build_object('active', false, 'source', 'retained state fixture'))`, actorID, occurredAt, resourceID); err != nil {
		t.Fatalf("create retained Resource history fixture: %v", err)
	}
}

func assertResourceStateActivationHistory(
	t *testing.T,
	admin *pgxpool.Pool,
	resourceID, actorID string,
	createdAt, activatedAt time.Time,
) {
	t.Helper()
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 2 || events[0].ActorID != actorID || events[0].Action != "resource.created" ||
		!events[0].OccurredAt.Equal(createdAt) || events[1].ActorID != actorID ||
		events[1].Action != "resource.activated" || !events[1].OccurredAt.Equal(activatedAt) ||
		!events[0].BookingIsNil || !events[1].BookingIsNil {
		t.Fatalf("retained Resource history and activation event = %+v", events)
	}
	before, beforeOK := events[1].Details["before"].(map[string]any)
	after, afterOK := events[1].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != false || after["active"] != true {
		t.Fatalf("activation event details = %#v, want inactive to active", events[1].Details)
	}
	var resourceEventCount int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid AND action = 'resource.activated'`, resourceID).
		Scan(&resourceEventCount); err != nil {
		t.Fatalf("count activation events: %v", err)
	}
	if resourceEventCount != 1 {
		t.Fatalf("activation event count = %d, want exactly one", resourceEventCount)
	}
}
