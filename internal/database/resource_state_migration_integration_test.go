package database

import (
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	resourceCapabilityMigrationVersion = "005_resource_mutations.sql"
	resourceStateMigrationVersion      = "006_resource_state.sql"
)

func readResourceStateMigrationDataEvidence(ctx context.Context, queryer resourceCapabilityQueryer) (string, error) {
	var evidence string
	err := queryer.QueryRow(ctx, `
		SELECT pg_catalog.jsonb_build_object(
			'accounts', COALESCE((SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(account) ORDER BY account.id) FROM public.accounts AS account), '[]'::jsonb),
			'resources', COALESCE((SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(resource) ORDER BY resource.id) FROM public.resources AS resource), '[]'::jsonb),
			'bookings', COALESCE((SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(booking) ORDER BY booking.id) FROM public.bookings AS booking), '[]'::jsonb),
			'events', COALESCE((SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(event) ORDER BY event.id) FROM public.activity_events AS event), '[]'::jsonb),
			'sessions', COALESCE((SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(session) ORDER BY session.token_hash) FROM public.sessions AS session), '[]'::jsonb)
		)::text`).Scan(&evidence)
	return evidence, err
}

func readResourceStateMigrationFixtureEvidence(
	ctx context.Context,
	queryer resourceCapabilityQueryer,
	accountID, ownerID, resourceID, confirmedBookingID, cancelledBookingID string,
	tokenHash []byte,
	activeResourceID string,
) (string, error) {
	var evidence string
	err := queryer.QueryRow(ctx, `
		SELECT pg_catalog.jsonb_build_object(
			'accounts', pg_catalog.jsonb_build_array(
				(SELECT pg_catalog.to_jsonb(account) FROM public.accounts AS account WHERE account.id = $2::uuid),
				(SELECT pg_catalog.to_jsonb(account) FROM public.accounts AS account WHERE account.id = $3::uuid)
			),
			'resources', pg_catalog.jsonb_build_array(
				(SELECT pg_catalog.to_jsonb(resource) FROM public.resources AS resource WHERE resource.id = $1::uuid),
				(SELECT pg_catalog.to_jsonb(resource) FROM public.resources AS resource WHERE resource.id = $7::uuid)
			),
			'bookings', COALESCE((
				SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(booking) ORDER BY booking.id)
				FROM public.bookings AS booking WHERE booking.id IN ($4::uuid, $5::uuid)
			), '[]'::jsonb),
			'events', COALESCE((
				SELECT pg_catalog.jsonb_agg(pg_catalog.to_jsonb(event) ORDER BY event.id)
				FROM public.activity_events AS event
				WHERE event.resource_id IN ($1::uuid, $7::uuid) OR event.booking_id IN ($4::uuid, $5::uuid)
			), '[]'::jsonb),
			'session', (SELECT pg_catalog.to_jsonb(session) FROM public.sessions AS session WHERE session.token_hash = $6)
		)::text`, resourceID, accountID, ownerID, confirmedBookingID, cancelledBookingID, tokenHash, activeResourceID).Scan(&evidence)
	return evidence, err
}

func readResourceStateMigrationCatalogEvidence(ctx context.Context, queryer resourceCapabilityQueryer) (string, error) {
	var evidence string
	err := queryer.QueryRow(ctx, `
		SELECT pg_catalog.jsonb_build_object(
			'indexes', COALESCE((
				SELECT pg_catalog.jsonb_agg(pg_catalog.pg_get_indexdef(indexed.indexrelid) ORDER BY indexed.indexrelid::regclass::text)
				FROM pg_catalog.pg_index AS indexed
			WHERE indexed.indexrelid IN (
				'public.resources_code_case_insensitive_uq'::regclass,
				'public.bookings_confirmed_resource_period_excl'::regclass
			)), '[]'::jsonb),
			'foreign_keys', COALESCE((
				SELECT pg_catalog.jsonb_agg(
					pg_catalog.jsonb_build_object(
						'table', constraint_row.conrelid::regclass::text,
						'name', constraint_row.conname,
						'definition', pg_catalog.pg_get_constraintdef(constraint_row.oid)
					) ORDER BY constraint_row.conrelid::regclass::text, constraint_row.conname
				)
				FROM pg_catalog.pg_constraint AS constraint_row
				WHERE constraint_row.contype = 'f'
				  AND constraint_row.conrelid IN ('public.bookings'::regclass, 'public.activity_events'::regclass, 'public.sessions'::regclass)
			), '[]'::jsonb),
			'prior_capabilities', COALESCE((
				SELECT pg_catalog.jsonb_agg(
					pg_catalog.jsonb_build_object(
						'name', procedure_row.oid::regprocedure::text,
						'definition', pg_catalog.pg_get_functiondef(procedure_row.oid),
						'owner', pg_catalog.pg_get_userbyid(procedure_row.proowner),
						'acl', procedure_row.proacl::text,
						'config', procedure_row.proconfig
					) ORDER BY procedure_row.proname
				)
				FROM pg_catalog.pg_proc AS procedure_row
			JOIN pg_catalog.pg_namespace AS namespace_row ON namespace_row.oid = procedure_row.pronamespace
			WHERE namespace_row.nspname = 'public'
			  AND procedure_row.proname IN (
				'labreserve_create_resource',
				'labreserve_edit_resource',
				'labreserve_lock_booking_resource'
			)
			), '[]'::jsonb)
		)::text`).Scan(&evidence)
	return evidence, err
}

