package database

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

const (
	resourceRestartFixtureEnv = "LABRESERVE_TEST_RESOURCE_RESTART_FIXTURE"
	resourceRestartReplayEnv  = "LABRESERVE_TEST_RESOURCE_RESTART_REPLAY"
	resourceRestartActiveCode = "TASK-4A-RESTART-ACTIVE"
	resourceRestartIdleCode   = "TASK-4A-RESTART-INACTIVE"
	resourceRestartRequestID  = "44444444-4444-4444-8444-444444444444"
)

func TestResourceMutationRestartFixture(t *testing.T) {
	if os.Getenv(resourceRestartFixtureEnv) != "1" {
		t.Skip("run by scripts/verify.sh before the PostgreSQL/application restart")
	}

	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("check current migration state before Resource restart fixture: %v", err)
	}
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	actorID := resourceRestartCoordinator(t, adminPool)
	store := NewStore(appPool)
	base := bookingCreationNow().UTC().Truncate(time.Microsecond)
	now := base
	resourceService := resources.NewService(store, func() time.Time { return now })

	for _, code := range []string{resourceRestartActiveCode, resourceRestartIdleCode} {
		if _, found, err := resourceByCode(t, store, code); err != nil || found {
			t.Fatalf("restart Resource fixture code %q already exists (found=%v error=%v); use a fresh verification volume", code, found, err)
		}
	}

	seeded, found, err := resourceByCode(t, store, "NET-01")
	if err != nil || !found {
		t.Fatalf("load seeded Resource for restart/seed preservation: found=%v error=%v", found, err)
	}
	if !seeded.Active {
		t.Fatalf("seeded NET-01 Resource unexpectedly inactive before lifecycle fixture")
	}
	if count := resourceRestartEventCount(t, adminPool, seeded.ID); count != 0 {
		t.Fatalf("seeded Resource has %d Resource-targeted events before this fixture, want bootstrap history to remain empty", count)
	}
	seededCreatedAt, _ := resourceRestartTimes(t, adminPool, seeded.ID)

	now = base.Add(time.Minute)
	active, err := resourceService.Create(t.Context(), resources.Actor{AccountID: actorID}, resources.CreateRequest{
		Code: resourceRestartActiveCode, Name: "Restart Active Original", Description: "before restart edit", Active: true,
	})
	if err != nil {
		t.Fatalf("create Active restart Resource through service: %v", err)
	}
	now = base.Add(2 * time.Minute)
	inactive, err := resourceService.Create(t.Context(), resources.Actor{AccountID: actorID}, resources.CreateRequest{
		Code: resourceRestartIdleCode, Name: "Restart Inactive Original", Description: "prepared only", Active: false,
	})
	if err != nil {
		t.Fatalf("create Inactive restart Resource through service: %v", err)
	}

	now = base.Add(3 * time.Minute)
	seededEdit, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
		ResourceID: seeded.ID, Code: seeded.Code,
		Name: "Task 4A seed-preserved name", Description: "Task 4A seed-preserved description",
	})
	if err != nil || !seededEdit.Changed {
		t.Fatalf("edit seeded Resource through service: result=%+v error=%v", seededEdit, err)
	}
	now = base.Add(4 * time.Minute)
	activeEdit, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
		ResourceID: active.Resource.ID, Code: active.Resource.Code,
		Name: "Restart Active Edited", Description: "active edit retained",
	})
	if err != nil || !activeEdit.Changed {
		t.Fatalf("edit Active restart Resource: result=%+v error=%v", activeEdit, err)
	}
	now = base.Add(5 * time.Minute)
	inactiveEdit, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
		ResourceID: inactive.Resource.ID, Code: inactive.Resource.Code,
		Name: "Restart Inactive Edited", Description: "inactive edit retained",
	})
	if err != nil || !inactiveEdit.Changed {
		t.Fatalf("edit Inactive restart Resource: result=%+v error=%v", inactiveEdit, err)
	}

	passwordHashes := map[string]string{
		"alex@example.test":   "unused-alex-hash",
		"sam@example.test":    "unused-sam-hash",
		"jordan@example.test": "unused-jordan-hash",
	}
	if err := Seed(t.Context(), adminPool, passwordHashes); err != nil {
		t.Fatalf("repeat seed after service-created Resource and seeded Resource edit: %v", err)
	}
	assertResourceRestartSnapshot(t, adminPool, store, actorID, seeded.ID, seeded.Code,
		"Task 4A seed-preserved name", "Task 4A seed-preserved description", seeded.Active,
		seededCreatedAt, base.Add(3*time.Minute), 1)
	assertResourceRestartSnapshot(t, adminPool, store, actorID, active.Resource.ID, resourceRestartActiveCode,
		"Restart Active Edited", "active edit retained", true,
		base.Add(time.Minute), base.Add(4*time.Minute), 2)
	assertResourceRestartSnapshot(t, adminPool, store, actorID, inactive.Resource.ID, resourceRestartIdleCode,
		"Restart Inactive Edited", "inactive edit retained", false,
		base.Add(2*time.Minute), base.Add(5*time.Minute), 2)

	bookingRequest := bookings.CreateRequest{
		ResourceID: active.Resource.ID, RequestID: resourceRestartRequestID,
		StartAt: base.Add(6 * time.Hour), EndAt: base.Add(7 * time.Hour), Purpose: "Task 4A restart replay",
	}
	bookingService := bookings.NewService(store, func() time.Time { return now })
	createdBooking, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: actorID}, bookingRequest)
	if err != nil || createdBooking.Replayed {
		t.Fatalf("create retained Booking for restart Resource: result=%+v error=%v", createdBooking, err)
	}
	now = base.Add(6 * time.Minute)
	cancelled, err := bookings.NewCancellationService(store, func() time.Time { return now }).Cancel(
		t.Context(), bookings.Actor{AccountID: actorID}, createdBooking.Booking.ID)
	if err != nil || !cancelled.Changed {
		t.Fatalf("cancel retained restart Booking: result=%+v error=%v", cancelled, err)
	}
	assertResourceRestartBooking(t, adminPool, createdBooking.Booking.ID, actorID, active.Resource.ID, bookingRequest, cancelled.Booking,
		base.Add(5*time.Minute), base.Add(6*time.Minute))
}

