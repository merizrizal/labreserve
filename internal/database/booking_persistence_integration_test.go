package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const insertBookingSQL = `
	INSERT INTO bookings (
		id, resource_id, owner_account_id, request_id, start_at, end_at,
		purpose, state, created_at, cancelled_by_account_id, cancelled_at
	) VALUES (
		gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
	)`

func TestBookingStaticConstraintsAndPersistence(t *testing.T) {
	pool := bookingTestPool(t)
	fixtures := newBookingTestFixtures(t, pool, 2, 2)
	start := bookingTestStart()

	persisted := fixtures.booking(t, 0, 0, start)
	if err := fixtures.insertBooking(t.Context(), persisted); err != nil {
		t.Fatalf("insert valid booking: %v", err)
	}
	var id, resourceID, ownerID, requestID, purpose, state string
	var gotStart, gotEnd, createdAt time.Time
	if err := pool.QueryRow(t.Context(), `
		SELECT id::text, resource_id::text, owner_account_id::text, request_id::text,
		       start_at, end_at, purpose, state, created_at
		FROM bookings WHERE owner_account_id = $1 AND request_id = $2
	`, persisted.ownerID, persisted.requestID).Scan(
		&id, &resourceID, &ownerID, &requestID, &gotStart, &gotEnd, &purpose, &state, &createdAt); err != nil {
		t.Fatalf("read persisted booking: %v", err)
	}
	if id == "" || resourceID != persisted.resourceID || ownerID != persisted.ownerID || requestID != persisted.requestID ||
		!gotStart.Equal(persisted.startAt) || !gotEnd.Equal(persisted.endAt) || purpose != persisted.purpose ||
		state != persisted.state || !createdAt.Equal(persisted.createdAt) {
		t.Fatalf("persisted booking differs from input: id=%q resource=%q owner=%q request=%q start=%v end=%v purpose=%q state=%q created=%v",
			id, resourceID, ownerID, requestID, gotStart, gotEnd, purpose, state, createdAt)
	}

	for _, boundary := range []struct {
		name  string
		start time.Time
		end   time.Time
		text  string
	}{
		{name: "minimum duration", start: start.Add(24 * time.Hour), end: start.Add(24*time.Hour + 30*time.Minute), text: "x"},
		{name: "maximum duration", start: start.Add(48 * time.Hour), end: start.Add(56 * time.Hour), text: strings.Repeat("🙂", 200)},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			input := fixtures.booking(t, 0, 0, boundary.start)
			input.endAt = boundary.end
			input.purpose = boundary.text
			if err := fixtures.insertBooking(t.Context(), input); err != nil {
				t.Fatalf("insert accepted boundary booking: %v", err)
			}
		})
	}

	invalidCases := []struct {
		name       string
		constraint string
		mutate     func(*bookingTestInput)
	}{
		{name: "end equals start", constraint: "bookings_interval_duration_check", mutate: func(input *bookingTestInput) { input.endAt = input.startAt }},
		{name: "end before start", constraint: "bookings_interval_duration_check", mutate: func(input *bookingTestInput) { input.endAt = input.startAt.Add(-time.Minute) }},
		{name: "duration below minimum", constraint: "bookings_interval_duration_check", mutate: func(input *bookingTestInput) { input.endAt = input.startAt.Add(29 * time.Minute) }},
		{name: "duration above maximum", constraint: "bookings_interval_duration_check", mutate: func(input *bookingTestInput) { input.endAt = input.startAt.Add(8*time.Hour + time.Second) }},
		{name: "empty purpose", constraint: "bookings_purpose_length_check", mutate: func(input *bookingTestInput) { input.purpose = "" }},
		{name: "purpose over 200 code points", constraint: "bookings_purpose_length_check", mutate: func(input *bookingTestInput) { input.purpose = strings.Repeat("界", 201) }},
		{name: "invalid state", mutate: func(input *bookingTestInput) { input.state = "past" }},
		{name: "cancelled without metadata", constraint: "bookings_cancellation_metadata_check", mutate: func(input *bookingTestInput) { input.state = "cancelled" }},
		{name: "confirmed with cancellation metadata", constraint: "bookings_cancellation_metadata_check", mutate: func(input *bookingTestInput) {
			input.cancelledByAccountID = stringPointer(fixtures.accounts[0])
			cancelledAt := input.startAt.Add(-time.Hour)
			input.cancelledAt = &cancelledAt
		}},
	}
	for index, invalid := range invalidCases {
		t.Run(invalid.name, func(t *testing.T) {
			input := fixtures.booking(t, 0, 0, start.Add(time.Duration(index+10)*24*time.Hour))
			invalid.mutate(&input)
			err := fixtures.insertBooking(t.Context(), input)
			requirePostgresConstraintError(t, err, "23514", invalid.constraint)
		})
	}

	t.Run("unknown owner foreign key", func(t *testing.T) {
		input := fixtures.booking(t, 0, 0, start.Add(30*24*time.Hour))
		input.ownerID = testDatabaseUUID(t, pool)
		requirePostgresConstraintError(t, fixtures.insertBooking(t.Context(), input), "23503", "bookings_owner_account_id_fkey")
	})
	t.Run("unknown resource foreign key", func(t *testing.T) {
		input := fixtures.booking(t, 0, 0, start.Add(31*24*time.Hour))
		input.resourceID = testDatabaseUUID(t, pool)
		requirePostgresConstraintError(t, fixtures.insertBooking(t.Context(), input), "23503", "bookings_resource_id_fkey")
	})
	t.Run("request identity is unique across states and scoped to owner", func(t *testing.T) {
		requestID := testDatabaseUUID(t, pool)
		first := fixtures.booking(t, 0, 0, start.Add(40*24*time.Hour))
		first.requestID = requestID
		if err := fixtures.insertBooking(t.Context(), first); err != nil {
			t.Fatalf("insert first request identity: %v", err)
		}

		duplicate := fixtures.booking(t, 1, 0, first.startAt)
		duplicate.requestID = requestID
		duplicate.state = "cancelled"
		duplicate.cancelledByAccountID = stringPointer(fixtures.accounts[0])
		cancelledAt := first.startAt.Add(-time.Hour)
		duplicate.cancelledAt = &cancelledAt
		requirePostgresConstraintError(t, fixtures.insertBooking(t.Context(), duplicate), "23505", "bookings_owner_request_id_uq")

		otherOwner := fixtures.booking(t, 1, 1, first.startAt)
		otherOwner.requestID = requestID
		if err := fixtures.insertBooking(t.Context(), otherOwner); err != nil {
			t.Fatalf("same request identity for another owner: %v", err)
		}
	})
}

