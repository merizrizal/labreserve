package database

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

const (
	resourceStateRestartFixtureEnv = "LABRESERVE_TEST_RESOURCE_STATE_RESTART_FIXTURE"
	resourceStateRestartReplayEnv  = "LABRESERVE_TEST_RESOURCE_STATE_RESTART_REPLAY"
	resourceStateRestartActiveCode = "TASK-4B-STATE-RESTART-ACTIVE"
	resourceStateRestartPastID     = "44444444-4444-4444-8444-444444444442"
	resourceStateRestartCancelID   = "44444444-4444-4444-8444-444444444443"
)

func TestResourceStateRestartFixture(t *testing.T) {
	if os.Getenv(resourceStateRestartFixtureEnv) != "1" {
		t.Skip("run by scripts/verify.sh before the PostgreSQL/application restart")
	}

	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("check current schema before Resource state restart fixture: %v", err)
	}
	app := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	assertResourceStateRestartRuntimeIdentity(t, app)
	store := NewStore(app)
	base := resourceStateRestartBase()
	coordinatorID := resourceStateRestartAccountID(t, admin, "jordan@example.test", "coordinator")
	ownerID := resourceStateRestartAccountID(t, admin, "alex@example.test", "engineer")

	if _, found, err := resourceByCode(t, store, resourceStateRestartActiveCode); err != nil || found {
		t.Fatalf("state restart activation Resource already exists (found=%v error=%v); require a fresh disposable verification database", found, err)
	}
	seeded, found, err := resourceByCode(t, store, "DEMO-01")
	if err != nil || !found || !seeded.Active {
		t.Fatalf("load active seeded Resource for deactivation fixture: found=%v resource=%+v error=%v", found, seeded, err)
	}
	var seededBookings, seededResourceEvents int
	if err := admin.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM public.bookings WHERE resource_id = $1::uuid),
		       (SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid)`, seeded.ID).
		Scan(&seededBookings, &seededResourceEvents); err != nil {
		t.Fatalf("inspect seeded Resource before isolated state lifecycle fixture: %v", err)
	}
	if seededResourceEvents != 0 {
		t.Fatalf("seeded DEMO-01 has unexpected Resource-target Activity before lifecycle fixture: bookings=%d ResourceEvents=%d",
			seededBookings, seededResourceEvents)
	}
	seededBookingHistoryBefore, err := resourceStateRestartBookingHistorySnapshots(t, admin, seeded.ID)
	if err != nil {
		t.Fatalf("snapshot existing seeded Bookings before state fixture: %v", err)
	}

	baseIdentity, err := resourceStateRestartSeedIdentity(t.Context(), admin, seeded.ID)
	if err != nil {
		t.Fatalf("snapshot seeded Resource identity before state transition: %v", err)
	}

	now := base
	resourceService := resources.NewService(store, func() time.Time { return now })
	activation, err := resourceService.Create(t.Context(), resources.Actor{AccountID: coordinatorID}, resources.CreateRequest{
		Code: resourceStateRestartActiveCode,
		Name: "State Restart Activation Lab", Description: "Created inactive, then activated", Active: false,
	})
	if err != nil || activation.Resource.Active {
		t.Fatalf("create Inactive Resource for restart activation: result=%+v error=%v", activation, err)
	}
	now = base.Add(time.Minute)
	activated, err := resources.NewStateService(store, func() time.Time { return now }).SetState(
		t.Context(), resources.Actor{AccountID: coordinatorID}, resources.StateRequest{
			ResourceID: activation.Resource.ID, Active: true,
		})
	if err != nil || !activated.Changed || !activated.Resource.Active ||
		!activated.Resource.UpdatedAt.Equal(now) || !activated.Resource.CreatedAt.Equal(base) {
		t.Fatalf("commit activation restart fixture: result=%+v error=%v", activated, err)
	}
	assertResourceStateRestartActivation(t, admin, activation.Resource.ID, coordinatorID, base, now)

	pastRequest := bookings.CreateRequest{
		ResourceID: seeded.ID, RequestID: resourceStateRestartPastID,
		StartAt: base.Add(time.Hour), EndAt: base.Add(90 * time.Minute), Purpose: "retained past Confirmed restart request",
	}
	cancelledRequest := bookings.CreateRequest{
		ResourceID: seeded.ID, RequestID: resourceStateRestartCancelID,
		StartAt: base.Add(2 * time.Hour), EndAt: base.Add(3 * time.Hour), Purpose: "retained Cancelled restart request",
	}
	bookingService := bookings.NewService(store, func() time.Time { return now })
	past, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: ownerID}, pastRequest)
	if err != nil || past.Replayed {
		t.Fatalf("create Confirmed Booking for restart fixture: result=%+v error=%v", past, err)
	}
	cancelled, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: ownerID}, cancelledRequest)
	if err != nil || cancelled.Replayed {
		t.Fatalf("create Booking for cancellation restart fixture: result=%+v error=%v", cancelled, err)
	}
	now = base.Add(15 * time.Minute)
	cancellation, err := bookings.NewCancellationService(store, func() time.Time { return now }).Cancel(
		t.Context(), bookings.Actor{AccountID: ownerID}, cancelled.Booking.ID)
	if err != nil || !cancellation.Changed || cancellation.Booking.CancelledAt == nil ||
		!cancellation.Booking.CancelledAt.Equal(now) {
		t.Fatalf("cancel restart fixture Booking: result=%+v error=%v", cancellation, err)
	}

	pastBefore := resourceStateBookingAndHistorySnapshot(t, admin, past.Booking.ID)
	cancelledBefore := resourceStateBookingAndHistorySnapshot(t, admin, cancelled.Booking.ID)
	seededBookingHistoryBefore[past.Booking.ID] = pastBefore
	seededBookingHistoryBefore[cancelled.Booking.ID] = cancelledBefore
	now = resourceStateRestartDeactivationAt()
	var blockingBookings int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FROM public.bookings
		WHERE resource_id = $1::uuid AND state = 'confirmed' AND end_at > $2`, seeded.ID, now).Scan(&blockingBookings); err != nil {
		t.Fatalf("verify seeded Resource is clear at deactivation operation time: %v", err)
	}
	if blockingBookings != 0 {
		t.Fatalf("seeded DEMO-01 has %d future-ending Confirmed Booking(s) at fixture operation time", blockingBookings)
	}
	deactivated, err := resources.NewStateService(store, func() time.Time { return now }).SetState(
		t.Context(), resources.Actor{AccountID: coordinatorID}, resources.StateRequest{
			ResourceID: seeded.ID, Active: false,
		})
	if err != nil || !deactivated.Changed || deactivated.Resource.Active || !deactivated.Resource.UpdatedAt.Equal(now) {
		t.Fatalf("commit deactivation of seeded Resource: result=%+v error=%v", deactivated, err)
	}
	assertResourceStateRestartDeactivation(t, admin, seeded.ID, coordinatorID, now)
	if got, err := resourceStateRestartSeedIdentity(t.Context(), admin, seeded.ID); err != nil || got != baseIdentity {
		t.Fatalf("deactivation changed seeded Resource identity: before=%s after=%s error=%v", baseIdentity, got, err)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, past.Booking.ID); got != pastBefore {
		t.Fatalf("deactivation changed past Confirmed Booking/request/history: before=%s after=%s", pastBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelled.Booking.ID); got != cancelledBefore {
		t.Fatalf("deactivation changed Cancelled Booking/request/cancellation/history: before=%s after=%s", cancelledBefore, got)
	}

	token := resourceStateRestartSessionToken()
	tokenBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(tokenBytes) != 32 {
		t.Fatalf("construct deterministic disposable session token: decoded bytes=%d error=%v", len(tokenBytes), err)
	}
	tokenHash := sha256.Sum256(tokenBytes)
	var existingSessions int
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM public.sessions WHERE token_hash = $1", tokenHash[:]).Scan(&existingSessions); err != nil {
		t.Fatalf("check state restart session fixture absence: %v", err)
	}
	if existingSessions != 0 {
		t.Fatal("state restart session fixture already exists; require a fresh disposable verification database")
	}
	csrfToken := make([]byte, 32)
	for index := range csrfToken {
		csrfToken[index] = byte(index + 1)
	}
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.sessions (token_hash, account_id, csrf_token, created_at, expires_at)
		VALUES ($1, $2::uuid, $3, $4, $5)`, tokenHash[:], coordinatorID, csrfToken, base, base.Add(12*time.Hour)); err != nil {
		t.Fatalf("insert scoped state restart session fixture: %v", err)
	}
	var sessionCountBeforeSeed, sessionCountAfterSeed int
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM public.sessions").Scan(&sessionCountBeforeSeed); err != nil {
		t.Fatalf("count sessions before repeat seed: %v", err)
	}
	passwordHashes := map[string]string{
		"alex@example.test":   "unused-alex-hash",
		"sam@example.test":    "unused-sam-hash",
		"jordan@example.test": "unused-jordan-hash",
	}
	if err := Seed(t.Context(), admin, passwordHashes); err != nil {
		t.Fatalf("repeat seed after state transitions and retained history: %v", err)
	}
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM public.sessions").Scan(&sessionCountAfterSeed); err != nil {
		t.Fatalf("count sessions after repeat seed: %v", err)
	}
	if sessionCountAfterSeed != sessionCountBeforeSeed {
		t.Fatalf("repeat seed changed session count from %d to %d", sessionCountBeforeSeed, sessionCountAfterSeed)
	}
	assertResourceStateRestartRuntimeIdentity(t, app)
	assertResourceStateRestartDeactivation(t, admin, seeded.ID, coordinatorID, resourceStateRestartDeactivationAt())
	assertResourceStateRestartBookingHistorySnapshots(t, admin, seeded.ID, seededBookingHistoryBefore)
	assertResourceStateRestartActivation(t, admin, activation.Resource.ID, coordinatorID, base, base.Add(time.Minute))
}

func TestResourceStateRestartReplay(t *testing.T) {
	if os.Getenv(resourceStateRestartReplayEnv) != "1" {
		t.Skip("run by scripts/verify.sh after the PostgreSQL/application restart")
	}

	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	app := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	assertResourceStateRestartRuntimeIdentity(t, app)
	if err := CheckSchema(t.Context(), app); err != nil {
		t.Fatalf("fresh runtime connection after restart has incompatible schema: %v", err)
	}
	store := NewStore(app)
	base := resourceStateRestartBase()
	coordinatorID := resourceStateRestartAccountID(t, admin, "jordan@example.test", "coordinator")
	ownerID := resourceStateRestartAccountID(t, admin, "alex@example.test", "engineer")

	seeded, found, err := resourceByCode(t, store, "DEMO-01")
	if err != nil || !found || seeded.Active {
		t.Fatalf("repeated seed did not preserve deactivated DEMO-01 after restart: found=%v resource=%+v error=%v", found, seeded, err)
	}
	seededID := seeded.ID
	if got, err := resourceStateRestartSeedIdentity(t.Context(), admin, seededID); err != nil ||
		!strings.Contains(got, `"code": "DEMO-01"`) || !strings.Contains(got, `"name": "Customer Demo Environment"`) {
		t.Fatalf("seeded Resource identity after restart = %s, error=%v", got, err)
	}
	assertResourceStateRestartDeactivation(t, admin, seededID, coordinatorID, resourceStateRestartDeactivationAt())

	activated, found, err := resourceByCode(t, store, resourceStateRestartActiveCode)
	if err != nil || !found || !activated.Active || activated.Code != resourceStateRestartActiveCode ||
		activated.Name != "State Restart Activation Lab" || activated.Description != "Created inactive, then activated" {
		t.Fatalf("activated Resource did not survive restart: found=%v resource=%+v error=%v", found, activated, err)
	}
	assertResourceStateRestartActivation(t, admin, activated.ID, coordinatorID, base, base.Add(time.Minute))

	pastRequest := resourceStateRestartPastRequest(seededID, base)
	cancelledRequest := resourceStateRestartCancelledRequest(seededID, base)
	pastBookingID := resourceStateRestartBookingID(t, admin, ownerID, pastRequest.RequestID)
	cancelledBookingID := resourceStateRestartBookingID(t, admin, ownerID, cancelledRequest.RequestID)
	pastBefore := resourceStateBookingAndHistorySnapshot(t, admin, pastBookingID)
	cancelledBefore := resourceStateBookingAndHistorySnapshot(t, admin, cancelledBookingID)
	assertResourceStateRestartBooking(t, admin, pastBookingID, ownerID, pastRequest, "confirmed", nil)
	cancelledAt := base.Add(15 * time.Minute)
	assertResourceStateRestartBooking(t, admin, cancelledBookingID, ownerID, cancelledRequest, "cancelled", &cancelledAt)

	resourceBefore, eventsBefore := resourceCapabilitySnapshot(t, admin, seededID)
	seededBookingHistoryBeforeReplay, err := resourceStateRestartBookingHistorySnapshots(t, admin, seededID)
	if err != nil {
		t.Fatalf("snapshot seeded Booking history before post-restart replay: %v", err)
	}
	replayNow := resourceStateRestartDeactivationAt().Add(time.Hour)
	bookingService := bookings.NewService(store, func() time.Time { return replayNow })
	for _, test := range []struct {
		name    string
		request bookings.CreateRequest
		id      string
		state   bookings.State
	}{
		{name: "past Confirmed", request: pastRequest, id: pastBookingID, state: bookings.StateConfirmed},
		{name: "Cancelled", request: cancelledRequest, id: cancelledBookingID, state: bookings.StateCancelled},
	} {
		t.Run(test.name+" retained replay", func(t *testing.T) {
			result, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: ownerID}, test.request)
			if err != nil || !result.Replayed || result.Booking.ID != test.id || result.Booking.State != test.state {
				t.Fatalf("post-restart replay on Inactive Resource: result=%+v error=%v", result, err)
			}
			assertBookingRequestCounts(t, admin, ownerID, test.request.RequestID, 1, 1)
		})
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, pastBookingID); got != pastBefore {
		t.Fatalf("post-restart past replay changed Booking/history: before=%s after=%s", pastBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledBookingID); got != cancelledBefore {
		t.Fatalf("post-restart Cancelled replay changed Booking/history: before=%s after=%s", cancelledBefore, got)
	}
	if got, events := resourceCapabilitySnapshot(t, admin, seededID); got != resourceBefore || events != eventsBefore {
		t.Fatalf("post-restart replays changed Resource/Activity: before=%s %s after=%s %s", resourceBefore, eventsBefore, got, events)
	}
	assertResourceStateRestartBookingHistorySnapshots(t, admin, seededID, seededBookingHistoryBeforeReplay)

	activatedSeed, err := resources.NewStateService(store, func() time.Time { return replayNow }).SetState(
		t.Context(), resources.Actor{AccountID: coordinatorID}, resources.StateRequest{
			ResourceID: seededID, Active: true,
		})
	if err != nil || !activatedSeed.Changed || !activatedSeed.Resource.Active ||
		!activatedSeed.Resource.UpdatedAt.Equal(replayNow) {
		t.Fatalf("post-restart opposite-state activation: result=%+v error=%v", activatedSeed, err)
	}
	assertResourceStateRestartOppositeTransition(t, admin, seededID, coordinatorID,
		resourceStateRestartDeactivationAt(), replayNow)
	if got := resourceStateBookingAndHistorySnapshot(t, admin, pastBookingID); got != pastBefore {
		t.Fatalf("post-restart opposite state transition changed past Booking/history: before=%s after=%s", pastBefore, got)
	}
	if got := resourceStateBookingAndHistorySnapshot(t, admin, cancelledBookingID); got != cancelledBefore {
		t.Fatalf("post-restart opposite state transition changed cancelled Booking/history: before=%s after=%s", cancelledBefore, got)
	}
	assertResourceStateRestartBookingHistorySnapshots(t, admin, seededID, seededBookingHistoryBeforeReplay)

	activeBeforeResource, activeBeforeEvents := resourceCapabilitySnapshot(t, admin, seededID)
	installResourceUpdateFailureTrigger(t, admin, seededID)
	removeNoOpActivityFailure := installResourceActivityFailureTrigger(t, admin, coordinatorID, "resource.activated")
	noOpClockCalls := atomic.Int32{}
	noOp, err := resources.NewStateService(store, func() time.Time {
		noOpClockCalls.Add(1)
		return replayNow.Add(time.Hour)
	}).SetState(t.Context(), resources.Actor{AccountID: coordinatorID}, resources.StateRequest{
		ResourceID: seededID, Active: true,
	})
	removeNoOpActivityFailure()
	if err != nil || noOp.Changed || !noOp.Resource.Active || !noOp.Resource.UpdatedAt.Equal(replayNow) || noOpClockCalls.Load() != 0 {
		t.Fatalf("post-restart matching-state no-op: result=%+v clock calls=%d error=%v",
			noOp, noOpClockCalls.Load(), err)
	}
	activeAfterNoOpResource, activeAfterNoOpEvents := resourceCapabilitySnapshot(t, admin, seededID)
	if activeBeforeResource != activeAfterNoOpResource || activeBeforeEvents != activeAfterNoOpEvents {
		t.Fatalf("post-restart matching-state no-op changed Resource/Activity: before=%s %s after=%s %s",
			activeBeforeResource, activeBeforeEvents, activeAfterNoOpResource, activeAfterNoOpEvents)
	}
}

func resourceStateRestartBase() time.Time {
	return time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
}

func resourceStateRestartDeactivationAt() time.Time {
	return resourceStateRestartBase().Add(60 * 24 * time.Hour)
}

func resourceStateRestartSessionToken() string {
	return strings.Repeat("A", base64.RawURLEncoding.EncodedLen(32))
}

func resourceStateRestartPastRequest(resourceID string, base time.Time) bookings.CreateRequest {
	return bookings.CreateRequest{
		ResourceID: resourceID, RequestID: resourceStateRestartPastID,
		StartAt: base.Add(time.Hour), EndAt: base.Add(90 * time.Minute), Purpose: "retained past Confirmed restart request",
	}
}

func resourceStateRestartCancelledRequest(resourceID string, base time.Time) bookings.CreateRequest {
	return bookings.CreateRequest{
		ResourceID: resourceID, RequestID: resourceStateRestartCancelID,
		StartAt: base.Add(2 * time.Hour), EndAt: base.Add(3 * time.Hour), Purpose: "retained Cancelled restart request",
	}
}

func resourceStateRestartAccountID(t *testing.T, admin *pgxpool.Pool, login, role string) string {
	t.Helper()
	var id string
	if err := admin.QueryRow(t.Context(), `SELECT id::text FROM public.accounts WHERE login = $1 AND role = $2`, login, role).Scan(&id); err != nil {
		t.Fatalf("resolve seeded %s account identity for restart: %v", role, err)
	}
	return id
}

func assertResourceStateRestartRuntimeIdentity(t *testing.T, app *pgxpool.Pool) {
	t.Helper()
	var role, databaseName string
	if err := app.QueryRow(t.Context(), "SELECT current_user, current_database()").Scan(&role, &databaseName); err != nil {
		t.Fatalf("inspect runtime PostgreSQL identity: %v", err)
	}
	if role != "labreserve_app" || databaseName != "labreserve_test" {
		t.Fatalf("Resource state lifecycle runtime identity=%s database=%s, want labreserve_app/labreserve_test", role, databaseName)
	}
}

func resourceStateRestartSeedIdentity(ctx context.Context, admin *pgxpool.Pool, resourceID string) (string, error) {
	var identity string
	err := admin.QueryRow(ctx, `
		SELECT jsonb_build_object('id', id, 'code', code, 'name', name, 'description', description, 'created_at', created_at)::text
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(&identity)
	return identity, err
}