func TestResourceMutationRestartReplay(t *testing.T) {
	if os.Getenv(resourceRestartReplayEnv) != "1" {
		t.Skip("run by scripts/verify.sh after the PostgreSQL/application restart")
	}

	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	appPool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	if err := CheckSchema(t.Context(), appPool); err != nil {
		t.Fatalf("runtime connection after application/database restart has incompatible schema: %v", err)
	}
	actorID := resourceRestartCoordinator(t, adminPool)
	store := NewStore(appPool)
	base := bookingCreationNow().UTC().Truncate(time.Microsecond)
	now := base.Add(8 * time.Hour)
	resourceService := resources.NewService(store, func() time.Time { return now })

	seeded, found, err := resourceByCode(t, store, "NET-01")
	if err != nil || !found {
		t.Fatalf("reload seeded Resource after restart: found=%v error=%v", found, err)
	}
	seededCreatedAt, _ := resourceRestartTimes(t, adminPool, seeded.ID)
	assertResourceRestartSnapshot(t, adminPool, store, actorID, seeded.ID, seeded.Code,
		"Task 4A seed-preserved name", "Task 4A seed-preserved description", true,
		seededCreatedAt, base.Add(3*time.Minute), 1)
	active, found, err := resourceByCode(t, store, resourceRestartActiveCode)
	if err != nil || !found {
		t.Fatalf("reload Active Resource after restart: found=%v error=%v", found, err)
	}
	assertResourceRestartSnapshot(t, adminPool, store, actorID, active.ID, resourceRestartActiveCode,
		"Restart Active Edited", "active edit retained", true,
		base.Add(time.Minute), base.Add(4*time.Minute), 2)
	inactive, found, err := resourceByCode(t, store, resourceRestartIdleCode)
	if err != nil || !found {
		t.Fatalf("reload Inactive Resource after restart: found=%v error=%v", found, err)
	}
	assertResourceRestartSnapshot(t, adminPool, store, actorID, inactive.ID, resourceRestartIdleCode,
		"Restart Inactive Edited", "inactive edit retained", false,
		base.Add(2*time.Minute), base.Add(5*time.Minute), 2)

	var createCapability, editCapability, directInsert, directUpdate, schemaCreate bool
	if err := appPool.QueryRow(t.Context(), `
		SELECT has_function_privilege(current_user, 'public.labreserve_create_resource(uuid,text,text,text,boolean,timestamp with time zone)', 'EXECUTE'),
		       has_function_privilege(current_user, 'public.labreserve_edit_resource(uuid,uuid,text,text,timestamp with time zone)', 'EXECUTE'),
		       has_table_privilege(current_user, 'public.resources', 'INSERT'),
		       has_table_privilege(current_user, 'public.resources', 'UPDATE'),
		       has_schema_privilege(current_user, 'public', 'CREATE')`).Scan(
		&createCapability, &editCapability, &directInsert, &directUpdate, &schemaCreate); err != nil {
		t.Fatalf("inspect runtime capabilities after restart: %v", err)
	}
	if !createCapability || !editCapability || directInsert || directUpdate || schemaCreate {
		t.Fatalf("runtime permission drift after restart: create/edit=%v/%v direct insert/update=%v/%v schema-create=%v",
			createCapability, editCapability, directInsert, directUpdate, schemaCreate)
	}
	if _, err := appPool.Exec(t.Context(), "UPDATE public.resources SET name = name WHERE id = $1::uuid", active.ID); err == nil {
		t.Fatal("runtime role gained direct Resource UPDATE permission after restart")
	}

	beforeActiveEvents := resourceRestartEventCount(t, adminPool, active.ID)
	noOp, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: actorID}, resources.EditRequest{
		ResourceID: active.ID, Code: "\u2003" + active.Code + "\u00a0",
		Name: "\u00a0Restart Active Edited\u2003", Description: "\u2003active edit retained\u00a0",
	})
	if err != nil || noOp.Changed || !noOp.Resource.UpdatedAt.Equal(base.Add(4*time.Minute)) {
		t.Fatalf("post-restart canonical edit no-op: result=%+v error=%v", noOp, err)
	}
	if got := resourceRestartEventCount(t, adminPool, active.ID); got != beforeActiveEvents {
		t.Fatalf("post-restart no-op changed Resource Activity count from %d to %d", beforeActiveEvents, got)
	}
	if _, err := resourceService.Create(t.Context(), resources.Actor{AccountID: actorID}, resources.CreateRequest{
		Code: strings.ToLower(resourceRestartActiveCode), Name: "Must not replace existing Resource", Active: false,
	}); !errors.Is(err, resources.ErrDuplicateCode) {
		t.Fatalf("post-restart case-insensitive duplicate creation = %v, want duplicate-code rejection", err)
	}
	if got := resourceRestartEventCount(t, adminPool, active.ID); got != beforeActiveEvents {
		t.Fatalf("post-restart duplicate changed Resource Activity count from %d to %d", beforeActiveEvents, got)
	}

	bookingID, err := resourceRestartBookingID(t, adminPool, actorID)
	if err != nil {
		t.Fatal(err)
	}
	bookingRequest := bookings.CreateRequest{
		ResourceID: active.ID, RequestID: resourceRestartRequestID,
		StartAt: base.Add(6 * time.Hour), EndAt: base.Add(7 * time.Hour), Purpose: "Task 4A restart replay",
	}
	assertResourceRestartBookingPersistence(t, adminPool, bookingID, actorID, active.ID, bookingRequest,
		base.Add(5*time.Minute), base.Add(6*time.Minute))
	replayed, err := bookings.NewService(store, func() time.Time { return now }).Create(
		t.Context(), bookings.Actor{AccountID: actorID}, bookingRequest)
	if err != nil || !replayed.Replayed || replayed.Booking.ID != bookingID || replayed.Booking.State != bookings.StateCancelled {
		t.Fatalf("post-restart original Booking creation replay = %+v, %v", replayed, err)
	}
	cancelNow := base.Add(8 * time.Hour)
	cancelReplay, err := bookings.NewCancellationService(store, func() time.Time { return cancelNow }).Cancel(
		t.Context(), bookings.Actor{AccountID: actorID}, bookingID)
	if err != nil || cancelReplay.Changed || cancelReplay.Booking.State != bookings.StateCancelled ||
		cancelReplay.Booking.CancelledAt == nil || !cancelReplay.Booking.CancelledAt.Equal(base.Add(6*time.Minute)) {
		t.Fatalf("post-restart owner cancellation replay = %+v, %v", cancelReplay, err)
	}
	var createdEvents, cancelledEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'),
		       count(*) FILTER (WHERE action = 'booking.cancelled')
		FROM public.activity_events WHERE booking_id = $1::uuid`, bookingID).Scan(&createdEvents, &cancelledEvents); err != nil {
		t.Fatalf("inspect Booking events after restart replays: %v", err)
	}
	if createdEvents != 1 || cancelledEvents != 1 {
		t.Fatalf("post-restart Booking replay changed event counts to %d/%d", createdEvents, cancelledEvents)
	}
	assertResourceRestartBookingPersistence(t, adminPool, bookingID, actorID, active.ID, bookingRequest,
		base.Add(5*time.Minute), base.Add(6*time.Minute))
}

func resourceRestartCoordinator(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var actorID string
	if err := pool.QueryRow(t.Context(), `
		SELECT id::text FROM public.accounts WHERE login = 'jordan@example.test' AND role = 'coordinator'
	`).Scan(&actorID); err != nil {
		t.Fatalf("resolve persisted Coordinator for restart lifecycle: %v", err)
	}
	return actorID
}

func resourceByCode(t *testing.T, store *Store, code string) (Resource, bool, error) {
	t.Helper()
	listed, err := store.ListResources(t.Context())
	if err != nil {
		return Resource{}, false, err
	}
	for _, resource := range listed {
		if strings.EqualFold(resource.Code, code) {
			return resource, true, nil
		}
	}
	return Resource{}, false, nil
}

func resourceRestartTimes(t *testing.T, pool *pgxpool.Pool, resourceID string) (time.Time, time.Time) {
	t.Helper()
	var createdAt, updatedAt time.Time
	if err := pool.QueryRow(t.Context(), `
		SELECT created_at, updated_at FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(&createdAt, &updatedAt); err != nil {
		t.Fatalf("read Resource timestamps for %s: %v", resourceID, err)
	}
	return createdAt, updatedAt
}

