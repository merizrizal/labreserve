package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/resources"
)

type resourceStateConcurrencyFixture struct {
	admin                *pgxpool.Pool
	app                  *pgxpool.Pool
	actors               *resourceCapabilityFixtures
	resourceID           string
	code                 string
	createdAt            time.Time
	operationAt          time.Time
	initialActive        bool
	initialName          string
	initialDescription   string
	createdEventSnapshot string
	bookingIDs           *[]string
	bookingSnapshots     map[string]string
}

type resourceStateConcurrencyEvent struct {
	actorID    string
	action     string
	occurredAt time.Time
	before     bool
	after      bool
}

func newResourceStateConcurrencyFixture(t *testing.T, active bool) *resourceStateConcurrencyFixture {
	t.Helper()
	admin, app, actors := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	operationAt := time.Date(2040, time.April, 5, 6, 7, 8, 123456000, time.UTC)
	createdAt := operationAt.Add(-24 * time.Hour)
	code := "State-Concurrent-" + strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	name := "\u2003Legacy concurrent lab\t"
	description := "  Raw\tlegacy  description\n"
	resourceID := insertResourceCapabilityFixture(t, admin, code, name, description, active, createdAt)
	actors.trackResource(resourceID)
	insertResourceStateConcurrencyCreationHistory(t, admin, actors.coordinator, resourceID, createdAt, active)

	pastBookingID := insertResourceStateBooking(t, admin, resourceID, actors.engineer,
		operationAt.Add(-5*time.Hour), operationAt.Add(-4*time.Hour), createdAt, "confirmed", "")
	cancelledBookingID := insertResourceStateBooking(t, admin, resourceID, actors.secondEngineer,
		operationAt.Add(2*time.Hour), operationAt.Add(3*time.Hour), createdAt, "cancelled", actors.secondEngineer)
	*bookingIDs = append(*bookingIDs, pastBookingID, cancelledBookingID)
	insertResourceStateBookingHistory(t, admin, pastBookingID, actors.engineer, "confirmed", createdAt)
	insertResourceStateBookingHistory(t, admin, cancelledBookingID, actors.secondEngineer, "cancelled", createdAt)

	createdEventSnapshot := resourceStateConcurrencyEventSnapshot(t, admin, resourceID, "resource.created")
	bookingSnapshots := map[string]string{
		pastBookingID:      resourceStateBookingAndHistorySnapshot(t, admin, pastBookingID),
		cancelledBookingID: resourceStateBookingAndHistorySnapshot(t, admin, cancelledBookingID),
	}
	return &resourceStateConcurrencyFixture{
		admin: admin, app: app, actors: actors, resourceID: resourceID, code: code,
		createdAt: createdAt, operationAt: operationAt, initialActive: active,
		initialName: name, initialDescription: description,
		createdEventSnapshot: createdEventSnapshot, bookingIDs: bookingIDs, bookingSnapshots: bookingSnapshots,
	}
}

func insertResourceStateConcurrencyCreationHistory(
	t *testing.T,
	admin *pgxpool.Pool,
	actorID, resourceID string,
	occurredAt time.Time,
	active bool,
) {
	t.Helper()
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'resource.created', $2, $3::uuid,
		        jsonb_build_object('active', $4::boolean, 'source', 'concurrent state fixture'))`,
		actorID, occurredAt, resourceID, active); err != nil {
		t.Fatalf("create retained Resource Activity fixture: %v", err)
	}
}

func resourceStateConcurrencyEventSnapshot(
	t *testing.T,
	admin *pgxpool.Pool,
	resourceID, action string,
) string {
	t.Helper()
	var snapshot string
	if err := admin.QueryRow(t.Context(), `
		SELECT to_jsonb(event)::text FROM public.activity_events AS event
		WHERE event.resource_id = $1::uuid AND event.action = $2`, resourceID, action).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot %s Activity for Resource %s: %v", action, resourceID, err)
	}
	return snapshot
}

func (fixture *resourceStateConcurrencyFixture) assertPersistedResource(
	t *testing.T,
	active bool,
	updatedAt time.Time,
	name, description string,
) {
	t.Helper()
	var got resources.Resource
	if err := fixture.admin.QueryRow(t.Context(), `
		SELECT id::text, code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, fixture.resourceID).Scan(
		&got.ID, &got.Code, &got.Name, &got.Description, &got.Active, &got.CreatedAt, &got.UpdatedAt); err != nil {
		t.Fatalf("read persisted Resource concurrency result: %v", err)
	}
	if got.ID != fixture.resourceID || got.Code != fixture.code || got.Name != name ||
		got.Description != description || got.Active != active ||
		!got.CreatedAt.Equal(fixture.createdAt) || !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("persisted Resource=%+v; want active=%v updated_at=%s name=%q description=%q",
			got, active, updatedAt, name, description)
	}
}

func (fixture *resourceStateConcurrencyFixture) assertBookingsUnchanged(t *testing.T) {
	t.Helper()
	for bookingID, before := range fixture.bookingSnapshots {
		if after := resourceStateBookingAndHistorySnapshot(t, fixture.admin, bookingID); after != before {
			t.Fatalf("Resource state operation changed retained Booking/history %s: before=%s after=%s",
				bookingID, before, after)
		}
	}
	var bookingCount int
	if err := fixture.admin.QueryRow(t.Context(),
		"SELECT count(*) FROM public.bookings WHERE resource_id = $1::uuid", fixture.resourceID).Scan(&bookingCount); err != nil {
		t.Fatalf("count retained Resource Bookings: %v", err)
	}
	if bookingCount != len(fixture.bookingSnapshots) {
		t.Fatalf("Resource has %d Bookings, want unchanged count %d", bookingCount, len(fixture.bookingSnapshots))
	}
}

