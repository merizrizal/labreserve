package database

import (
	"context"
	"errors"
	"fmt"
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

func TestResourceStateOppositeTransitionsAtEqualOperationTime(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	operationAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456789, time.UTC).Truncate(time.Microsecond)
	createdAt := operationAt.Add(-time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Equal-Time-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Equal Time Lab", "Retained", false, createdAt)
	fixtures.trackResource(resourceID)
	insertResourceStateCreationHistory(t, admin, fixtures.coordinator, resourceID, createdAt)

	clock := &atomicBookingClock{}
	clock.Set(operationAt)
	service := resources.NewStateService(NewStore(app), clock.Now)
	activated, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: true,
	})
	if err != nil || !activated.Changed || !activated.Resource.Active || !activated.Resource.UpdatedAt.Equal(operationAt) || clock.calls.Load() != 1 {
		t.Fatalf("first transition at fixed operation time: result=%+v clock calls=%d error=%v",
			activated, clock.calls.Load(), err)
	}
	assertResourceStateActivationHistory(t, admin, resourceID, fixtures.coordinator, createdAt, operationAt)

	// Observe the first committed state before requesting the opposite transition.
	// Equal event times and random IDs do not establish causal event ordering.
	var intermediateActive bool
	var intermediateUpdatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT active, updated_at FROM public.resources WHERE id = $1::uuid`, resourceID).
		Scan(&intermediateActive, &intermediateUpdatedAt); err != nil {
		t.Fatalf("read Resource after activation: %v", err)
	}
	if !intermediateActive || !intermediateUpdatedAt.Equal(operationAt) {
		t.Fatalf("persisted state after first transition: active=%v updated_at=%s", intermediateActive, intermediateUpdatedAt)
	}

	deactivated, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: false,
	})
	if err != nil || !deactivated.Changed || deactivated.Resource.Active || !deactivated.Resource.UpdatedAt.Equal(operationAt) || clock.calls.Load() != 2 {
		t.Fatalf("opposite transition at the same operation time: result=%+v clock calls=%d error=%v",
			deactivated, clock.calls.Load(), err)
	}

	var persistedActive bool
	var persistedUpdatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT active, updated_at FROM public.resources WHERE id = $1::uuid`, resourceID).
		Scan(&persistedActive, &persistedUpdatedAt); err != nil {
		t.Fatalf("read Resource after deactivation: %v", err)
	}
	if persistedActive || !persistedUpdatedAt.Equal(operationAt) {
		t.Fatalf("final persisted state after equal-time transitions: active=%v updated_at=%s", persistedActive, persistedUpdatedAt)
	}

	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 3 {
		t.Fatalf("equal-time transition history has %d events, want creation plus both transitions: %+v", len(events), events)
	}
	eventsByAction := make(map[string]*resourceCapabilityEvent, 3)
	for i := range events {
		action := events[i].Action
		if action != "resource.created" && action != "resource.activated" && action != "resource.deactivated" {
			t.Fatalf("unexpected Activity action in equal-time transitions: %+v", events[i])
		}
		if eventsByAction[action] != nil {
			t.Fatalf("duplicate %s event: %+v", action, events)
		}
		eventsByAction[action] = &events[i]
	}
	created := eventsByAction["resource.created"]
	if created == nil || created.ActorID != fixtures.coordinator || !created.OccurredAt.Equal(createdAt) || !created.BookingIsNil {
		t.Fatalf("retained creation event after equal-time transitions = %+v", created)
	}
	for _, expected := range []struct {
		action string
		before bool
		after  bool
	}{
		{action: "resource.activated", before: false, after: true},
		{action: "resource.deactivated", before: true, after: false},
	} {
		event := eventsByAction[expected.action]
		if event == nil || event.ActorID != fixtures.coordinator || !event.BookingIsNil || !event.OccurredAt.Equal(operationAt) {
			t.Fatalf("persisted %s event = %+v, want one Coordinator event at %s", expected.action, event, operationAt)
		}
		before, beforeOK := event.Details["before"].(map[string]any)
		after, afterOK := event.Details["after"].(map[string]any)
		if !beforeOK || !afterOK || before["active"] != expected.before || after["active"] != expected.after {
			t.Fatalf("persisted %s transition details = %#v, want %v -> %v",
				expected.action, event.Details, expected.before, expected.after)
		}
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

func TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	createdAt := operationAt.Add(-24 * time.Hour)
	activeID := insertResourceCapabilityFixture(t, admin,
		"State-Noop-Active-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Active No-op", "Retained", true, createdAt)
	inactiveID := insertResourceCapabilityFixture(t, admin,
		"State-Noop-Inactive-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Inactive No-op", "Retained", false, createdAt)
	fixtures.trackResource(activeID)
	fixtures.trackResource(inactiveID)
	futureStart, futureEnd := operationAt.Add(2*time.Hour), operationAt.Add(3*time.Hour)
	futureBookingID := insertResourceStateBooking(t, admin, inactiveID, fixtures.engineer,
		futureStart, futureEnd, createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, futureBookingID)
	insertResourceStateBookingHistory(t, admin, futureBookingID, fixtures.engineer, "confirmed", createdAt)

	before := make(map[string][2]string, 2)
	for _, resourceID := range []string{activeID, inactiveID} {
		resourceSnapshot, eventSnapshot := resourceCapabilitySnapshot(t, admin, resourceID)
		before[resourceID] = [2]string{resourceSnapshot, eventSnapshot}
		installResourceUpdateFailureTrigger(t, admin, resourceID)
	}
	futureBookingBefore := resourceStateBookingAndHistorySnapshot(t, admin, futureBookingID)
	removeActivationFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.activated")
	removeDeactivationFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
	defer removeActivationFailure()
	defer removeDeactivationFailure()

	clock := &atomicBookingClock{}
	clock.Set(operationAt)
	stateService := resources.NewStateService(NewStore(app), clock.Now)
	for _, test := range []struct {
		name       string
		resourceID string
		active     bool
	}{
		{name: "Active to Active", resourceID: activeID, active: true},
		{name: "Inactive to Inactive with future occupancy", resourceID: inactiveID, active: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := stateService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
				ResourceID: test.resourceID, Active: test.active,
			})
			if err != nil || result.Changed || result.Resource.Active != test.active || !result.Resource.UpdatedAt.Equal(createdAt) {
				t.Fatalf("matching desired-state result=%+v error=%v", result, err)
			}
			resourceSnapshot, eventSnapshot := resourceCapabilitySnapshot(t, admin, test.resourceID)
			if resourceSnapshot != before[test.resourceID][0] || eventSnapshot != before[test.resourceID][1] {
				t.Fatalf("no-op changed Resource/Activity: before=%s %s after=%s %s",
					before[test.resourceID][0], before[test.resourceID][1], resourceSnapshot, eventSnapshot)
			}
		})
	}
	if clock.calls.Load() != 0 {
		t.Fatalf("matching-state no-ops read operation time %d times", clock.calls.Load())
	}
	if after := resourceStateBookingAndHistorySnapshot(t, admin, futureBookingID); after != futureBookingBefore {
		t.Fatalf("inactive no-op changed future Booking/history: before=%s after=%s", futureBookingBefore, after)
	}

	bookingService := bookings.NewService(NewStore(app), func() time.Time { return operationAt })
	request := bookings.CreateRequest{
		ResourceID: inactiveID, RequestID: testDatabaseUUID(t, admin),
		StartAt: futureStart, EndAt: futureEnd, Purpose: "inactive takes precedence over overlap",
	}
	if _, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.secondEngineer}, request); !errors.Is(err, bookings.ErrInactiveResource) {
		t.Fatalf("overlapping creation on Inactive Resource error=%v, want inactive-resource outcome", err)
	}
	assertBookingRequestCounts(t, admin, fixtures.secondEngineer, request.RequestID, 0, 0)
	if after := resourceStateBookingAndHistorySnapshot(t, admin, futureBookingID); after != futureBookingBefore {
		t.Fatalf("rejected inactive creation changed retained Booking/history: before=%s after=%s", futureBookingBefore, after)
	}

	for _, resourceID := range []string{activeID, inactiveID} {
		for _, engineerID := range []string{fixtures.engineer, fixtures.secondEngineer} {
			if _, err := stateService.SetState(t.Context(), resources.Actor{AccountID: engineerID}, resources.StateRequest{
				ResourceID: resourceID, Active: resourceID == activeID,
			}); !errors.Is(err, resources.ErrForbidden) {
				t.Fatalf("Engineer %s matching-state request for Resource %s error=%v, want forbidden", engineerID, resourceID, err)
			}
		}
		resourceSnapshot, eventSnapshot := resourceCapabilitySnapshot(t, admin, resourceID)
		if resourceSnapshot != before[resourceID][0] || eventSnapshot != before[resourceID][1] {
			t.Fatalf("unauthorized no-op changed Resource/Activity: Resource=%s before=%s %s after=%s %s",
				resourceID, before[resourceID][0], before[resourceID][1], resourceSnapshot, eventSnapshot)
		}
	}
}

