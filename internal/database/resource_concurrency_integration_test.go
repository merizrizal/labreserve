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

type resourceOperationClock struct {
	micros atomic.Int64
	calls  atomic.Int64
}

func (clock *resourceOperationClock) Set(value time.Time) {
	clock.micros.Store(value.UTC().Truncate(time.Microsecond).UnixMicro())
}

func (clock *resourceOperationClock) Now() time.Time {
	clock.calls.Add(1)
	return time.UnixMicro(clock.micros.Add(1)).UTC()
}

type resourceCreateOutcome struct {
	result resources.CreateResult
	err    error
}

type resourceEditOutcome struct {
	result resources.EditResult
	err    error
}

func startResourceCreate(ctx context.Context, service *resources.Service, actorID string, request resources.CreateRequest) <-chan resourceCreateOutcome {
	result := make(chan resourceCreateOutcome, 1)
	go func() {
		created, err := service.Create(ctx, resources.Actor{AccountID: actorID}, request)
		result <- resourceCreateOutcome{result: created, err: err}
	}()
	return result
}

func receiveResourceCreate(t *testing.T, result <-chan resourceCreateOutcome) resourceCreateOutcome {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("Resource creation did not finish")
		return resourceCreateOutcome{}
	}
}

func startResourceEdit(ctx context.Context, service *resources.Service, actorID string, request resources.EditRequest) <-chan resourceEditOutcome {
	result := make(chan resourceEditOutcome, 1)
	go func() {
		edited, err := service.Edit(ctx, resources.Actor{AccountID: actorID}, request)
		result <- resourceEditOutcome{result: edited, err: err}
	}()
	return result
}

func receiveResourceEdit(t *testing.T, result <-chan resourceEditOutcome) resourceEditOutcome {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("Resource edit did not finish")
		return resourceEditOutcome{}
	}
}

func holdResourceActivityTableLock(t *testing.T, pool *pgxpool.Pool) *heldBookingLock {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire Activity lock-test connection: %v", err)
	}
	tx, err := conn.Begin(t.Context())
	if err != nil {
		conn.Release()
		t.Fatalf("begin Activity lock-test transaction: %v", err)
	}
	var pid int32
	if err := tx.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		_ = tx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("read Activity lock-test backend PID: %v", err)
	}
	if _, err := tx.Exec(t.Context(), "LOCK TABLE public.activity_events IN ACCESS EXCLUSIVE MODE"); err != nil {
		_ = tx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("hold Activity insertion lock: %v", err)
	}
	held := &heldBookingLock{conn: conn, tx: tx, pid: pid}
	t.Cleanup(func() {
		if !held.released {
			_ = held.release()
		}
	})
	return held
}

func addResourceCoordinatorFixture(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := insertResourceCapabilityAccount(t, pool, "coordinator")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM public.activity_events WHERE actor_account_id = $1::uuid", id); err != nil {
			t.Errorf("delete second Coordinator Activity fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM public.accounts WHERE id = $1::uuid", id); err != nil {
			t.Errorf("delete second Coordinator fixture: %v", err)
		}
	})
	return id
}

func TestConcurrentResourceCreationUsesCanonicalCodeUniqueness(t *testing.T) {
	for _, variant := range []string{"identical", "case variant", "Unicode whitespace variant"} {
		t.Run(variant, func(t *testing.T) {
			runConcurrentResourceCreation(t, variant, false)
		})
	}
}

func TestResourceCreationActivityFailureReleasesCanonicalCodeForWaiter(t *testing.T) {
	runConcurrentResourceCreation(t, "case variant", true)
}