func (fixture *resourceStateConcurrencyFixture) assertHistory(
	t *testing.T,
	expected ...resourceStateConcurrencyEvent,
) {
	t.Helper()
	events := readResourceCapabilityEvents(t, fixture.admin, fixture.resourceID)
	if len(events) != len(expected)+1 {
		t.Fatalf("Resource Activity count=%d, want creation plus %d transitions: %+v", len(events), len(expected), events)
	}
	if created := resourceStateConcurrencyEventSnapshot(t, fixture.admin, fixture.resourceID, "resource.created"); created != fixture.createdEventSnapshot {
		t.Fatalf("state operation changed original Resource creation history: before=%s after=%s",
			fixture.createdEventSnapshot, created)
	}
	createdCount := 0
	for _, event := range events {
		if event.Action == "resource.created" {
			createdCount++
			if event.ActorID != fixture.actors.coordinator || !event.OccurredAt.Equal(fixture.createdAt) || !event.BookingIsNil {
				t.Fatalf("retained Resource creation event changed: %+v", event)
			}
		}
	}
	if createdCount != 1 {
		t.Fatalf("Resource has %d creation events, want one", createdCount)
	}

	seen := make(map[string]bool, len(expected))
	for _, want := range expected {
		if seen[want.action] {
			t.Fatalf("test expectation repeats Activity action %q", want.action)
		}
		seen[want.action] = true
		var matched int
		for _, event := range events {
			if event.Action != want.action {
				continue
			}
			matched++
			before, beforeOK := event.Details["before"].(map[string]any)
			after, afterOK := event.Details["after"].(map[string]any)
			if event.ActorID != want.actorID || !event.BookingIsNil || !event.OccurredAt.Equal(want.occurredAt) ||
				!beforeOK || !afterOK || before["active"] != want.before || after["active"] != want.after {
				t.Fatalf("persisted %s Activity=%+v; want actor=%s at=%s active %v -> %v",
					event.Action, event, want.actorID, want.occurredAt, want.before, want.after)
			}
		}
		if matched != 1 {
			t.Fatalf("Resource has %d %s events, want one", matched, want.action)
		}
	}
}

func installResourceStateNoOpUpdateFailureTrigger(t *testing.T, admin *pgxpool.Pool, resourceID string) {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := pgx.Identifier{"public", "labreserve_test_state_noop_update_" + suffix}.Sanitize()
	triggerName := pgx.Identifier{"labreserve_test_state_noop_update_" + suffix}.Sanitize()
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF OLD.id = '%s'::uuid AND OLD.active = NEW.active THEN
		        RAISE EXCEPTION 'unexpected Resource state UPDATE without a state transition';
		    END IF;
		    RETURN NEW;
		END;
		$test$`, functionName, resourceID)); err != nil {
		t.Fatalf("create state-aware no-op UPDATE detector: %v", err)
	}
	cleanup := installResourceStateConcurrencyTriggerCleanup(t, admin, "public.resources", triggerName, functionName)
	if _, err := admin.Exec(t.Context(), "CREATE TRIGGER "+triggerName+
		" BEFORE UPDATE ON public.resources FOR EACH ROW EXECUTE FUNCTION "+functionName+"()"); err != nil {
		cleanup()
		t.Fatalf("install state-aware no-op UPDATE detector: %v", err)
	}
}

func installResourceStateConcurrencyTriggerCleanup(
	t *testing.T,
	admin *pgxpool.Pool,
	tableName, triggerName, functionName string,
) func() {
	t.Helper()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON "+tableName); err != nil {
				t.Errorf("drop state concurrency trigger: %v", err)
			}
			if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS "+functionName+"()"); err != nil {
				t.Errorf("drop state concurrency trigger function: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)
	return cleanup
}

func installResourceStateMutationFailureAt(
	t *testing.T,
	admin *pgxpool.Pool,
	resourceID string,
	occurredAt time.Time,
) func() {
	t.Helper()
	condition := fmt.Sprintf(
		"OLD.id = %s::uuid AND OLD.active IS TRUE AND NEW.active IS FALSE AND NEW.updated_at = %s::timestamptz",
		sqlLiteral(resourceID), sqlLiteral(occurredAt.UTC().Format(time.RFC3339Nano)))
	return installResourceStateConcurrencyFailureTrigger(
		t, admin, "public.resources", "AFTER UPDATE", "labreserve_test_state_mutation_failure_", condition,
		"injected Resource state mutation failure")
}

func installResourceStateActivityFailureAt(
	t *testing.T,
	admin *pgxpool.Pool,
	actorID, resourceID string,
	occurredAt time.Time,
) func() {
	t.Helper()
	condition := fmt.Sprintf(
		"NEW.actor_account_id = %s::uuid AND NEW.resource_id = %s::uuid AND NEW.action = 'resource.deactivated' AND NEW.occurred_at = %s::timestamptz",
		sqlLiteral(actorID), sqlLiteral(resourceID), sqlLiteral(occurredAt.UTC().Format(time.RFC3339Nano)))
	return installResourceStateConcurrencyFailureTrigger(
		t, admin, "public.activity_events", "BEFORE INSERT", "labreserve_test_state_activity_failure_", condition,
		"injected Resource state Activity failure")
}

func installResourceStateConcurrencyFailureTrigger(
	t *testing.T,
	admin *pgxpool.Pool,
	tableName, operation, prefix, condition, message string,
) func() {
	t.Helper()
	if operation != "AFTER UPDATE" && operation != "BEFORE INSERT" {
		t.Fatalf("unsupported Resource state failure trigger operation %q", operation)
	}
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := pgx.Identifier{"public", prefix + suffix}.Sanitize()
	triggerName := pgx.Identifier{prefix + suffix}.Sanitize()
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF %s THEN
		        RAISE EXCEPTION %s;
		    END IF;
		    RETURN NEW;
		END;
		$test$`, functionName, condition, sqlLiteral(message))); err != nil {
		t.Fatalf("create Resource state failure-injection trigger function: %v", err)
	}
	cleanup := installResourceStateConcurrencyTriggerCleanup(t, admin, tableName, triggerName, functionName)
	if _, err := admin.Exec(t.Context(), "CREATE TRIGGER "+triggerName+
		" "+operation+" ON "+tableName+" FOR EACH ROW EXECUTE FUNCTION "+functionName+"()"); err != nil {
		cleanup()
		t.Fatalf("install Resource state failure-injection trigger: %v", err)
	}
	return cleanup
}

