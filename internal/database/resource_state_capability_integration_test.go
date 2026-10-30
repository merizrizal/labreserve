package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const setResourceStateCapabilitySQL = `
	SELECT resource_id::text, code, name, description, active, created_at, updated_at, changed
	FROM public.labreserve_set_resource_state($1::uuid, $2::uuid, $3::boolean, $4::timestamptz)`

func callSetResourceStateCapability(
	ctx context.Context,
	queryer resourceCapabilityQueryer,
	actorID, resourceID, active, occurredAt any,
) (resourceCapabilityRow, error) {
	var result resourceCapabilityRow
	err := queryer.QueryRow(ctx, setResourceStateCapabilitySQL, actorID, resourceID, active, occurredAt).Scan(
		&result.ID, &result.Code, &result.Name, &result.Description, &result.Active,
		&result.CreatedAt, &result.UpdatedAt, &result.Changed)
	return result, err
}

func registerResourceStateBookingCleanup(t *testing.T, admin *pgxpool.Pool) *[]string {
	t.Helper()
	bookingIDs := []string{}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, bookingID := range bookingIDs {
			if _, err := admin.Exec(ctx, "DELETE FROM public.activity_events WHERE booking_id = $1::uuid", bookingID); err != nil {
				t.Errorf("delete Resource state fixture Activity for Booking %s: %v", bookingID, err)
			}
			if _, err := admin.Exec(ctx, "DELETE FROM public.bookings WHERE id = $1::uuid", bookingID); err != nil {
				t.Errorf("delete Resource state fixture Booking %s: %v", bookingID, err)
			}
		}
	})
	return &bookingIDs
}

func insertResourceStateBooking(
	t *testing.T,
	admin *pgxpool.Pool,
	resourceID, ownerID string,
	startAt, endAt, createdAt time.Time,
	state, cancelledBy string,
) string {
	t.Helper()
	var cancelledByValue any
	var cancelledAtValue any
	if state == "cancelled" {
		cancelledByValue = cancelledBy
		cancelledAtValue = createdAt
	}
	var bookingID string
	err := admin.QueryRow(t.Context(), `
		INSERT INTO public.bookings (
			id, resource_id, owner_account_id, request_id, start_at, end_at,
			purpose, state, created_at, cancelled_by_account_id, cancelled_at
		)
		VALUES (
			gen_random_uuid(), $1::uuid, $2::uuid, gen_random_uuid(), $3, $4,
			'Resource state capability fixture', $5, $6, $7::uuid, $8
		)
		RETURNING id::text`, resourceID, ownerID, startAt, endAt, state, createdAt, cancelledByValue, cancelledAtValue).Scan(&bookingID)
	if err != nil {
		t.Fatalf("create Resource state Booking fixture: %v", err)
	}
	return bookingID
}

func resourceStateBookingSnapshot(t *testing.T, admin *pgxpool.Pool, bookingID string) string {
	t.Helper()
	var snapshot string
	if err := admin.QueryRow(t.Context(), `
		SELECT to_jsonb(booking)::text
		FROM public.bookings AS booking
		WHERE booking.id = $1::uuid`, bookingID).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot Resource state Booking %s: %v", bookingID, err)
	}
	return snapshot
}

func assertResourceStateEvent(
	t *testing.T,
	admin *pgxpool.Pool,
	resourceID, actorID, action string,
	occurredAt time.Time,
	beforeActive, afterActive bool,
) {
	t.Helper()
	events := readResourceCapabilityEvents(t, admin, resourceID)
	if len(events) != 1 || events[0].ActorID != actorID || events[0].Action != action ||
		!events[0].OccurredAt.Equal(occurredAt) || !events[0].BookingIsNil {
		t.Fatalf("state Activity = %+v, want one %s event for actor %s at %s", events, action, actorID, occurredAt)
	}
	before, beforeOK := events[0].Details["before"].(map[string]any)
	after, afterOK := events[0].Details["after"].(map[string]any)
	if !beforeOK || !afterOK || before["active"] != beforeActive || after["active"] != afterActive {
		t.Fatalf("state Activity details = %#v, want active %v -> %v", events[0].Details, beforeActive, afterActive)
	}
}