func assertResourceStateRestartActivation(t *testing.T, admin *pgxpool.Pool, resourceID, actorID string, createdAt, activatedAt time.Time) {
	t.Helper()
	var code, name, description string
	var active bool
	var storedCreatedAt, updatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(
		&code, &name, &description, &active, &storedCreatedAt, &updatedAt); err != nil {
		t.Fatalf("inspect activated restart Resource: %v", err)
	}
	if code != resourceStateRestartActiveCode || name != "State Restart Activation Lab" ||
		description != "Created inactive, then activated" || !active ||
		!storedCreatedAt.Equal(createdAt) || !updatedAt.Equal(activatedAt) {
		t.Fatalf("activated restart Resource fields=%q/%q/%q active=%v created=%s updated=%s",
			code, name, description, active, storedCreatedAt, updatedAt)
	}
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 2 || events[0].ActorID != actorID || events[0].Action != "resource.created" ||
		!events[0].OccurredAt.Equal(createdAt) || events[1].ActorID != actorID ||
		events[1].Action != "resource.activated" || !events[1].OccurredAt.Equal(activatedAt) ||
		!events[0].BookingIsNil || !events[1].BookingIsNil {
		t.Fatalf("activated restart Resource Activity=%+v", events)
	}
	before, beforeOK := events[1].Details["before"].(map[string]any)
	after, afterOK := events[1].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != false || after["active"] != true {
		t.Fatalf("restart activation event details=%#v, want false -> true", events[1].Details)
	}
}