func TestResourceStateConcurrentRepeatedDesiredStatesAreTrueNoOps(t *testing.T) {
	for _, test := range []struct {
		name          string
		initialActive bool
		requested     bool
		action        string
	}{
		{name: "repeated Active", initialActive: false, requested: true, action: "resource.activated"},
		{name: "repeated Inactive", initialActive: true, requested: false, action: "resource.deactivated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, test.initialActive)
			installResourceStateNoOpUpdateFailureTrigger(t, fixture.admin, fixture.resourceID)
			operationAt := fixture.operationAt
			group := newBookingRaceGroup(t)
			leaderGate := group.newGate()
			leaderStarted := make(chan int32, 1)
			leaderCalls := &atomic.Int32{}
			leaderClock := newAtomicBookingClockAt(operationAt)
			leaderService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: leaderStarted,
				beforeCommit: leaderGate, stateCalls: leaderCalls,
			}, leaderClock.Now)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			leader := startResourceStateRaceCall(group, ctx, leaderService,
				resources.Actor{AccountID: fixture.actors.coordinator},
				resources.StateRequest{ResourceID: fixture.resourceID, Active: test.requested})
			leaderPID := awaitBookingRacePID(t, ctx, leaderStarted, "leading repeated desired-state transaction")
			if gatePID := leaderGate.await(t, ctx, "leader after state mutation and Activity"); gatePID != leaderPID {
				t.Fatalf("leader gate backend PID=%d, transaction PID=%d", gatePID, leaderPID)
			}

			waiterStarted := make(chan int32, 1)
			waiterCalls := &atomic.Int32{}
			waiterClock := newAtomicBookingClockAt(operationAt.Add(time.Hour))
			waiterService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: waiterStarted, stateCalls: waiterCalls,
			}, waiterClock.Now)
			waiter := startResourceStateRaceCall(group, ctx, waiterService,
				resources.Actor{AccountID: fixture.actors.coordinator},
				resources.StateRequest{ResourceID: fixture.resourceID, Active: test.requested})
			waiterPID := awaitBookingRacePID(t, ctx, waiterStarted, "waiting repeated desired-state transaction")
			waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
			wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, leaderPID)
			cancelWait()
			if err != nil {
				t.Fatalf("observe repeated desired-state waiter: %v", err)
			}
			logBookingRaceWait(t, "repeated desired-state waiter", wait)
			if got, events := resourceCapabilitySnapshot(t, fixture.admin, fixture.resourceID); got !=
				resourceStateConcurrencyResourceJSON(t, fixture, test.initialActive, fixture.createdAt, fixture.initialName, fixture.initialDescription) ||
				len(readResourceCapabilityEvents(t, fixture.admin, fixture.resourceID)) != 1 {
				t.Fatalf("leader's uncommitted state/event became visible before release: resource=%s events=%s", got, events)
			}

			t.Logf("leader pid=%d holds Resource; waiter pid=%d observed blocked edge; release leader gate and commit before waiter resumes", leaderPID, waiterPID)
			leaderGate.open()
			leaderOutcome := awaitResourceStateRaceCall(t, ctx, leader)
			waiterOutcome := awaitResourceStateRaceCall(t, ctx, waiter)
			if leaderOutcome.err != nil || !leaderOutcome.result.Changed || leaderOutcome.result.Resource.Active != test.requested {
				t.Fatalf("leading transition result=%+v error=%v", leaderOutcome.result, leaderOutcome.err)
			}
			if waiterOutcome.err != nil || waiterOutcome.result.Changed || waiterOutcome.result.Resource.Active != test.requested ||
				!waiterOutcome.result.Resource.UpdatedAt.Equal(operationAt) {
				t.Fatalf("waiting matching-state no-op result=%+v error=%v", waiterOutcome.result, waiterOutcome.err)
			}
			if leaderClock.calls.Load() != 1 || waiterClock.calls.Load() != 0 || leaderCalls.Load() != 1 || waiterCalls.Load() != 0 {
				t.Fatalf("clock/capability calls leader=%d/%d waiter=%d/%d; want 1/1 and 0/0",
					leaderClock.calls.Load(), leaderCalls.Load(), waiterClock.calls.Load(), waiterCalls.Load())
			}
			fixture.assertPersistedResource(t, test.requested, operationAt, fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t, resourceStateConcurrencyEvent{
				actorID: fixture.actors.coordinator, action: test.action, occurredAt: operationAt,
				before: test.initialActive, after: test.requested,
			})
			fixture.assertBookingsUnchanged(t)
		})
	}
}

func resourceStateConcurrencyResourceJSON(
	t *testing.T,
	fixture *resourceStateConcurrencyFixture,
	active bool,
	updatedAt time.Time,
	name, description string,
) string {
	t.Helper()
	var snapshot string
	if err := fixture.admin.QueryRow(t.Context(), `
		SELECT to_jsonb(resource)::text FROM public.resources AS resource
		WHERE resource.id = $1::uuid AND resource.active = $2 AND resource.updated_at = $3
		  AND resource.name = $4 AND resource.description = $5`,
		fixture.resourceID, active, updatedAt, name, description).Scan(&snapshot); err != nil {
		t.Fatalf("read Resource state snapshot for expected values: %v", err)
	}
	return snapshot
}

