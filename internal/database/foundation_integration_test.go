package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFoundationMigrationSeedAndPrivileges(t *testing.T) {
	migrationPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Migrate(ctx, migrationPool); err != nil {
		t.Fatalf("repeat migrations: %v", err)
	}
	if err := CheckSchema(ctx, migrationPool); err != nil {
		t.Fatalf("check schema: %v", err)
	}

	initialHashes := map[string]string{
		"alex@example.test":   "initial-alex-hash",
		"sam@example.test":    "initial-sam-hash",
		"jordan@example.test": "initial-jordan-hash",
	}
	if err := Seed(ctx, migrationPool, initialHashes); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	var originalHash string
	if err := migrationPool.QueryRow(ctx, "SELECT password_hash FROM accounts WHERE login = 'alex@example.test'").Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, migrationPool, map[string]string{
		"alex@example.test":   "replacement-alex-hash",
		"sam@example.test":    "replacement-sam-hash",
		"jordan@example.test": "replacement-jordan-hash",
	}); err != nil {
		t.Fatalf("repeat seed: %v", err)
	}
	var accountCount, resourceCount int
	if err := migrationPool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE login IN
		('alex@example.test', 'sam@example.test', 'jordan@example.test')`).Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if err := migrationPool.QueryRow(ctx, `SELECT count(*) FROM resources WHERE lower(code) IN
		('net-01', 'k8s-01', 'demo-01')`).Scan(&resourceCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 3 || resourceCount != 3 {
		t.Fatalf("repeat seed counts: accounts=%d resources=%d, want 3 each", accountCount, resourceCount)
	}
	var retainedHash string
	if err := migrationPool.QueryRow(ctx, "SELECT password_hash FROM accounts WHERE login = 'alex@example.test'").Scan(&retainedHash); err != nil {
		t.Fatal(err)
	}
	if retainedHash != originalHash {
		t.Fatal("repeat seed overwrote an existing demo credential")
	}

	var originalCode, originalName, originalDescription string
	var originalActive bool
	if err := migrationPool.QueryRow(ctx, `
		SELECT code, name, description, active FROM resources WHERE lower(code) = 'net-01'
	`).Scan(&originalCode, &originalName, &originalDescription, &originalActive); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = migrationPool.Exec(context.Background(), `
			UPDATE resources SET code = $1, name = $2, description = $3, active = $4
			WHERE lower(code) = 'net-01'`, originalCode, originalName, originalDescription, originalActive)
	}()
	if _, err := migrationPool.Exec(ctx, `
		UPDATE resources SET code = 'net-01', name = 'preserved-name', description = 'preserved-description', active = FALSE
		WHERE lower(code) = 'net-01'`); err != nil {
		t.Fatalf("prepare case-variant seed fixture: %v", err)
	}
	if err := Seed(ctx, migrationPool, initialHashes); err != nil {
		t.Fatalf("seed case variant: %v", err)
	}
	var keptCode, keptName string
	var keptActive bool
	if err := migrationPool.QueryRow(ctx, `
		SELECT code, name, active FROM resources WHERE lower(code) = 'net-01'
	`).Scan(&keptCode, &keptName, &keptActive); err != nil {
		t.Fatal(err)
	}
	if keptCode != "net-01" || keptName != "preserved-name" || keptActive {
		t.Fatalf("seed changed a case-variant existing resource: code=%q name=%q active=%v", keptCode, keptName, keptActive)
	}

	for _, code := range []string{"NET-01", "net-01", "Net-01"} {
		_, err := migrationPool.Exec(ctx, `
			INSERT INTO resources (id, code, name, description) VALUES (gen_random_uuid(), $1, 'duplicate', '')`, code)
		var postgresErr *pgconn.PgError
		if !errors.As(err, &postgresErr) || postgresErr.Code != "23505" {
			t.Errorf("inserting duplicate case-insensitive code %q: got %v, want unique violation", code, err)
		}
	}

	var activityTablePresent bool
	if err := migrationPool.QueryRow(ctx, `
		SELECT to_regclass('public.activity_events') IS NOT NULL
	`).Scan(&activityTablePresent); err != nil {
		t.Fatal(err)
	}
	if !activityTablePresent {
		t.Fatal("booking migration did not create activity-event persistence")
	}
	var activityCount int
	if err := migrationPool.QueryRow(ctx, "SELECT count(*) FROM activity_events").Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 0 {
		t.Fatalf("foundation seeding created %d product activity events, want none", activityCount)
	}

	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	if err := CheckSchema(ctx, runtimePool); err != nil {
		t.Fatalf("runtime role cannot read schema ledger: %v", err)
	}
	var canReadBookings, canInsertBookings, canReadEvents, canInsertEvents bool
	if err := runtimePool.QueryRow(ctx, `
		SELECT has_table_privilege(current_user, 'bookings', 'SELECT'),
		       has_table_privilege(current_user, 'bookings', 'INSERT'),
		       has_table_privilege(current_user, 'activity_events', 'SELECT'),
		       has_table_privilege(current_user, 'activity_events', 'INSERT')
	`).Scan(&canReadBookings, &canInsertBookings, &canReadEvents, &canInsertEvents); err != nil {
		t.Fatal(err)
	}
	if !canReadBookings || !canInsertBookings || !canReadEvents || !canInsertEvents {
		t.Fatalf("runtime booking/activity privileges: bookings=%v/%v events=%v/%v", canReadBookings, canInsertBookings, canReadEvents, canInsertEvents)
	}
	if _, err := runtimePool.Exec(ctx, "UPDATE bookings SET purpose = purpose"); err == nil {
		t.Fatal("runtime role unexpectedly has booking update permission")
	}
	if _, err := runtimePool.Exec(ctx, "UPDATE activity_events SET action = action"); err == nil {
		t.Fatal("runtime role unexpectedly has activity-event update permission")
	}
	if _, err := runtimePool.Exec(ctx, "DELETE FROM activity_events"); err == nil {
		t.Fatal("runtime role unexpectedly has activity-event delete permission")
	}
	if _, err := runtimePool.Exec(ctx, "UPDATE resources SET active = active"); err == nil {
		t.Fatal("runtime role unexpectedly has resource mutation permission")
	}
	if _, err := runtimePool.Exec(ctx, "CREATE TABLE unauthorized_table (id INT)"); err == nil {
		t.Fatal("runtime role unexpectedly has DDL permission")
	}
}

func TestCheckSchemaRequiresExactMigrationLedger(t *testing.T) {
	pool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("prepare migration ledger: %v", err)
	}
	if err := CheckSchema(ctx, pool); err != nil {
		t.Fatalf("exact migration state should be accepted: %v", err)
	}

	t.Run("missing required migration", func(t *testing.T) {
		t.Cleanup(func() { restoreExpectedMigrationLedger(t, pool) })
		if _, err := pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", requiredSchemaVersion); err != nil {
			t.Fatal(err)
		}
		if err := CheckSchema(ctx, pool); err == nil || !strings.Contains(err.Error(), "database schema is missing "+requiredSchemaVersion) {
			t.Fatalf("startup schema check error = %v, want missing required migration", err)
		}
	})

	t.Run("unknown newer migration", func(t *testing.T) {
		t.Cleanup(func() { restoreExpectedMigrationLedger(t, pool) })
		if _, err := pool.Exec(ctx, `
			INSERT INTO schema_migrations (version, checksum)
			VALUES ('999_incompatible_schema.sql', repeat('0', 64))`); err != nil {
			t.Fatal(err)
		}
		if err := CheckSchema(ctx, pool); err == nil || !strings.Contains(err.Error(), "unknown migration 999_incompatible_schema.sql") {
			t.Fatalf("startup schema check error = %v, want unknown migration rejection", err)
		}
	})

	t.Run("known checksum mismatch", func(t *testing.T) {
		t.Cleanup(func() { restoreExpectedMigrationLedger(t, pool) })
		if _, err := pool.Exec(ctx, "UPDATE schema_migrations SET checksum = repeat('0', 64) WHERE version = $1", requiredSchemaVersion); err != nil {
			t.Fatal(err)
		}
		if err := CheckSchema(ctx, pool); err == nil || !strings.Contains(err.Error(), "checksum changed after application") {
			t.Fatalf("startup schema check error = %v, want checksum mismatch rejection", err)
		}
	})
}

func restoreExpectedMigrationLedger(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	migrations, err := readEmbeddedMigrationChecksums()
	if err != nil {
		t.Errorf("read expected migration ledger for cleanup: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("begin migration-ledger cleanup: %v", err)
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations"); err != nil {
		t.Errorf("clear migration ledger during cleanup: %v", err)
		return
	}
	for _, migration := range migrations {
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", migration.version, migration.checksum); err != nil {
			t.Errorf("restore migration %s during cleanup: %v", migration.version, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("commit migration-ledger cleanup: %v", err)
	}
}

func TestConcurrentCaseInsensitiveResourceCodeUniqueness(t *testing.T) {
	pool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("prepare resource schema: %v", err)
	}

	var code string
	if err := pool.QueryRow(ctx, `
		SELECT 'RACE-' || upper(substr(replace(gen_random_uuid()::text, '-', ''), 1, 16))`).Scan(&code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM resources WHERE lower(code) = lower($1)", code); err != nil {
			t.Errorf("remove concurrent uniqueness fixture: %v", err)
		}
	})

	firstConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer firstConn.Release()
	firstTx, err := firstConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer firstTx.Rollback(context.Background())

	secondConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConn.Release()
	secondTx, err := secondConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer secondTx.Rollback(context.Background())

	var firstPID, secondPID int32
	if err := firstConn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := secondConn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&secondPID); err != nil {
		t.Fatal(err)
	}
	if firstPID == secondPID {
		t.Fatal("concurrent inserts did not use independent PostgreSQL connections")
	}
	if _, err := firstTx.Exec(ctx, `
		INSERT INTO resources (id, code, name, description)
		VALUES (gen_random_uuid(), $1, 'concurrent uniqueness fixture', '')`, code); err != nil {
		t.Fatalf("insert first resource code: %v", err)
	}

	secondResult := make(chan error, 1)
	go func() {
		_, err := secondTx.Exec(ctx, `
			INSERT INTO resources (id, code, name, description)
			VALUES (gen_random_uuid(), $1, 'concurrent uniqueness fixture', '')`, strings.ToLower(code))
		secondResult <- err
	}()

	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer waitCancel()
	if err := waitForPostgresBlock(waitCtx, pool, secondPID); err != nil {
		_ = firstTx.Rollback(context.Background())
		select {
		case insertErr := <-secondResult:
			_ = secondTx.Rollback(context.Background())
			t.Fatalf("second case variant did not wait on the uncommitted unique key: %v (insert result: %v)", err, insertErr)
		case <-time.After(5 * time.Second):
			t.Fatalf("second case variant did not finish after releasing the first transaction: %v", err)
		}
	}

	if err := firstTx.Commit(ctx); err != nil {
		t.Fatalf("commit first case variant: %v", err)
	}
	var secondErr error
	select {
	case secondErr = <-secondResult:
	case <-ctx.Done():
		t.Fatalf("second insert did not finish after the first transaction committed: %v", ctx.Err())
	}
	var postgresErr *pgconn.PgError
	if !errors.As(secondErr, &postgresErr) || postgresErr.Code != "23505" {
		t.Fatalf("second case variant error = %v, want PostgreSQL unique violation", secondErr)
	}
	if err := secondTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback rejected second insert: %v", err)
	}

	var retained int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM resources WHERE lower(code) = lower($1)", code).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("concurrent case variants retained %d resource rows, want exactly one", retained)
	}
}

func waitForPostgresBlock(ctx context.Context, pool *pgxpool.Pool, pid int32) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, "SELECT cardinality(pg_blocking_pids($1)) > 0", pid).Scan(&blocked); err != nil {
			return fmt.Errorf("observe concurrent unique-index wait: %w", err)
		}
		if blocked {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func testPool(t *testing.T, environment string) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv(environment)
	if databaseURL == "" {
		t.Skipf("%s is not configured; use make verify for real PostgreSQL verification", environment)
	}
	parsed, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", environment, err)
	}
	if parsed.ConnConfig.Database != "labreserve_test" {
		t.Fatalf("refusing integration test database %q; expected labreserve_test", parsed.ConnConfig.Database)
	}
	pool, err := Open(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("open %s: %v", environment, err)
	}
	t.Cleanup(pool.Close)
	return pool
}