func TestResourceStateDeactivationOccupancyAndEndBoundary(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.February, 3, 4, 5, 6, 123456000, time.UTC)
	createdAt := operationAt.Add(-24 * time.Hour)
	cases := []struct {
		name          string
		startOffset   time.Duration
		endOffset     time.Duration
		bookingState  string
		otherResource bool
		alsoRetained  bool
		otherOwner    bool
		blocked       bool
	}{
		{name: "all-owner all-date upcoming Confirmed blocks", startOffset: 48 * time.Hour, endOffset: 49 * time.Hour, bookingState: "confirmed", otherOwner: true, blocked: true},
		{name: "in-use Confirmed at exact start blocks", startOffset: 0, endOffset: time.Hour, bookingState: "confirmed", blocked: true},
		{name: "Confirmed end one microsecond later blocks", startOffset: -time.Hour, endOffset: time.Microsecond, bookingState: "confirmed", blocked: true},
		{name: "exact end permits deactivation", startOffset: -time.Hour, endOffset: 0, bookingState: "confirmed"},
		{name: "after end permits deactivation", startOffset: -time.Hour, endOffset: -time.Microsecond, bookingState: "confirmed"},
		{name: "Cancelled future Booking does not block", startOffset: time.Hour, endOffset: 2 * time.Hour, bookingState: "cancelled"},
		{name: "completed Confirmed Booking does not block", startOffset: -2 * time.Hour, endOffset: -time.Microsecond, bookingState: "confirmed"},
		{name: "another Resource does not block", startOffset: time.Hour, endOffset: 2 * time.Hour, bookingState: "confirmed", otherResource: true},
		{name: "one blocker among nonblockers still blocks", startOffset: 5 * time.Hour, endOffset: 6 * time.Hour, bookingState: "confirmed", alsoRetained: true, blocked: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resourceCode := "State-Service-Occupancy-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
			resourceID := insertResourceCapabilityFixture(t, admin,
				resourceCode, "Occupancy Lab", "Raw  retained text ", true, createdAt)
			fixtures.trackResource(resourceID)
			bookingResourceID := resourceID
			if test.otherResource {
				bookingResourceID = insertResourceCapabilityFixture(t, admin,
					"State-Service-Other-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
					"Other Lab", "Other Resource", true, createdAt)
				fixtures.trackResource(bookingResourceID)
			}
			var retainedIDs []string
			if test.alsoRetained {
				endedID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
					operationAt.Add(-4*time.Hour), operationAt.Add(-3*time.Hour), createdAt, "confirmed", "")
				cancelledID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
					operationAt.Add(8*time.Hour), operationAt.Add(9*time.Hour), createdAt, "cancelled", fixtures.engineer)
				for _, fixture := range []struct{ id, owner, state string }{
					{endedID, fixtures.secondEngineer, "confirmed"}, {cancelledID, fixtures.engineer, "cancelled"},
				} {
					insertResourceStateBookingHistory(t, admin, fixture.id, fixture.owner, fixture.state, createdAt)
				}
				retainedIDs = append(retainedIDs, endedID, cancelledID)
				*bookingIDs = append(*bookingIDs, endedID, cancelledID)
			}
			bookingOwner := fixtures.engineer
			if test.otherOwner {
				bookingOwner = fixtures.secondEngineer
			}
			cancelledBy := ""
			if test.bookingState == "cancelled" {
				cancelledBy = bookingOwner
			}
			bookingID := insertResourceStateBooking(t, admin, bookingResourceID, bookingOwner,
				operationAt.Add(test.startOffset), operationAt.Add(test.endOffset), createdAt, test.bookingState, cancelledBy)
			insertResourceStateBookingHistory(t, admin, bookingID, bookingOwner, test.bookingState, createdAt)
			*bookingIDs = append(*bookingIDs, bookingID)
			allBookingIDs := append(retainedIDs, bookingID)
			beforeBookings := make(map[string]string, len(allBookingIDs))
			for _, id := range allBookingIDs {
				beforeBookings[id] = resourceStateBookingAndHistorySnapshot(t, admin, id)
			}
			beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
			if test.blocked {
				installResourceUpdateFailureTrigger(t, admin, resourceID)
				installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
			}
			service := resources.NewStateService(NewStore(app), func() time.Time { return operationAt })

			result, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
				ResourceID: resourceID, Active: false,
			})
			if test.blocked {
				if !errors.Is(err, resources.ErrResourceOccupied) || errors.Is(err, resources.ErrOperational) || result != (resources.StateResult{}) {
					t.Fatalf("blocked deactivation result=%+v error=%v, want distinct occupancy rejection", result, err)
				}
				afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID)
				if afterResource != beforeResource || afterEvents != beforeEvents {
					t.Fatalf("occupancy rejection changed Resource/Activity: before=%s %s after=%s %s",
						beforeResource, beforeEvents, afterResource, afterEvents)
				}
			} else {
				if err != nil || !result.Changed || result.Resource.Active || result.Resource.ID != resourceID ||
					result.Resource.Code != resourceCode || result.Resource.Name != "Occupancy Lab" ||
					result.Resource.Description != "Raw  retained text " ||
					!result.Resource.CreatedAt.Equal(createdAt) || !result.Resource.UpdatedAt.Equal(operationAt) {
					t.Fatalf("eligible deactivation result=%+v error=%v", result, err)
				}
				assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", operationAt, true, false)
				var persisted resources.Resource
				if err := admin.QueryRow(t.Context(), `
					SELECT id::text, code, name, description, active, created_at, updated_at
					FROM public.resources WHERE id = $1::uuid`, resourceID).
					Scan(&persisted.ID, &persisted.Code, &persisted.Name, &persisted.Description, &persisted.Active,
						&persisted.CreatedAt, &persisted.UpdatedAt); err != nil {
					t.Fatalf("read deactivated Resource: %v", err)
				}
				if persisted.ID != resourceID || persisted.Code != resourceCode || persisted.Name != "Occupancy Lab" ||
					persisted.Description != "Raw  retained text " || persisted.Active ||
					!persisted.CreatedAt.Equal(createdAt) || !persisted.UpdatedAt.Equal(operationAt) {
					t.Fatalf("persisted deactivation changed Resource fields: %+v", persisted)
				}
			}
			for _, id := range allBookingIDs {
				if after := resourceStateBookingAndHistorySnapshot(t, admin, id); after != beforeBookings[id] {
					t.Fatalf("deactivation changed retained Booking/history %s: before=%s after=%s", id, beforeBookings[id], after)
				}
			}
		})
	}
}