func TestResourceStateCapabilityTransitionsNoOpsAndAuthorization(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state capability migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, admin)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	operationAt := createdAt.Add(time.Hour)

	inactiveID := insertResourceCapabilityFixture(t, admin, "State-Activate-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "  Legacy Name  ", "<b>legacy</b>", false, createdAt)
	fixtures.trackResource(inactiveID)
	activated, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, inactiveID, true, operationAt)
	if err != nil || !activated.Changed || !activated.Active || activated.Name != "  Legacy Name  " ||
		activated.Description != "<b>legacy</b>" || !activated.CreatedAt.Equal(createdAt) || !activated.UpdatedAt.Equal(operationAt) {
		t.Fatalf("activate retained Resource: result=%+v error=%v", activated, err)
	}
	assertResourceStateEvent(t, admin, inactiveID, fixtures.coordinator, "resource.activated", operationAt, false, true)

	installResourceUpdateFailureTrigger(t, admin, inactiveID)
	removeActivationFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.activated")
	beforeNoOp, beforeNoOpEvents := resourceCapabilitySnapshot(t, admin, inactiveID)
	noOp, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, inactiveID, true, operationAt.Add(time.Hour))
	if err != nil || noOp.Changed || !noOp.Active || !noOp.UpdatedAt.Equal(operationAt) {
		t.Fatalf("active desired-state no-op: result=%+v error=%v", noOp, err)
	}
	removeActivationFailure()
	afterNoOp, afterNoOpEvents := resourceCapabilitySnapshot(t, admin, inactiveID)
	if beforeNoOp != afterNoOp || beforeNoOpEvents != afterNoOpEvents {
		t.Fatalf("active no-op changed Resource/Activity: before=%s %s after=%s %s", beforeNoOp, beforeNoOpEvents, afterNoOp, afterNoOpEvents)
	}

	inactiveNoOpID := insertResourceCapabilityFixture(t, admin, "State-Noop-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Inactive Lab", "Retained", false, createdAt)
	fixtures.trackResource(inactiveNoOpID)
	futureBookingID := insertResourceStateBooking(t, admin, inactiveNoOpID, fixtures.engineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, futureBookingID)
	beforeNoOp, beforeNoOpEvents = resourceCapabilitySnapshot(t, admin, inactiveNoOpID)
	installResourceUpdateFailureTrigger(t, admin, inactiveNoOpID)
	removeDeactivationFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, "resource.deactivated")
	inactiveNoOp, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, inactiveNoOpID, false, operationAt)
	if err != nil || inactiveNoOp.Changed || inactiveNoOp.Active || !inactiveNoOp.UpdatedAt.Equal(createdAt) {
		t.Fatalf("inactive desired-state no-op with future occupancy: result=%+v error=%v", inactiveNoOp, err)
	}
	removeDeactivationFailure()
	afterNoOp, afterNoOpEvents = resourceCapabilitySnapshot(t, admin, inactiveNoOpID)
	if beforeNoOp != afterNoOp || beforeNoOpEvents != afterNoOpEvents {
		t.Fatalf("inactive no-op changed Resource/Activity: before=%s %s after=%s %s", beforeNoOp, beforeNoOpEvents, afterNoOp, afterNoOpEvents)
	}

	activationWithOccupancyID := insertResourceCapabilityFixture(t, admin, "State-Activate-Occupied-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Inactive With Booking", "Retained", false, createdAt)
	fixtures.trackResource(activationWithOccupancyID)
	occupiedBookingID := insertResourceStateBooking(t, admin, activationWithOccupancyID, fixtures.engineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, occupiedBookingID)
	occupiedBookingBefore := resourceStateBookingSnapshot(t, admin, occupiedBookingID)
	activatedWithOccupancy, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator,
		activationWithOccupancyID, true, operationAt)
	if err != nil || !activatedWithOccupancy.Changed || !activatedWithOccupancy.Active {
		t.Fatalf("activation with retained occupancy: result=%+v error=%v", activatedWithOccupancy, err)
	}
	assertResourceStateEvent(t, admin, activationWithOccupancyID, fixtures.coordinator, "resource.activated", operationAt, false, true)
	if got := resourceStateBookingSnapshot(t, admin, occupiedBookingID); got != occupiedBookingBefore {
		t.Fatalf("activation changed retained Booking: before=%s after=%s", occupiedBookingBefore, got)
	}

	for _, actorID := range []string{fixtures.engineer, fixtures.secondEngineer, fixtures.unknown} {
		beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, inactiveID)
		if _, err := callSetResourceStateCapability(t.Context(), runtime, actorID, inactiveID, true, operationAt); err == nil {
			t.Fatalf("non-Coordinator actor %s changed Resource state", actorID)
		} else {
			assertResourceCapabilityErrorCode(t, err, "42501")
		}
		afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, inactiveID)
		if beforeResource != afterResource || beforeEvents != afterEvents {
			t.Fatalf("rejected state actor changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
		}
	}
	if _, err := callSetResourceStateCapability(t.Context(), runtime, nil, inactiveID, true, operationAt); err == nil {
		t.Fatal("null actor used the Resource state capability")
	} else {
		assertResourceCapabilityErrorCode(t, err, "42501")
	}
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, nil, false, operationAt); err == nil {
		t.Fatal("null Resource ID was accepted by the state capability")
	} else {
		assertResourceCapabilityErrorCode(t, err, "22023")
	}
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, inactiveID, nil, operationAt); err == nil {
		t.Fatal("null desired state was accepted by the state capability")
	} else {
		assertResourceCapabilityErrorCode(t, err, "22023")
	}
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, inactiveID, false, nil); err == nil {
		t.Fatal("null operation time was accepted by the state capability")
	} else {
		assertResourceCapabilityErrorCode(t, err, "22023")
	}
	unknownResourceID := testDatabaseUUID(t, admin)
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, unknownResourceID, false, operationAt); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown Resource result error = %v, want no row", err)
	}
}