func TestBookingOverlapExclusionConstraint(t *testing.T) {
	pool := bookingTestPool(t)
	fixtures := newBookingTestFixtures(t, pool, 2, 2)
	start := bookingTestStart()
	base := fixtures.booking(t, 0, 0, start)
	if err := fixtures.insertBooking(t.Context(), base); err != nil {
		t.Fatalf("insert overlap baseline: %v", err)
	}

	conflicts := []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "partial overlap", start: start.Add(30 * time.Minute), end: start.Add(90 * time.Minute)},
		{name: "contained", start: start.Add(15 * time.Minute), end: start.Add(45 * time.Minute)},
		{name: "enclosing", start: start.Add(-time.Hour), end: start.Add(2 * time.Hour)},
		{name: "identical", start: start, end: start.Add(time.Hour)},
	}
	for _, conflict := range conflicts {
		t.Run(conflict.name, func(t *testing.T) {
			input := fixtures.booking(t, 0, 1, conflict.start)
			input.endAt = conflict.end
			requirePostgresConstraintError(t, fixtures.insertBooking(t.Context(), input), "23P01", "bookings_confirmed_resource_period_excl")
		})
	}

	for _, adjacent := range []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "ends at existing start", start: start.Add(-time.Hour), end: start},
		{name: "starts at existing end", start: start.Add(time.Hour), end: start.Add(2 * time.Hour)},
	} {
		t.Run(adjacent.name, func(t *testing.T) {
			input := fixtures.booking(t, 0, 1, adjacent.start)
			input.endAt = adjacent.end
			if err := fixtures.insertBooking(t.Context(), input); err != nil {
				t.Fatalf("adjacent booking was rejected: %v", err)
			}
		})
	}

	t.Run("same interval on another resource", func(t *testing.T) {
		input := fixtures.booking(t, 1, 1, start)
		if err := fixtures.insertBooking(t.Context(), input); err != nil {
			t.Fatalf("different-resource booking was rejected: %v", err)
		}
	})

	t.Run("cancelled booking does not occupy interval", func(t *testing.T) {
		cancelled := fixtures.booking(t, 0, 1, start.Add(24*time.Hour))
		cancelled.state = "cancelled"
		cancelled.cancelledByAccountID = stringPointer(fixtures.accounts[0])
		cancelledAt := start.Add(23 * time.Hour)
		cancelled.cancelledAt = &cancelledAt
		if err := fixtures.insertBooking(t.Context(), cancelled); err != nil {
			t.Fatalf("insert cancelled booking: %v", err)
		}
		confirmed := fixtures.booking(t, 0, 0, cancelled.startAt)
		if err := fixtures.insertBooking(t.Context(), confirmed); err != nil {
			t.Fatalf("cancelled booking blocked interval: %v", err)
		}
	})
}