func TestResourceStateMigrationPreservesVersion005Data(t *testing.T) {
	pool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), pool); err != nil {
		t.Fatalf("prepare current schema before Migration 005 upgrade simulation: %v", err)
	}
	migrations, err := readEmbeddedMigrationChecksums()
	if err != nil {
		t.Fatalf("read embedded migration checksums: %v", err)
	}
	var migrationsThrough005 []migrationChecksum
	var stateMigration *migrationChecksum
	for index := range migrations {
		if migrations[index].version == resourceStateMigrationVersion {
			stateMigration = &migrations[index]
			break
		}
		migrationsThrough005 = append(migrationsThrough005, migrations[index])
	}
	if stateMigration == nil || len(migrationsThrough005) != 5 ||
		migrationsThrough005[4].version != resourceCapabilityMigrationVersion || requiredSchemaVersion != resourceStateMigrationVersion {
		t.Fatalf("migration ladder through 005=%v (count %d), state=%v, required=%q; want five prior migrations and required 006",
			len(migrationsThrough005) == 5, len(migrationsThrough005), stateMigration != nil, requiredSchemaVersion)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin disposable additive migration simulation: %v", err)
	}
	defer tx.Rollback(context.Background())

	var accountID, ownerID, resourceID, activeResourceID, confirmedBookingID, cancelledBookingID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.accounts (id, login, display_name, password_hash, role)
		VALUES (gen_random_uuid(), 'state-migration-coordinator-' || replace(gen_random_uuid()::text, '-', '') || '@example.test',
		        'Migration Coordinator', 'unused', 'coordinator')
		RETURNING id::text`).Scan(&accountID); err != nil {
		t.Fatalf("create retained migration Coordinator: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.accounts (id, login, display_name, password_hash, role)
		VALUES (gen_random_uuid(), 'state-migration-engineer-' || replace(gen_random_uuid()::text, '-', '') || '@example.test',
		        'Migration Engineer', 'unused', 'engineer')
		RETURNING id::text`).Scan(&ownerID); err != nil {
		t.Fatalf("create retained migration Engineer: %v", err)
	}
	createdAt := time.Date(2040, time.May, 6, 7, 8, 9, 123456000, time.UTC)
	legacyDescription := "raw  description\u2003"
	activeLegacyDescription := "<b>raw  active text</b>\u2003"
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
		VALUES (gen_random_uuid(), 'STATE-MIGRATION-' || replace(gen_random_uuid()::text, '-', ''),
		        '  Retained legacy Resource  ', $1, FALSE, $2, $2)
		RETURNING id::text`, legacyDescription, createdAt).Scan(&resourceID); err != nil {
		t.Fatalf("create retained migration Resource: %v", err)
	}
	bookingStart := createdAt.Add(24 * time.Hour)
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.resources (id, code, name, description, active, created_at, updated_at)
		VALUES (gen_random_uuid(), 'STATE-MIGRATION-ACTIVE-' || replace(gen_random_uuid()::text, '-', ''),
		        ' Active legacy Resource ', $1, TRUE, $2, $2)
		RETURNING id::text`, activeLegacyDescription, createdAt).Scan(&activeResourceID); err != nil {
		t.Fatalf("create retained active migration Resource: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.bookings (id, resource_id, owner_account_id, request_id, start_at, end_at, purpose, state, created_at)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, gen_random_uuid(), $3::timestamptz, $3::timestamptz + interval '1 hour',
		        'Retained confirmed request', 'confirmed', $4)
		RETURNING id::text`, resourceID, ownerID, bookingStart, createdAt).Scan(&confirmedBookingID); err != nil {
		t.Fatalf("create retained Confirmed Booking: %v", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO public.bookings (
			id, resource_id, owner_account_id, request_id, start_at, end_at, purpose, state, created_at,
			cancelled_by_account_id, cancelled_at
		)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, gen_random_uuid(), $3::timestamptz, $3::timestamptz + interval '1 hour',
		        'Retained cancelled request', 'cancelled', $4, $5::uuid, $4)
		RETURNING id::text`, resourceID, ownerID, bookingStart, createdAt, accountID).Scan(&cancelledBookingID); err != nil {
		t.Fatalf("create retained Cancelled Booking: %v", err)
	}
	for _, event := range []struct {
		actorID  string
		action   string
		targetID string
		resource bool
		details  string
	}{
		{actorID: accountID, action: "resource.created", targetID: resourceID, resource: true, details: `{"retained":"inactive-resource"}`},
		{actorID: accountID, action: "resource.created", targetID: activeResourceID, resource: true, details: `{"retained":"active-resource"}`},
		{actorID: ownerID, action: "booking.created", targetID: confirmedBookingID, details: `{"retained":"confirmed"}`},
		{actorID: ownerID, action: "booking.created", targetID: cancelledBookingID, details: `{"retained":"cancelled-created"}`},
		{actorID: accountID, action: "booking.cancelled", targetID: cancelledBookingID, details: `{"retained":"cancelled-event"}`},
	} {
		var err error
		if event.resource {
			_, err = tx.Exec(ctx, `
				INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, resource_id, details)
				VALUES (gen_random_uuid(), $1::uuid, $2, $3, $4::uuid, $5::jsonb)`, event.actorID, event.action, createdAt, event.targetID, event.details)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO public.activity_events (id, actor_account_id, action, occurred_at, booking_id, details)
				VALUES (gen_random_uuid(), $1::uuid, $2, $3, $4::uuid, $5::jsonb)`, event.actorID, event.action, createdAt, event.targetID, event.details)
		}
		if err != nil {
			t.Fatalf("create retained %s Activity event: %v", event.action, err)
		}
	}
	tokenHash := sha256.Sum256([]byte("state-migration-session-" + testDatabaseUUID(t, pool)))
	csrfToken := sha256.Sum256([]byte("state-migration-csrf-" + testDatabaseUUID(t, pool)))
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.sessions (token_hash, account_id, csrf_token, created_at, expires_at)
		VALUES ($1, $2::uuid, $3, $4, $5)`, tokenHash[:], accountID, csrfToken[:], createdAt, createdAt.Add(time.Hour)); err != nil {
		t.Fatalf("create retained migration session: %v", err)
	}

	dataBefore, err := readResourceStateMigrationFixtureEvidence(ctx, tx, accountID, ownerID, resourceID,
		confirmedBookingID, cancelledBookingID, tokenHash[:], activeResourceID)
	if err != nil {
		t.Fatalf("snapshot retained 005 fixture data before upgrade: %v", err)
	}
	allDataBefore, err := readResourceStateMigrationDataEvidence(ctx, tx)
	if err != nil {
		t.Fatalf("snapshot all retained 005 data before upgrade: %v", err)
	}
	catalogBefore, err := readResourceStateMigrationCatalogEvidence(ctx, tx)
	if err != nil {
		t.Fatalf("snapshot existing schema/capabilities before upgrade: %v", err)
	}

	if _, err := tx.Exec(ctx, "DROP FUNCTION public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ)"); err != nil {
		t.Fatalf("simulate the compatible 005 schema by removing only the new capability: %v", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM public.schema_migrations WHERE version = $1", resourceStateMigrationVersion); err != nil {
		t.Fatalf("simulate the compatible 005 ledger: %v", err)
	}
	if err := validateMigrationLedger(ctx, tx, migrationsThrough005, true); err != nil {
		t.Fatalf("validate compatible Migration 005 ledger before upgrade: %v", err)
	}
	if _, err := tx.Exec(ctx, string(stateMigration.contents)); err != nil {
		t.Fatalf("apply additive Migration 006 to compatible retained 005 data: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO public.schema_migrations (version, checksum) VALUES ($1, $2)`, stateMigration.version, stateMigration.checksum); err != nil {
		t.Fatalf("record additive Migration 006: %v", err)
	}
	if err := validateMigrationLedger(ctx, tx, migrations, true); err != nil {
		t.Fatalf("validate complete 006 ledger after upgrade: %v", err)
	}
	dataAfter, err := readResourceStateMigrationFixtureEvidence(ctx, tx, accountID, ownerID, resourceID,
		confirmedBookingID, cancelledBookingID, tokenHash[:], activeResourceID)
	if err != nil {
		t.Fatalf("snapshot retained 005 fixture data after upgrade: %v", err)
	}
	allDataAfter, err := readResourceStateMigrationDataEvidence(ctx, tx)
	if err != nil {
		t.Fatalf("snapshot all retained 005 data after upgrade: %v", err)
	}
	catalogAfter, err := readResourceStateMigrationCatalogEvidence(ctx, tx)
	if err != nil {
		t.Fatalf("snapshot existing schema/capabilities after upgrade: %v", err)
	}
	if dataAfter != dataBefore || allDataAfter != allDataBefore || catalogAfter != catalogBefore {
		t.Fatalf("Migration 006 changed retained data or existing schema capabilities:\nfixture data before=%s\nfixture data after=%s\nall data before=%s\nall data after=%s\ncatalog before=%s\ncatalog after=%s",
			dataBefore, dataAfter, allDataBefore, allDataAfter, catalogBefore, catalogAfter)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("roll back disposable 005-to-006 migration simulation: %v", err)
	}

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("repeat explicit migration after compatible upgrade simulation: %v", err)
	}
	if err := CheckSchema(ctx, pool); err != nil {
		t.Fatalf("check exact schema after repeat migration: %v", err)
	}
}

func TestResourceStateMigrationRejectsIncompatible005Ledger(t *testing.T) {
	migrationPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), migrationPool); err != nil {
		t.Fatalf("prepare disposable database for incompatible-ledger test: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataBefore, err := readResourceStateMigrationDataEvidence(ctx, migrationPool)
	if err != nil {
		t.Fatalf("snapshot disposable business data before mismatch test: %v", err)
	}

	schema := "state_ledger_" + strings.ReplaceAll(testDatabaseUUID(t, migrationPool), "-", "")
	schemaIdentifier := pgx.Identifier{schema}.Sanitize()
	if _, err := migrationPool.Exec(ctx, "CREATE SCHEMA "+schemaIdentifier); err != nil {
		t.Fatalf("create isolated ledger-test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := migrationPool.Exec(cleanupCtx, "DROP SCHEMA "+schemaIdentifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated ledger-test schema: %v", err)
		}
	})

	poolConfig, err := pgxpool.ParseConfig(os.Getenv("LABRESERVE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse disposable migration database configuration: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schemaIdentifier + ",public"
	upgradePool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open isolated migration-ledger schema: %v", err)
	}
	defer upgradePool.Close()
	if err := upgradePool.Ping(ctx); err != nil {
		t.Fatalf("connect to isolated migration-ledger schema: %v", err)
	}
	if _, err := upgradePool.Exec(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("create disposable incompatible migration ledger: %v", err)
	}
	migrations, err := readEmbeddedMigrationChecksums()
	if err != nil {
		t.Fatalf("read expected migration checksums: %v", err)
	}
	var recordedVersions int
	for _, migration := range migrations {
		if migration.version == resourceStateMigrationVersion {
			break
		}
		checksum := migration.checksum
		if migration.version == resourceCapabilityMigrationVersion {
			checksum = strings.Repeat("0", 64)
		}
		if _, err := upgradePool.Exec(ctx, `
			INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`, migration.version, checksum); err != nil {
			t.Fatalf("prepare isolated historical ledger row %s: %v", migration.version, err)
		}
		recordedVersions++
	}
	if recordedVersions != 5 {
		t.Fatalf("prepared %d historical ledger rows, want 5 (001-005)", recordedVersions)
	}

	if err := Migrate(ctx, upgradePool); err == nil || !strings.Contains(err.Error(), resourceCapabilityMigrationVersion+" checksum changed after application") {
		t.Fatalf("migration runner error for incompatible historical 005 ledger = %v, want refusal before Migration 006", err)
	}
	var ledgerRows, migration006Rows int
	var recorded005 string
	if err := upgradePool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE version = $1),
		       (SELECT checksum FROM schema_migrations WHERE version = $2)
		FROM schema_migrations`, resourceStateMigrationVersion, resourceCapabilityMigrationVersion).Scan(
		&ledgerRows, &migration006Rows, &recorded005); err != nil {
		t.Fatalf("inspect unchanged incompatible migration ledger: %v", err)
	}
	if ledgerRows != 5 || migration006Rows != 0 || recorded005 != strings.Repeat("0", 64) {
		t.Fatalf("migration runner altered mismatch ledger: rows=%d migration006=%d 005_checksum=%q", ledgerRows, migration006Rows, recorded005)
	}
	dataAfter, err := readResourceStateMigrationDataEvidence(ctx, migrationPool)
	if err != nil {
		t.Fatalf("snapshot disposable business data after mismatch refusal: %v", err)
	}
	if dataAfter != dataBefore {
		t.Fatalf("incompatible-ledger refusal changed retained business data:\nbefore=%s\nafter=%s", dataBefore, dataAfter)
	}
}
