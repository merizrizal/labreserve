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

type resourceStateTransaction struct {
	*resourceCreationTransaction
}

func (s *Store) BeginResourceStateChange(ctx context.Context) (resources.StateTransaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin Resource state transaction: %w", err)
	}
	return &resourceStateTransaction{resourceCreationTransaction: &resourceCreationTransaction{tx: tx}}, nil
}

func (tx *resourceStateTransaction) HasBlockingBookings(ctx context.Context, resourceID string, operationTime time.Time) (bool, error) {
	var occupied bool
	err := tx.tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM public.bookings
			WHERE resource_id = $1::uuid AND state = 'confirmed' AND end_at > $2::timestamptz
		)`, resourceID, operationTime).Scan(&occupied)
	if err != nil {
		return false, fmt.Errorf("query Resource deactivation occupancy: %w", err)
	}
	return occupied, nil
}

func (tx *resourceStateTransaction) SetResourceState(
	ctx context.Context,
	actorID, resourceID string,
	active bool,
	occurredAt time.Time,
) (resources.Resource, bool, error) {
	var resource resources.Resource
	var changed bool
	err := tx.tx.QueryRow(ctx, `
		SELECT resource_id::text, code, name, description, active, created_at, updated_at, changed
		FROM public.labreserve_set_resource_state($1::uuid, $2::uuid, $3::boolean, $4::timestamptz)`,
		actorID, resourceID, active, occurredAt).Scan(
		&resource.ID, &resource.Code, &resource.Name, &resource.Description, &resource.Active,
		&resource.CreatedAt, &resource.UpdatedAt, &changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Resource{}, false, resources.ErrUnknownResource
	}
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) {
			if postgresErr.Code == "42501" && postgresErr.Message == "coordinator authority required" {
				return resources.Resource{}, false, resources.ErrForbidden
			}
			if postgresErr.Code == "P0001" && postgresErr.Message == "resource has blocking confirmed bookings" {
				return resources.Resource{}, false, resources.ErrResourceOccupied
			}
		}
		return resources.Resource{}, false, fmt.Errorf("invoke guarded Resource state capability: %w", err)
	}
	return resource, changed, nil
}

var _ resources.StateRepository = (*Store)(nil)
var _ resources.StateTransaction = (*resourceStateTransaction)(nil)