func TestResourceStateActivityPermissionFailureIsOperational(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	operationAt := time.Date(2040, time.May, 6, 7, 8, 9, 123456000, time.UTC)
	createdAt := operationAt.Add(-time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Activity-Permission-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Permission Failure Lab", "Retained", true, createdAt)
	fixtures.trackResource(resourceID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	removeFailure := installResourceStateActivityPermissionFailureTrigger(t, admin, fixtures.coordinator)
	service := resources.NewStateService(NewStore(app), func() time.Time { return operationAt })
	result, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: false,
	})
	removeFailure()
	if !errors.Is(err, resources.ErrOperational) || errors.Is(err, resources.ErrForbidden) || result != (resources.StateResult{}) {
		t.Fatalf("authorized deactivation with Activity SQLSTATE 42501: result=%+v error=%v, want safe operational failure", result, err)
	}
	if strings.Contains(err.Error(), "permission denied for Activity insertion") {
		t.Fatalf("operational error exposed the database diagnostic: %v", err)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("Activity SQLSTATE 42501 left partial deactivation: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}

	tx, err := NewStore(app).BeginResourceStateChange(t.Context())
	if err != nil {
		t.Fatalf("begin direct Coordinator-guard regression: %v", err)
	}
	_, changed, guardErr := tx.SetResourceState(t.Context(), fixtures.engineer, resourceID, false, operationAt)
	rollbackErr := tx.Rollback(t.Context())
	if !errors.Is(guardErr, resources.ErrForbidden) || changed || rollbackErr != nil {
		t.Fatalf("guarded capability Coordinator rejection: changed=%v error=%v rollback=%v, want forbidden", changed, guardErr, rollbackErr)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("Coordinator-guard rejection changed Resource/Activity: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
}

func installResourceStateActivityPermissionFailureTrigger(t *testing.T, admin *pgxpool.Pool, actorID string) func() {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := "labreserve_test_state_activity_42501_" + suffix
	triggerName := "labreserve_test_state_activity_42501_" + suffix
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION public.%s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF NEW.actor_account_id = '%s'::uuid AND NEW.action = 'resource.deactivated' THEN
		        RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'permission denied for Activity insertion';
		    END IF;
		    RETURN NEW;
		END;
		$test$`, functionName, actorID)); err != nil {
		t.Fatalf("create Activity SQLSTATE 42501 trigger function: %v", err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON public.activity_events"); err != nil {
			t.Errorf("drop Activity SQLSTATE 42501 trigger: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS public."+functionName+"()"); err != nil {
			t.Errorf("drop Activity SQLSTATE 42501 trigger function: %v", err)
		}
	}
	t.Cleanup(cleanup)
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE INSERT ON public.activity_events FOR EACH ROW EXECUTE FUNCTION public.%s()",
		triggerName, functionName)); err != nil {
		t.Fatalf("install Activity SQLSTATE 42501 trigger: %v", err)
	}
	return cleanup
}

func TestResourceStateDeactivationAdapterMapsGuardedOccupancyRejection(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.March, 3, 4, 5, 6, 123456000, time.UTC)
	createdAt := operationAt.Add(-24 * time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Adapter-Occupied-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Adapter Occupied Lab", "Retained", true, createdAt)
	fixtures.trackResource(resourceID)
	bookingID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, bookingID)
	insertResourceStateBookingHistory(t, admin, bookingID, fixtures.secondEngineer, "confirmed", createdAt)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	beforeBooking := resourceStateBookingAndHistorySnapshot(t, admin, bookingID)
	installResourceUpdateFailureTrigger(t, admin, resourceID)
	installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")

	tx, err := NewStore(app).BeginResourceStateChange(t.Context())
	if err != nil {
		t.Fatalf("begin guarded occupancy mapping transaction: %v", err)
	}
	resource, changed, stateErr := tx.SetResourceState(t.Context(), fixtures.coordinator, resourceID, false, operationAt)
	rollbackErr := tx.Rollback(t.Context())
	if !errors.Is(stateErr, resources.ErrResourceOccupied) || resource != (resources.Resource{}) || changed || rollbackErr != nil {
		t.Fatalf("guarded capability occupancy mapping: Resource=%+v changed=%v error=%v rollback=%v",
			resource, changed, stateErr, rollbackErr)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("guarded occupancy rejection changed Resource/Activity: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
	if after := resourceStateBookingAndHistorySnapshot(t, admin, bookingID); after != beforeBooking {
		t.Fatalf("guarded occupancy rejection changed Booking/history: before=%s after=%s", beforeBooking, after)
	}
}

func TestResourceStateDeactivationUsesFreshTimeAfterResourceLockWait(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	initialTime := time.Date(2040, time.March, 4, 5, 6, 7, 123456000, time.UTC)
	bookingEnd := initialTime.Add(time.Hour)
	cases := []struct {
		name      string
		releaseAt time.Time
		blocked   bool
	}{
		{name: "still one microsecond before end remains blocked", releaseAt: bookingEnd.Add(-time.Microsecond), blocked: true},
		{name: "wait across Booking end permits deactivation", releaseAt: bookingEnd.Add(time.Microsecond)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resourceID := insertResourceCapabilityFixture(t, admin,
				"State-Service-Wait-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
				"Wait Lab", "Retained after wait", true, initialTime.Add(-24*time.Hour))
			fixtures.trackResource(resourceID)
			bookingID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
				initialTime.Add(-30*time.Minute), bookingEnd, initialTime.Add(-24*time.Hour), "confirmed", "")
			insertResourceStateBookingHistory(t, admin, bookingID, fixtures.engineer, "confirmed", initialTime.Add(-24*time.Hour))
			*bookingIDs = append(*bookingIDs, bookingID)
			beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
			beforeBooking := resourceStateBookingAndHistorySnapshot(t, admin, bookingID)
			if test.blocked {
				installResourceUpdateFailureTrigger(t, admin, resourceID)
				installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
			}

			clock := &atomicBookingClock{}
			clock.Set(initialTime)
			service := resources.NewStateService(NewStore(app), clock.Now)
			held := holdBookingResourceLock(t, admin, resourceID)
			callCtx, cancelCall := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancelCall()
			type stateOutcome struct {
				result resources.StateResult
				err    error
			}
			resultCh := make(chan stateOutcome, 1)
			go func() {
				result, err := service.SetState(callCtx, resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
					ResourceID: resourceID, Active: false,
				})
				resultCh <- stateOutcome{result: result, err: err}
			}()
			waitCtx, cancelWait := context.WithTimeout(t.Context(), 10*time.Second)
			if err := waitForBlockedBookingCalls(waitCtx, admin, held.pid, 1); err != nil {
				cancelWait()
				_ = held.release()
				t.Fatalf("observe service waiting for Resource FOR UPDATE: %v", err)
			}
			cancelWait()
			if calls := clock.calls.Load(); calls != 0 {
				_ = held.release()
				t.Fatalf("deactivation clock read %d times before Resource lock release", calls)
			}
			clock.Set(test.releaseAt)
			if err := held.release(); err != nil {
				t.Fatalf("release Resource lock after observed deactivation wait: %v", err)
			}

			var outcome stateOutcome
			select {
			case outcome = <-resultCh:
			case <-callCtx.Done():
				t.Fatalf("deactivation did not finish after Resource lock release: %v", callCtx.Err())
			}
			if clock.calls.Load() != 1 {
				t.Fatalf("post-lock eligibility clock calls=%d, want exactly one", clock.calls.Load())
			}
			if test.blocked {
				if !errors.Is(outcome.err, resources.ErrResourceOccupied) || outcome.result != (resources.StateResult{}) {
					t.Fatalf("fresh-time before-end outcome=%+v error=%v, want occupancy rejection", outcome.result, outcome.err)
				}
				afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID)
				if afterResource != beforeResource || afterEvents != beforeEvents {
					t.Fatalf("pre-end lock-wait rejection changed Resource/Activity: before=%s %s after=%s %s",
						beforeResource, beforeEvents, afterResource, afterEvents)
				}
			} else {
				if outcome.err != nil || !outcome.result.Changed || outcome.result.Resource.Active ||
					!outcome.result.Resource.UpdatedAt.Equal(test.releaseAt) {
					t.Fatalf("fresh-time exact-end outcome=%+v error=%v", outcome.result, outcome.err)
				}
				events := readResourceCapabilityEvents(t, admin, resourceID)
				if len(events) != 1 || events[0].Action != "resource.deactivated" ||
					!events[0].OccurredAt.Equal(test.releaseAt) || events[0].ActorID != fixtures.coordinator {
					t.Fatalf("post-lock Activity time/action = %+v, want deactivation at %s", events, test.releaseAt)
				}
			}
			if after := resourceStateBookingAndHistorySnapshot(t, admin, bookingID); after != beforeBooking {
				t.Fatalf("lock-wait deactivation changed Booking/history: before=%s after=%s", beforeBooking, after)
			}
		})
	}
}

func TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.April, 5, 6, 7, 8, 123456000, time.UTC)
	createdAt := operationAt.Add(-24 * time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Service-Rollback-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Rollback Lab", "Raw legacy description ", true, createdAt)
	fixtures.trackResource(resourceID)
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'resource.created', $2, $3::uuid, '{"active":true,"fixture":true}'::jsonb)`,
		fixtures.coordinator, createdAt, resourceID); err != nil {
		t.Fatalf("insert retained Resource creation history: %v", err)
	}
	endedID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
		operationAt.Add(-3*time.Hour), operationAt.Add(-2*time.Hour), createdAt, "confirmed", "")
	cancelledID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "cancelled", fixtures.secondEngineer)
	for _, fixture := range []struct{ id, owner, state string }{
		{endedID, fixtures.engineer, "confirmed"}, {cancelledID, fixtures.secondEngineer, "cancelled"},
	} {
		insertResourceStateBookingHistory(t, admin, fixture.id, fixture.owner, fixture.state, createdAt)
	}
	*bookingIDs = append(*bookingIDs, endedID, cancelledID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	beforeBookings := map[string]string{
		endedID:     resourceStateBookingAndHistorySnapshot(t, admin, endedID),
		cancelledID: resourceStateBookingAndHistorySnapshot(t, admin, cancelledID),
	}

	outer, err := NewStore(app).BeginResourceStateChange(t.Context())
	if err != nil {
		t.Fatalf("begin outer deactivation transaction: %v", err)
	}
	rolledBack, changed, err := outer.SetResourceState(t.Context(), fixtures.coordinator, resourceID, false, operationAt)
	if err != nil || !changed || rolledBack.Active || !rolledBack.UpdatedAt.Equal(operationAt) {
		_ = outer.Rollback(context.Background())
		t.Fatalf("guarded deactivation inside outer transaction: Resource=%+v changed=%v error=%v", rolledBack, changed, err)
	}
	if err := outer.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back outer deactivation transaction: %v", err)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("outer rollback retained deactivation: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}

	clock := &atomicBookingClock{}
	clock.Set(operationAt)
	service := resources.NewStateService(NewStore(app), clock.Now)
	removeFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
	if _, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: false,
	}); !errors.Is(err, resources.ErrOperational) || strings.Contains(err.Error(), "injected Resource Activity failure") {
		removeFailure()
		t.Fatalf("deactivation Activity failure=%v, want safe operational error", err)
	}
	removeFailure()
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("Activity failure retained partial deactivation: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
	for id, snapshot := range beforeBookings {
		if after := resourceStateBookingAndHistorySnapshot(t, admin, id); after != snapshot {
			t.Fatalf("failed deactivation changed Booking/history %s: before=%s after=%s", id, snapshot, after)
		}
	}

	retry, err := service.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: resourceID, Active: false,
	})
	if err != nil || !retry.Changed || retry.Resource.Active || !retry.Resource.UpdatedAt.Equal(operationAt) {
		t.Fatalf("explicit deactivation retry: result=%+v error=%v", retry, err)
	}
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 2 || events[0].Action != "resource.created" || events[1].Action != "resource.deactivated" ||
		events[1].ActorID != fixtures.coordinator || !events[1].OccurredAt.Equal(operationAt) || !events[1].BookingIsNil {
		t.Fatalf("deactivation retry history=%+v, want retained creation plus one state event", events)
	}
	before, beforeOK := events[1].Details["before"].(map[string]any)
	after, afterOK := events[1].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != true || after["active"] != false {
		t.Fatalf("deactivation retry details=%#v, want active true -> false", events[1].Details)
	}
	for id, snapshot := range beforeBookings {
		if after := resourceStateBookingAndHistorySnapshot(t, admin, id); after != snapshot {
			t.Fatalf("successful deactivation changed retained Booking/history %s: before=%s after=%s", id, snapshot, after)
		}
	}
}