func TestResourceStateCapabilityDeactivationUsesConfirmedEndBoundary(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state capability migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, admin)
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
		blocked       bool
	}{
		{name: "upcoming confirmed booking blocks", startOffset: time.Hour, endOffset: 2 * time.Hour, bookingState: "confirmed", blocked: true},
		{name: "in-use confirmed booking blocks at exact start", startOffset: 0, endOffset: 30 * time.Minute, bookingState: "confirmed", blocked: true},
		{name: "confirmed booking blocks one microsecond before end", startOffset: -30 * time.Minute, endOffset: time.Microsecond, bookingState: "confirmed", blocked: true},
		{name: "confirmed booking does not block at exact end", startOffset: -30 * time.Minute, endOffset: 0, bookingState: "confirmed"},
		{name: "cancelled future booking does not block", startOffset: time.Hour, endOffset: 2 * time.Hour, bookingState: "cancelled"},
		{name: "booking on another Resource does not block", startOffset: time.Hour, endOffset: 2 * time.Hour, bookingState: "confirmed", otherResource: true},
		{name: "one blocker among retained nonblocking bookings blocks", startOffset: 5 * time.Hour, endOffset: 6 * time.Hour, bookingState: "confirmed", alsoRetained: true, blocked: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resourceCode := "State-Boundary-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
			resourceID := insertResourceCapabilityFixture(t, admin,
				resourceCode, "Boundary Lab", "Raw  description ", true, createdAt)
			fixtures.trackResource(resourceID)
			bookingResourceID := resourceID
			if test.otherResource {
				bookingResourceID = insertResourceCapabilityFixture(t, admin,
					"State-Other-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
					"Other Lab", "", true, createdAt)
				fixtures.trackResource(bookingResourceID)
			}
			var retainedBookingIDs []string
			if test.alsoRetained {
				endedID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
					operationAt.Add(-2*time.Hour), operationAt, createdAt, "confirmed", "")
				cancelledID := insertResourceStateBooking(t, admin, resourceID, fixtures.secondEngineer,
					operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "cancelled", fixtures.secondEngineer)
				retainedBookingIDs = append(retainedBookingIDs, endedID, cancelledID)
				*bookingIDs = append(*bookingIDs, endedID, cancelledID)
			}
			bookingID := insertResourceStateBooking(t, admin, bookingResourceID, fixtures.engineer,
				operationAt.Add(test.startOffset), operationAt.Add(test.endOffset), createdAt, test.bookingState,
				fixtures.engineer)
			*bookingIDs = append(*bookingIDs, bookingID)
			allBookingIDs := append(retainedBookingIDs, bookingID)
			beforeBookings := make(map[string]string, len(allBookingIDs))
			for _, id := range allBookingIDs {
				beforeBookings[id] = resourceStateBookingSnapshot(t, admin, id)
			}
			beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

			result, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, false, operationAt)
			if test.blocked {
				assertResourceCapabilityErrorCode(t, err, "P0001")
				if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
					t.Fatalf("blocked deactivation changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
				}
			} else {
				if err != nil || !result.Changed || result.Active || result.Code != resourceCode ||
					result.Name != "Boundary Lab" || result.Description != "Raw  description " ||
					!result.CreatedAt.Equal(createdAt) || !result.UpdatedAt.Equal(operationAt) {
					t.Fatalf("eligible deactivation: result=%+v error=%v", result, err)
				}
				assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", operationAt, true, false)
			}
			for _, id := range allBookingIDs {
				if after := resourceStateBookingSnapshot(t, admin, id); after != beforeBookings[id] {
					t.Fatalf("deactivation changed retained Booking %s: before=%s after=%s", id, beforeBookings[id], after)
				}
			}
		})
	}
}