func TestResourceStateConcurrentOpposingDesiredStatesSerializeBothOrders(t *testing.T) {
	for _, test := range []struct {
		name          string
		initialActive bool
		leaderActive  bool
	}{
		{name: "Inactive to Active, then Inactive", initialActive: false, leaderActive: true},
		{name: "Inactive no-op, then Active", initialActive: false, leaderActive: false},
		{name: "Active to Inactive, then Active", initialActive: true, leaderActive: false},
		{name: "Active no-op, then Inactive", initialActive: true, leaderActive: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, test.initialActive)
			installResourceStateNoOpUpdateFailureTrigger(t, fixture.admin, fixture.resourceID)
			leaderActive, waiterActive := test.leaderActive, !test.leaderActive
			operationAt := fixture.operationAt
			leaderChanged := test.initialActive != leaderActive
			group := newBookingRaceGroup(t)
			leaderGate := group.newGate()
			leaderStarted := make(chan int32, 1)
			leaderCalls := &atomic.Int32{}
			leaderClock := newAtomicBookingClockAt(operationAt)
			leaderRepository := &resourceStateRaceRepository{
				base: NewStore(fixture.app), started: leaderStarted, stateCalls: leaderCalls,
			}
			if leaderChanged {
				leaderRepository.beforeCommit = leaderGate
			} else {
				leaderRepository.afterResourceLock = leaderGate
			}
			leaderService := resources.NewStateService(leaderRepository, leaderClock.Now)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			leader := startResourceStateRaceCall(group, ctx, leaderService,
				resources.Actor{AccountID: fixture.actors.coordinator},
				resources.StateRequest{ResourceID: fixture.resourceID, Active: leaderActive})
			leaderPID := awaitBookingRacePID(t, ctx, leaderStarted, "leading opposite desired-state transaction")
			gateDescription := "leader after state mutation and Activity"
			if !leaderChanged {
				gateDescription = "leader after Resource lock before no-op decision"
			}
			if gatePID := leaderGate.await(t, ctx, gateDescription); gatePID != leaderPID {
				t.Fatalf("leader gate backend PID=%d, transaction PID=%d", gatePID, leaderPID)
			}

			waiterGate := group.newGate()
			waiterStarted := make(chan int32, 1)
			waiterCalls := &atomic.Int32{}
			waiterClock := newAtomicBookingClockAt(operationAt)
			waiterService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: waiterStarted,
				beforeSet: waiterGate, stateCalls: waiterCalls,
			}, waiterClock.Now)
			waiter := startResourceStateRaceCall(group, ctx, waiterService,
				resources.Actor{AccountID: fixture.actors.coordinator},
				resources.StateRequest{ResourceID: fixture.resourceID, Active: waiterActive})
			waiterPID := awaitBookingRacePID(t, ctx, waiterStarted, "waiting opposite desired-state transaction")
			waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
			wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, leaderPID)
			cancelWait()
			if err != nil {
				t.Fatalf("observe opposite desired-state waiter: %v", err)
			}
			logBookingRaceWait(t, "opposite desired-state waiter", wait)

			t.Logf("leader pid=%d owns Resource lock; waiter pid=%d blocked; release leader, await its commit/no-op, then inspect waiter before its guarded transition", leaderPID, waiterPID)
			leaderGate.open()
			leaderOutcome := awaitResourceStateRaceCall(t, ctx, leader)
			waiterGatePID := waiterGate.await(t, ctx, "opposite waiter after lock and before guarded state transition")
			if waiterGatePID != waiterPID {
				t.Fatalf("waiter gate backend PID=%d, transaction PID=%d", waiterGatePID, waiterPID)
			}
			leaderUpdatedAt := fixture.createdAt
			if leaderChanged {
				leaderUpdatedAt = operationAt
			}
			fixture.assertPersistedResource(t, leaderActive, leaderUpdatedAt, fixture.initialName, fixture.initialDescription)
			if leaderChanged {
				fixture.assertHistory(t, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: resourceStateConcurrencyAction(leaderActive),
					occurredAt: operationAt, before: test.initialActive, after: leaderActive,
				})
			} else {
				fixture.assertHistory(t)
			}
			if leaderOutcome.err != nil || leaderOutcome.result.Changed != leaderChanged ||
				leaderOutcome.result.Resource.Active != leaderActive || !leaderOutcome.result.Resource.UpdatedAt.Equal(leaderUpdatedAt) {
				t.Fatalf("leading state outcome=%+v error=%v; changed=%v active=%v updated_at=%s",
					leaderOutcome.result, leaderOutcome.err, leaderChanged, leaderActive, leaderUpdatedAt)
			}

			waiterGate.open()
			waiterOutcome := awaitResourceStateRaceCall(t, ctx, waiter)
			if waiterOutcome.err != nil || !waiterOutcome.result.Changed || waiterOutcome.result.Resource.Active != waiterActive ||
				!waiterOutcome.result.Resource.UpdatedAt.Equal(operationAt) {
				t.Fatalf("waiting opposite state outcome=%+v error=%v", waiterOutcome.result, waiterOutcome.err)
			}
			wantLeaderCalls, wantLeaderClockCalls := int32(0), int64(0)
			if leaderChanged {
				wantLeaderCalls, wantLeaderClockCalls = 1, 1
			}
			if leaderCalls.Load() != wantLeaderCalls || leaderClock.calls.Load() != wantLeaderClockCalls ||
				waiterCalls.Load() != 1 || waiterClock.calls.Load() != 1 {
				t.Fatalf("state capability/clock calls leader=%d/%d waiter=%d/%d; want %d/%d and 1/1",
					leaderCalls.Load(), leaderClock.calls.Load(), waiterCalls.Load(), waiterClock.calls.Load(),
					wantLeaderCalls, wantLeaderClockCalls)
			}
			expectedEvents := make([]resourceStateConcurrencyEvent, 0, 2)
			if leaderChanged {
				expectedEvents = append(expectedEvents, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: resourceStateConcurrencyAction(leaderActive),
					occurredAt: operationAt, before: test.initialActive, after: leaderActive,
				})
			}
			expectedEvents = append(expectedEvents, resourceStateConcurrencyEvent{
				actorID: fixture.actors.coordinator, action: resourceStateConcurrencyAction(waiterActive),
				occurredAt: operationAt, before: leaderActive, after: waiterActive,
			})
			fixture.assertPersistedResource(t, waiterActive, operationAt, fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t, expectedEvents...)
			fixture.assertBookingsUnchanged(t)
		})
	}
}

func resourceStateConcurrencyAction(active bool) string {
	if active {
		return "resource.activated"
	}
	return "resource.deactivated"
}

func (fixture *resourceStateConcurrencyFixture) assertEditOnlyHistory(
	t *testing.T,
	editAt time.Time,
	beforeName, beforeDescription, afterName, afterDescription string,
) {
	t.Helper()
	events := readResourceCapabilityEvents(t, fixture.admin, fixture.resourceID)
	if len(events) != 2 {
		t.Fatalf("Resource Activity count=%d, want creation and descriptive edit before state waiter runs: %+v", len(events), events)
	}
	if created := resourceStateConcurrencyEventSnapshot(t, fixture.admin, fixture.resourceID, "resource.created"); created != fixture.createdEventSnapshot {
		t.Fatalf("descriptive edit changed original Resource creation history: before=%s after=%s", fixture.createdEventSnapshot, created)
	}
	editCount := 0
	for _, event := range events {
		if event.Action != "resource.descriptive_edited" {
			continue
		}
		editCount++
		before, beforeOK := event.Details["before"].(map[string]any)
		after, afterOK := event.Details["after"].(map[string]any)
		if event.ActorID != fixture.actors.coordinator || !event.BookingIsNil || !event.OccurredAt.Equal(editAt) ||
			!beforeOK || !afterOK || before["name"] != beforeName || before["description"] != beforeDescription ||
			after["name"] != afterName || after["description"] != afterDescription {
			t.Fatalf("persisted descriptive-edit Activity before state waiter=%+v", event)
		}
	}
	if editCount != 1 {
		t.Fatalf("Resource has %d descriptive-edit events before state waiter, want one", editCount)
	}
}

func (fixture *resourceStateConcurrencyFixture) assertEditAndStateHistory(
	t *testing.T,
	state resourceStateConcurrencyEvent,
	editAt time.Time,
	beforeName, beforeDescription, afterName, afterDescription string,
) {
	t.Helper()
	events := readResourceCapabilityEvents(t, fixture.admin, fixture.resourceID)
	if len(events) != 3 {
		t.Fatalf("Resource Activity count=%d, want creation, state, and descriptive-edit events: %+v", len(events), events)
	}
	if created := resourceStateConcurrencyEventSnapshot(t, fixture.admin, fixture.resourceID, "resource.created"); created != fixture.createdEventSnapshot {
		t.Fatalf("state/edit race changed original Resource creation history: before=%s after=%s",
			fixture.createdEventSnapshot, created)
	}
	var stateCount, editCount, createdCount int
	for _, event := range events {
		switch event.Action {
		case "resource.created":
			createdCount++
		case state.action:
			stateCount++
			before, beforeOK := event.Details["before"].(map[string]any)
			after, afterOK := event.Details["after"].(map[string]any)
			if event.ActorID != state.actorID || !event.BookingIsNil || !event.OccurredAt.Equal(state.occurredAt) ||
				!beforeOK || !afterOK || before["active"] != state.before || after["active"] != state.after {
				t.Fatalf("persisted %s Activity=%+v; want active %v -> %v at %s",
					event.Action, event, state.before, state.after, state.occurredAt)
			}
		case "resource.descriptive_edited":
			editCount++
			before, beforeOK := event.Details["before"].(map[string]any)
			after, afterOK := event.Details["after"].(map[string]any)
			if event.ActorID != fixture.actors.coordinator || !event.BookingIsNil || !event.OccurredAt.Equal(editAt) ||
				!beforeOK || !afterOK || before["name"] != beforeName || before["description"] != beforeDescription ||
				after["name"] != afterName || after["description"] != afterDescription {
				t.Fatalf("persisted descriptive-edit Activity=%+v; want name %q -> %q and description %q -> %q at %s",
					event, beforeName, afterName, beforeDescription, afterDescription, editAt)
			}
		default:
			t.Fatalf("unexpected Activity action in state/edit race: %+v", event)
		}
	}
	if createdCount != 1 || stateCount != 1 || editCount != 1 {
		t.Fatalf("Resource event counts creation/state/edit=%d/%d/%d; want one each", createdCount, stateCount, editCount)
	}
}

