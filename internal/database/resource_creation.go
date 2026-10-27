package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"labreserve.local/labreserve/internal/resources"
)

const resourceCodeUniqueConstraint = "resources_code_case_insensitive_uq"

type resourceCreationTransaction struct {
	tx pgx.Tx
}

func (s *Store) BeginResourceCreation(ctx context.Context) (resources.CreationTransaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin Resource creation transaction: %w", err)
	}
	return &resourceCreationTransaction{tx: tx}, nil
}

func (s *Store) BeginResourceEdit(ctx context.Context) (resources.EditTransaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin Resource edit transaction: %w", err)
	}
	return &resourceCreationTransaction{tx: tx}, nil
}

func (tx *resourceCreationTransaction) ResolveCoordinator(ctx context.Context, actorID string) (bool, bool, error) {
	var coordinator bool
	err := tx.tx.QueryRow(ctx, `
		SELECT role = 'coordinator' FROM public.accounts WHERE id = $1::uuid`, actorID).Scan(&coordinator)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("resolve Resource actor role: %w", err)
	}
	return true, coordinator, nil
}

func (tx *resourceCreationTransaction) CreateResource(ctx context.Context, actorID string, input resources.CreateRequest, occurredAt time.Time) (resources.Resource, error) {
	var resource resources.Resource
	err := tx.tx.QueryRow(ctx, `
		SELECT resource_id::text, code, name, description, active, created_at, updated_at
		FROM public.labreserve_create_resource($1::uuid, $2, $3, $4, $5, $6)`,
		actorID, input.Code, input.Name, input.Description, input.Active, occurredAt).Scan(
		&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active,
		&resource.CreatedAt, &resource.UpdatedAt)
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) {
			if postgresErr.Code == "23505" && postgresErr.ConstraintName == resourceCodeUniqueConstraint {
				return resources.Resource{}, resources.ErrDuplicateCode
			}
			if postgresErr.Code == "42501" {
				return resources.Resource{}, resources.ErrForbidden
			}
		}
		return resources.Resource{}, fmt.Errorf("invoke guarded Resource creation: %w", err)
	}
	return resource, nil
}

func (tx *resourceCreationTransaction) LockResource(ctx context.Context, resourceID string) (resources.Resource, bool, error) {
	var locked bool
	if err := tx.tx.QueryRow(ctx, "SELECT public.labreserve_lock_booking_resource($1::uuid)", resourceID).Scan(&locked); err != nil {
		return resources.Resource{}, false, fmt.Errorf("lock Resource for descriptive edit: %w", err)
	}
	if !locked {
		return resources.Resource{}, false, nil
	}

	var resource resources.Resource
	err := tx.tx.QueryRow(ctx, `
		SELECT id::text, code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(
		&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active,
		&resource.CreatedAt, &resource.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Resource{}, false, nil
	}
	if err != nil {
		return resources.Resource{}, false, fmt.Errorf("read locked Resource for descriptive edit: %w", err)
	}
	return resource, true, nil
}

func (tx *resourceCreationTransaction) EditResource(ctx context.Context, actorID, resourceID, name, description string, occurredAt time.Time) (resources.Resource, bool, error) {
	var resource resources.Resource
	var changed bool
	err := tx.tx.QueryRow(ctx, `
		SELECT resource_id::text, code, name, description, active, created_at, updated_at, changed
		FROM public.labreserve_edit_resource($1::uuid, $2::uuid, $3, $4, $5)`,
		actorID, resourceID, name, description, occurredAt).Scan(
		&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active,
		&resource.CreatedAt, &resource.UpdatedAt, &changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Resource{}, false, resources.ErrUnknownResource
	}
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "42501" {
			return resources.Resource{}, false, resources.ErrForbidden
		}
		return resources.Resource{}, false, fmt.Errorf("invoke guarded Resource descriptive edit: %w", err)
	}
	return resource, changed, nil
}

func (tx *resourceCreationTransaction) Commit(ctx context.Context) error {
	if err := tx.tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Resource creation transaction: %w", err)
	}
	return nil
}

func (tx *resourceCreationTransaction) Rollback(ctx context.Context) error {
	if err := tx.tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("roll back Resource creation transaction: %w", err)
	}
	return nil
}

var _ resources.Repository = (*Store)(nil)
var _ resources.CreationTransaction = (*resourceCreationTransaction)(nil)
var _ resources.EditTransaction = (*resourceCreationTransaction)(nil)