func TestResourceStateCapabilityActivityFailureAndOuterRollback(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state capability migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, admin)
	createdAt := time.Date(2040, time.March, 4, 5, 6, 7, 0, time.UTC)
	operationAt := createdAt.Add(time.Hour)

	for _, desiredActive := range []bool{true, false} {
		direction := map[bool]string{true: "activation", false: "deactivation"}[desiredActive]
		resourceID := insertResourceCapabilityFixture(t, admin,
			"State-Rollback-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
			"Rollback Lab", "Raw legacy", !desiredActive, createdAt)
		fixtures.trackResource(resourceID)
		action := map[bool]string{true: "resource.activated", false: "resource.deactivated"}[desiredActive]
		beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

		removeFailure := installResourceActivityFailureTrigger(t, admin, fixtures.coordinator, action)
		if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, desiredActive, operationAt); err == nil {
			t.Fatalf("%s committed despite required Activity failure", direction)
		}
		removeFailure()
		if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
			t.Fatalf("%s Activity failure left partial state/history: before=%s %s after=%s %s", direction, beforeResource, beforeEvents, afterResource, afterEvents)
		}

		result, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, desiredActive, operationAt)
		if err != nil || !result.Changed || result.Active != desiredActive || !result.UpdatedAt.Equal(operationAt) {
			t.Fatalf("explicit %s retry: result=%+v error=%v", direction, result, err)
		}
		assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, action, operationAt, !desiredActive, desiredActive)
		afterSuccessResource, afterSuccessEvents := resourceCapabilitySnapshot(t, admin, resourceID)

		tx, err := runtime.Begin(t.Context())
		if err != nil {
			t.Fatalf("begin outer %s rollback transaction: %v", direction, err)
		}
		rolledBack, err := callSetResourceStateCapability(t.Context(), tx, fixtures.coordinator, resourceID, !desiredActive, operationAt.Add(time.Minute))
		if err != nil || !rolledBack.Changed || rolledBack.Active != !desiredActive {
			_ = tx.Rollback(context.Background())
			t.Fatalf("invoke guarded capability in outer %s transaction: result=%+v error=%v", direction, rolledBack, err)
		}
		if err := tx.Rollback(t.Context()); err != nil {
			t.Fatalf("roll back outer %s transaction: %v", direction, err)
		}
		if afterRollbackResource, afterRollbackEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterRollbackResource != afterSuccessResource || afterRollbackEvents != afterSuccessEvents {
			t.Fatalf("outer %s rollback retained Resource/Activity changes: before=%s %s after=%s %s",
				direction, afterSuccessResource, afterSuccessEvents, afterRollbackResource, afterRollbackEvents)
		}
	}
}