func TestConcurrentConflictingBookingInsertsUseExclusionConstraint(t *testing.T) {
	pool := bookingTestPool(t)
	fixtures := newBookingTestFixtures(t, pool, 2, 1)
	start := bookingTestStart()
	firstConn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer firstConn.Release()
	firstTx, err := firstConn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer firstTx.Rollback(context.Background())

	secondConn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer secondConn.Release()
	secondTx, err := secondConn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer secondTx.Rollback(context.Background())

	var firstPID, secondPID int32
	if err := firstConn.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := secondConn.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&secondPID); err != nil {
		t.Fatal(err)
	}
	if firstPID == secondPID {
		t.Fatal("concurrent booking inserts did not use independent PostgreSQL connections")
	}

	first := fixtures.booking(t, 0, 0, start)
	if _, err := firstTx.Exec(t.Context(), insertBookingSQL,
		first.resourceID, first.ownerID, first.requestID, first.startAt, first.endAt,
		first.purpose, first.state, first.createdAt, first.cancelledByAccountID, first.cancelledAt); err != nil {
		t.Fatalf("insert first concurrent booking: %v", err)
	}
	second := fixtures.booking(t, 0, 1, start)
	secondResult := make(chan error, 1)
	go func() {
		_, insertErr := secondTx.Exec(t.Context(), insertBookingSQL,
			second.resourceID, second.ownerID, second.requestID, second.startAt, second.endAt,
			second.purpose, second.state, second.createdAt, second.cancelledByAccountID, second.cancelledAt)
		secondResult <- insertErr
	}()

	waitCtx, waitCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer waitCancel()
	if err := waitForPostgresBlock(waitCtx, pool, secondPID); err != nil {
		_ = firstTx.Rollback(context.Background())
		select {
		case insertErr := <-secondResult:
			_ = secondTx.Rollback(context.Background())
			t.Fatalf("second booking did not wait on the conflicting range: %v (insert result: %v)", err, insertErr)
		case <-time.After(5 * time.Second):
			t.Fatalf("second booking did not finish after releasing the first transaction: %v", err)
		}
	}

	if err := firstTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit first concurrent booking: %v", err)
	}
	var secondErr error
	select {
	case secondErr = <-secondResult:
	case <-t.Context().Done():
		t.Fatalf("second booking did not finish after first commit: %v", t.Context().Err())
	}
	requirePostgresConstraintError(t, secondErr, "23P01", "bookings_confirmed_resource_period_excl")
	if err := secondTx.Rollback(t.Context()); err != nil {
		t.Fatalf("rollback rejected conflicting transaction: %v", err)
	}

	var retained int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM bookings WHERE resource_id = $1 AND state = 'confirmed'", first.resourceID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("concurrent conflicting inserts retained %d confirmed bookings, want exactly one", retained)
	}
}

