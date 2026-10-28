package database

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	createResourceCapabilitySQL = `
		SELECT resource_id::text, code, name, description, active, created_at, updated_at
		FROM public.labreserve_create_resource($1::uuid, $2, $3, $4, $5, $6)`
	editResourceCapabilitySQL = `
		SELECT resource_id::text, code, name, description, active, created_at, updated_at, changed
		FROM public.labreserve_edit_resource($1::uuid, $2::uuid, $3, $4, $5)`
)

type resourceCapabilityRow struct {
	ID          string
	Code        string
	Name        string
	Description string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Changed     bool
}

type resourceCapabilityQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type resourceCapabilityFixtures struct {
	t              *testing.T
	admin          *pgxpool.Pool
	coordinator    string
	engineer       string
	secondEngineer string
	unknown        string
	resources      []string
}

func newResourceCapabilityFixtures(t *testing.T, admin *pgxpool.Pool) *resourceCapabilityFixtures {
	t.Helper()
	fixtures := &resourceCapabilityFixtures{t: t, admin: admin}
	fixtures.coordinator = insertResourceCapabilityAccount(t, admin, "coordinator")
	fixtures.engineer = insertResourceCapabilityAccount(t, admin, "engineer")
	fixtures.secondEngineer = insertResourceCapabilityAccount(t, admin, "engineer")
	if err := admin.QueryRow(t.Context(), "SELECT gen_random_uuid()::text").Scan(&fixtures.unknown); err != nil {
		t.Fatalf("generate unknown actor ID: %v", err)
	}
	t.Cleanup(fixtures.cleanup)
	return fixtures
}

func insertResourceCapabilityAccount(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(t.Context(), `
		INSERT INTO public.accounts (id, login, display_name, password_hash, role)
		VALUES (
			gen_random_uuid(),
			'resource-capability-' || replace(gen_random_uuid()::text, '-', '') || '@example.test',
			'Resource capability fixture', 'unused', $1
		)
		RETURNING id::text`, role).Scan(&id)
	if err != nil {
		t.Fatalf("create Resource capability actor: %v", err)
	}
	return id
}

func (f *resourceCapabilityFixtures) trackResource(id string) {
	f.resources = append(f.resources, id)
}

func (f *resourceCapabilityFixtures) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, id := range f.resources {
		if _, err := f.admin.Exec(ctx, "DELETE FROM public.activity_events WHERE resource_id = $1::uuid", id); err != nil {
			f.t.Errorf("delete Resource capability events: %v", err)
		}
		if _, err := f.admin.Exec(ctx, "DELETE FROM public.resources WHERE id = $1::uuid", id); err != nil {
			f.t.Errorf("delete Resource capability fixture: %v", err)
		}
	}
	for _, id := range []string{f.coordinator, f.engineer, f.secondEngineer} {
		if _, err := f.admin.Exec(ctx, "DELETE FROM public.activity_events WHERE actor_account_id = $1::uuid", id); err != nil {
			f.t.Errorf("delete Resource capability actor events: %v", err)
		}
		if _, err := f.admin.Exec(ctx, "DELETE FROM public.accounts WHERE id = $1::uuid", id); err != nil {
			f.t.Errorf("delete Resource capability actor: %v", err)
		}
	}
}

func callCreateResourceCapability(ctx context.Context, queryer resourceCapabilityQueryer, actorID, code, name, description string, active bool, occurredAt time.Time) (resourceCapabilityRow, error) {
	var result resourceCapabilityRow
	err := queryer.QueryRow(ctx, createResourceCapabilitySQL, actorID, code, name, description, active, occurredAt).Scan(
		&result.ID, &result.Code, &result.Name, &result.Description, &result.Active, &result.CreatedAt, &result.UpdatedAt)
	return result, err
}

func callEditResourceCapability(ctx context.Context, queryer resourceCapabilityQueryer, actorID, resourceID, name, description string, occurredAt time.Time) (resourceCapabilityRow, error) {
	var result resourceCapabilityRow
	err := queryer.QueryRow(ctx, editResourceCapabilitySQL, actorID, resourceID, name, description, occurredAt).Scan(
		&result.ID, &result.Code, &result.Name, &result.Description, &result.Active,
		&result.CreatedAt, &result.UpdatedAt, &result.Changed)
	return result, err
}

func insertResourceCapabilityFixture(t *testing.T, pool *pgxpool.Pool, code, name, description string, active bool, createdAt time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(t.Context(), `
		INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $5)
		RETURNING id::text`, code, name, description, active, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("create Resource capability fixture: %v", err)
	}
	return id
}

type resourceCapabilityEvent struct {
	ActorID      string
	Action       string
	OccurredAt   time.Time
	BookingIsNil bool
	Details      map[string]any
}

