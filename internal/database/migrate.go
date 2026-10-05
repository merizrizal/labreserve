package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const requiredSchemaVersion = "003_booking_creation_locks.sql"
const migrationLockID int64 = 0x4c61625265736572

type migrationChecksum struct {
	version  string
	checksum string
	contents []byte
}

type migrationLedgerQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readEmbeddedMigrationChecksums() ([]migrationChecksum, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] == ".sql" {
			versions = append(versions, entry.Name())
		}
	}
	sort.Strings(versions)

	migrations := make([]migrationChecksum, 0, len(versions))
	requiredFound := false
	for _, version := range versions {
		contents, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", version, err)
		}
		digest := sha256.Sum256(contents)
		migrations = append(migrations, migrationChecksum{
			version: version, checksum: hex.EncodeToString(digest[:]), contents: contents,
		})
		if version == requiredSchemaVersion {
			requiredFound = true
		}
	}
	if !requiredFound {
		return nil, fmt.Errorf("embedded migrations are missing %s", requiredSchemaVersion)
	}
	return migrations, nil
}

func validateMigrationLedger(ctx context.Context, db migrationLedgerQueryer, expected []migrationChecksum, requireComplete bool) error {
	known := make(map[string]string, len(expected))
	for _, migration := range expected {
		known[migration.version] = migration.checksum
	}

	rows, err := db.Query(ctx, "SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	defer rows.Close()
	recorded := make(map[string]struct{}, len(expected))
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return fmt.Errorf("read migration ledger row: %w", err)
		}
		expectedChecksum, ok := known[version]
		if !ok {
			return fmt.Errorf("database contains unknown migration %s", version)
		}
		if checksum != expectedChecksum {
			return fmt.Errorf("migration %s checksum changed after application", version)
		}
		recorded[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read migration ledger rows: %w", err)
	}
	if requireComplete {
		for _, migration := range expected {
			if _, ok := recorded[migration.version]; !ok {
				return fmt.Errorf("database schema is missing %s; run labreserve migrate", migration.version)
			}
		}
	}
	return nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) (resultErr error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("unlock migrations: %w", err))
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	if _, err := conn.Exec(ctx, "GRANT SELECT ON schema_migrations TO labreserve_app"); err != nil {
		return fmt.Errorf("grant migration ledger access: %w", err)
	}
	expected, err := readEmbeddedMigrationChecksums()
	if err != nil {
		return err
	}
	if err := validateMigrationLedger(ctx, conn, expected, false); err != nil {
		return fmt.Errorf("validate existing migration state: %w", err)
	}

	for _, migration := range expected {
		name, contents, checksum := migration.version, migration.contents, migration.checksum
		var recorded string
		err = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version = $1", name).Scan(&recorded)
		if err == nil {
			if recorded != checksum {
				return fmt.Errorf("migration %s checksum changed after application", name)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read migration ledger for %s: %w", name, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	expected, err := readEmbeddedMigrationChecksums()
	if err != nil {
		return fmt.Errorf("read expected database migrations: %w", err)
	}
	if err := validateMigrationLedger(ctx, pool, expected, true); err != nil {
		return fmt.Errorf("check database schema (run labreserve migrate): %w", err)
	}
	return nil
}