func resourceRestartEventCount(t *testing.T, pool *pgxpool.Pool, resourceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid`, resourceID).Scan(&count); err != nil {
		t.Fatalf("count Resource Activity events for %s: %v", resourceID, err)
	}
	return count
}

func assertResourceRestartSnapshot(t *testing.T, adminPool *pgxpool.Pool, store *Store, actorID, id, code, name, description string, active bool,
	createdAt, updatedAt time.Time, wantEventCount int) {
	t.Helper()
	resource, found, err := store.ResourceByID(t.Context(), id)
	if err != nil || !found {
		t.Fatalf("reload Resource %s: found=%v error=%v", id, found, err)
	}
	if resource.ID != id || resource.Code != code || resource.Name != name || resource.Description != description || resource.Active != active {
		t.Fatalf("Resource read after lifecycle operation = %+v; want id=%s code=%q name=%q description=%q active=%v",
			resource, id, code, name, description, active)
	}
	storedCreatedAt, storedUpdatedAt := resourceRestartTimes(t, adminPool, id)
	if !storedCreatedAt.Equal(createdAt) || !storedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("Resource timestamps after lifecycle operation = %s/%s, want %s/%s", storedCreatedAt, storedUpdatedAt, createdAt, updatedAt)
	}
	events := readResourceCapabilityEvents(t, adminPool, id)
	if len(events) != wantEventCount {
		t.Fatalf("Resource %s has %d events, want %d: %+v", id, len(events), wantEventCount, events)
	}
	if wantEventCount == 1 {
		event := events[0]
		before, beforeOK := event.Details["before"].(map[string]any)
		after, afterOK := event.Details["after"].(map[string]any)
		if !beforeOK || !afterOK || event.ActorID != actorID || event.Action != "resource.descriptive_edited" ||
			!event.OccurredAt.Equal(updatedAt) || before["name"] != "Network Test Bench" ||
			before["description"] != "Shared environment for network integration tests." ||
			after["name"] != name || after["description"] != description {
			t.Fatalf("seeded Resource lifecycle event = %+v", event)
		}
		return
	}
	originalName := map[bool]string{true: "Restart Active Original", false: "Restart Inactive Original"}[active]
	originalDescription := map[bool]string{true: "before restart edit", false: "prepared only"}[active]
	if events[0].ActorID != actorID || events[0].Action != "resource.created" || !events[0].BookingIsNil ||
		!events[0].OccurredAt.Equal(createdAt) || events[0].Details["code"] != code ||
		events[0].Details["name"] != originalName || events[0].Details["description"] != originalDescription ||
		events[0].Details["active"] != active {
		t.Fatalf("Resource creation lifecycle event = %+v", events[0])
	}
	before, beforeOK := events[1].Details["before"].(map[string]any)
	after, afterOK := events[1].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || events[1].ActorID != actorID || events[1].Action != "resource.descriptive_edited" ||
		!events[1].BookingIsNil || !events[1].OccurredAt.Equal(updatedAt) || after["name"] != name ||
		after["description"] != description || before["name"] != originalName || before["description"] != originalDescription {
		t.Fatalf("Resource edit lifecycle event = %+v", events[1])
	}
}

func assertResourceRestartBooking(t *testing.T, pool *pgxpool.Pool, bookingID, actorID, resourceID string,
	request bookings.CreateRequest, cancelled bookings.Booking, createdAt, cancelledAt time.Time) {
	t.Helper()
	var storedResource, owner, requestID, purpose, state, cancelledBy string
	var startAt, endAt, storedCreatedAt, storedCancelledAt time.Time
	if err := pool.QueryRow(t.Context(), `
		SELECT resource_id::text, owner_account_id::text, request_id::text, purpose, state,
		       start_at, end_at, created_at, cancelled_by_account_id::text, cancelled_at
		FROM public.bookings WHERE id = $1::uuid`, bookingID).Scan(
		&storedResource, &owner, &requestID, &purpose, &state, &startAt, &endAt, &storedCreatedAt, &cancelledBy, &storedCancelledAt); err != nil {
		t.Fatalf("inspect retained restart Booking: %v", err)
	}
	if storedResource != resourceID || owner != actorID || requestID != request.RequestID || purpose != request.Purpose ||
		state != string(bookings.StateCancelled) || !startAt.Equal(request.StartAt) || !endAt.Equal(request.EndAt) ||
		!storedCreatedAt.Equal(createdAt) || cancelledBy != actorID || !storedCancelledAt.Equal(cancelledAt) ||
		cancelled.CancelledAt == nil || !cancelled.CancelledAt.Equal(storedCancelledAt) {
		t.Fatalf("restart fixture did not retain Booking identity/history: resource=%s owner=%s request=%s purpose=%q state=%s interval=%s..%s created=%s cancelledBy=%s cancelledAt=%s",
			storedResource, owner, requestID, purpose, state, startAt, endAt, storedCreatedAt, cancelledBy, storedCancelledAt)
	}
	var createdEvents, cancelledEvents int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE action = 'booking.created'), count(*) FILTER (WHERE action = 'booking.cancelled')
		FROM public.activity_events WHERE booking_id = $1::uuid`, bookingID).Scan(&createdEvents, &cancelledEvents); err != nil {
		t.Fatalf("inspect retained restart Booking events: %v", err)
	}
	if createdEvents != 1 || cancelledEvents != 1 {
		t.Fatalf("restart fixture Booking events = %d/%d, want 1/1", createdEvents, cancelledEvents)
	}
}