func readResourceCapabilityEvents(t *testing.T, pool *pgxpool.Pool, resourceID string) []resourceCapabilityEvent {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT actor_account_id::text, action, occurred_at, booking_id IS NULL, details::text
		FROM public.activity_events WHERE resource_id = $1::uuid ORDER BY occurred_at, id`, resourceID)
	if err != nil {
		t.Fatalf("read Resource Activity events: %v", err)
	}
	defer rows.Close()
	var events []resourceCapabilityEvent
	for rows.Next() {
		var event resourceCapabilityEvent
		var details string
		if err := rows.Scan(&event.ActorID, &event.Action, &event.OccurredAt, &event.BookingIsNil, &details); err != nil {
			t.Fatalf("scan Resource Activity event: %v", err)
		}
		if err := json.Unmarshal([]byte(details), &event.Details); err != nil {
			t.Fatalf("decode Resource Activity details: %v", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Resource Activity events: %v", err)
	}
	return events
}

func assertResourceCapabilityErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var postgresErr *pgconn.PgError
	if !errors.As(err, &postgresErr) || postgresErr.Code != want {
		t.Fatalf("PostgreSQL error = %v, want SQLSTATE %s", err, want)
	}
}

func resourceCapabilitySnapshot(t *testing.T, pool *pgxpool.Pool, resourceID string) (string, string) {
	t.Helper()
	var resourceJSON, eventsJSON string
	if err := pool.QueryRow(t.Context(), `
		SELECT to_jsonb(resource)::text,
		       COALESCE((
		           SELECT jsonb_agg(to_jsonb(event) ORDER BY event.id)::text
		           FROM public.activity_events AS event WHERE event.resource_id = resource.id
		       ), '[]')
		FROM public.resources AS resource WHERE resource.id = $1::uuid`, resourceID).Scan(&resourceJSON, &eventsJSON); err != nil {
		t.Fatalf("snapshot Resource and Activity for %s: %v", resourceID, err)
	}
	return resourceJSON, eventsJSON
}

func TestResourceCapabilitiesCreateEditAndNoOpAreGuardedAndAtomic(t *testing.T) {
	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("apply Resource capability migration: %v", err)
	}
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, adminPool)

	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	for _, active := range []bool{true, false} {
		code := fmt.Sprintf("Capability-%t-%s", active, strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""))
		created, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator, code, "Canonical Lab", "Mixed Case", active, createdAt)
		if err != nil {
			t.Fatalf("create Resource with active=%v: %v", active, err)
		}
		fixtures.trackResource(created.ID)
		if created.Code != code || created.Name != "Canonical Lab" || created.Description != "Mixed Case" ||
			created.Active != active || !created.CreatedAt.Equal(createdAt) || !created.UpdatedAt.Equal(createdAt) {
			t.Fatalf("created Resource = %+v", created)
		}
		events := readResourceCapabilityEvents(t, adminPool, created.ID)
		if len(events) != 1 || events[0].ActorID != fixtures.coordinator || events[0].Action != "resource.created" ||
			!events[0].OccurredAt.Equal(createdAt) || !events[0].BookingIsNil ||
			events[0].Details["code"] != code || events[0].Details["name"] != "Canonical Lab" ||
			events[0].Details["description"] != "Mixed Case" || events[0].Details["active"] != active {
			t.Fatalf("creation Activity = %+v", events)
		}
	}

	resourceID := fixtures.resources[0]
	before, err := adminPool.Query(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID)
	if err != nil {
		t.Fatalf("read Resource before edit: %v", err)
	}
	var beforeCode, beforeName, beforeDescription string
	var beforeActive bool
	var beforeCreatedAt, beforeUpdatedAt time.Time
	if !before.Next() {
		before.Close()
		t.Fatal("created Resource disappeared before edit")
	}
	if err := before.Scan(&beforeCode, &beforeName, &beforeDescription, &beforeActive, &beforeCreatedAt, &beforeUpdatedAt); err != nil {
		before.Close()
		t.Fatalf("scan Resource before edit: %v", err)
	}
	before.Close()

	editedAt := createdAt.Add(time.Minute)
	edited, err := callEditResourceCapability(t.Context(), runtimePool, fixtures.coordinator, resourceID, "Renamed Lab", "New description", editedAt)
	if err != nil {
		t.Fatalf("edit Resource: %v", err)
	}
	if !edited.Changed || edited.ID != resourceID || edited.Code != beforeCode || edited.Name != "Renamed Lab" ||
		edited.Description != "New description" || edited.Active != beforeActive ||
		!edited.CreatedAt.Equal(beforeCreatedAt) || !edited.UpdatedAt.Equal(editedAt) {
		t.Fatalf("edited Resource = %+v; prior code=%q", edited, beforeCode)
	}
	events := readResourceCapabilityEvents(t, adminPool, resourceID)
	if len(events) != 2 || events[1].ActorID != fixtures.coordinator || events[1].Action != "resource.descriptive_edited" ||
		!events[1].OccurredAt.Equal(editedAt) || !events[1].BookingIsNil ||
		events[1].Details["before"].(map[string]any)["name"] != beforeName ||
		events[1].Details["before"].(map[string]any)["description"] != beforeDescription ||
		events[1].Details["after"].(map[string]any)["name"] != "Renamed Lab" ||
		events[1].Details["after"].(map[string]any)["description"] != "New description" {
		t.Fatalf("descriptive-edit Activity = %+v", events)
	}

	installResourceUpdateFailureTrigger(t, adminPool, resourceID)
	installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.descriptive_edited")
	noOp, err := callEditResourceCapability(t.Context(), runtimePool, fixtures.coordinator, resourceID, "Renamed Lab", "New description", editedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("unchanged edit invoked a guarded write: %v", err)
	}
	if noOp.Changed || !noOp.UpdatedAt.Equal(editedAt) || len(readResourceCapabilityEvents(t, adminPool, resourceID)) != 2 {
		t.Fatalf("unchanged edit changed persisted Resource/Activity: result=%+v", noOp)
	}
	resourceBefore, eventsBefore := resourceCapabilitySnapshot(t, adminPool, resourceID)

	for _, actorID := range []string{fixtures.engineer, fixtures.secondEngineer, fixtures.unknown} {
		createCode := "Forbidden-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
		if _, err := callCreateResourceCapability(t.Context(), runtimePool, actorID,
			createCode, "Name", "", true, editedAt); err == nil {
			t.Fatalf("actor %s created a Resource", actorID)
		} else {
			assertResourceCapabilityErrorCode(t, err, "42501")
		}
		assertResourceCreationAbsent(t, adminPool, createCode)
		if _, err := callEditResourceCapability(t.Context(), runtimePool, actorID, resourceID, "Renamed Lab", "New description", editedAt); err == nil {
			t.Fatalf("actor %s used descriptive-edit no-op", actorID)
		} else {
			assertResourceCapabilityErrorCode(t, err, "42501")
		}
	}
	resourceAfter, eventsAfter := resourceCapabilitySnapshot(t, adminPool, resourceID)
	if resourceAfter != resourceBefore || eventsAfter != eventsBefore {
		t.Fatalf("rejected direct capability calls changed Resource/Activity:\nbefore=%s %s\nafter=%s %s",
			resourceBefore, eventsBefore, resourceAfter, eventsAfter)
	}
}

func TestResourceCapabilitiesRejectNoncanonicalTextAndDuplicateCodes(t *testing.T) {
	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("apply Resource capability migration: %v", err)
	}
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, adminPool)
	at := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	prefix := "Invalid-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")

	invalid := []struct {
		name         string
		code         string
		resourceName string
		description  string
	}{
		{name: "empty code", code: "", resourceName: "Name", description: ""},
		{name: "unicode whitespace code", code: "\u2003\u00a0", resourceName: "Name", description: ""},
		{name: "whitespace-only name", code: prefix + "-name", resourceName: "\u2003\u00a0", description: ""},
		{name: "edge-whitespace name", code: prefix + "-edge", resourceName: " Name", description: ""},
		{name: "edge-whitespace description", code: prefix + "-description", resourceName: "Name", description: "\u0085description\u00a0"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator,
				test.code, test.resourceName, test.description, true, at)
			assertResourceCapabilityErrorCode(t, err, "22023")
			assertResourceCreationAbsent(t, adminPool, test.code)
		})
	}

	code := prefix + "-valid"
	created, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator, code, "Valid Name", "", false, at)
	if err != nil {
		t.Fatalf("create duplicate-code fixture: %v", err)
	}
	fixtures.trackResource(created.ID)
	resourceBeforeDuplicate, eventsBeforeDuplicate := resourceCapabilitySnapshot(t, adminPool, created.ID)
	if _, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator,
		strings.ToLower(code), "Another Name", "", true, at.Add(time.Second)); err == nil {
		t.Fatal("case variant of an existing Resource code was accepted")
	} else {
		assertResourceCapabilityErrorCode(t, err, "23505")
	}
	var resourceCount, eventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*),
		       (SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid)
		FROM public.resources WHERE lower(code) = lower($2)`, created.ID, code).Scan(&resourceCount, &eventCount); err != nil {
		t.Fatalf("inspect duplicate-code outcome: %v", err)
	}
	if resourceCount != 1 || eventCount != 1 {
		t.Fatalf("duplicate code persisted resources/events %d/%d, want 1/1", resourceCount, eventCount)
	}
	resourceAfterDuplicate, eventsAfterDuplicate := resourceCapabilitySnapshot(t, adminPool, created.ID)
	if resourceAfterDuplicate != resourceBeforeDuplicate || eventsAfterDuplicate != eventsBeforeDuplicate {
		t.Fatalf("duplicate direct creation changed existing Resource/Activity:\nbefore=%s %s\nafter=%s %s",
			resourceBeforeDuplicate, eventsBeforeDuplicate, resourceAfterDuplicate, eventsAfterDuplicate)
	}
	resourceBeforeInvalidEdit, eventsBeforeInvalidEdit := resourceCapabilitySnapshot(t, adminPool, created.ID)

	for _, invalidEdit := range []struct {
		name        string
		description string
	}{
		{name: "\u2003\u00a0", description: ""},
		{name: "Valid Name", description: " trailing "},
	} {
		if _, err := callEditResourceCapability(t.Context(), runtimePool, fixtures.coordinator, created.ID,
			invalidEdit.name, invalidEdit.description, at.Add(time.Minute)); err == nil {
			t.Fatalf("noncanonical edit %q/%q was accepted", invalidEdit.name, invalidEdit.description)
		} else {
			assertResourceCapabilityErrorCode(t, err, "22023")
		}
	}
	resourceAfterInvalidEdit, eventsAfterInvalidEdit := resourceCapabilitySnapshot(t, adminPool, created.ID)
	if resourceAfterInvalidEdit != resourceBeforeInvalidEdit || eventsAfterInvalidEdit != eventsBeforeInvalidEdit {
		t.Fatalf("invalid direct edit changed Resource/Activity:\nbefore=%s %s\nafter=%s %s",
			resourceBeforeInvalidEdit, eventsBeforeInvalidEdit, resourceAfterInvalidEdit, eventsAfterInvalidEdit)
	}
}

