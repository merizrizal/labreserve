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

const requiredSchemaVersion = "001_foundation.sql"
const migrationLockID int64 = 0x4c61625265736572

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

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] == ".sql" {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		digest := sha256.Sum256(contents)
		checksum := hex.EncodeToString(digest[:])
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
	var applied bool
	err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", requiredSchemaVersion).Scan(&applied)
	if err != nil {
		return fmt.Errorf("check database schema (run labreserve migrate): %w", err)
	}
	if !applied {
		return fmt.Errorf("database schema is missing %s; run labreserve migrate", requiredSchemaVersion)
	}
	return nil
}