func TestActivityEventPersistenceTargetsExactlyOneRecord(t *testing.T) {
	pool := bookingTestPool(t)
	fixtures := newBookingTestFixtures(t, pool, 1, 1)
	booking := fixtures.booking(t, 0, 0, bookingTestStart())
	if err := fixtures.insertBooking(t.Context(), booking); err != nil {
		t.Fatalf("insert activity target booking: %v", err)
	}
	var bookingID string
	if err := pool.QueryRow(t.Context(), "SELECT id::text FROM bookings WHERE owner_account_id = $1 AND request_id = $2", booking.ownerID, booking.requestID).Scan(&bookingID); err != nil {
		t.Fatal(err)
	}

	if err := insertTestActivityEvent(t.Context(), pool, fixtures.accounts[0], bookingID, "", `{"purpose":"verification"}`); err != nil {
		t.Fatalf("insert booking activity event: %v", err)
	}
	if err := insertTestActivityEvent(t.Context(), pool, fixtures.accounts[0], "", fixtures.resources[0], `{}`); err != nil {
		t.Fatalf("insert resource activity event: %v", err)
	}
	for _, invalid := range []struct {
		name       string
		bookingID  string
		resourceID string
		details    string
		constraint string
	}{
		{name: "missing target", details: `{}`, constraint: "activity_events_exactly_one_target_check"},
		{name: "both targets", bookingID: bookingID, resourceID: fixtures.resources[0], details: `{}`, constraint: "activity_events_exactly_one_target_check"},
		{name: "non-object details", resourceID: fixtures.resources[0], details: `[]`, constraint: "activity_events_details_object_check"},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			err := insertTestActivityEvent(t.Context(), pool, fixtures.accounts[0], invalid.bookingID, invalid.resourceID, invalid.details)
			requirePostgresConstraintError(t, err, "23514", invalid.constraint)
		})
	}
	t.Run("unknown actor foreign key", func(t *testing.T) {
		actorID := testDatabaseUUID(t, pool)
		err := insertTestActivityEvent(t.Context(), pool, actorID, bookingID, "", `{}`)
		requirePostgresConstraintError(t, err, "23503", "activity_events_actor_account_id_fkey")
	})

	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM activity_events WHERE actor_account_id = $1", fixtures.accounts[0]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("persisted %d valid activity events, want 2", count)
	}
}

func TestBookingMigrationPreservesFoundationRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	migrationPool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	schema := "upgrade_" + strings.ReplaceAll(testDatabaseUUID(t, migrationPool), "-", "")
	schemaIdentifier := pgx.Identifier{schema}.Sanitize()
	if _, err := migrationPool.Exec(ctx, "CREATE SCHEMA "+schemaIdentifier); err != nil {
		t.Fatalf("create isolated upgrade schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := migrationPool.Exec(cleanupCtx, "DROP SCHEMA "+schemaIdentifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated upgrade schema: %v", err)
		}
	})

	poolConfig, err := pgxpool.ParseConfig(os.Getenv("LABRESERVE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schemaIdentifier + ",public"
	upgradePool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("open isolated upgrade schema: %v", err)
	}
	defer upgradePool.Close()
	if err := upgradePool.Ping(ctx); err != nil {
		t.Fatalf("connect to isolated upgrade schema: %v", err)
	}

	migrations, err := readEmbeddedMigrationChecksums()
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var foundation *migrationChecksum
	for index := range migrations {
		if migrations[index].version == "001_foundation.sql" {
			foundation = &migrations[index]
			break
		}
	}
	if foundation == nil {
		t.Fatal("foundation migration is missing")
	}
	if _, err := upgradePool.Exec(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		t.Fatalf("create pre-upgrade migration ledger: %v", err)
	}
	if _, err := upgradePool.Exec(ctx, string(foundation.contents)); err != nil {
		t.Fatalf("apply foundation schema: %v", err)
	}
	var accountID, resourceID string
	if err := upgradePool.QueryRow(ctx, `
		INSERT INTO accounts (id, login, display_name, password_hash, role)
		VALUES (gen_random_uuid(), 'upgrade@example.test', 'Retained account', 'unused', 'engineer')
		RETURNING id::text
	`).Scan(&accountID); err != nil {
		t.Fatalf("insert pre-upgrade account: %v", err)
	}
	if err := upgradePool.QueryRow(ctx, `
		INSERT INTO resources (id, code, name, description)
		VALUES (gen_random_uuid(), 'UPGRADE-01', 'Retained resource', 'Retained description')
		RETURNING id::text
	`).Scan(&resourceID); err != nil {
		t.Fatalf("insert pre-upgrade resource: %v", err)
	}
	if _, err := upgradePool.Exec(ctx, `
		INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)
	`, foundation.version, foundation.checksum); err != nil {
		t.Fatalf("record foundation migration: %v", err)
	}

	if err := Migrate(ctx, upgradePool); err != nil {
		t.Fatalf("apply booking migration to existing foundation data: %v", err)
	}
	if err := CheckSchema(ctx, upgradePool); err != nil {
		t.Fatalf("check upgraded schema: %v", err)
	}
	var login, displayName, code, name, description string
	if err := upgradePool.QueryRow(ctx, `
		SELECT a.login, a.display_name, r.code, r.name, r.description
		FROM accounts a CROSS JOIN resources r
		WHERE a.id = $1 AND r.id = $2
	`, accountID, resourceID).Scan(&login, &displayName, &code, &name, &description); err != nil {
		t.Fatalf("read retained foundation records: %v", err)
	}
	if login != "upgrade@example.test" || displayName != "Retained account" || code != "UPGRADE-01" ||
		name != "Retained resource" || description != "Retained description" {
		t.Fatalf("foundation records changed during upgrade: login=%q display=%q code=%q name=%q description=%q",
			login, displayName, code, name, description)
	}
}

func bookingTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testPool(t, "LABRESERVE_TEST_DATABASE_URL")
	if err := Migrate(t.Context(), pool); err != nil {
		t.Fatalf("apply booking persistence migration: %v", err)
	}
	return pool
}

type bookingTestFixtures struct {
	pool      *pgxpool.Pool
	accounts  []string
	resources []string
}

func newBookingTestFixtures(t *testing.T, pool *pgxpool.Pool, accountCount, resourceCount int) *bookingTestFixtures {
	t.Helper()
	fixtures := &bookingTestFixtures{pool: pool}
	t.Cleanup(func() { fixtures.cleanup(t) })
	for range accountCount {
		var id string
		err := pool.QueryRow(t.Context(), `
			INSERT INTO accounts (id, login, display_name, password_hash, role)
			VALUES (
				gen_random_uuid(),
				'booking-test-' || replace(gen_random_uuid()::text, '-', '') || '@example.test',
				'Booking test fixture', 'unused', 'engineer'
			)
			RETURNING id::text
		`).Scan(&id)
		if err != nil {
			t.Fatalf("create booking test account: %v", err)
		}
		fixtures.accounts = append(fixtures.accounts, id)
	}
	for range resourceCount {
		var id string
		err := pool.QueryRow(t.Context(), `
			INSERT INTO resources (id, code, name, description)
			VALUES (
				gen_random_uuid(),
				'BT-' || upper(substr(replace(gen_random_uuid()::text, '-', ''), 1, 16)),
				'Booking test fixture', ''
			)
			RETURNING id::text
		`).Scan(&id)
		if err != nil {
			t.Fatalf("create booking test resource: %v", err)
		}
		fixtures.resources = append(fixtures.resources, id)
	}
	return fixtures
}

