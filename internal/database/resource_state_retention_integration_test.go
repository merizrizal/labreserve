package database

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceStateRetentionAcrossTransitionsAndInactiveReplay(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	base := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	createdAt := base.Add(-24 * time.Hour)
	activationCode := "State-Retention-Activate-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	activationName := "  Legacy \u2003 Resource Name  "
	activationDescription := "<b>retained description</b> \u2003"
	activationID := insertResourceCapabilityFixture(t, admin, activationCode,
		activationName, activationDescription, false, createdAt)
	fixtures.trackResource(activationID)
	insertResourceStateRetentionCreationEvent(t, admin, fixtures.coordinator, activationID,
		activationCode, activationName, activationDescription, false, createdAt)

	confirmedFixtureID := insertResourceStateBooking(t, admin, activationID, fixtures.engineer,
		base.Add(time.Hour), base.Add(2*time.Hour), createdAt, "confirmed", "")
	cancelledFixtureID := insertResourceStateBooking(t, admin, activationID, fixtures.secondEngineer,
		base.Add(3*time.Hour), base.Add(4*time.Hour), createdAt, "cancelled", fixtures.secondEngineer)
	*bookingIDs = append(*bookingIDs, confirmedFixtureID, cancelledFixtureID)
	insertResourceStateBookingHistory(t, admin, confirmedFixtureID, fixtures.engineer, "confirmed", createdAt)
	insertResourceStateBookingHistory(t, admin, cancelledFixtureID, fixtures.secondEngineer, "cancelled", createdAt)
	confirmedFixtureBefore := resourceStateBookingAndHistorySnapshot(t, admin, confirmedFixtureID)
	cancelledFixtureBefore := resourceStateBookingAndHistorySnapshot(t, admin, cancelledFixtureID)
	activationIdentityBefore := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, activationID)

	clockCalls := atomic.Int32{}
	activationService := resources.NewStateService(NewStore(app), func() time.Time {
		clockCalls.Add(1)
		return base
	})
	activated, err := activationService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: activationID, Active: true,
	})
	if err != nil || !activated.Changed || !activated.Resource.Active ||
		activated.Resource.ID != activationID || activated.Resource.Code != activationCode ||
		activated.Resource.Name != activationName || activated.Resource.Description != activationDescription ||
		!activated.Resource.CreatedAt.Equal(createdAt) || !activated.Resource.UpdatedAt.Equal(base) || clockCalls.Load() != 1 {
		t.Fatalf("activate Resource retaining future Booking history: result=%+v clock calls=%d error=%v",
			activated, clockCalls.Load(), err)
	}
	if got := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, activationID); got != activationIdentityBefore {
		t.Fatalf("activation changed immutable Resource identity/text/creation/prior Activity: before=%s after=%s",
			activationIdentityBefore, got)
	}
	assertResourceStateActivationHistory(t, admin, activationID, fixtures.coordinator, createdAt, base)
	if got := resourceStateBookingAndHistorySnapshot(t, admin, confirmedFixtureID); got != confirmedFixtureBefore {
		t.Fatalf("activation changed retained Confirmed Booking/request/history: before=%s after=%s", confirmedFixtureBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledFixtureID); got != cancelledFixtureBefore {
		t.Fatalf("activation changed retained Cancelled Booking/request/history: before=%s after=%s", cancelledFixtureBefore, got)
	}

	noOpBeforeResource, noOpBeforeEvents := resourceCapabilitySnapshot(t, admin, activationID)
	installResourceUpdateFailureTrigger(t, admin, activationID)
	removeNoOpActivityFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.activated")
	noOpClockCalls := atomic.Int32{}
	noOpService := resources.NewStateService(NewStore(app), func() time.Time {
		noOpClockCalls.Add(1)
		return base.Add(time.Hour)
	})
	activeNoOp, err := noOpService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: activationID, Active: true,
	})
	removeNoOpActivityFailure()
	if err != nil || activeNoOp.Changed || !activeNoOp.Resource.Active ||
		!activeNoOp.Resource.UpdatedAt.Equal(base) || noOpClockCalls.Load() != 0 {
		t.Fatalf("post-activation matching-state no-op: result=%+v clock calls=%d error=%v",
			activeNoOp, noOpClockCalls.Load(), err)
	}
	noOpAfterResource, noOpAfterEvents := resourceCapabilitySnapshot(t, admin, activationID)
	if noOpBeforeResource != noOpAfterResource || noOpBeforeEvents != noOpAfterEvents {
		t.Fatalf("matching-state no-op changed Resource/Activity: before=%s %s after=%s %s",
			noOpBeforeResource, noOpBeforeEvents, noOpAfterResource, noOpAfterEvents)
	}

	blockedBeforeResource, blockedBeforeEvents := resourceCapabilitySnapshot(t, admin, activationID)
	blockedBeforeIdentity := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, activationID)
	blockedBeforeConfirmed := resourceStateBookingAndHistorySnapshot(t, admin, confirmedFixtureID)
	blockedBeforeCancelled := resourceStateBookingAndHistorySnapshot(t, admin, cancelledFixtureID)
	blocked, err := activationService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: activationID, Active: false,
	})
	if !errors.Is(err, resources.ErrResourceOccupied) || blocked != (resources.StateResult{}) {
		t.Fatalf("deactivation with retained future Confirmed Booking: result=%+v error=%v, want occupancy rejection", blocked, err)
	}
	blockedAfterResource, blockedAfterEvents := resourceCapabilitySnapshot(t, admin, activationID)
	if blockedBeforeResource != blockedAfterResource || blockedBeforeEvents != blockedAfterEvents {
		t.Fatalf("blocked deactivation changed Resource/Activity: before=%s %s after=%s %s",
			blockedBeforeResource, blockedBeforeEvents, blockedAfterResource, blockedAfterEvents)
	}
	if got := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, activationID); got != blockedBeforeIdentity {
		t.Fatalf("blocked deactivation changed retained identity/history: before=%s after=%s", blockedBeforeIdentity, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, confirmedFixtureID); got != blockedBeforeConfirmed {
		t.Fatalf("blocked deactivation changed Confirmed Booking/history: before=%s after=%s", blockedBeforeConfirmed, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledFixtureID); got != blockedBeforeCancelled {
		t.Fatalf("blocked deactivation changed Cancelled Booking/history: before=%s after=%s", blockedBeforeCancelled, got)
	}

	replayCode := "State-Retention-Replay-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	replayName := "  Replay Resource <script>legacy</script>  "
	replayDescription := "raw description\u2003 with edge whitespace "
	replayResourceID := insertResourceCapabilityFixture(t, admin, replayCode,
		replayName, replayDescription, true, createdAt)
	fixtures.trackResource(replayResourceID)
	insertResourceStateRetentionCreationEvent(t, admin, fixtures.coordinator, replayResourceID,
		replayCode, replayName, replayDescription, true, createdAt)

	var now = base
	bookingService := bookings.NewService(NewStore(app), func() time.Time { return now })
	pastRequest := bookings.CreateRequest{
		ResourceID: replayResourceID, RequestID: testDatabaseUUID(t, admin),
		StartAt: base.Add(time.Hour), EndAt: base.Add(2 * time.Hour), Purpose: "retained past confirmed request",
	}
	pastCreated, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, pastRequest)
	if err != nil || pastCreated.Replayed {
		t.Fatalf("create retained Booking to become Past: result=%+v error=%v", pastCreated, err)
	}
	*bookingIDs = append(*bookingIDs, pastCreated.Booking.ID)
	cancelledRequest := bookings.CreateRequest{
		ResourceID: replayResourceID, RequestID: testDatabaseUUID(t, admin),
		StartAt: base.Add(3 * time.Hour), EndAt: base.Add(4 * time.Hour), Purpose: "retained cancelled request",
	}
	cancelledCreated, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, cancelledRequest)
	if err != nil || cancelledCreated.Replayed {
		t.Fatalf("create retained Booking to cancel: result=%+v error=%v", cancelledCreated, err)
	}
	*bookingIDs = append(*bookingIDs, cancelledCreated.Booking.ID)
	now = base.Add(15 * time.Minute)
	cancelledResult, err := bookings.NewCancellationService(NewStore(app), func() time.Time { return now }).Cancel(
		t.Context(), bookings.Actor{AccountID: fixtures.engineer}, cancelledCreated.Booking.ID)
	if err != nil || !cancelledResult.Changed || cancelledResult.Booking.CancelledAt == nil ||
		!cancelledResult.Booking.CancelledAt.Equal(now) {
		t.Fatalf("cancel retained request before its start: result=%+v error=%v", cancelledResult, err)
	}

	pastBefore := resourceStateBookingAndHistorySnapshot(t, admin, pastCreated.Booking.ID)
	cancelledBefore := resourceStateBookingAndHistorySnapshot(t, admin, cancelledCreated.Booking.ID)
	replayIdentityBefore := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, replayResourceID)
	deactivationAt := base.Add(5 * time.Hour)
	now = deactivationAt
	deactivated, err := resources.NewStateService(NewStore(app), func() time.Time { return now }).SetState(
		t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
			ResourceID: replayResourceID, Active: false,
		})
	if err != nil || !deactivated.Changed || deactivated.Resource.Active || !deactivated.Resource.UpdatedAt.Equal(deactivationAt) {
		t.Fatalf("deactivate after retained Booking end/cancellation: result=%+v error=%v", deactivated, err)
	}
	if got := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, replayResourceID); got != replayIdentityBefore {
		t.Fatalf("deactivation changed immutable Resource identity/text/creation/prior Activity: before=%s after=%s",
			replayIdentityBefore, got)
	}
	assertResourceStateDeactivationRetentionEvent(t, admin, replayResourceID, fixtures.coordinator, deactivationAt)
	if got := resourceStateBookingAndHistorySnapshot(t, admin, pastCreated.Booking.ID); got != pastBefore {
		t.Fatalf("deactivation changed past Confirmed Booking/request/history: before=%s after=%s", pastBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledCreated.Booking.ID); got != cancelledBefore {
		t.Fatalf("deactivation changed Cancelled Booking/request/cancellation/history: before=%s after=%s", cancelledBefore, got)
	}

	deactivatedBeforeResource, deactivatedBeforeEvents := resourceCapabilitySnapshot(t, admin, replayResourceID)
	installResourceUpdateFailureTrigger(t, admin, replayResourceID)
	removeDeactivationNoOpFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
	noOpClockCalls = atomic.Int32{}
	noOpService = resources.NewStateService(NewStore(app), func() time.Time {
		noOpClockCalls.Add(1)
		return deactivationAt.Add(time.Hour)
	})
	inactiveNoOp, err := noOpService.SetState(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{
		ResourceID: replayResourceID, Active: false,
	})
	removeDeactivationNoOpFailure()
	if err != nil || inactiveNoOp.Changed || inactiveNoOp.Resource.Active ||
		!inactiveNoOp.Resource.UpdatedAt.Equal(deactivationAt) || noOpClockCalls.Load() != 0 {
		t.Fatalf("post-deactivation matching-state no-op: result=%+v clock calls=%d error=%v",
			inactiveNoOp, noOpClockCalls.Load(), err)
	}
	deactivatedAfterResource, deactivatedAfterEvents := resourceCapabilitySnapshot(t, admin, replayResourceID)
	if deactivatedBeforeResource != deactivatedAfterResource || deactivatedBeforeEvents != deactivatedAfterEvents {
		t.Fatalf("Inactive matching-state no-op changed Resource/Activity: before=%s %s after=%s %s",
			deactivatedBeforeResource, deactivatedBeforeEvents, deactivatedAfterResource, deactivatedAfterEvents)
	}

	now = deactivationAt.Add(3 * time.Hour)
	for _, expected := range []struct {
		name    string
		request bookings.CreateRequest
		id      string
		state   bookings.State
	}{
		{name: "Past Confirmed", request: pastRequest, id: pastCreated.Booking.ID, state: bookings.StateConfirmed},
		{name: "Cancelled", request: cancelledRequest, id: cancelledCreated.Booking.ID, state: bookings.StateCancelled},
	} {
		t.Run(expected.name+" identical inactive replay", func(t *testing.T) {
			resourceBefore, eventsBefore := resourceCapabilitySnapshot(t, admin, replayResourceID)
			bookingBefore := resourceStateBookingAndHistorySnapshot(t, admin, expected.id)
			replay, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, expected.request)
			if err != nil || !replay.Replayed || replay.Booking.ID != expected.id || replay.Booking.State != expected.state {
				t.Fatalf("identical retained request replay while Inactive: result=%+v error=%v", replay, err)
			}
			assertBookingRequestCounts(t, admin, fixtures.engineer, expected.request.RequestID, 1, 1)
			if after := resourceStateBookingAndHistorySnapshot(t, admin, expected.id); after != bookingBefore {
				t.Fatalf("replay changed retained Booking/cancellation/history: before=%s after=%s", bookingBefore, after)
			}
			resourceAfter, eventsAfter := resourceCapabilitySnapshot(t, admin, replayResourceID)
			if resourceBefore != resourceAfter || eventsBefore != eventsAfter {
				t.Fatalf("replay changed Resource/Activity: before=%s %s after=%s %s",
					resourceBefore, eventsBefore, resourceAfter, eventsAfter)
			}
		})
	}

	changedRequest := pastRequest
	changedRequest.Purpose = "changed data must win before inactive eligibility"
	if _, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, changedRequest); !errors.Is(err, bookings.ErrRequestIDReuse) || errors.Is(err, bookings.ErrInactiveResource) {
		t.Fatalf("changed-data reuse on Inactive Resource = %v, want request-ID reuse before inactive eligibility", err)
	}
	assertBookingRequestCounts(t, admin, fixtures.engineer, pastRequest.RequestID, 1, 1)
	if got := resourceStateBookingAndHistorySnapshot(t, admin, pastCreated.Booking.ID); got != pastBefore {
		t.Fatalf("changed-data reuse changed retained Booking/history: before=%s after=%s", pastBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledCreated.Booking.ID); got != cancelledBefore {
		t.Fatalf("changed-data reuse changed retained cancellation/history: before=%s after=%s", cancelledBefore, got)
	}

	// The same request UUID remains independent for a different authenticated owner.
	otherOwnerRequest := bookings.CreateRequest{
		ResourceID: activationID, RequestID: pastRequest.RequestID,
		StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), Purpose: "same UUID, separate owner scope",
	}
	otherOwnerResult, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.secondEngineer}, otherOwnerRequest)
	if err != nil || otherOwnerResult.Replayed || otherOwnerResult.Booking.ID == pastCreated.Booking.ID {
		t.Fatalf("same request UUID for a different owner: result=%+v error=%v", otherOwnerResult, err)
	}
	*bookingIDs = append(*bookingIDs, otherOwnerResult.Booking.ID)
	assertBookingRequestCounts(t, admin, fixtures.secondEngineer, pastRequest.RequestID, 1, 1)
	if got := resourceStateRetentionIdentityAndPriorHistorySnapshot(t, admin, replayResourceID); got != replayIdentityBefore {
		t.Fatalf("cross-owner request identity changed original Resource/history: before=%s after=%s", replayIdentityBefore, got)
	}
}

