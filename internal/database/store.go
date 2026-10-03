package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) AccountByLogin(ctx context.Context, login string) (Account, bool, error) {
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, login, display_name, password_hash, role
		FROM accounts WHERE login = $1`, strings.ToLower(strings.TrimSpace(login))).Scan(
		&account.ID, &account.Login, &account.DisplayName, &account.PasswordHash, &account.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("load account")
	}
	return account, true, nil
}

func (s *Store) InsertSession(ctx context.Context, session Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, account_id, csrf_token, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, session.TokenHash, session.AccountID, session.CSRFToken, session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create session")
	}
	return nil
}

func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (Session, bool, error) {
	var session Session
	var accountID, identityID, login, displayName, role string
	err := s.pool.QueryRow(ctx, `
		SELECT s.token_hash, COALESCE(s.account_id::text, ''), s.csrf_token, s.created_at, s.expires_at,
		       COALESCE(a.id::text, ''), COALESCE(a.login, ''), COALESCE(a.display_name, ''), COALESCE(a.role, '')
		FROM sessions s
		LEFT JOIN accounts a ON a.id = s.account_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).Scan(
		&session.TokenHash, &accountID, &session.CSRFToken, &session.CreatedAt, &session.ExpiresAt,
		&identityID, &login, &displayName, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("load session")
	}
	if accountID != "" {
		session.AccountID = &accountID
	}
	if identityID != "" {
		session.Identity = &Identity{ID: identityID, Login: login, DisplayName: displayName, Role: Role(role)}
	}
	return session, true, nil
}

func (s *Store) RotateSession(ctx context.Context, previousTokenHash []byte, session Session) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin session rotation")
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", previousTokenHash)
	if err != nil || tag.RowsAffected() != 1 {
		return fmt.Errorf("invalidate previous session")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sessions (token_hash, account_id, csrf_token, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, session.TokenHash, session.AccountID, session.CSRFToken, session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create rotated session")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit session rotation")
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", tokenHash); err != nil {
		return fmt.Errorf("revoke session")
	}
	return nil
}

func (s *Store) ListResources(ctx context.Context) ([]Resource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, code, name, description, active
		FROM resources ORDER BY lower(code), id`)
	if err != nil {
		return nil, fmt.Errorf("list resources")
	}
	defer rows.Close()
	resources := make([]Resource, 0)
	for rows.Next() {
		var resource Resource
		if err := rows.Scan(&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active); err != nil {
			return nil, fmt.Errorf("read resources")
		}
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read resources")
	}
	return resources, nil
}

func (s *Store) ResourceByID(ctx context.Context, id string) (Resource, bool, error) {
	var resource Resource
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, code, name, description, active
		FROM resources WHERE id = $1`, id).Scan(
		&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resource{}, false, nil
	}
	if err != nil {
		return Resource{}, false, fmt.Errorf("load resource")
	}
	return resource, true, nil
}