func TestResourceCapabilitiesRollbackWithActivityFailureAndOuterTransaction(t *testing.T) {
	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("apply Resource capability migration: %v", err)
	}
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	fixtures := newResourceCapabilityFixtures(t, adminPool)
	at := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	code := "Rollback-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")

	removeCreateFailure := installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.created")
	if _, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator, code, "Rollback Lab", "", true, at); err == nil {
		t.Fatal("creation committed despite Activity insertion failure")
	}
	var count int
	if err := adminPool.QueryRow(t.Context(), "SELECT count(*) FROM public.resources WHERE code = $1", code).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed Activity creation left %d Resource rows", count)
	}
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*) FROM public.activity_events WHERE actor_account_id = $1::uuid AND action = 'resource.created'`,
		fixtures.coordinator).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed Activity creation left %d successful events", count)
	}
	removeCreateFailure()

	created, err := callCreateResourceCapability(t.Context(), runtimePool, fixtures.coordinator,
		code, "Rollback Lab", "Original", false, at)
	if err != nil {
		t.Fatalf("create Resource after removing the creation failure: %v", err)
	}
	fixtures.trackResource(created.ID)

	removeEditFailure := installResourceActivityFailureTrigger(t, adminPool, fixtures.coordinator, "resource.descriptive_edited")
	if _, err := callEditResourceCapability(t.Context(), runtimePool, fixtures.coordinator,
		created.ID, "Changed Name", "Changed description", at.Add(time.Minute)); err == nil {
		t.Fatal("descriptive edit committed despite Activity insertion failure")
	}
	var name, description string
	var updatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT name, description, updated_at FROM public.resources WHERE id = $1::uuid`, created.ID).Scan(
		&name, &description, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if name != "Rollback Lab" || description != "Original" || !updatedAt.Equal(at) ||
		len(readResourceCapabilityEvents(t, adminPool, created.ID)) != 1 {
		t.Fatalf("failed Activity edit changed Resource/history: name=%q description=%q updated=%s", name, description, updatedAt)
	}
	removeEditFailure()
	retryAt := at.Add(2 * time.Minute)
	retriedEdit, err := callEditResourceCapability(t.Context(), runtimePool, fixtures.coordinator,
		created.ID, "Changed Name", "Changed description", retryAt)
	if err != nil || !retriedEdit.Changed || retriedEdit.Name != "Changed Name" ||
		retriedEdit.Description != "Changed description" || !retriedEdit.UpdatedAt.Equal(retryAt) {
		t.Fatalf("retry edit after removing Activity failure: result=%+v error=%v", retriedEdit, err)
	}
	retriedEvents := readResourceCapabilityEvents(t, adminPool, created.ID)
	if len(retriedEvents) != 2 || retriedEvents[1].Action != "resource.descriptive_edited" ||
		!retriedEvents[1].OccurredAt.Equal(retryAt) ||
		retriedEvents[1].Details["before"].(map[string]any)["name"] != "Rollback Lab" ||
		retriedEvents[1].Details["after"].(map[string]any)["name"] != "Changed Name" {
		t.Fatalf("successful edit retry Activity = %+v", retriedEvents)
	}

	rollbackCode := "Outer-Rollback-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	tx, err := runtimePool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin outer Resource transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	rolledBack, err := callCreateResourceCapability(t.Context(), tx, fixtures.coordinator, rollbackCode, "Outer rollback", "", true, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("invoke Resource capability in outer transaction: %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back outer Resource transaction: %v", err)
	}
	if err := adminPool.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM public.resources WHERE id = $1::uuid) +
		       (SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid)`, rolledBack.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("outer rollback retained %d Resource/event rows", count)
	}
}

func TestResourceCapabilitiesExposeOnlyGuardedRuntimeWrites(t *testing.T) {
	adminPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), adminPool); err != nil {
		t.Fatalf("apply Resource capability migration: %v", err)
	}
	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var runtimeRole string
	if err := runtimePool.QueryRow(t.Context(), "SELECT current_user").Scan(&runtimeRole); err != nil || runtimeRole != "labreserve_app" {
		t.Fatalf("privilege test must use actual labreserve_app login: current_user=%q error=%v", runtimeRole, err)
	}
	fixtures := newResourceCapabilityFixtures(t, adminPool)
	resourceID := insertResourceCapabilityFixture(t, adminPool,
		"Permission-"+strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""), "Permission Lab", "", true,
		time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC))
	fixtures.trackResource(resourceID)

	var accountInsert, accountUpdate, accountDelete, accountTruncate, canCreateRole, isSuperuser, migratorMember bool
	if err := runtimePool.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.accounts', 'INSERT'),
		       has_table_privilege(current_user, 'public.accounts', 'UPDATE'),
		       has_table_privilege(current_user, 'public.accounts', 'DELETE'),
		       has_table_privilege(current_user, 'public.accounts', 'TRUNCATE'),
		       role.rolcreaterole, role.rolsuper,
		       pg_catalog.pg_has_role(current_user, 'labreserve_migrator', 'MEMBER')
		FROM pg_catalog.pg_roles AS role WHERE role.rolname = current_user`).Scan(
		&accountInsert, &accountUpdate, &accountDelete, &accountTruncate, &canCreateRole, &isSuperuser, &migratorMember); err != nil {
		t.Fatalf("inspect effective runtime Account/role privileges: %v", err)
	}
	if accountInsert || accountUpdate || accountDelete || accountTruncate || canCreateRole || isSuperuser || migratorMember {
		t.Fatalf("runtime Account/role privileges are broader than approved: insert/update/delete/truncate=%v/%v/%v/%v create-role=%v superuser=%v migrator-member=%v",
			accountInsert, accountUpdate, accountDelete, accountTruncate, canCreateRole, isSuperuser, migratorMember)
	}

	for _, privilege := range []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE"} {
		var granted bool
		if err := runtimePool.QueryRow(t.Context(), `
			SELECT has_table_privilege(current_user, 'public.resources', $1)`, privilege).Scan(&granted); err != nil {
			t.Fatalf("inspect Resource %s privilege: %v", privilege, err)
		}
		if granted {
			t.Errorf("runtime role unexpectedly has Resource %s privilege", privilege)
		}
	}
	rows, err := runtimePool.Query(t.Context(), `
		SELECT column_name, has_column_privilege(current_user, 'public.resources', column_name, 'UPDATE')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'resources'`)
	if err != nil {
		t.Fatalf("inspect Resource column privileges: %v", err)
	}
	for rows.Next() {
		var column string
		var granted bool
		if err := rows.Scan(&column, &granted); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if granted {
			t.Errorf("runtime role unexpectedly has UPDATE on resources.%s", column)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("read Resource column privileges: %v", err)
	}
	rows.Close()

	for _, operation := range []struct {
		name string
		sql  string
		args []any
	}{
		{name: "Account insert", sql: `INSERT INTO public.accounts (id, login, display_name, password_hash, role) VALUES (gen_random_uuid(), 'runtime-denied@example.test', 'Denied', 'unused', 'engineer')`},
		{name: "Account role update", sql: `UPDATE public.accounts SET role = role WHERE id = $1::uuid`, args: []any{fixtures.coordinator}},
		{name: "Account delete", sql: `DELETE FROM public.accounts WHERE id = $1::uuid`, args: []any{fixtures.coordinator}},
		{name: "Account truncate", sql: `TRUNCATE public.accounts`},
	} {
		t.Run(operation.name, func(t *testing.T) {
			assertRuntimeStatementDenied(t, runtimePool, operation.sql, operation.args...)
		})
	}

	disallowed := []struct {
		name string
		sql  string
		args []any
	}{
		{name: "direct insert", sql: `INSERT INTO public.resources (id, code, name, description) VALUES (gen_random_uuid(), 'denied', 'Denied', '')`},
		{name: "identity update", sql: `UPDATE public.resources SET id = id WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "code update", sql: `UPDATE public.resources SET code = code WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "name update", sql: `UPDATE public.resources SET name = name WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "description update", sql: `UPDATE public.resources SET description = description WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "state update", sql: `UPDATE public.resources SET active = active WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "creation time update", sql: `UPDATE public.resources SET created_at = created_at WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "update time update", sql: `UPDATE public.resources SET updated_at = updated_at WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "delete", sql: `DELETE FROM public.resources WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "truncate", sql: `TRUNCATE public.resources`},
	}
	for _, operation := range disallowed {
		t.Run(operation.name, func(t *testing.T) {
			if _, err := runtimePool.Exec(t.Context(), operation.sql, operation.args...); err == nil {
				t.Fatalf("runtime role executed forbidden Resource operation %q", operation.name)
			}
		})
	}

	for _, function := range []struct {
		name      string
		signature string
	}{
		{name: "public.labreserve_create_resource", signature: "public.labreserve_create_resource(uuid,text,text,text,boolean,timestamp with time zone)"},
		{name: "public.labreserve_edit_resource", signature: "public.labreserve_edit_resource(uuid,uuid,text,text,timestamp with time zone)"},
		{name: "public.labreserve_set_resource_state", signature: "public.labreserve_set_resource_state(uuid,uuid,boolean,timestamp with time zone)"},
	} {
		var securityDefiner, ownerSuperuser, runtimeExecute, publicExecute, otherExecute, grantOption, schemaCreate, ownerMembership bool
		var owner, searchPath string
		err := adminPool.QueryRow(t.Context(), `
			SELECT procedure.prosecdef,
			       owner.rolname,
			       owner.rolsuper,
			       has_function_privilege('labreserve_app', procedure.oid, 'EXECUTE'),
		       EXISTS (
			           SELECT 1 FROM pg_catalog.aclexplode(
			               COALESCE(procedure.proacl, pg_catalog.acldefault('f', procedure.proowner))
			           ) AS acl
			           WHERE acl.grantee = 0 AND acl.privilege_type = 'EXECUTE'
		       ),
			       EXISTS (
			           SELECT 1 FROM pg_catalog.aclexplode(
			               COALESCE(procedure.proacl, pg_catalog.acldefault('f', procedure.proowner))
			           ) AS acl
			           WHERE acl.grantee NOT IN (0, procedure.proowner, 'labreserve_app'::regrole::oid)
			             AND acl.privilege_type = 'EXECUTE'
		       ),
			       EXISTS (
			           SELECT 1 FROM pg_catalog.aclexplode(
			               COALESCE(procedure.proacl, pg_catalog.acldefault('f', procedure.proowner))
			           ) AS acl
			           WHERE acl.grantee = 'labreserve_app'::regrole::oid
			             AND acl.privilege_type = 'EXECUTE' AND acl.is_grantable
		       ),
		       procedure.proconfig[1],
		       has_schema_privilege('labreserve_app', 'public', 'CREATE'),
		       pg_catalog.pg_has_role('labreserve_app', 'labreserve_migrator', 'MEMBER')
			FROM pg_catalog.pg_proc AS procedure
			JOIN pg_catalog.pg_roles AS owner ON owner.oid = procedure.proowner
			WHERE procedure.oid = $1::regprocedure`, function.signature).Scan(
			&securityDefiner, &owner, &ownerSuperuser, &runtimeExecute, &publicExecute,
			&otherExecute, &grantOption, &searchPath, &schemaCreate, &ownerMembership)
		if err != nil {
			t.Fatalf("inspect guarded function %s: %v", function.name, err)
		}
		if !securityDefiner || owner != "labreserve_migrator" || ownerSuperuser || !runtimeExecute ||
			publicExecute || otherExecute || grantOption || searchPath != "search_path=pg_catalog" || schemaCreate || ownerMembership {
			t.Errorf("unsafe runtime capability %s: definer=%v owner=%q superuser=%v runtime=%v public=%v other=%v grant-option=%v path=%q schema-create=%v owner-membership=%v",
				function.name, securityDefiner, owner, ownerSuperuser, runtimeExecute, publicExecute,
				otherExecute, grantOption, searchPath, schemaCreate, ownerMembership)
		}
	}
	var stateCapabilityCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM pg_catalog.pg_proc AS procedure
		JOIN pg_catalog.pg_namespace AS function_schema ON function_schema.oid = procedure.pronamespace
		WHERE function_schema.nspname = 'public' AND procedure.proname = 'labreserve_set_resource_state'`).Scan(&stateCapabilityCount); err != nil {
		t.Fatalf("count guarded Resource state capabilities: %v", err)
	}
	if stateCapabilityCount != 1 {
		t.Fatalf("Resource state capability overload count = %d, want exactly one guarded signature", stateCapabilityCount)
	}
	var canUpdateActivity, canDeleteActivity, canTruncateActivity bool
	if err := runtimePool.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.activity_events', 'UPDATE'),
		       has_table_privilege(current_user, 'public.activity_events', 'DELETE'),
		       has_table_privilege(current_user, 'public.activity_events', 'TRUNCATE')`).Scan(
		&canUpdateActivity, &canDeleteActivity, &canTruncateActivity); err != nil {
		t.Fatal(err)
	}
	if canUpdateActivity || canDeleteActivity || canTruncateActivity {
		t.Fatalf("Resource migration broadened Activity mutation privileges: update/delete/truncate=%v/%v/%v",
			canUpdateActivity, canDeleteActivity, canTruncateActivity)
	}

	tx, err := runtimePool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin shadow-object test transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	for _, statement := range []string{
		`CREATE TEMP TABLE accounts (id uuid, role text)`,
		`CREATE TEMP TABLE resources (id uuid, code text, name text, description text, active boolean, created_at timestamptz, updated_at timestamptz)`,
		`CREATE TEMP TABLE activity_events (id uuid, actor_account_id uuid, action text, occurred_at timestamptz, resource_id uuid, details jsonb)`,
		`SET LOCAL search_path = pg_temp, public`,
	} {
		if _, err := tx.Exec(t.Context(), statement); err != nil {
			t.Fatalf("configure caller-controlled search path: %v", err)
		}
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO pg_temp.accounts (id, role) VALUES ($1::uuid, 'coordinator')`, fixtures.coordinator); err != nil {
		t.Fatalf("prepare shadow actor: %v", err)
	}
	shadowCode := "Shadow-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	shadowed, err := callCreateResourceCapability(t.Context(), tx, fixtures.coordinator, shadowCode, "Real Resource", "", true,
		time.Date(2040, time.January, 3, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("invoke capability with shadow objects and caller search_path: %v", err)
	}
	fixtures.trackResource(shadowed.ID)
	var shadowResourceCount, shadowEventCount int
	if err := tx.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM pg_temp.resources), (SELECT count(*) FROM pg_temp.activity_events)`).Scan(
		&shadowResourceCount, &shadowEventCount); err != nil {
		t.Fatalf("inspect shadow tables in capability transaction: %v", err)
	}
	if shadowResourceCount != 0 || shadowEventCount != 0 {
		t.Fatalf("guarded function used shadow objects: resources/events=%d/%d", shadowResourceCount, shadowEventCount)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit shadow-object test: %v", err)
	}
	var realResourceCount, realEventCount int
	if err := adminPool.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM public.resources WHERE id = $1::uuid),
		       (SELECT count(*) FROM public.activity_events WHERE resource_id = $1::uuid)`, shadowed.ID).Scan(
		&realResourceCount, &realEventCount); err != nil {
		t.Fatal(err)
	}
	if realResourceCount != 1 || realEventCount != 1 {
		t.Fatalf("guarded function wrote to wrong schema: real resources/events=%d/%d", realResourceCount, realEventCount)
	}

	for _, statement := range []struct {
		name string
		sql  string
	}{
		{name: "assume migrator role", sql: `SET LOCAL ROLE labreserve_migrator`},
		{name: "create public function", sql: `CREATE FUNCTION public.labreserve_runtime_probe() RETURNS integer LANGUAGE SQL AS $$ SELECT 1 $$`},
		{name: "replace Resource capability", sql: `CREATE OR REPLACE FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ) RETURNS TABLE(resource_id UUID, code TEXT, name TEXT, description TEXT, active BOOLEAN, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ) LANGUAGE SQL AS $$ SELECT NULL::UUID, NULL::TEXT, NULL::TEXT, NULL::TEXT, NULL::BOOLEAN, NULL::TIMESTAMPTZ, NULL::TIMESTAMPTZ $$`},
		{name: "alter Resource capability", sql: `ALTER FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ) RESET search_path`},
		{name: "replace Resource state capability", sql: `CREATE OR REPLACE FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) RETURNS TABLE(resource_id UUID, code TEXT, name TEXT, description TEXT, active BOOLEAN, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, changed BOOLEAN) LANGUAGE SQL AS $$ SELECT NULL::UUID, NULL::TEXT, NULL::TEXT, NULL::TEXT, NULL::BOOLEAN, NULL::TIMESTAMPTZ, NULL::TIMESTAMPTZ, NULL::BOOLEAN $$`},
		{name: "alter Resource state capability", sql: `ALTER FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) RESET search_path`},
	} {
		t.Run(statement.name, func(t *testing.T) {
			assertRuntimeStatementDenied(t, runtimePool, statement.sql)
		})
	}
}

func assertRuntimeStatementDenied(t *testing.T, pool *pgxpool.Pool, statement string, args ...any) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin isolated forbidden-operation check: %v", err)
	}
	_, execErr := tx.Exec(t.Context(), statement, args...)
	rollbackErr := tx.Rollback(t.Context())
	if execErr == nil {
		t.Fatalf("runtime role executed forbidden operation: %s", statement)
	}
	if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		t.Fatalf("roll back forbidden-operation check: %v", rollbackErr)
	}
}

func TestResourceCapabilityMigrationPreservesVersion004Data(t *testing.T) {
	pool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), pool); err != nil {
		t.Fatalf("prepare current schema before upgrade simulation: %v", err)
	}
	migrations, err := readEmbeddedMigrationChecksums()
	if err != nil {
		t.Fatalf("read migration checksums: %v", err)
	}
	var capabilityMigration *migrationChecksum
	var version004, migrationsThrough005 []migrationChecksum
	for index := range migrations {
		if migrations[index].version == "006_resource_state.sql" {
			break
		}
		migrationsThrough005 = append(migrationsThrough005, migrations[index])
		if migrations[index].version == "005_resource_mutations.sql" {
			capabilityMigration = &migrations[index]
			continue
		}
		version004 = append(version004, migrations[index])
	}
	if capabilityMigration == nil || len(version004) != 4 || len(migrationsThrough005) != 5 {
		t.Fatalf("migration ladder has Migration 005=%v, %d pre-capability versions, and %d versions through 005; want 1, 4, and 5",
			capabilityMigration != nil, len(version004), len(migrationsThrough005))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin additive migration simulation: %v", err)
	}
	defer tx.Rollback(context.Background())

	var accountID, resourceID, bookingID, eventID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.accounts (id, login, display_name, password_hash, role)
		VALUES (gen_random_uuid(), 'upgrade-004-' || replace(gen_random_uuid()::text, '-', '') || '@example.test',
		        'Migration upgrade actor', 'unused', 'coordinator')
		RETURNING id::text`).Scan(&accountID); err != nil {
		t.Fatalf("create migration-004 actor fixture: %v", err)
	}
	createdAt := time.Date(2040, time.January, 4, 3, 4, 5, 0, time.UTC)
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
		VALUES (gen_random_uuid(), 'UPGRADE-004-' || replace(gen_random_uuid()::text, '-', ''),
		        'Retained Resource', 'Retained description', TRUE, $1, $1)
		RETURNING id::text`, createdAt).Scan(&resourceID); err != nil {
		t.Fatalf("create migration-004 Resource fixture: %v", err)
	}
	requestID := testDatabaseUUID(t, pool)
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.bookings (id, resource_id, owner_account_id, request_id, start_at, end_at, purpose, state, created_at)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3::uuid, $4::timestamptz, $4::timestamptz + interval '30 minutes',
		        'Retained booking request', 'confirmed', $5)
		RETURNING id::text`, resourceID, accountID, requestID, createdAt.Add(24*time.Hour), createdAt).Scan(&bookingID); err != nil {
		t.Fatalf("create migration-004 Booking fixture: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
		VALUES (gen_random_uuid(), $1::uuid, 'booking.created', $2, $3::uuid, '{"retained":true}'::jsonb)
		RETURNING id::text`, accountID, createdAt, bookingID).Scan(&eventID); err != nil {
		t.Fatalf("create migration-004 Activity fixture: %v", err)
	}
	tokenHash := sha256.Sum256([]byte("resource-capability-upgrade-session"))
	csrfToken := sha256.Sum256([]byte("resource-capability-upgrade-csrf"))
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.sessions (token_hash, account_id, csrf_token, created_at, expires_at)
		VALUES ($1, $2::uuid, $3, $4, $5)`, tokenHash[:], accountID, csrfToken[:], createdAt, createdAt.Add(time.Hour)); err != nil {
		t.Fatalf("create migration-004 session fixture: %v", err)
	}

	if _, err := tx.Exec(ctx, "DROP FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ)"); err != nil {
		t.Fatalf("remove current state capability in the Migration 004 simulation: %v", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.schema_migrations WHERE version = '006_resource_state.sql'"); err != nil {
		t.Fatalf("simulate Migration 004 ledger without Migration 006: %v", err)
	}
	if _, err := tx.Exec(ctx, "DROP FUNCTION public.labreserve_create_resource(UUID, TEXT, TEXT, TEXT, BOOLEAN, TIMESTAMPTZ)"); err != nil {
		t.Fatalf("remove current creation capability in simulation: %v", err)
	}
	if _, err := tx.Exec(ctx, "DROP FUNCTION public.labreserve_edit_resource(UUID, UUID, TEXT, TEXT, TIMESTAMPTZ)"); err != nil {
		t.Fatalf("remove current edit capability in simulation: %v", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.schema_migrations WHERE version = $1", capabilityMigration.version); err != nil {
		t.Fatalf("simulate migration-004 ledger: %v", err)
	}
	if err := validateMigrationLedger(ctx, tx, version004, true); err != nil {
		t.Fatalf("validate pre-capability migration-004 ledger: %v", err)
	}
	if _, err := tx.Exec(ctx, string(capabilityMigration.contents)); err != nil {
		t.Fatalf("apply additive Resource capability migration to existing data: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.schema_migrations (version, checksum) VALUES ($1, $2)`,
		capabilityMigration.version, capabilityMigration.checksum); err != nil {
		t.Fatalf("record additive Resource capability migration: %v", err)
	}
	if err := validateMigrationLedger(ctx, tx, migrationsThrough005, true); err != nil {
		t.Fatalf("validate schema ledger after the preserved 004-to-005 upgrade: %v", err)
	}

	var retainedResource, retainedBooking, retainedEvent, retainedSession int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM public.resources WHERE id = $1::uuid AND code LIKE 'UPGRADE-004-%'
		        AND name = 'Retained Resource' AND description = 'Retained description' AND active),
		       (SELECT count(*) FROM public.bookings WHERE id = $2::uuid AND resource_id = $1::uuid AND request_id = $3::uuid
		        AND purpose = 'Retained booking request' AND state = 'confirmed'),
		       (SELECT count(*) FROM public.activity_events WHERE id = $4::uuid AND booking_id = $2::uuid
		        AND details = '{"retained":true}'::jsonb),
		       (SELECT count(*) FROM public.sessions WHERE token_hash = $5 AND account_id = $6::uuid)`,
		resourceID, bookingID, requestID, eventID, tokenHash[:], accountID).Scan(
		&retainedResource, &retainedBooking, &retainedEvent, &retainedSession); err != nil {
		t.Fatalf("inspect retained migration-004 data: %v", err)
	}
	if retainedResource != 1 || retainedBooking != 1 || retainedEvent != 1 || retainedSession != 1 {
		t.Fatalf("additive migration changed existing records: resource/booking/event/session=%d/%d/%d/%d",
			retainedResource, retainedBooking, retainedEvent, retainedSession)
	}
}

func installResourceUpdateFailureTrigger(t *testing.T, admin *pgxpool.Pool, resourceID string) {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := pgx.Identifier{"labreserve_test_resource_update_block_" + suffix}.Sanitize()
	triggerName := pgx.Identifier{"labreserve_test_resource_update_block_" + suffix}.Sanitize()
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION public.%s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF OLD.id = '%s'::uuid THEN
		        RAISE EXCEPTION 'unexpected Resource UPDATE';
		    END IF;
		    RETURN NEW;
		END;
		$test$`, functionName, resourceID)); err != nil {
		t.Fatalf("create Resource UPDATE detector: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON public.resources"); err != nil {
			t.Errorf("drop Resource UPDATE detector trigger: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS public."+functionName+"()"); err != nil {
			t.Errorf("drop Resource UPDATE detector function: %v", err)
		}
	})
	if _, err := admin.Exec(t.Context(), "CREATE TRIGGER "+triggerName+" BEFORE UPDATE ON public.resources FOR EACH ROW EXECUTE FUNCTION public."+functionName+"()"); err != nil {
		t.Fatalf("install Resource UPDATE detector: %v", err)
	}
}