func insertResourceStateBookingHistory(t *testing.T, admin *pgxpool.Pool, bookingID, actorID, state string, occurredAt time.Time) {
	t.Helper()
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'booking.created', $2, $3::uuid, '{"fixture":true}'::jsonb)`,
		actorID, occurredAt, bookingID); err != nil {
		t.Fatalf("insert retained Booking creation history: %v", err)
	}
	if state == "cancelled" {
		if _, err := admin.Exec(t.Context(), `
			INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
			VALUES (gen_random_uuid(), $1::uuid, 'booking.cancelled', $2, $3::uuid, '{"fixture":true}'::jsonb)`,
			actorID, occurredAt.Add(time.Microsecond), bookingID); err != nil {
			t.Fatalf("insert retained Booking cancellation history: %v", err)
		}
	}
}

func resourceStateBookingAndHistorySnapshot(t *testing.T, admin *pgxpool.Pool, bookingID string) string {
	t.Helper()
	var snapshot string
	if err := admin.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'booking', to_jsonb(booking),
			'activity', COALESCE((
				SELECT jsonb_agg(to_jsonb(event) ORDER BY event.id)
				FROM public.activity_events AS event WHERE event.booking_id = booking.id
			), '[]'::jsonb)
		)::text
		FROM public.bookings AS booking
		WHERE booking.id = $1::uuid`, bookingID).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot Resource state Booking and Activity %s: %v", bookingID, err)
	}
	return snapshot
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