func insertResourceStateRetentionCreationEvent(
	t *testing.T,
	admin *pgxpool.Pool,
	actorID, resourceID, code, name, description string,
	active bool,
	createdAt time.Time,
) {
	t.Helper()
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'resource.created', $2, $3::uuid,
		        jsonb_build_object('code', $4::text, 'name', $5::text, 'description', $6::text, 'active', $7::boolean))`,
		actorID, createdAt, resourceID, code, name, description, active); err != nil {
		t.Fatalf("insert retained Resource creation Activity: %v", err)
	}
}

func resourceStateRetentionIdentityAndPriorHistorySnapshot(t *testing.T, admin *pgxpool.Pool, resourceID string) string {
	t.Helper()
	var snapshot string
	if err := admin.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'resource', jsonb_build_object(
				'id', resource.id, 'code', resource.code, 'name', resource.name,
				'description', resource.description, 'created_at', resource.created_at
			),
			'prior_events', COALESCE((
				SELECT jsonb_agg(to_jsonb(event) ORDER BY event.id)
				FROM public.activity_events AS event
				WHERE event.resource_id = resource.id
				  AND event.action NOT IN ('resource.activated', 'resource.deactivated')
			), '[]'::jsonb)
		)::text
		FROM public.resources AS resource WHERE resource.id = $1::uuid`, resourceID).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot retained Resource identity and prior Activity: %v", err)
	}
	return snapshot
}

func assertResourceStateDeactivationRetentionEvent(t *testing.T, admin *pgxpool.Pool, resourceID, actorID string, occurredAt time.Time) {
	t.Helper()
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 2 || events[0].Action != "resource.created" || events[1].ActorID != actorID ||
		events[1].Action != "resource.deactivated" || !events[1].OccurredAt.Equal(occurredAt) || !events[1].BookingIsNil {
		t.Fatalf("deactivation retention history = %+v, want original creation and one deactivation event", events)
	}
	before, beforeOK := events[1].Details["before"].(map[string]any)
	after, afterOK := events[1].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != true || after["active"] != false {
		t.Fatalf("deactivation retention details = %#v, want active true -> false", events[1].Details)
	}
}