func runConcurrentResourceCreation(t *testing.T, variant string, failFirst bool) {
	t.Helper()
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	secondCoordinator := addResourceCoordinatorFixture(t, adminPool)
	suffix := strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	canonicalCode := "Concurrent-" + suffix
	firstCode, secondCode := canonicalCode, canonicalCode
	switch variant {
	case "case variant":
		secondCode = strings.ToLower(canonicalCode)
	case "Unicode whitespace variant":
		firstCode = "\u2003" + canonicalCode + "\t"
		secondCode = "\u00a0" + canonicalCode + "\u3000"
	}
	clock := &resourceOperationClock{}
	clock.Set(time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC))
	service := resources.NewService(NewStore(appPool), clock.Now)
	firstRequest := resources.CreateRequest{Code: firstCode, Name: "First contender", Description: "first", Active: false}
	secondRequest := resources.CreateRequest{Code: secondCode, Name: "Second contender", Description: "second", Active: true}
	if failFirst {
		installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.created")
	}

	held := holdResourceActivityTableLock(t, adminPool)
	first := startResourceCreate(t.Context(), service, fixtures.coordinator, firstRequest)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("first Resource creation did not wait at its Activity append: %v", err)
	}
	cancel()
	second := startResourceCreate(t.Context(), service, secondCoordinator, secondRequest)
	waitCtx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("canonical duplicate did not wait on the uncommitted unique key: %v", err)
	}
	cancel()
	if err := held.release(); err != nil {
		t.Fatalf("release Activity insertion lock: %v", err)
	}

	firstResult := receiveResourceCreate(t, first)
	secondResult := receiveResourceCreate(t, second)
	var winner resourceCreateOutcome
	var winnerActor string
	if failFirst {
		if !errors.Is(firstResult.err, resources.ErrOperational) || secondResult.err != nil {
			t.Fatalf("failed first contender/waiter outcomes = (%+v, %v), (%+v, %v)",
				firstResult.result, firstResult.err, secondResult.result, secondResult.err)
		}
		winner, winnerActor = secondResult, secondCoordinator
	} else {
		if firstResult.err != nil || !errors.Is(secondResult.err, resources.ErrDuplicateCode) {
			t.Fatalf("canonical duplicate outcomes = (%+v, %v), (%+v, %v)",
				firstResult.result, firstResult.err, secondResult.result, secondResult.err)
		}
		winner, winnerActor = firstResult, fixtures.coordinator
	}
	fixtures.trackResource(winner.result.Resource.ID)

	var resourceCount, eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*),
		       (SELECT count(*) FROM public.activity_events AS event
		        JOIN public.resources AS resource ON resource.id = event.resource_id
		        WHERE lower(resource.code) = lower($1) AND event.action = 'resource.created')
		FROM public.resources WHERE lower(code) = lower($1)`, canonicalCode).Scan(&resourceCount, &eventCount); err != nil {
		t.Fatalf("inspect canonical Resource race: %v", err)
	}
	if resourceCount != 1 || eventCount != 1 {
		t.Fatalf("canonical creation race committed resources/events %d/%d, want exactly one each", resourceCount, eventCount)
	}
	persisted, found, err := NewStore(appPool).ResourceByID(t.Context(), winner.result.Resource.ID)
	if err != nil || !found {
		t.Fatalf("read winning Resource: found=%v error=%v", found, err)
	}
	wantCode := strings.TrimSpace(firstCode)
	wantName, wantDescription, wantActive := firstRequest.Name, firstRequest.Description, firstRequest.Active
	if failFirst {
		wantCode = strings.TrimSpace(secondCode)
		wantName, wantDescription, wantActive = secondRequest.Name, secondRequest.Description, secondRequest.Active
	}
	if persisted.Code != wantCode || persisted.Name != wantName || persisted.Description != wantDescription || persisted.Active != wantActive {
		t.Fatalf("winning persisted Resource = %+v, want canonical code and winning request values", persisted)
	}
	var persistedCreatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT created_at FROM public.resources WHERE id = $1::uuid`, winner.result.Resource.ID).Scan(&persistedCreatedAt); err != nil {
		t.Fatalf("read persisted Resource creation time: %v", err)
	}
	events := readResourceCapabilityEvents(t, adminPool, winner.result.Resource.ID)
	if len(events) != 1 || events[0].ActorID != winnerActor || events[0].Action != "resource.created" ||
		!events[0].BookingIsNil || events[0].Details["code"] != wantCode ||
		events[0].Details["name"] != wantName || events[0].Details["description"] != wantDescription ||
		events[0].Details["active"] != wantActive || !events[0].OccurredAt.Equal(persistedCreatedAt) {
		t.Fatalf("canonical creation event = %+v, want only the winning Resource's event", events)
	}
	for _, actorID := range []string{fixtures.coordinator, secondCoordinator} {
		wantActorEvents := 0
		if actorID == winnerActor {
			wantActorEvents = 1
		}
		var actorEvents int
		if err := adminPool.QueryRow(t.Context(), `
			SELECT count(*) FROM public.activity_events WHERE actor_account_id = $1::uuid AND action = 'resource.created'`, actorID).Scan(&actorEvents); err != nil {
			t.Fatalf("inspect rejected contender Activity events: %v", err)
		}
		if actorEvents != wantActorEvents {
			t.Fatalf("actor %s has %d creation events, want %d", actorID, actorEvents, wantActorEvents)
		}
	}
}

func TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	clock := &resourceOperationClock{}
	base := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	clock.Set(base)
	service := resources.NewService(NewStore(appPool), clock.Now)
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Concurrent-edit-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Original Lab", Description: "Original description", Active: false,
	})
	if err != nil {
		t.Fatalf("create concurrent edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)

	operationBase := base.Add(time.Hour)
	clock.Set(operationBase)
	callsBeforeWait := clock.calls.Load()
	held := holdBookingResourceLock(t, adminPool, created.Resource.ID)
	first := startResourceEdit(t.Context(), service, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "First edit", Description: "first description",
	})
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("first edit did not wait for the Resource lock: %v", err)
	}
	cancel()
	second := startResourceEdit(t.Context(), service, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "Second edit", Description: "second description",
	})
	waitCtx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("both edits did not wait on the same Resource lock: %v", err)
	}
	cancel()
	if clock.calls.Load() != callsBeforeWait {
		_ = held.release()
		t.Fatalf("edit clock was read before the Resource lock: calls before=%d while waiting=%d", callsBeforeWait, clock.calls.Load())
	}
	if err := held.release(); err != nil {
		t.Fatalf("release concurrent edit Resource lock: %v", err)
	}
	firstResult, secondResult := receiveResourceEdit(t, first), receiveResourceEdit(t, second)
	if firstResult.err != nil || secondResult.err != nil || !firstResult.result.Changed || !secondResult.result.Changed {
		t.Fatalf("different concurrent edit outcomes = (%+v, %v), (%+v, %v)",
			firstResult.result, firstResult.err, secondResult.result, secondResult.err)
	}
	if clock.calls.Load() != callsBeforeWait+2 {
		t.Fatalf("post-lock edit clock calls = %d, want two", clock.calls.Load()-callsBeforeWait)
	}

	firstAt, secondAt := operationBase.Add(time.Microsecond), operationBase.Add(2*time.Microsecond)
	events := readResourceCapabilityEvents(t, adminPool, created.Resource.ID)
	if len(events) != 3 || events[1].Action != "resource.descriptive_edited" || events[2].Action != "resource.descriptive_edited" ||
		!events[1].OccurredAt.Equal(firstAt) || !events[2].OccurredAt.Equal(secondAt) {
		t.Fatalf("serialized edit events = %+v, want creation and ordered post-lock timestamps %s/%s", events, firstAt, secondAt)
	}
	initialValues := map[string]any{"name": "Original Lab", "description": "Original description"}
	firstAfter := events[1].Details["after"].(map[string]any)
	secondBefore := events[2].Details["before"].(map[string]any)
	secondAfter := events[2].Details["after"].(map[string]any)
	firstBefore := events[1].Details["before"].(map[string]any)
	if !sameResourceTextSnapshot(firstBefore, initialValues) || !sameResourceTextSnapshot(secondBefore, firstAfter) {
		t.Fatalf("serialized edits used stale before-values: first=%v -> %v, second=%v -> %v",
			firstBefore, firstAfter, secondBefore, secondAfter)
	}
	firstName := firstAfter["name"]
	secondName := secondAfter["name"]
	if !((firstName == "First edit" && secondName == "Second edit") ||
		(firstName == "Second edit" && secondName == "First edit")) {
		t.Fatalf("serialized edit after-values = %v then %v, want both submitted edits", firstName, secondName)
	}
	var code, name, description string
	var active bool
	var createdAt, updatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, created.Resource.ID).Scan(
		&code, &name, &description, &active, &createdAt, &updatedAt); err != nil {
		t.Fatalf("read final concurrently edited Resource: %v", err)
	}
	if code != created.Resource.Code || name != secondName || description != secondAfter["description"] || active ||
		!createdAt.Equal(created.Resource.CreatedAt) || !updatedAt.Equal(secondAt) {
		t.Fatalf("final concurrent Resource = code %q, name %q, description %q, active %v, created %s, updated %s",
			code, name, description, active, createdAt, updatedAt)
	}
}

func TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	clock := &resourceOperationClock{}
	base := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	clock.Set(base)
	service := resources.NewService(NewStore(appPool), clock.Now)
	created, err := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Identical-edit-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Original Lab", Description: "Original description", Active: true,
	})
	if err != nil {
		t.Fatalf("create identical-edit fixture: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	installResourceNoOpUpdateFailureTrigger(t, adminPool, created.Resource.ID)

	operationBase := base.Add(time.Hour)
	clock.Set(operationBase)
	callsBeforeWait := clock.calls.Load()
	held := holdBookingResourceLock(t, adminPool, created.Resource.ID)
	first := startResourceEdit(t.Context(), service, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "Canonical next", Description: "same description",
	})
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 1); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("first canonical edit did not wait for the Resource lock: %v", err)
	}
	cancel()
	second := startResourceEdit(t.Context(), service, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: "\u2003" + created.Resource.Code + "\u00a0",
		Name: "\u00a0Canonical next\u2003", Description: "\u2003same description\u00a0",
	})
	waitCtx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("both canonical edits did not wait on the Resource lock: %v", err)
	}
	cancel()
	if clock.calls.Load() != callsBeforeWait {
		_ = held.release()
		t.Fatalf("canonical edit clock was read while waiting: calls before=%d after=%d", callsBeforeWait, clock.calls.Load())
	}
	if err := held.release(); err != nil {
		t.Fatalf("release identical-edit Resource lock: %v", err)
	}
	firstResult, secondResult := receiveResourceEdit(t, first), receiveResourceEdit(t, second)
	if firstResult.err != nil || secondResult.err != nil || firstResult.result.Changed == secondResult.result.Changed {
		t.Fatalf("identical canonical edit outcomes = (%+v, %v), (%+v, %v); want one mutation and one no-op",
			firstResult.result, firstResult.err, secondResult.result, secondResult.err)
	}
	if firstResult.result.Resource.Name != "Canonical next" || secondResult.result.Resource.Name != "Canonical next" ||
		firstResult.result.Resource.Description != "same description" || secondResult.result.Resource.Description != "same description" {
		t.Fatalf("identical edit results differ from canonical values: %+v, %+v", firstResult.result, secondResult.result)
	}
	if clock.calls.Load() != callsBeforeWait+2 {
		t.Fatalf("post-lock identical-edit clock calls = %d, want two", clock.calls.Load()-callsBeforeWait)
	}

	updatedAt := operationBase.Add(time.Microsecond)
	var storedUpdatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT updated_at FROM public.resources WHERE id = $1::uuid`, created.Resource.ID).Scan(&storedUpdatedAt); err != nil {
		t.Fatalf("read timestamp after identical edits: %v", err)
	}
	events := readResourceCapabilityEvents(t, adminPool, created.Resource.ID)
	if !storedUpdatedAt.Equal(updatedAt) || len(events) != 2 || events[1].Action != "resource.descriptive_edited" ||
		!events[1].OccurredAt.Equal(updatedAt) || events[1].Details["before"].(map[string]any)["name"] != "Original Lab" ||
		events[1].Details["after"].(map[string]any)["name"] != "Canonical next" {
		t.Fatalf("identical edit persisted more than one mutation: updated=%s events=%+v", storedUpdatedAt, events)
	}
}

func cleanupResourceBookingFixture(t *testing.T, adminPool *pgxpool.Pool, bookingID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(ctx, "DELETE FROM public.activity_events WHERE booking_id = $1::uuid", bookingID); err != nil {
			t.Errorf("delete Resource concurrency Booking events: %v", err)
		}
		if _, err := adminPool.Exec(ctx, "DELETE FROM public.bookings WHERE id = $1::uuid", bookingID); err != nil {
			t.Errorf("delete Resource concurrency Booking: %v", err)
		}
	})
}

func sameResourceTextSnapshot(actual, expected map[string]any) bool {
	return actual["name"] == expected["name"] && actual["description"] == expected["description"]
}

func installResourceNoOpUpdateFailureTrigger(t *testing.T, admin *pgxpool.Pool, resourceID string) {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := "labreserve_test_resource_noop_update_" + suffix
	triggerName := "labreserve_test_resource_noop_update_" + suffix
	if _, err := admin.Exec(t.Context(), `
		CREATE FUNCTION public.`+functionName+`() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF OLD.id = '`+resourceID+`'::uuid AND NEW.name = OLD.name AND NEW.description = OLD.description THEN
		        RAISE EXCEPTION 'unexpected no-op Resource UPDATE';
		    END IF;
		    RETURN NEW;
		END;
		$test$`); err != nil {
		t.Fatalf("create no-op UPDATE detector: %v", err)
	}
	if _, err := admin.Exec(t.Context(), "CREATE TRIGGER "+triggerName+
		" BEFORE UPDATE ON public.resources FOR EACH ROW EXECUTE FUNCTION public."+functionName+"()"); err != nil {
		_, _ = admin.Exec(t.Context(), "DROP FUNCTION public."+functionName+"()")
		t.Fatalf("install no-op UPDATE detector: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON public.resources"); err != nil {
			t.Errorf("drop no-op UPDATE detector: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS public."+functionName+"()"); err != nil {
			t.Errorf("drop no-op UPDATE detector function: %v", err)
		}
	})
}

func TestResourceEditsRemainCompatibleWithBookingCreation(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	clock := &resourceOperationClock{}
	base := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	clock.Set(base)
	resourceService := resources.NewService(NewStore(appPool), clock.Now)
	bookingService := bookings.NewService(NewStore(appPool), clock.Now)
	created, err := resourceService.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Edit-booking-create-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Eligible Lab", Active: true,
	})
	if err != nil {
		t.Fatalf("create Booking compatibility Resource: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	request := bookings.CreateRequest{
		ResourceID: created.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: base.Add(4 * time.Hour), EndAt: base.Add(5 * time.Hour), Purpose: "booking during descriptive edit",
	}

	clock.Set(base.Add(time.Hour))
	callsBeforeWait := clock.calls.Load()
	held := holdBookingResourceLock(t, adminPool, created.Resource.ID)
	edit := startResourceEdit(t.Context(), resourceService, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "Edited Eligible Lab", Description: "retained booking compatible",
	})
	booking := startBookingCreation(t.Context(), bookingService, bookings.Actor{AccountID: fixtures.coordinator}, request)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("Resource edit and Booking creation did not wait on the shared Resource lock: %v", err)
	}
	cancel()
	if clock.calls.Load() != callsBeforeWait {
		_ = held.release()
		t.Fatalf("cross-operation clock was read while Resource lock was held: before=%d after=%d", callsBeforeWait, clock.calls.Load())
	}
	if err := held.release(); err != nil {
		t.Fatalf("release edit/creation Resource lock: %v", err)
	}
	editResult, bookingResult := receiveResourceEdit(t, edit), receiveBookingCreation(t, booking)
	if editResult.err != nil || !editResult.result.Changed || bookingResult.err != nil || bookingResult.result.Replayed {
		t.Fatalf("edit/creation outcomes = (%+v, %v), (%+v, %v)", editResult.result, editResult.err, bookingResult.result, bookingResult.err)
	}
	cleanupResourceBookingFixture(t, adminPool, bookingResult.result.Booking.ID)
	if bookingResult.result.Booking.OwnerAccountID != fixtures.coordinator || bookingResult.result.Booking.ResourceID != created.Resource.ID ||
		bookingResult.result.Booking.State != bookings.StateConfirmed || editResult.result.Resource.Active != created.Resource.Active {
		t.Fatalf("edit changed Booking owner/eligibility or Resource active state: booking=%+v resource=%+v",
			bookingResult.result.Booking, editResult.result.Resource)
	}
	callsBeforeReplay := clock.calls.Load()
	replayed, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, request)
	if err != nil || !replayed.Replayed || replayed.Booking.ID != bookingResult.result.Booking.ID || clock.calls.Load() != callsBeforeReplay {
		t.Fatalf("Booking replay after concurrent Resource edit = %+v, error=%v clock calls delta=%d",
			replayed, err, clock.calls.Load()-callsBeforeReplay)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.coordinator, request.RequestID, 1, 1)

	inactive, err := resourceService.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Edit-booking-inactive-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Inactive Lab", Active: false,
	})
	if err != nil {
		t.Fatalf("create inactive Booking eligibility fixture: %v", err)
	}
	fixtures.trackResource(inactive.Resource.ID)
	inactiveEdit, err := resourceService.Edit(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.EditRequest{
		ResourceID: inactive.Resource.ID, Code: inactive.Resource.Code, Name: "Still Inactive Lab", Description: "descriptive only",
	})
	if err != nil || !inactiveEdit.Changed || inactiveEdit.Resource.Active {
		t.Fatalf("descriptive edit changed inactive Resource eligibility: %+v, %v", inactiveEdit, err)
	}
	inactiveRequest := bookings.CreateRequest{
		ResourceID: inactive.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: base.Add(6 * time.Hour), EndAt: base.Add(7 * time.Hour), Purpose: "must remain inactive",
	}
	if _, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, inactiveRequest); !errors.Is(err, bookings.ErrInactiveResource) {
		t.Fatalf("Booking on descriptively edited inactive Resource = %v, want existing inactive outcome", err)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.coordinator, inactiveRequest.RequestID, 0, 0)
}

func TestResourceEditsRemainCompatibleWithOwnerCancellationAndReplay(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	clock := &resourceOperationClock{}
	base := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	clock.Set(base)
	resourceService := resources.NewService(NewStore(appPool), clock.Now)
	bookingService := bookings.NewService(NewStore(appPool), clock.Now)
	cancellationService := bookings.NewCancellationService(NewStore(appPool), clock.Now)
	created, err := resourceService.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: "Edit-cancel-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Cancellation Lab", Active: true,
	})
	if err != nil {
		t.Fatalf("create cancellation compatibility Resource: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	request := bookings.CreateRequest{
		ResourceID: created.Resource.ID, RequestID: testDatabaseUUID(t, adminPool),
		StartAt: base.Add(4 * time.Hour), EndAt: base.Add(5 * time.Hour), Purpose: "retained cancellation request",
	}
	bookingResult, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, request)
	if err != nil {
		t.Fatalf("create cancellation compatibility Booking: %v", err)
	}
	cleanupResourceBookingFixture(t, adminPool, bookingResult.Booking.ID)

	clock.Set(base.Add(time.Hour))
	callsBeforeWait := clock.calls.Load()
	held := holdBookingResourceLock(t, adminPool, created.Resource.ID)
	edit := startResourceEdit(t.Context(), resourceService, fixtures.coordinator, resources.EditRequest{
		ResourceID: created.Resource.ID, Code: created.Resource.Code, Name: "Edited Cancellation Lab", Description: "cancellation remains eligible",
	})
	cancellation := startBookingCancellation(t.Context(), cancellationService, fixtures.coordinator, bookingResult.Booking.ID)
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	if err := waitForBlockedBookingCalls(waitCtx, adminPool, held.pid, 2); err != nil {
		cancel()
		_ = held.release()
		t.Fatalf("Resource edit and owner cancellation did not wait on the shared Resource lock: %v", err)
	}
	cancel()
	if clock.calls.Load() != callsBeforeWait {
		_ = held.release()
		t.Fatalf("edit/cancellation clock was read while waiting: before=%d after=%d", callsBeforeWait, clock.calls.Load())
	}
	if err := held.release(); err != nil {
		t.Fatalf("release edit/cancellation Resource lock: %v", err)
	}
	editResult, cancellationResult := receiveResourceEdit(t, edit), receiveBookingCancellation(t, cancellation)
	if editResult.err != nil || !editResult.result.Changed || cancellationResult.err != nil || !cancellationResult.result.Changed {
		t.Fatalf("edit/cancellation outcomes = (%+v, %v), (%+v, %v)",
			editResult.result, editResult.err, cancellationResult.result, cancellationResult.err)
	}
	var state, ownerID, requestID string
	var cancelledBy string
	var startAt, endAt, createdAt, cancelledAt time.Time
	var cancellationEvents int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT state, owner_account_id::text, request_id::text, start_at, end_at, created_at,
		       cancelled_by_account_id::text, cancelled_at,
		       (SELECT count(*) FROM public.activity_events WHERE booking_id = $1::uuid AND action = 'booking.cancelled')
		FROM public.bookings WHERE id = $1::uuid`, bookingResult.Booking.ID).Scan(
		&state, &ownerID, &requestID, &startAt, &endAt, &createdAt, &cancelledBy, &cancelledAt, &cancellationEvents); err != nil {
		t.Fatalf("read Booking after concurrent edit/cancellation: %v", err)
	}
	if state != string(bookings.StateCancelled) || ownerID != fixtures.coordinator || requestID != request.RequestID ||
		!startAt.Equal(request.StartAt) || !endAt.Equal(request.EndAt) || !createdAt.Equal(bookingResult.Booking.CreatedAt) ||
		cancelledBy != fixtures.coordinator || cancellationResult.result.Booking.CancelledAt == nil ||
		!cancelledAt.Equal(*cancellationResult.result.Booking.CancelledAt) || cancellationEvents != 1 {
		t.Fatalf("edit changed Booking ownership/request/interval or cancellation integrity: state=%s owner=%s request=%s interval=%s..%s created=%s cancelledBy=%s cancelledAt=%s events=%d",
			state, ownerID, requestID, startAt, endAt, createdAt, cancelledBy, cancelledAt, cancellationEvents)
	}
	if _, err := cancellationService.Cancel(t.Context(), bookings.Actor{AccountID: fixtures.engineer}, bookingResult.Booking.ID); !errors.Is(err, bookings.ErrBookingNotAccessible) {
		t.Fatalf("non-owner cancellation of retained Cancelled Booking = %v, want inaccessible", err)
	}
	callsBeforeReplay := clock.calls.Load()
	replayed, err := bookingService.Create(t.Context(), bookings.Actor{AccountID: fixtures.coordinator}, request)
	if err != nil || !replayed.Replayed || replayed.Booking.ID != bookingResult.Booking.ID || replayed.Booking.State != bookings.StateCancelled ||
		clock.calls.Load() != callsBeforeReplay {
		t.Fatalf("creation replay after concurrent edit/cancellation = %+v, error=%v clock calls delta=%d",
			replayed, err, clock.calls.Load()-callsBeforeReplay)
	}
	var resourceEventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid`, created.Resource.ID).Scan(&resourceEventCount); err != nil {
		t.Fatalf("count Resource events after edit/cancellation: %v", err)
	}
	if resourceEventCount != 2 {
		t.Fatalf("Resource edit/cancellation produced %d Resource events, want creation and edit", resourceEventCount)
	}
	assertBookingRequestCounts(t, adminPool, fixtures.coordinator, request.RequestID, 1, 1)
}