func assertResourceStateRestartDeactivation(t *testing.T, admin *pgxpool.Pool, resourceID, actorID string, deactivatedAt time.Time) {
	t.Helper()
	var code, name, description string
	var active bool
	var updatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT code, name, description, active, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(
		&code, &name, &description, &active, &updatedAt); err != nil {
		t.Fatalf("inspect deactivated seeded restart Resource: %v", err)
	}
	if code != "DEMO-01" || name != "Customer Demo Environment" ||
		description != "Shared environment for demonstrations." || active || !updatedAt.Equal(deactivatedAt) {
		t.Fatalf("deactivated seeded Resource fields=%q/%q/%q active=%v updated=%s", code, name, description, active, updatedAt)
	}
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 1 || events[0].ActorID != actorID || events[0].Action != "resource.deactivated" ||
		!events[0].OccurredAt.Equal(deactivatedAt) || !events[0].BookingIsNil {
		t.Fatalf("deactivated seeded Resource Activity=%+v", events)
	}
	before, beforeOK := events[0].Details["before"].(map[string]any)
	after, afterOK := events[0].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != true || after["active"] != false {
		t.Fatalf("restart deactivation event details=%#v, want true -> false", events[0].Details)
	}
}

func assertResourceStateRestartOppositeTransition(t *testing.T, admin *pgxpool.Pool, resourceID, actorID string, deactivatedAt, activatedAt time.Time) {
	t.Helper()
	var active bool
	var updatedAt time.Time
	if err := admin.QueryRow(t.Context(), `SELECT active, updated_at FROM public.resources WHERE id = $1::uuid`, resourceID).
		Scan(&active, &updatedAt); err != nil {
		t.Fatalf("inspect post-restart opposite Resource state: %v", err)
	}
	if !active || !updatedAt.Equal(activatedAt) {
		t.Fatalf("post-restart opposite Resource state active=%v updated_at=%s, want Active at %s", active, updatedAt, activatedAt)
	}
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 2 || events[0].ActorID != actorID || events[0].Action != "resource.deactivated" ||
		!events[0].OccurredAt.Equal(deactivatedAt) || !events[0].BookingIsNil || events[1].ActorID != actorID ||
		events[1].Action != "resource.activated" || !events[1].OccurredAt.Equal(activatedAt) || !events[1].BookingIsNil {
		t.Fatalf("post-restart opposite state Activity=%+v", events)
	}
	beforeDeactivation, beforeDeactivationOK := events[0].Details["before"].(map[string]any)
	afterDeactivation, afterDeactivationOK := events[0].Details["after"].(map[string]any)
	beforeActivation, beforeActivationOK := events[1].Details["before"].(map[string]any)
	afterActivation, afterActivationOK := events[1].Details["after"].(map[string]any)
	if !beforeDeactivationOK || !afterDeactivationOK || beforeDeactivation["active"] != true || afterDeactivation["active"] != false ||
		!beforeActivationOK || !afterActivationOK || beforeActivation["active"] != false || afterActivation["active"] != true {
		t.Fatalf("post-restart opposite state event details=%#v/%#v, want true -> false -> true", events[0].Details, events[1].Details)
	}
}

