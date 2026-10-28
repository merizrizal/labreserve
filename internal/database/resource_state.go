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

// HasBlockingBookings stays unavailable until the deactivation slice adds its
// post-lock occupancy decision; callers must not interpret an error as empty.
func (*resourceStateTransaction) HasBlockingBookings(context.Context, string, time.Time) (bool, error) {
	return false, errors.New("resource deactivation occupancy check is not implemented")
}

func (tx *resourceStateTransaction) SetResourceState(
	ctx context.Context,
	actorID, resourceID string,
	active bool,
	occurredAt time.Time,
) (resources.Resource, bool, error) {
	if !active {
		return resources.Resource{}, false, errors.New("resource deactivation is not implemented")
	}

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
		if errors.As(err, &postgresErr) && postgresErr.Code == "42501" {
			return resources.Resource{}, false, resources.ErrForbidden
		}
		return resources.Resource{}, false, fmt.Errorf("invoke guarded Resource activation: %w", err)
	}
	return resource, changed, nil
}

var _ resources.StateRepository = (*Store)(nil)
var _ resources.StateTransaction = (*resourceStateTransaction)(nil)