type resourceEditRaceExpectation struct {
	actorID    string
	occurredAt time.Time
	before     resources.Resource
	after      resources.Resource
}

type resourceEditRaceRepository struct {
	resources.Repository
	started      chan<- int32
	beforeCommit *bookingRaceGate
}

func (repository *resourceEditRaceRepository) BeginResourceEdit(ctx context.Context) (resources.EditTransaction, error) {
	transaction, err := repository.Repository.BeginResourceEdit(ctx)
	if err != nil {
		return nil, err
	}
	concrete, ok := transaction.(*resourceCreationTransaction)
	if !ok {
		_ = transaction.Rollback(context.Background())
		return nil, fmt.Errorf("unexpected Resource edit transaction type %T", transaction)
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
	return &resourceEditRaceTransaction{
		EditTransaction: transaction, raw: concrete, pid: pid, beforeCommit: repository.beforeCommit,
	}, nil
}

type resourceEditRaceTransaction struct {
	resources.EditTransaction
	raw          *resourceCreationTransaction
	pid          int32
	before       resources.Resource
	hasBefore    bool
	beforeCommit *bookingRaceGate
	expected     *resourceEditRaceExpectation
}

func (tx *resourceEditRaceTransaction) LockResource(ctx context.Context, resourceID string) (resources.Resource, bool, error) {
	resource, found, err := tx.EditTransaction.LockResource(ctx, resourceID)
	if err == nil && found {
		tx.before = resource
		tx.hasBefore = true
	}
	return resource, found, err
}

func (tx *resourceEditRaceTransaction) EditResource(
	ctx context.Context,
	actorID, resourceID, name, description string,
	occurredAt time.Time,
) (resources.Resource, bool, error) {
	resource, changed, err := tx.EditTransaction.EditResource(ctx, actorID, resourceID, name, description, occurredAt)
	if err == nil && changed {
		if !tx.hasBefore {
			return resources.Resource{}, false, fmt.Errorf("descriptive edit reached guarded mutation without locking the Resource")
		}
		tx.expected = &resourceEditRaceExpectation{
			actorID: actorID, occurredAt: occurredAt, before: tx.before, after: resource,
		}
	}
	return resource, changed, err
}

func (tx *resourceEditRaceTransaction) Commit(ctx context.Context) error {
	if tx.beforeCommit != nil {
		if err := tx.assertPendingEdit(ctx); err != nil {
			return err
		}
		if err := tx.beforeCommit.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.EditTransaction.Commit(ctx)
}

func (tx *resourceEditRaceTransaction) assertPendingEdit(ctx context.Context) error {
	if tx.expected == nil {
		return fmt.Errorf("edit transaction reached commit barrier without a changed descriptive edit")
	}
	want := tx.expected
	var id, code, name, description string
	var active bool
	var createdAt, updatedAt time.Time
	if err := tx.raw.tx.QueryRow(ctx, `
		SELECT id::text, code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, want.after.ID).Scan(
		&id, &code, &name, &description, &active, &createdAt, &updatedAt); err != nil {
		return fmt.Errorf("inspect uncommitted Resource descriptive edit: %w", err)
	}
	var eventCount int
	var beforeName, beforeDescription, afterName, afterDescription string
	if err := tx.raw.tx.QueryRow(ctx, `
		SELECT count(*),
		       COALESCE(max(event.details->'before'->>'name'), ''),
		       COALESCE(max(event.details->'before'->>'description'), ''),
		       COALESCE(max(event.details->'after'->>'name'), ''),
		       COALESCE(max(event.details->'after'->>'description'), '')
		FROM public.activity_events AS event
		WHERE event.resource_id = $1::uuid AND event.actor_account_id = $2::uuid
		  AND event.action = 'resource.descriptive_edited' AND event.occurred_at = $3::timestamptz
		  AND event.booking_id IS NULL`, want.after.ID, want.actorID, want.occurredAt).Scan(
		&eventCount, &beforeName, &beforeDescription, &afterName, &afterDescription); err != nil {
		return fmt.Errorf("inspect uncommitted descriptive-edit Activity: %w", err)
	}
	if id != want.after.ID || code != want.before.Code || name != want.after.Name || description != want.after.Description ||
		active != want.before.Active || !createdAt.Equal(want.before.CreatedAt) || !updatedAt.Equal(want.occurredAt) || eventCount != 1 ||
		beforeName != want.before.Name || beforeDescription != want.before.Description ||
		afterName != want.after.Name || afterDescription != want.after.Description {
		return fmt.Errorf("uncommitted Resource/edit-event pair is incomplete: resource=%s/%s/%q/%q/%v created=%s updated=%s event_count=%d before=%q/%q after=%q/%q",
			id, code, name, description, active, createdAt, updatedAt, eventCount,
			beforeName, beforeDescription, afterName, afterDescription)
	}
	return nil
}

type resourceEditRaceOutcome struct {
	result resources.EditResult
	err    error
}

type resourceEditRaceCall struct {
	result chan resourceEditRaceOutcome
	done   chan struct{}
}

func startResourceEditRaceCall(
	group *bookingRaceGroup,
	ctx context.Context,
	service *resources.Service,
	actor resources.Actor,
	request resources.EditRequest,
) *resourceEditRaceCall {
	call := &resourceEditRaceCall{result: make(chan resourceEditRaceOutcome, 1), done: make(chan struct{})}
	group.track(call.done)
	go func() {
		defer close(call.done)
		result, err := service.Edit(ctx, actor, request)
		call.result <- resourceEditRaceOutcome{result: result, err: err}
	}()
	return call
}

func awaitResourceEditRaceCall(t *testing.T, ctx context.Context, call *resourceEditRaceCall) resourceEditRaceOutcome {
	t.Helper()
	select {
	case <-call.done:
		return <-call.result
	case <-ctx.Done():
		t.Fatalf("Resource descriptive edit did not finish: %v", ctx.Err())
		return resourceEditRaceOutcome{}
	}
}

func TestResourceStateAndDescriptiveEditSerializeBothOrders(t *testing.T) {
	for _, editLeads := range []bool{true, false} {
		name := "descriptive edit then state transition"
		if !editLeads {
			name = "state transition then descriptive edit"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, true)
			oldName, oldDescription := fixture.initialName, fixture.initialDescription
			newName, newDescription := "Edited concurrent lab", "Edited  descriptive text"
			editAt, stateAt := fixture.operationAt, fixture.operationAt.Add(time.Minute)
			if !editLeads {
				stateAt, editAt = fixture.operationAt, fixture.operationAt.Add(time.Minute)
			}
			group := newBookingRaceGroup(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			editGate := group.newGate()
			editStarted := make(chan int32, 1)
			editClock := newAtomicBookingClockAt(editAt)
			editService := resources.NewService(&resourceEditRaceRepository{
				Repository: NewStore(fixture.app), started: editStarted, beforeCommit: editGate,
			}, editClock.Now)
			actor := resources.Actor{AccountID: fixture.actors.coordinator}
			editCall := func() *resourceEditRaceCall {
				return startResourceEditRaceCall(group, ctx, editService, actor, resources.EditRequest{
					ResourceID: fixture.resourceID, Code: fixture.code,
					Name: "  " + newName + "  ", Description: "\t" + newDescription + " \n",
				})
			}
			stateGate := group.newGate()
			stateStarted := make(chan int32, 1)
			stateCalls := &atomic.Int32{}
			stateClock := newAtomicBookingClockAt(stateAt)
			stateService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: stateStarted,
				beforeCommit: stateGate, stateCalls: stateCalls,
			}, stateClock.Now)
			observedStateCalls := stateCalls
			observedStateClock := stateClock
			stateCall := func() *resourceStateRaceCall {
				return startResourceStateRaceCall(group, ctx, stateService, actor,
					resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
			}

			var editOutcome resourceEditRaceOutcome
			var stateOutcome resourceStateRaceOutcome
			var editPID, statePID int32
			if editLeads {
				edit := editCall()
				editPID = awaitBookingRacePID(t, ctx, editStarted, "leading descriptive-edit transaction")
				if gatePID := editGate.await(t, ctx, "edit after Resource and Activity mutation"); gatePID != editPID {
					t.Fatalf("edit gate backend PID=%d, transaction PID=%d", gatePID, editPID)
				}
				stateWaiterGate := group.newGate()
				stateWaiterStarted := make(chan int32, 1)
				stateWaiterCalls := &atomic.Int32{}
				stateWaiterClock := newAtomicBookingClockAt(stateAt)
				stateWaiterService := resources.NewStateService(&resourceStateRaceRepository{
					base: NewStore(fixture.app), started: stateWaiterStarted,
					beforeSet: stateWaiterGate, stateCalls: stateWaiterCalls,
				}, stateWaiterClock.Now)
				stateWaiter := startResourceStateRaceCall(group, ctx, stateWaiterService, actor,
					resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
				observedStateCalls = stateWaiterCalls
				observedStateClock = stateWaiterClock
				statePID = awaitBookingRacePID(t, ctx, stateWaiterStarted, "state waiter behind descriptive edit")
				waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
				wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, statePID, editPID)
				cancelWait()
				if err != nil {
					t.Fatalf("observe state waiter blocked behind edit: %v", err)
				}
				logBookingRaceWait(t, "state waiter behind descriptive edit", wait)
				t.Logf("edit pid=%d holds Resource lock after pending edit pair; state pid=%d is blocked; release edit and commit before state resumes", editPID, statePID)
				editGate.open()
				editOutcome = awaitResourceEditRaceCall(t, ctx, edit)
				if gatePID := stateWaiterGate.await(t, ctx, "state waiter after edit commit and before transition"); gatePID != statePID {
					t.Fatalf("state waiter gate PID=%d, transaction PID=%d", gatePID, statePID)
				}
				fixture.assertPersistedResource(t, true, editAt, newName, newDescription)
				fixture.assertEditOnlyHistory(t, editAt, oldName, oldDescription, newName, newDescription)
				stateWaiterGate.open()
				stateOutcome = awaitResourceStateRaceCall(t, ctx, stateWaiter)
				if stateWaiterCalls.Load() != 1 || stateWaiterClock.calls.Load() != 1 {
					t.Fatalf("state waiter capability/clock calls=%d/%d, want one each", stateWaiterCalls.Load(), stateWaiterClock.calls.Load())
				}
			} else {
				state := stateCall()
				statePID = awaitBookingRacePID(t, ctx, stateStarted, "leading state transaction")
				if gatePID := stateGate.await(t, ctx, "state after Resource and Activity mutation"); gatePID != statePID {
					t.Fatalf("state gate backend PID=%d, transaction PID=%d", gatePID, statePID)
				}
				editWaiterStarted := make(chan int32, 1)
				editWaiterGate := group.newGate()
				editWaiterClock := newAtomicBookingClockAt(editAt)
				editWaiterService := resources.NewService(&resourceEditRaceRepository{
					Repository: NewStore(fixture.app), started: editWaiterStarted, beforeCommit: editWaiterGate,
				}, editWaiterClock.Now)
				editWaiter := startResourceEditRaceCall(group, ctx, editWaiterService, actor, resources.EditRequest{
					ResourceID: fixture.resourceID, Code: fixture.code,
					Name: "  " + newName + "  ", Description: "\t" + newDescription + " \n",
				})
				editPID = awaitBookingRacePID(t, ctx, editWaiterStarted, "edit waiter behind state transition")
				waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
				wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, editPID, statePID)
				cancelWait()
				if err != nil {
					t.Fatalf("observe descriptive-edit waiter blocked behind state: %v", err)
				}
				logBookingRaceWait(t, "descriptive-edit waiter behind state", wait)
				t.Logf("state pid=%d holds Resource lock after pending state/event pair; edit pid=%d is blocked; release state and commit before edit resumes", statePID, editPID)
				stateGate.open()
				stateOutcome = awaitResourceStateRaceCall(t, ctx, state)
				if gatePID := editWaiterGate.await(t, ctx, "edit waiter after state commit with pending edit pair"); gatePID != editPID {
					t.Fatalf("edit waiter gate PID=%d, transaction PID=%d", gatePID, editPID)
				}
				fixture.assertPersistedResource(t, false, stateAt, oldName, oldDescription)
				fixture.assertHistory(t, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: "resource.deactivated", occurredAt: stateAt,
					before: true, after: false,
				})
				editWaiterGate.open()
				editOutcome = awaitResourceEditRaceCall(t, ctx, editWaiter)
				if editWaiterClock.calls.Load() != 1 {
					t.Fatalf("edit waiter clock calls=%d, want one after acquiring Resource lock", editWaiterClock.calls.Load())
				}
			}

			editActive := editLeads
			if editOutcome.err != nil || !editOutcome.result.Changed || editOutcome.result.Resource.Name != newName ||
				editOutcome.result.Resource.Description != newDescription || editOutcome.result.Resource.Active != editActive ||
				!editOutcome.result.Resource.UpdatedAt.Equal(editAt) {
				t.Fatalf("descriptive-edit outcome=%+v error=%v", editOutcome.result, editOutcome.err)
			}
			if stateOutcome.err != nil || !stateOutcome.result.Changed || stateOutcome.result.Resource.Active ||
				!stateOutcome.result.Resource.UpdatedAt.Equal(stateAt) || observedStateCalls.Load() != 1 || observedStateClock.calls.Load() != 1 {
				t.Fatalf("state outcome=%+v error=%v state capability/clock calls=%d/%d",
					stateOutcome.result, stateOutcome.err, observedStateCalls.Load(), observedStateClock.calls.Load())
			}
			fixture.assertPersistedResource(t, false, func() time.Time {
				if editLeads {
					return stateAt
				}
				return editAt
			}(), newName, newDescription)
			fixture.assertEditAndStateHistory(t, resourceStateConcurrencyEvent{
				actorID: fixture.actors.coordinator, action: "resource.deactivated", occurredAt: stateAt,
				before: true, after: false,
			}, editAt, oldName, oldDescription, newName, newDescription)
			fixture.assertBookingsUnchanged(t)
		})
	}
}

func TestResourceStateFailureRollbacksWakeWaitersForFreshStateAndTime(t *testing.T) {
	for _, failure := range []struct {
		name    string
		install func(*testing.T, *resourceStateConcurrencyFixture, time.Time) func()
	}{
		{
			name: "Resource mutation rejected",
			install: func(t *testing.T, fixture *resourceStateConcurrencyFixture, at time.Time) func() {
				return installResourceStateMutationFailureAt(t, fixture.admin, fixture.resourceID, at)
			},
		},
		{
			name: "Activity append rejected after uncommitted Resource mutation",
			install: func(t *testing.T, fixture *resourceStateConcurrencyFixture, at time.Time) func() {
				return installResourceStateActivityFailureAt(t, fixture.admin, fixture.actors.coordinator, fixture.resourceID, at)
			},
		},
	} {
		t.Run(failure.name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, true)
			leaderAt, waiterAt := fixture.operationAt, fixture.operationAt.Add(time.Minute)
			removeFailure := failure.install(t, fixture, leaderAt)
			group := newBookingRaceGroup(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			leaderGate := group.newGate()
			leaderStarted := make(chan int32, 1)
			leaderCalls := &atomic.Int32{}
			leaderClock := newAtomicBookingClockAt(leaderAt)
			leaderService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: leaderStarted,
				beforeSet: leaderGate, stateCalls: leaderCalls,
			}, leaderClock.Now)
			actor := resources.Actor{AccountID: fixture.actors.coordinator}
			leader := startResourceStateRaceCall(group, ctx, leaderService, actor,
				resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
			leaderPID := awaitBookingRacePID(t, ctx, leaderStarted, "leading failure-injected state transaction")
			if gatePID := leaderGate.await(t, ctx, "leader after Resource lock and eligibility check"); gatePID != leaderPID {
				t.Fatalf("failure-injected leader gate PID=%d, transaction PID=%d", gatePID, leaderPID)
			}

			waiterGate := group.newGate()
			waiterStarted := make(chan int32, 1)
			waiterCalls := &atomic.Int32{}
			waiterClock := newAtomicBookingClockAt(waiterAt)
			waiterService := resources.NewStateService(&resourceStateRaceRepository{
				base: NewStore(fixture.app), started: waiterStarted,
				beforeSet: waiterGate, stateCalls: waiterCalls,
			}, waiterClock.Now)
			waiter := startResourceStateRaceCall(group, ctx, waiterService, actor,
				resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
			waiterPID := awaitBookingRacePID(t, ctx, waiterStarted, "state waiter behind failing transaction")
			waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
			wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, leaderPID)
			cancelWait()
			if err != nil {
				t.Fatalf("observe waiter blocked behind failure-injected state transaction: %v", err)
			}
			logBookingRaceWait(t, "waiter behind failure-injected state transaction", wait)

			t.Logf("leader pid=%d owns Resource lock; waiter pid=%d is blocked; release leader into injected failure, require rollback, then pause waiter after it re-reads committed state and samples its own clock", leaderPID, waiterPID)
			leaderGate.open()
			leaderOutcome := awaitResourceStateRaceCall(t, ctx, leader)
			if !errors.Is(leaderOutcome.err, resources.ErrOperational) || strings.Contains(leaderOutcome.err.Error(), "injected Resource state") {
				t.Fatalf("injected failure result error=%v; want safe operational failure without trigger diagnostics", leaderOutcome.err)
			}
			if gatePID := waiterGate.await(t, ctx, "waiter after rollback, fresh state check, and own timestamp"); gatePID != waiterPID {
				t.Fatalf("rollback waiter gate PID=%d, transaction PID=%d", gatePID, waiterPID)
			}
			if leaderCalls.Load() != 1 || leaderClock.calls.Load() != 1 || waiterCalls.Load() != 1 || waiterClock.calls.Load() != 1 {
				t.Fatalf("capability/clock calls leader=%d/%d waiter=%d/%d; want one each after transition is required",
					leaderCalls.Load(), leaderClock.calls.Load(), waiterCalls.Load(), waiterClock.calls.Load())
			}
			fixture.assertPersistedResource(t, true, fixture.createdAt, fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t)
			fixture.assertBookingsUnchanged(t)

			waiterGate.open()
			waiterOutcome := awaitResourceStateRaceCall(t, ctx, waiter)
			removeFailure()
			if waiterOutcome.err != nil || !waiterOutcome.result.Changed || waiterOutcome.result.Resource.Active ||
				!waiterOutcome.result.Resource.UpdatedAt.Equal(waiterAt) {
				t.Fatalf("state waiter after rollback result=%+v error=%v", waiterOutcome.result, waiterOutcome.err)
			}
			fixture.assertPersistedResource(t, false, waiterAt, fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t, resourceStateConcurrencyEvent{
				actorID: fixture.actors.coordinator, action: "resource.deactivated", occurredAt: waiterAt,
				before: true, after: false,
			})
			fixture.assertBookingsUnchanged(t)
		})
	}
}

func TestResourceStateConcurrentWaiterUsesFreshPostLockTime(t *testing.T) {
	fixture := newResourceStateConcurrencyFixture(t, false)
	startAt := fixture.operationAt.Add(2 * time.Hour)
	endAt := fixture.operationAt.Add(3 * time.Hour)
	bookingID := insertResourceStateBooking(t, fixture.admin, fixture.resourceID, fixture.actors.engineer,
		startAt, endAt, fixture.createdAt, "confirmed", "")
	*fixture.bookingIDs = append(*fixture.bookingIDs, bookingID)
	insertResourceStateBookingHistory(t, fixture.admin, bookingID, fixture.actors.engineer, "confirmed", fixture.createdAt)
	fixture.bookingSnapshots[bookingID] = resourceStateBookingAndHistorySnapshot(t, fixture.admin, bookingID)
	installResourceStateNoOpUpdateFailureTrigger(t, fixture.admin, fixture.resourceID)

	group := newBookingRaceGroup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	leaderGate := group.newGate()
	leaderStarted := make(chan int32, 1)
	leaderCalls, leaderBlockingChecks := &atomic.Int32{}, &atomic.Int32{}
	leaderBlockingTime := &atomic.Int64{}
	leaderClock := newAtomicBookingClockAt(fixture.operationAt)
	leaderService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(fixture.app), started: leaderStarted, beforeCommit: leaderGate,
		stateCalls: leaderCalls, blockingChecks: leaderBlockingChecks, blockingTimeMicros: leaderBlockingTime,
	}, leaderClock.Now)
	actor := resources.Actor{AccountID: fixture.actors.coordinator}
	leader := startResourceStateRaceCall(group, ctx, leaderService, actor,
		resources.StateRequest{ResourceID: fixture.resourceID, Active: true})
	leaderPID := awaitBookingRacePID(t, ctx, leaderStarted, "activation leader before time-sensitive waiter")
	if gatePID := leaderGate.await(t, ctx, "activation leader after mutation and Activity"); gatePID != leaderPID {
		t.Fatalf("activation gate backend PID=%d, transaction PID=%d", gatePID, leaderPID)
	}

	waiterGate := group.newGate()
	waiterStarted := make(chan int32, 1)
	waiterCalls, waiterBlockingChecks := &atomic.Int32{}, &atomic.Int32{}
	waiterBlockingTime := &atomic.Int64{}
	waiterClock := newAtomicBookingClockAt(endAt.Add(-time.Microsecond))
	waiterService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(fixture.app), started: waiterStarted, beforeSet: waiterGate,
		stateCalls: waiterCalls, blockingChecks: waiterBlockingChecks, blockingTimeMicros: waiterBlockingTime,
	}, waiterClock.Now)
	waiter := startResourceStateRaceCall(group, ctx, waiterService, actor,
		resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
	waiterPID := awaitBookingRacePID(t, ctx, waiterStarted, "deactivation waiter before Booking end")
	waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, leaderPID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe time-sensitive waiter blocked behind activation: %v", err)
	}
	logBookingRaceWait(t, "deactivation waiter while Booking still ends in future", wait)
	if waiterClock.calls.Load() != 0 || waiterBlockingChecks.Load() != 0 {
		t.Fatalf("blocked waiter sampled time or checked occupancy early: clock=%d occupancy=%d",
			waiterClock.calls.Load(), waiterBlockingChecks.Load())
	}

	// Exact end is no longer blocking under end_at > operation_time; this value
	// is installed only after PostgreSQL proves the waiter is blocked.
	waiterClock.Set(endAt)
	t.Logf("advance waiter clock to exact Booking end %s while pid=%d is blocked; release activation pid=%d before the waiter rechecks occupancy", endAt, waiterPID, leaderPID)
	leaderGate.open()
	leaderOutcome := awaitResourceStateRaceCall(t, ctx, leader)
	if leaderOutcome.err != nil || !leaderOutcome.result.Changed || !leaderOutcome.result.Resource.Active {
		t.Fatalf("activation leader result=%+v error=%v", leaderOutcome.result, leaderOutcome.err)
	}
	if gatePID := waiterGate.await(t, ctx, "deactivation waiter after fresh-time occupancy query"); gatePID != waiterPID {
		t.Fatalf("deactivation waiter gate backend PID=%d, transaction PID=%d", gatePID, waiterPID)
	}
	if leaderCalls.Load() != 1 || leaderClock.calls.Load() != 1 || leaderBlockingChecks.Load() != 0 ||
		waiterCalls.Load() != 1 || waiterClock.calls.Load() != 1 || waiterBlockingChecks.Load() != 1 ||
		waiterBlockingTime.Load() != endAt.UnixMicro() {
		t.Fatalf("leader calls state/clock/occupancy=%d/%d/%d waiter=%d/%d/%d at=%d; want 1/1/0 and 1/1/1 at exact end %d",
			leaderCalls.Load(), leaderClock.calls.Load(), leaderBlockingChecks.Load(),
			waiterCalls.Load(), waiterClock.calls.Load(), waiterBlockingChecks.Load(), waiterBlockingTime.Load(), endAt.UnixMicro())
	}
	fixture.assertPersistedResource(t, true, fixture.operationAt, fixture.initialName, fixture.initialDescription)
	fixture.assertHistory(t, resourceStateConcurrencyEvent{
		actorID: fixture.actors.coordinator, action: "resource.activated", occurredAt: fixture.operationAt,
		before: false, after: true,
	})
	fixture.assertBookingsUnchanged(t)

	waiterGate.open()
	waiterOutcome := awaitResourceStateRaceCall(t, ctx, waiter)
	if waiterOutcome.err != nil || !waiterOutcome.result.Changed || waiterOutcome.result.Resource.Active ||
		!waiterOutcome.result.Resource.UpdatedAt.Equal(endAt) {
		t.Fatalf("deactivation waiter using fresh exact-end time result=%+v error=%v", waiterOutcome.result, waiterOutcome.err)
	}
	fixture.assertPersistedResource(t, false, endAt, fixture.initialName, fixture.initialDescription)
	fixture.assertHistory(t,
		resourceStateConcurrencyEvent{
			actorID: fixture.actors.coordinator, action: "resource.activated", occurredAt: fixture.operationAt,
			before: false, after: true,
		},
		resourceStateConcurrencyEvent{
			actorID: fixture.actors.coordinator, action: "resource.deactivated", occurredAt: endAt,
			before: true, after: false,
		},
	)
	fixture.assertBookingsUnchanged(t)
}