func assertResourceRestartBookingPersistence(t *testing.T, pool *pgxpool.Pool, bookingID, actorID, resourceID string,
	request bookings.CreateRequest, createdAt, cancelledAt time.Time) {
	t.Helper()
	var storedResource, ownerID, requestID, purpose, state, cancelledBy string
	var startAt, endAt, storedCreatedAt, storedCancelledAt time.Time
	if err := pool.QueryRow(t.Context(), `
		SELECT resource_id::text, owner_account_id::text, request_id::text, purpose, state,
		       start_at, end_at, created_at, cancelled_by_account_id::text, cancelled_at
		FROM public.bookings WHERE id = $1::uuid`, bookingID).Scan(
		&storedResource, &ownerID, &requestID, &purpose, &state,
		&startAt, &endAt, &storedCreatedAt, &cancelledBy, &storedCancelledAt); err != nil {
		t.Fatalf("reload retained Booking after restart: %v", err)
	}
	if storedResource != resourceID || ownerID != actorID || requestID != request.RequestID || purpose != request.Purpose ||
		state != string(bookings.StateCancelled) || !startAt.Equal(request.StartAt) || !endAt.Equal(request.EndAt) ||
		!storedCreatedAt.Equal(createdAt) || cancelledBy != actorID || !storedCancelledAt.Equal(cancelledAt) {
		t.Fatalf("retained Booking changed across restart/replay: resource=%s owner=%s request=%s purpose=%q state=%s interval=%s..%s created=%s cancelledBy=%s cancelledAt=%s",
			storedResource, ownerID, requestID, purpose, state, startAt, endAt, storedCreatedAt, cancelledBy, storedCancelledAt)
	}
}

func resourceRestartBookingID(t *testing.T, pool *pgxpool.Pool, actorID string) (string, error) {
	t.Helper()
	var bookingID string
	err := pool.QueryRow(t.Context(), `
		SELECT booking.id::text FROM public.bookings AS booking
		JOIN public.resources AS resource ON resource.id = booking.resource_id
		WHERE resource.code = $1 AND booking.owner_account_id = $2::uuid AND booking.request_id = $3::uuid
	`, resourceRestartActiveCode, actorID, resourceRestartRequestID).Scan(&bookingID)
	return bookingID, err
}