func installResourceActivityFailureTrigger(t *testing.T, admin *pgxpool.Pool, actorID, action string) func() {
	t.Helper()
	if action != "resource.created" && action != "resource.descriptive_edited" &&
		action != "resource.activated" && action != "resource.deactivated" {
		t.Fatalf("unsupported Resource Activity failure action %q", action)
	}
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := pgx.Identifier{"labreserve_test_resource_event_fail_" + suffix}.Sanitize()
	triggerName := pgx.Identifier{"labreserve_test_resource_event_fail_" + suffix}.Sanitize()
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION public.%s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    IF NEW.actor_account_id = '%s'::uuid AND NEW.action = '%s' THEN
		        RAISE EXCEPTION 'injected Resource Activity failure';
		    END IF;
		    RETURN NEW;
		END;
		$test$`, functionName, actorID, action)); err != nil {
		t.Fatalf("create Resource Activity failure trigger function: %v", err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON public.activity_events"); err != nil {
			t.Errorf("drop Resource Activity failure trigger: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS public."+functionName+"()"); err != nil {
			t.Errorf("drop Resource Activity failure function: %v", err)
		}
	}
	t.Cleanup(cleanup)
	if _, err := admin.Exec(t.Context(), "CREATE TRIGGER "+triggerName+" BEFORE INSERT ON public.activity_events FOR EACH ROW EXECUTE FUNCTION public."+functionName+"()"); err != nil {
		t.Fatalf("install Resource Activity failure trigger: %v", err)
	}
	return cleanup
}
