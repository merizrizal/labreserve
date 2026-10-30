package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const resourceStateCapabilitySignature = "public.labreserve_set_resource_state(uuid,uuid,boolean,timestamp with time zone)"

func TestResourceStatePrivilegeBoundary(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state privilege migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var currentUser string
	if err := runtime.QueryRow(t.Context(), "SELECT current_user").Scan(&currentUser); err != nil || currentUser != "labreserve_app" {
		t.Fatalf("privilege evidence must use labreserve_app: current_user=%q error=%v", currentUser, err)
	}

	fixtures := newResourceCapabilityFixtures(t, admin)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	operationAt := createdAt.Add(time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Privilege-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Privilege Lab", "Retained", false, createdAt)
	fixtures.trackResource(resourceID)

	assertResourceStatePrivilegeCatalog(t, admin, runtime)

	activated, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, true, operationAt)
	if err != nil || !activated.Changed || !activated.Active || !activated.UpdatedAt.Equal(operationAt) {
		t.Fatalf("authorized runtime capability invocation: result=%+v error=%v", activated, err)
	}
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.activated", operationAt, false, true)

	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	noOp, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, true, operationAt.Add(time.Minute))
	if err != nil || noOp.Changed || !noOp.Active {
		t.Fatalf("matching-state capability no-op: result=%+v error=%v", noOp, err)
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("capability no-op changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}

	for _, actor := range []struct {
		name string
		id   any
	}{
		{name: "Engineer", id: fixtures.engineer},
		{name: "unknown actor", id: fixtures.unknown},
		{name: "null actor", id: nil},
	} {
		t.Run(actor.name+" cannot use matching-state no-op", func(t *testing.T) {
			beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
			if _, err := callSetResourceStateCapability(t.Context(), runtime, actor.id, resourceID, true, operationAt); err == nil {
				t.Fatal("unauthorized actor received matching-state success")
			} else {
				assertResourceCapabilityErrorCode(t, err, "42501")
			}
			if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
				t.Fatalf("unauthorized no-op changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
			}
		})
	}

	blockedID := insertResourceCapabilityFixture(t, admin,
		"State-Privilege-Blocked-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Blocked Lab", "Retained", true, createdAt)
	fixtures.trackResource(blockedID)
	bookingID := insertResourceStateBooking(t, admin, blockedID, fixtures.engineer,
		operationAt.Add(time.Hour), operationAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, bookingID)
	beforeResource, beforeEvents = resourceCapabilitySnapshot(t, admin, blockedID)
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, blockedID, false, operationAt); err == nil {
		t.Fatal("runtime capability deactivated a Resource with a Confirmed future-ending Booking")
	} else {
		assertResourceCapabilityErrorCode(t, err, "P0001")
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, blockedID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("blocked capability call changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
}

func assertResourceStatePrivilegeCatalog(t *testing.T, admin, runtime *pgxpool.Pool) {
	t.Helper()

	var canLogin, isSuperuser, canCreateDatabase, canCreateRole, inherits, migratorMember bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT role.rolcanlogin, role.rolsuper, role.rolcreatedb, role.rolcreaterole, role.rolinherit,
		       pg_catalog.pg_has_role(current_user, 'labreserve_migrator', 'MEMBER')
		FROM pg_catalog.pg_roles AS role WHERE role.rolname = current_user`).Scan(
		&canLogin, &isSuperuser, &canCreateDatabase, &canCreateRole, &inherits, &migratorMember); err != nil {
		t.Fatalf("inspect runtime role attributes: %v", err)
	}
	if !canLogin || isSuperuser || canCreateDatabase || canCreateRole || inherits || migratorMember {
		t.Fatalf("runtime role is not least-privileged: login/superuser/createdb/createrole/inherit/migrator-member=%v/%v/%v/%v/%v/%v",
			canLogin, isSuperuser, canCreateDatabase, canCreateRole, inherits, migratorMember)
	}
	var membershipCount int
	if err := admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_catalog.pg_auth_members WHERE member = 'labreserve_app'::regrole`).Scan(&membershipCount); err != nil {
		t.Fatalf("inspect runtime role memberships: %v", err)
	}
	if membershipCount != 0 {
		t.Fatalf("runtime role has %d direct role memberships", membershipCount)
	}

	var resourceSelect, resourceInsert, resourceUpdate, resourceDelete, resourceTruncate bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.resources', 'SELECT'),
		       has_table_privilege(current_user, 'public.resources', 'INSERT'),
		       has_table_privilege(current_user, 'public.resources', 'UPDATE'),
		       has_table_privilege(current_user, 'public.resources', 'DELETE'),
		       has_table_privilege(current_user, 'public.resources', 'TRUNCATE')`).Scan(
		&resourceSelect, &resourceInsert, &resourceUpdate, &resourceDelete, &resourceTruncate); err != nil {
		t.Fatalf("inspect effective Resource table privileges: %v", err)
	}
	if !resourceSelect || resourceInsert || resourceUpdate || resourceDelete || resourceTruncate {
		t.Fatalf("effective Resource privileges = select/insert/update/delete/truncate %v/%v/%v/%v/%v",
			resourceSelect, resourceInsert, resourceUpdate, resourceDelete, resourceTruncate)
	}

	assertNoRuntimeColumnPrivilege(t, runtime, "resources", "INSERT")
	assertNoRuntimeColumnPrivilege(t, runtime, "resources", "UPDATE")

	for _, relation := range []string{"public.resources", "public.accounts", "public.bookings", "public.activity_events"} {
		var publicWrite bool
		if err := admin.QueryRow(t.Context(), `
			SELECT EXISTS (
				SELECT 1
				FROM pg_catalog.pg_class AS relation
				CROSS JOIN LATERAL pg_catalog.aclexplode(
					COALESCE(relation.relacl, pg_catalog.acldefault('r', relation.relowner))
				) AS acl
				WHERE relation.oid = $1::regclass AND acl.grantee = 0
				  AND acl.privilege_type IN ('INSERT', 'UPDATE', 'DELETE', 'TRUNCATE', 'REFERENCES', 'TRIGGER')
			)`, relation).Scan(&publicWrite); err != nil {
			t.Fatalf("inspect PUBLIC table privileges on %s: %v", relation, err)
		}
		if publicWrite {
			t.Errorf("PUBLIC has write-related privileges on %s", relation)
		}
	}

	assertRuntimeObjectOwnershipAndNoGrantOptions(t, admin)

	var schemaUsage, schemaCreate, publicSchemaPrivilege bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT has_schema_privilege(current_user, 'public', 'USAGE'),
		       has_schema_privilege(current_user, 'public', 'CREATE'),
		       EXISTS (
				SELECT 1 FROM pg_catalog.pg_namespace AS schema
				CROSS JOIN LATERAL pg_catalog.aclexplode(
					COALESCE(schema.nspacl, pg_catalog.acldefault('n', schema.nspowner))
				) AS acl
				WHERE schema.nspname = 'public' AND acl.grantee = 0
				  AND acl.privilege_type IN ('USAGE', 'CREATE')
			)`).Scan(&schemaUsage, &schemaCreate, &publicSchemaPrivilege); err != nil {
		t.Fatalf("inspect effective schema privileges: %v", err)
	}
	if !schemaUsage || schemaCreate || publicSchemaPrivilege {
		t.Fatalf("public schema privileges usage/create/PUBLIC=%v/%v/%v", schemaUsage, schemaCreate, publicSchemaPrivilege)
	}

	var accountSelect, accountInsert, accountUpdate, accountDelete, accountTruncate bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.accounts', 'SELECT'),
		       has_table_privilege(current_user, 'public.accounts', 'INSERT'),
		       has_table_privilege(current_user, 'public.accounts', 'UPDATE'),
		       has_table_privilege(current_user, 'public.accounts', 'DELETE'),
		       has_table_privilege(current_user, 'public.accounts', 'TRUNCATE')`).Scan(
		&accountSelect, &accountInsert, &accountUpdate, &accountDelete, &accountTruncate); err != nil {
		t.Fatalf("inspect effective Account privileges: %v", err)
	}
	if !accountSelect || accountInsert || accountUpdate || accountDelete || accountTruncate {
		t.Fatalf("effective Account privileges select/insert/update/delete/truncate=%v/%v/%v/%v/%v",
			accountSelect, accountInsert, accountUpdate, accountDelete, accountTruncate)
	}
	assertNoRuntimeColumnPrivilege(t, runtime, "accounts", "INSERT")
	assertNoRuntimeColumnPrivilege(t, runtime, "accounts", "UPDATE")

	var bookingSelect, bookingInsert, bookingUpdate, bookingDelete, bookingTruncate bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.bookings', 'SELECT'),
		       has_table_privilege(current_user, 'public.bookings', 'INSERT'),
		       has_table_privilege(current_user, 'public.bookings', 'UPDATE'),
		       has_table_privilege(current_user, 'public.bookings', 'DELETE'),
		       has_table_privilege(current_user, 'public.bookings', 'TRUNCATE')`).Scan(
		&bookingSelect, &bookingInsert, &bookingUpdate, &bookingDelete, &bookingTruncate); err != nil {
		t.Fatalf("inspect effective Booking privileges: %v", err)
	}
	if !bookingSelect || !bookingInsert || bookingUpdate || bookingDelete || bookingTruncate {
		t.Fatalf("effective Booking privileges select/insert/update/delete/truncate=%v/%v/%v/%v/%v",
			bookingSelect, bookingInsert, bookingUpdate, bookingDelete, bookingTruncate)
	}
	bookingColumns, err := runtime.Query(t.Context(), `
		SELECT column_name, has_column_privilege(current_user, 'public.bookings', column_name, 'UPDATE')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'bookings'
		ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("inspect effective Booking column privileges: %v", err)
	}
	bookingColumnCount := 0
	for bookingColumns.Next() {
		var column string
		var canUpdate bool
		if err := bookingColumns.Scan(&column, &canUpdate); err != nil {
			bookingColumns.Close()
			t.Fatalf("scan Booking column privilege: %v", err)
		}
		bookingColumnCount++
		wantUpdate := column == "state" || column == "cancelled_by_account_id" || column == "cancelled_at"
		if canUpdate != wantUpdate {
			t.Errorf("runtime UPDATE privilege on bookings.%s = %v, want %v", column, canUpdate, wantUpdate)
		}
	}
	if err := bookingColumns.Err(); err != nil {
		bookingColumns.Close()
		t.Fatalf("read Booking column privileges: %v", err)
	}
	bookingColumns.Close()
	if bookingColumnCount == 0 {
		t.Fatal("Booking privilege inspection found no columns")
	}

	var activitySelect, activityInsert, activityUpdate, activityDelete, activityTruncate bool
	if err := runtime.QueryRow(t.Context(), `
		SELECT has_table_privilege(current_user, 'public.activity_events', 'SELECT'),
		       has_table_privilege(current_user, 'public.activity_events', 'INSERT'),
		       has_table_privilege(current_user, 'public.activity_events', 'UPDATE'),
		       has_table_privilege(current_user, 'public.activity_events', 'DELETE'),
		       has_table_privilege(current_user, 'public.activity_events', 'TRUNCATE')`).Scan(
		&activitySelect, &activityInsert, &activityUpdate, &activityDelete, &activityTruncate); err != nil {
		t.Fatalf("inspect effective Activity privileges: %v", err)
	}
	if !activitySelect || !activityInsert || activityUpdate || activityDelete || activityTruncate {
		t.Fatalf("effective Activity privileges select/insert/update/delete/truncate=%v/%v/%v/%v/%v",
			activitySelect, activityInsert, activityUpdate, activityDelete, activityTruncate)
	}
	assertNoRuntimeColumnPrivilege(t, runtime, "activity_events", "UPDATE")

	for _, function := range []string{
		"public.labreserve_create_resource(uuid,text,text,text,boolean,timestamp with time zone)",
		"public.labreserve_edit_resource(uuid,uuid,text,text,timestamp with time zone)",
		resourceStateCapabilitySignature,
	} {
		var securityDefiner, ownerSuperuser, runtimeExecute bool
		var owner, searchPath string
		if err := admin.QueryRow(t.Context(), `
			SELECT procedure.prosecdef, owner.rolname, owner.rolsuper,
			       COALESCE(procedure.proconfig[1], ''),
			       has_function_privilege('labreserve_app', procedure.oid, 'EXECUTE')
			FROM pg_catalog.pg_proc AS procedure
			JOIN pg_catalog.pg_roles AS owner ON owner.oid = procedure.proowner
			WHERE procedure.oid = $1::regprocedure`, function).Scan(
			&securityDefiner, &owner, &ownerSuperuser, &searchPath, &runtimeExecute); err != nil {
			t.Fatalf("inspect guarded function %s: %v", function, err)
		}
		if !securityDefiner || owner != "labreserve_migrator" || ownerSuperuser || searchPath != "search_path=pg_catalog" || !runtimeExecute {
			t.Errorf("guarded function %s definer/owner/superuser/path/runtime-execute=%v/%q/%v/%q/%v",
				function, securityDefiner, owner, ownerSuperuser, searchPath, runtimeExecute)
		}
		acl := readResourceStateFunctionACL(t, admin, function)
		wantACL := []string{
			"labreserve_app|EXECUTE|labreserve_migrator|false",
			"labreserve_migrator|EXECUTE|labreserve_migrator|false",
		}
		sort.Strings(wantACL)
		if strings.Join(acl, "\n") != strings.Join(wantACL, "\n") {
			t.Errorf("guarded function %s ACL = %v, want only owner and runtime execute without grant option", function, acl)
		}
	}
	var stateFunctionOverloads int
	if err := admin.QueryRow(t.Context(), `
		SELECT count(*) FROM pg_catalog.pg_proc AS procedure
		JOIN pg_catalog.pg_namespace AS function_schema ON function_schema.oid = procedure.pronamespace
		WHERE function_schema.nspname = 'public' AND procedure.proname = 'labreserve_set_resource_state'`).Scan(&stateFunctionOverloads); err != nil {
		t.Fatalf("count Resource state capability overloads: %v", err)
	}
	if stateFunctionOverloads != 1 {
		t.Fatalf("Resource state capability overload count = %d, want exactly one", stateFunctionOverloads)
	}
}

func assertNoRuntimeColumnPrivilege(t *testing.T, runtime *pgxpool.Pool, table, privilege string) {
	t.Helper()
	rows, err := runtime.Query(t.Context(), `
		SELECT column_name, has_column_privilege(current_user, 'public.' || $1::text, column_name, $2::text)
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1::text
		ORDER BY ordinal_position`, table, privilege)
	if err != nil {
		t.Fatalf("inspect effective %s column privileges on %s: %v", privilege, table, err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var column string
		var granted bool
		if err := rows.Scan(&column, &granted); err != nil {
			t.Fatalf("scan effective %s privilege on %s: %v", privilege, table, err)
		}
		count++
		if granted {
			t.Errorf("runtime role unexpectedly has %s on public.%s.%s", privilege, table, column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read effective %s column privileges on %s: %v", privilege, table, err)
	}
	if count == 0 {
		t.Fatalf("effective %s privilege inspection found no columns on %s", privilege, table)
	}
}

func assertRuntimeObjectOwnershipAndNoGrantOptions(t *testing.T, admin *pgxpool.Pool) {
	t.Helper()
	rows, err := admin.Query(t.Context(), `
		SELECT relation.relname::text, pg_catalog.pg_get_userbyid(relation.relowner)
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS schema ON schema.oid = relation.relnamespace
		WHERE schema.nspname = 'public'
		  AND relation.relname IN ('resources', 'accounts', 'bookings', 'activity_events')`)
	if err != nil {
		t.Fatalf("inspect runtime-facing relation owners: %v", err)
	}
	seen := map[string]bool{}
	for rows.Next() {
		var relation, owner string
		if err := rows.Scan(&relation, &owner); err != nil {
			rows.Close()
			t.Fatalf("scan runtime-facing relation owner: %v", err)
		}
		seen[relation] = true
		if owner != "labreserve_migrator" {
			t.Errorf("runtime-facing relation %s owner = %q, want labreserve_migrator", relation, owner)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("read runtime-facing relation owners: %v", err)
	}
	rows.Close()
	for _, relation := range []string{"resources", "accounts", "bookings", "activity_events"} {
		if !seen[relation] {
			t.Errorf("runtime-facing relation %s was not found for owner inspection", relation)
		}
	}

	grantOptions, err := admin.Query(t.Context(), `
		SELECT object_name, privilege_type FROM (
			SELECT relation.relname::text AS object_name, acl.privilege_type::text AS privilege_type
			FROM pg_catalog.pg_class AS relation
			JOIN pg_catalog.pg_namespace AS schema ON schema.oid = relation.relnamespace
			CROSS JOIN LATERAL pg_catalog.aclexplode(
				COALESCE(relation.relacl, pg_catalog.acldefault('r', relation.relowner))
			) AS acl
			WHERE schema.nspname = 'public'
			  AND relation.relname IN ('resources', 'accounts', 'bookings', 'activity_events')
			  AND (acl.grantee = 0 OR acl.grantee = 'labreserve_app'::regrole)
			  AND acl.is_grantable
			UNION ALL
			SELECT relation.relname::text || '.' || attribute.attname::text, acl.privilege_type::text
			FROM pg_catalog.pg_attribute AS attribute
			JOIN pg_catalog.pg_class AS relation ON relation.oid = attribute.attrelid
			JOIN pg_catalog.pg_namespace AS schema ON schema.oid = relation.relnamespace
			CROSS JOIN LATERAL pg_catalog.aclexplode(attribute.attacl) AS acl
			WHERE schema.nspname = 'public'
			  AND relation.relname IN ('resources', 'accounts', 'bookings', 'activity_events')
			  AND attribute.attnum > 0 AND NOT attribute.attisdropped
			  AND (acl.grantee = 0 OR acl.grantee = 'labreserve_app'::regrole)
			  AND acl.is_grantable
			UNION ALL
			SELECT schema.nspname::text, acl.privilege_type::text
			FROM pg_catalog.pg_namespace AS schema
			CROSS JOIN LATERAL pg_catalog.aclexplode(
				COALESCE(schema.nspacl, pg_catalog.acldefault('n', schema.nspowner))
			) AS acl
			WHERE schema.nspname = 'public'
			  AND (acl.grantee = 0 OR acl.grantee = 'labreserve_app'::regrole)
			  AND acl.is_grantable
		) AS grants ORDER BY object_name, privilege_type`)
	if err != nil {
		t.Fatalf("inspect runtime and PUBLIC grant options: %v", err)
	}
	defer grantOptions.Close()
	for grantOptions.Next() {
		var object, privilege string
		if err := grantOptions.Scan(&object, &privilege); err != nil {
			t.Fatalf("scan runtime grant option: %v", err)
		}
		t.Errorf("runtime role or PUBLIC has grant option %s on %s", privilege, object)
	}
	if err := grantOptions.Err(); err != nil {
		t.Fatalf("read runtime and PUBLIC grant options: %v", err)
	}
}

func readResourceStateFunctionACL(t *testing.T, admin *pgxpool.Pool, signature string) []string {
	t.Helper()
	rows, err := admin.Query(t.Context(), `
		SELECT CASE WHEN acl.grantee = 0 THEN 'PUBLIC' ELSE acl.grantee::regrole::text END,
		       acl.privilege_type, acl.grantor::regrole::text, acl.is_grantable
	FROM pg_catalog.pg_proc AS procedure
	CROSS JOIN LATERAL pg_catalog.aclexplode(
		COALESCE(procedure.proacl, pg_catalog.acldefault('f', procedure.proowner))
	) AS acl
	WHERE procedure.oid = $1::regprocedure`, signature)
	if err != nil {
		t.Fatalf("read ACL for %s: %v", signature, err)
	}
	defer rows.Close()
	var acl []string
	for rows.Next() {
		var grantee, privilege, grantor string
		var grantable bool
		if err := rows.Scan(&grantee, &privilege, &grantor, &grantable); err != nil {
			t.Fatalf("scan ACL for %s: %v", signature, err)
		}
		acl = append(acl, fmt.Sprintf("%s|%s|%s|%v", grantee, privilege, grantor, grantable))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read ACL rows for %s: %v", signature, err)
	}
	sort.Strings(acl)
	return acl
}

func TestResourceStatePrivilegeDeniedOperationsUseRuntimeRole(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state privilege migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var currentUser string
	if err := runtime.QueryRow(t.Context(), "SELECT current_user").Scan(&currentUser); err != nil || currentUser != "labreserve_app" {
		t.Fatalf("denial evidence must use labreserve_app: current_user=%q error=%v", currentUser, err)
	}
	fixtures := newResourceCapabilityFixtures(t, admin)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	createdAt := time.Date(2040, time.February, 3, 4, 5, 6, 0, time.UTC)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Denial-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Denial Lab", "Retained", true, createdAt)
	fixtures.trackResource(resourceID)
	bookingID := insertResourceStateBooking(t, admin, resourceID, fixtures.engineer,
		createdAt.Add(time.Hour), createdAt.Add(2*time.Hour), createdAt, "confirmed", "")
	*bookingIDs = append(*bookingIDs, bookingID)
	if _, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, true, createdAt.Add(time.Minute)); err != nil {
		t.Fatalf("prepare Activity row using guarded runtime capability: %v", err)
	}

	columns, err := runtime.Query(t.Context(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'resources' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("discover Resource columns for actual denied UPDATEs: %v", err)
	}
	var resourceColumnNames []string
	for columns.Next() {
		var column string
		if err := columns.Scan(&column); err != nil {
			columns.Close()
			t.Fatalf("scan Resource column name: %v", err)
		}
		resourceColumnNames = append(resourceColumnNames, column)
	}
	if err := columns.Err(); err != nil {
		columns.Close()
		t.Fatalf("read Resource column names: %v", err)
	}
	columns.Close()
	if len(resourceColumnNames) == 0 {
		t.Fatal("Resource UPDATE denial discovery found no columns")
	}

	for _, column := range resourceColumnNames {
		quoted := pgx.Identifier{column}.Sanitize()
		assertRuntimePrivilegeDenied(t, runtime,
			fmt.Sprintf("UPDATE public.resources SET %s = %s WHERE id = $1::uuid", quoted, quoted), resourceID)
	}
	for _, operation := range []struct {
		name      string
		statement string
		args      []any
	}{
		{name: "Resource INSERT", statement: `INSERT INTO public.resources (id, code, name, description) VALUES (gen_random_uuid(), 'runtime-denied', 'Denied', '')`},
		{name: "Resource DELETE", statement: `DELETE FROM public.resources WHERE id = $1::uuid`, args: []any{resourceID}},
		{name: "Resource TRUNCATE", statement: `TRUNCATE public.resources`},
		{name: "Account INSERT", statement: `INSERT INTO public.accounts (id, login, display_name, password_hash, role) VALUES (gen_random_uuid(), 'runtime-denied@example.test', 'Denied', 'unused', 'engineer')`},
		{name: "Account UPDATE", statement: `UPDATE public.accounts SET role = role WHERE id = $1::uuid`, args: []any{fixtures.coordinator}},
		{name: "Booking immutable UPDATE", statement: `UPDATE public.bookings SET purpose = purpose WHERE id = $1::uuid`, args: []any{bookingID}},
		{name: "Activity UPDATE", statement: `UPDATE public.activity_events SET action = action WHERE resource_id = $1::uuid`, args: []any{resourceID}},
		{name: "Activity DELETE", statement: `DELETE FROM public.activity_events WHERE resource_id = $1::uuid`, args: []any{resourceID}},
		{name: "Activity TRUNCATE", statement: `TRUNCATE public.activity_events`},
		{name: "SET ROLE migrator", statement: `SET LOCAL ROLE labreserve_migrator`},
		{name: "CREATE SCHEMA", statement: `CREATE SCHEMA labreserve_runtime_denied`},
		{name: "CREATE TABLE", statement: `CREATE TABLE public.labreserve_runtime_denied (id integer)`},
		{name: "CREATE OR REPLACE state capability", statement: `CREATE OR REPLACE FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) RETURNS TABLE(resource_id UUID, code TEXT, name TEXT, description TEXT, active BOOLEAN, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, changed BOOLEAN) LANGUAGE SQL AS $$ SELECT NULL::UUID, NULL::TEXT, NULL::TEXT, NULL::TEXT, NULL::BOOLEAN, NULL::TIMESTAMPTZ, NULL::TIMESTAMPTZ, NULL::BOOLEAN $$`},
		{name: "ALTER state capability", statement: `ALTER FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) RESET search_path`},
		{name: "change state capability owner", statement: `ALTER FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) OWNER TO labreserve_app`},
	} {
		t.Run(operation.name, func(t *testing.T) {
			assertRuntimePrivilegeDenied(t, runtime, operation.statement, operation.args...)
		})
	}
}

func assertRuntimePrivilegeDenied(t *testing.T, runtime *pgxpool.Pool, statement string, args ...any) {
	t.Helper()
	tx, err := runtime.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin isolated privilege denial check: %v", err)
	}
	_, operationErr := tx.Exec(t.Context(), statement, args...)
	rollbackErr := tx.Rollback(t.Context())
	if operationErr == nil {
		t.Fatalf("runtime role executed forbidden operation: %s", statement)
	}
	if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		t.Fatalf("roll back isolated privilege denial check: %v", rollbackErr)
	}
	var postgresErr *pgconn.PgError
	if !errors.As(operationErr, &postgresErr) || postgresErr.Code != "42501" {
		t.Fatalf("forbidden operation error = %v, want insufficient_privilege SQLSTATE 42501", operationErr)
	}
}

func TestResourceStatePrivilegeExecuteRevocationRestoresExactACL(t *testing.T) {
	admin := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), admin); err != nil {
		t.Fatalf("apply Resource state privilege migration: %v", err)
	}
	runtime := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	var currentUser string
	if err := runtime.QueryRow(t.Context(), "SELECT current_user").Scan(&currentUser); err != nil || currentUser != "labreserve_app" {
		t.Fatalf("revocation evidence must use labreserve_app: current_user=%q error=%v", currentUser, err)
	}
	fixtures := newResourceCapabilityFixtures(t, admin)
	createdAt := time.Date(2040, time.March, 4, 5, 6, 7, 0, time.UTC)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"State-Execute-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""), "Execute Lab", "Retained", true, createdAt)
	fixtures.trackResource(resourceID)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	lockConn, err := admin.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire serialized ACL-fixture connection: %v", err)
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(t.Context(), "SELECT pg_catalog.pg_advisory_lock(404, 5)"); err != nil {
		t.Fatalf("serialize state capability ACL mutation: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := lockConn.Exec(ctx, "SELECT pg_catalog.pg_advisory_unlock(404, 5)"); err != nil {
			t.Errorf("release state capability ACL fixture lock: %v", err)
		}
	}()

	originalACL := readResourceStateFunctionACL(t, admin, resourceStateCapabilitySignature)
	wantACL := []string{
		"labreserve_app|EXECUTE|labreserve_migrator|false",
		"labreserve_migrator|EXECUTE|labreserve_migrator|false",
	}
	sort.Strings(wantACL)
	if strings.Join(originalACL, "\n") != strings.Join(wantACL, "\n") {
		t.Fatalf("refuse to modify unexpected state capability ACL: %v", originalACL)
	}

	aclChangeAttempted := true
	restoreACL := func() error {
		if !aclChangeAttempted {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "GRANT EXECUTE ON FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) TO labreserve_app"); err != nil {
			return fmt.Errorf("restore state capability EXECUTE grant: %w", err)
		}
		restoredACL := readResourceStateFunctionACL(t, admin, resourceStateCapabilitySignature)
		if strings.Join(restoredACL, "\n") != strings.Join(originalACL, "\n") {
			return fmt.Errorf("restored state capability ACL = %v, original = %v", restoredACL, originalACL)
		}
		aclChangeAttempted = false
		return nil
	}
	t.Cleanup(func() {
		if err := restoreACL(); err != nil {
			t.Errorf("restore exact state capability ACL during cleanup: %v", err)
		}
	})
	defer func() {
		if err := restoreACL(); err != nil {
			t.Errorf("restore exact state capability ACL before releasing serialization lock: %v", err)
		}
	}()
	if _, err := admin.Exec(t.Context(), "REVOKE EXECUTE ON FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ) FROM labreserve_app"); err != nil {
		t.Fatalf("temporarily revoke runtime state capability EXECUTE: %v", err)
	}
	withoutRuntimeACL := readResourceStateFunctionACL(t, admin, resourceStateCapabilitySignature)
	if strings.Contains(strings.Join(withoutRuntimeACL, "\n"), "labreserve_app|EXECUTE|") {
		t.Fatalf("runtime EXECUTE remained after temporary revoke: %v", withoutRuntimeACL)
	}

	freshRuntime, err := pgx.Connect(t.Context(), os.Getenv("LABRESERVE_TEST_APP_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open fresh actual runtime connection while EXECUTE is revoked: %v", err)
	}
	var freshUser string
	if err := freshRuntime.QueryRow(t.Context(), "SELECT current_user").Scan(&freshUser); err != nil || freshUser != "labreserve_app" {
		_ = freshRuntime.Close(context.Background())
		t.Fatalf("fresh revocation connection is not labreserve_app: current_user=%q error=%v", freshUser, err)
	}
	_, invocationErr := callSetResourceStateCapability(t.Context(), freshRuntime, fixtures.coordinator, resourceID, false, createdAt.Add(time.Hour))
	closeErr := freshRuntime.Close(context.Background())
	if closeErr != nil {
		t.Errorf("close fresh runtime connection: %v", closeErr)
	}
	if invocationErr == nil {
		t.Fatal("fresh runtime connection invoked the state capability without EXECUTE")
	}
	assertResourceCapabilityErrorCode(t, invocationErr, "42501")
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("ungranted state capability invocation changed Resource/Activity: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}

	if err := restoreACL(); err != nil {
		t.Fatal(err)
	}
	restored, err := callSetResourceStateCapability(t.Context(), runtime, fixtures.coordinator, resourceID, false, createdAt.Add(time.Hour))
	if err != nil || !restored.Changed || restored.Active {
		t.Fatalf("state capability after exact ACL restoration: result=%+v error=%v", restored, err)
	}
}