func resourceStateRestartBookingHistorySnapshots(t *testing.T, admin *pgxpool.Pool, resourceID string) (map[string]string, error) {
	t.Helper()
	rows, err := admin.Query(t.Context(), `SELECT id::text FROM public.bookings WHERE resource_id = $1::uuid ORDER BY id`, resourceID)
	if err != nil {
		return nil, err
	}
	var bookingIDs []string
	for rows.Next() {
		var bookingID string
		if err := rows.Scan(&bookingID); err != nil {
			rows.Close()
			return nil, err
		}
		bookingIDs = append(bookingIDs, bookingID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	snapshots := make(map[string]string, len(bookingIDs))
	for _, bookingID := range bookingIDs {
		snapshots[bookingID] = resourceStateBookingAndHistorySnapshot(t, admin, bookingID)
	}
	return snapshots, nil
}

func assertResourceStateRestartBookingHistorySnapshots(t *testing.T, admin *pgxpool.Pool, resourceID string, expected map[string]string) {
	t.Helper()
	actual, err := resourceStateRestartBookingHistorySnapshots(t, admin, resourceID)
	if err != nil {
		t.Fatalf("snapshot all retained Resource Bookings and Activity: %v", err)
	}
	if len(actual) != len(expected) {
		t.Fatalf("Resource Booking count changed: before=%d after=%d", len(expected), len(actual))
	}
	for bookingID, before := range expected {
		if after, found := actual[bookingID]; !found || after != before {
			t.Fatalf("Resource Booking/request/cancellation/history %s changed: before=%s after=%s found=%v", bookingID, before, after, found)
		}
	}
}

func resourceStateRestartBookingID(t *testing.T, admin *pgxpool.Pool, ownerID, requestID string) string {
	t.Helper()
	var bookingID string
	if err := admin.QueryRow(t.Context(), `
		SELECT id::text FROM public.bookings WHERE owner_account_id = $1::uuid AND request_id = $2::uuid`,
		ownerID, requestID).Scan(&bookingID); err != nil {
		t.Fatalf("resolve retained restart Booking for request %s: %v", requestID, err)
	}
	return bookingID
}

func assertResourceStateRestartBooking(
	t *testing.T,
	admin *pgxpool.Pool,
	bookingID, ownerID string,
	request bookings.CreateRequest,
	state string,
	cancelledAt *time.Time,
) {
	t.Helper()
	var resourceID, storedOwner, requestID, purpose, storedState string
	var startAt, endAt, createdAt time.Time
	var cancelledBy *string
	var storedCancelledAt *time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT resource_id::text, owner_account_id::text, request_id::text, purpose, state,
		       start_at, end_at, created_at, cancelled_by_account_id::text, cancelled_at
		FROM public.bookings WHERE id = $1::uuid`, bookingID).Scan(
		&resourceID, &storedOwner, &requestID, &purpose, &storedState,
		&startAt, &endAt, &createdAt, &cancelledBy, &storedCancelledAt); err != nil {
		t.Fatalf("inspect retained restart Booking: %v", err)
	}
	if resourceID != request.ResourceID || storedOwner != ownerID || requestID != request.RequestID ||
		purpose != request.Purpose || storedState != state || !startAt.Equal(request.StartAt) ||
		!endAt.Equal(request.EndAt) || !createdAt.Equal(resourceStateRestartBase().Add(time.Minute)) {
		t.Fatalf("restart Booking identity/data changed: resource=%s owner=%s request=%s purpose=%q state=%s interval=%s..%s created=%s",
			resourceID, storedOwner, requestID, purpose, storedState, startAt, endAt, createdAt)
	}
	if cancelledAt == nil {
		if cancelledBy != nil || storedCancelledAt != nil {
			t.Fatalf("Confirmed restart Booking unexpectedly has cancellation metadata: by=%v at=%v", cancelledBy, storedCancelledAt)
		}
	} else if cancelledBy == nil || *cancelledBy != ownerID || storedCancelledAt == nil || !storedCancelledAt.Equal(*cancelledAt) {
		t.Fatalf("Cancelled restart Booking metadata by=%v at=%v, want owner=%s at=%s", cancelledBy, storedCancelledAt, ownerID, *cancelledAt)
	}
	var createdEvents, cancelledEvents int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'),
		       count(*) FILTER (WHERE action = 'booking.cancelled')
		FROM public.activity_events WHERE booking_id = $1::uuid`, bookingID).Scan(&createdEvents, &cancelledEvents); err != nil {
		t.Fatalf("inspect restart Booking Activity counts: %v", err)
	}
	wantCancelledEvents := 0
	if cancelledAt != nil {
		wantCancelledEvents = 1
	}
	if createdEvents != 1 || cancelledEvents != wantCancelledEvents {
		t.Fatalf("restart Booking event counts=%d/%d, want 1/%d", createdEvents, cancelledEvents, wantCancelledEvents)
	}
}