func (f *bookingTestFixtures) cleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, resourceID := range f.resources {
		if _, err := f.pool.Exec(ctx, `
			DELETE FROM activity_events
			WHERE resource_id = $1 OR booking_id IN (SELECT id FROM bookings WHERE resource_id = $1)
		`, resourceID); err != nil {
			t.Errorf("remove activity events for booking fixture resource: %v", err)
		}
	}
	for _, accountID := range f.accounts {
		if _, err := f.pool.Exec(ctx, `
			DELETE FROM activity_events
			WHERE actor_account_id = $1 OR booking_id IN (
				SELECT id FROM bookings WHERE owner_account_id = $1 OR cancelled_by_account_id = $1
			)
		`, accountID); err != nil {
			t.Errorf("remove activity events for booking fixture account: %v", err)
		}
	}
	for _, resourceID := range f.resources {
		if _, err := f.pool.Exec(ctx, "DELETE FROM bookings WHERE resource_id = $1", resourceID); err != nil {
			t.Errorf("remove bookings for fixture resource: %v", err)
		}
	}
	for _, accountID := range f.accounts {
		if _, err := f.pool.Exec(ctx, "DELETE FROM bookings WHERE owner_account_id = $1 OR cancelled_by_account_id = $1", accountID); err != nil {
			t.Errorf("remove bookings for fixture account: %v", err)
		}
	}
	for _, resourceID := range f.resources {
		if _, err := f.pool.Exec(ctx, "DELETE FROM resources WHERE id = $1", resourceID); err != nil {
			t.Errorf("remove fixture resource: %v", err)
		}
	}
	for _, accountID := range f.accounts {
		if _, err := f.pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID); err != nil {
			t.Errorf("remove fixture account: %v", err)
		}
	}
}

type bookingTestInput struct {
	resourceID           string
	ownerID              string
	requestID            string
	startAt              time.Time
	endAt                time.Time
	purpose              string
	state                string
	createdAt            time.Time
	cancelledByAccountID *string
	cancelledAt          *time.Time
}

func (f *bookingTestFixtures) booking(t *testing.T, resourceIndex, ownerIndex int, start time.Time) bookingTestInput {
	t.Helper()
	return bookingTestInput{
		resourceID: f.resources[resourceIndex], ownerID: f.accounts[ownerIndex], requestID: testDatabaseUUID(t, f.pool),
		startAt: start, endAt: start.Add(time.Hour), purpose: "test booking", state: "confirmed", createdAt: start.Add(-24 * time.Hour),
	}
}

func (f *bookingTestFixtures) insertBooking(ctx context.Context, input bookingTestInput) error {
	_, err := f.pool.Exec(ctx, insertBookingSQL,
		input.resourceID, input.ownerID, input.requestID, input.startAt, input.endAt,
		input.purpose, input.state, input.createdAt, input.cancelledByAccountID, input.cancelledAt)
	return err
}

func insertTestActivityEvent(ctx context.Context, pool *pgxpool.Pool, actorID, bookingID, resourceID, details string) error {
	var bookingTarget, resourceTarget any
	action := "booking.created"
	if bookingID != "" {
		bookingTarget = bookingID
	} else {
		action = "resource.created"
	}
	if resourceID != "" {
		resourceTarget = resourceID
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO activity_events (id, actor_account_id, action, occurred_at, booking_id, resource_id, details)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6::jsonb)
	`, actorID, action, bookingTestStart(), bookingTarget, resourceTarget, details)
	return err
}

func testDatabaseUUID(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), "SELECT gen_random_uuid()::text").Scan(&id); err != nil {
		t.Fatalf("generate test UUID: %v", err)
	}
	return id
}

func bookingTestStart() time.Time {
	return time.Date(2040, time.January, 2, 10, 0, 0, 0, time.UTC)
}

func stringPointer(value string) *string {
	return &value
}

func requirePostgresConstraintError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var postgresErr *pgconn.PgError
	if !errors.As(err, &postgresErr) {
		t.Fatalf("error = %v, want PostgreSQL %s constraint error %s", err, code, constraint)
	}
	if postgresErr.Code != code {
		t.Fatalf("PostgreSQL error code = %s, want %s", postgresErr.Code, code)
	}
	if constraint != "" && postgresErr.ConstraintName != constraint {
		t.Fatalf("PostgreSQL constraint = %s, want %s", postgresErr.ConstraintName, constraint)
	}
}