func TestResourceStateCapabilityUsesQualifiedOccupancyAndWrites(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state capability migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, admin)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	createdAt := time.Date(2040, time.April, 5, 6, 7, 8, 0, time.UTC)
	operationAt := createdAt.Add(time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Shadow-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Shadow Target", "", true, createdAt)
	fixtures.trackResource(resourceID)
	bookingID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, bookingID)

	blockedTx, err := runtime.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin caller-search-path occupancy test: %v", err)
	}
	defer blockedTx.Rollback(context.Background())
	for _, statement := range []string{
		`CREATE TEMP TABLE accounts (id uuid, role text) ON COMMIT DROP`,
		`CREATE TEMP TABLE resources (id uuid, code text, name text, description text, active boolean, created_at timestamptz, updated_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE bookings (resource_id uuid, state text, end_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE activity_events (id uuid, actor_account_id uuid, action text, occurred_at timestamptz, resource_id uuid, details jsonb) ON COMMIT DROP`,
		`SET LOCAL search_path = pg_temp, public`,
	} {
		if _, err := blockedTx.Exec(t.Context(), statement); err != nil {
			_ = blockedTx.Rollback(context.Background())
			t.Fatalf("create caller-controlled shadow object: %v", err)
		}
	}
	if _, err := blockedTx.Exec(t.Context(), `INSERT INTO pg_temp.accounts (id, role) VALUES ($1::uuid, 'engineer')`, fixtures.coordinator); err != nil {
		_ = blockedTx.Rollback(context.Background())
		t.Fatalf("insert shadow actor: %v", err)
	}
	if _, err := callSetResourceStateCapability(t.Context(), blockedTx, fixtures.coordinator, resourceID, false, operationAt); err == nil {
		_ = blockedTx.Rollback(context.Background())
		t.Fatal("state capability ignored the qualified public Confirmed occupancy row")
	} else {
		assertResourceCapabilityErrorCode(t, err, "P0001")
	}
	if err := blockedTx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back blocked caller-search-path test: %v", err)
	}

	shadowed, err := callCreateResourceCapability(t.Context(), runtime, fixtures.coordinator,
		"State-Shadow-Create-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Shadow Create", "", true, operationAt)
	if err != nil {
		t.Fatalf("create Resource for qualified state-write check: %v", err)
	}
	fixtures.trackResource(shadowed.ID)
	tx, err := runtime.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin caller-search-path state-write test: %v", err)
	}
	defer tx.Rollback(context.Background())
	for _, statement := range []string{
		`CREATE TEMP TABLE accounts (id uuid, role text) ON COMMIT DROP`,
		`CREATE TEMP TABLE resources (id uuid, code text, name text, description text, active boolean, created_at timestamptz, updated_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE bookings (resource_id uuid, state text, end_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE activity_events (id uuid, actor_account_id uuid, action text, occurred_at timestamptz, resource_id uuid, details jsonb) ON COMMIT DROP`,
		`SET LOCAL search_path = pg_temp, public`,
	} {
		if _, err := tx.Exec(t.Context(), statement); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("create caller-controlled shadow object: %v", err)
		}
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO pg_temp.accounts (id, role) VALUES ($1::uuid, 'engineer')`, fixtures.coordinator); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("insert shadow actor: %v", err)
	}
	stateResult, err := callSetResourceStateCapability(t.Context(), tx, fixtures.coordinator, shadowed.ID, false, operationAt.Add(time.Minute))
	if err != nil || !stateResult.Changed || stateResult.Active {
		_ = tx.Rollback(context.Background())
		t.Fatalf("state capability with caller shadow objects: result=%+v error=%v", stateResult, err)
	}
	var shadowResources, shadowEvents int
	if err := tx.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM pg_temp.resources), (SELECT count(*) FROM pg_temp.activity_events)`).Scan(&shadowResources, &shadowEvents); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("inspect shadow tables: %v", err)
	}
	if shadowResources != 0 || shadowEvents != 0 {
		_ = tx.Rollback(context.Background())
		t.Fatalf("state capability wrote to shadow tables: resources=%d events=%d", shadowResources, shadowEvents)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit caller-search-path state-write test: %v", err)
	}
	state, events := resourceCapabilitySnapshot(t, admin, shadowed.ID)
	if !strings.Contains(state, `"active": false`) || !strings.Contains(events, `resource.deactivated`) {
		t.Fatalf("state capability did not persist to qualified public tables: Resource=%s Activity=%s", state, events)
	}
}

