package database

import (
	"context"
	"errors"
	"os"
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
	if activityTablePresent {
		t.Fatal("foundation must not create speculative activity-event schema")
	}

	runtimePool := testPool(t, "LABRESERVE_TEST_APP_DATABASE_URL")
	if err := CheckSchema(ctx, runtimePool); err != nil {
		t.Fatalf("runtime role cannot read schema ledger: %v", err)
	}
	if _, err := runtimePool.Exec(ctx, "UPDATE resources SET active = active"); err == nil {
		t.Fatal("runtime role unexpectedly has resource mutation permission")
	}
	if _, err := runtimePool.Exec(ctx, "CREATE TABLE unauthorized_table (id INT)"); err == nil {
		t.Fatal("runtime role unexpectedly has DDL permission")
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