func TestResourceStateCapabilityWaitsForResourceAndSeesCommittedBooking(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state capability migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, admin)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	createdAt := time.Date(2040, time.June, 7, 8, 9, 10, 0, time.UTC)
	operationAt := createdAt.Add(time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Lock-Wait-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Lock Wait Lab", "", true, createdAt)
	fixtures.trackResource(resourceID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	held := holdBookingResourceLock(t, admin, resourceID)
	callCtx, cancelCall := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelCall()
	type stateCall struct {
		result resourceCapabilityRow
		err    error
	}
	resultCh := make(chan stateCall, 1)
	go func() {
		result, err := callSetResourceStateCapability(callCtx, runtime, fixtures.coordinator, resourceID, false, operationAt)
		resultCh <- stateCall{result: result, err: err}
	}()
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelWait()
	if err := waitForBlockedBookingCalls(waitCtx, admin, held.pid, 1); err != nil {
		t.Fatalf("observe state capability waiting on Resource FOR UPDATE: %v", err)
	}

	var bookingID string
	bookingStart := operationAt.Add(time.Hour)
	if err := held.tx.QueryRow(t.Context(), `
		INSERT INTO public.bookings (
			id, resource_id, owner_account_id, request_id, start_at, end_at, purpose, state, created_at
		)
		VALUES (
			gen_random_uuid(), $1::uuid, $2::uuid, gen_random_uuid(), $3::timestamptz, $3::timestamptz + interval '1 hour',
			'Committed while state capability waits', 'confirmed', $4
		)
		RETURNING id::text`, resourceID, fixtures.engineer, bookingStart, createdAt).Scan(&bookingID); err != nil {
		t.Fatalf("insert Booking in the observed Resource lock holder: %v", err)
	}
	if _, err := held.tx.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'booking.created', $2, $3::uuid, '{"lock_wait":true}'::jsonb)`,
		fixtures.engineer, createdAt, bookingID); err != nil {
		t.Fatalf("insert committed Booking Activity while holding Resource: %v", err)
	}
	*bookingIDs = append(*bookingIDs, bookingID)
	if err := held.release(); err != nil {
		t.Fatalf("commit Booking holder after observed state wait: %v", err)
	}

	var outcome stateCall
	select {
	case outcome = <-resultCh:
	case <-callCtx.Done():
		t.Fatalf("state capability did not finish after Resource lock release: %v", callCtx.Err())
	}
	assertResourceCapabilityErrorCode(t, outcome.err, "P0001")
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("waiting deactivation did not preserve Resource/Activity after seeing committed occupancy: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
	bookingSnapshot := resourceStateBookingSnapshot(t, admin, bookingID)
	var bookingEventCount int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events
		WHERE booking_id = $1::uuid AND action = 'booking.created'`, bookingID).Scan(&bookingEventCount); err != nil {
		t.Fatalf("read committed Booking Activity after state rejection: %v", err)
	}
	if bookingSnapshot == "" || bookingEventCount != 1 {
		t.Fatalf("lock-holder Booking/event were not retained: booking=%s events=%d", bookingSnapshot, bookingEventCount)
	}
}
