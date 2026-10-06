package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"labreserve.local/labreserve/internal/bookings"
)

const (
	bookingOverlapConstraint = "bookings_confirmed_resource_period_excl"
	bookingRequestConstraint = "bookings_owner_request_id_uq"
)

type bookingCreationTransaction struct {
	tx pgx.Tx
}

func (s *Store) BeginBookingCreation(ctx context.Context) (bookings.Transaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin booking transaction: %w", err)
	}
	return &bookingCreationTransaction{tx: tx}, nil
}

func (tx *bookingCreationTransaction) LockAccount(ctx context.Context, accountID string) (bool, error) {
	var locked bool
	if err := tx.tx.QueryRow(ctx, "SELECT public.labreserve_lock_booking_owner($1)", accountID).Scan(&locked); err != nil {
		return false, fmt.Errorf("lock booking owner: %w", err)
	}
	return locked, nil
}

func (tx *bookingCreationTransaction) FindBookingByRequest(ctx context.Context, ownerID, requestID string) (bookings.Booking, bool, error) {
	var booking bookings.Booking
	var state string
	err := tx.tx.QueryRow(ctx, `
		SELECT id::text, resource_id::text, owner_account_id::text, request_id::text,
		       start_at, end_at, purpose, state, created_at
		FROM bookings WHERE owner_account_id = $1 AND request_id = $2`, ownerID, requestID).Scan(
		&booking.ID, &booking.ResourceID, &booking.OwnerAccountID, &booking.RequestID,
		&booking.StartAt, &booking.EndAt, &booking.Purpose, &state, &booking.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookings.Booking{}, false, nil
	}
	if err != nil {
		return bookings.Booking{}, false, fmt.Errorf("find booking by request identity: %w", err)
	}
	booking.State = bookings.State(state)
	return booking, true, nil
}

func (tx *bookingCreationTransaction) LockResource(ctx context.Context, resourceID string) (bookings.Resource, bool, error) {
	var locked bool
	if err := tx.tx.QueryRow(ctx, "SELECT public.labreserve_lock_booking_resource($1)", resourceID).Scan(&locked); err != nil {
		return bookings.Resource{}, false, fmt.Errorf("lock booking resource: %w", err)
	}
	if !locked {
		return bookings.Resource{}, false, nil
	}
	var resource bookings.Resource
	if err := tx.tx.QueryRow(ctx, "SELECT id::text, active FROM resources WHERE id = $1", resourceID).Scan(&resource.ID, &resource.Active); err != nil {
		return bookings.Resource{}, false, fmt.Errorf("read locked booking resource: %w", err)
	}
	return resource, true, nil
}

func (tx *bookingCreationTransaction) InsertBooking(ctx context.Context, booking bookings.Booking) (bookings.Booking, error) {
	var state string
	err := tx.tx.QueryRow(ctx, `
		INSERT INTO bookings (
			id, resource_id, owner_account_id, request_id, start_at, end_at,
			purpose, state, created_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8
		)
		RETURNING id::text, resource_id::text, owner_account_id::text, request_id::text,
		          start_at, end_at, purpose, state, created_at`,
		booking.ResourceID, booking.OwnerAccountID, booking.RequestID,
		booking.StartAt, booking.EndAt, booking.Purpose, booking.State, booking.CreatedAt).Scan(
		&booking.ID, &booking.ResourceID, &booking.OwnerAccountID, &booking.RequestID,
		&booking.StartAt, &booking.EndAt, &booking.Purpose, &state, &booking.CreatedAt)
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23P01" && postgresErr.ConstraintName == bookingOverlapConstraint {
			return bookings.Booking{}, bookings.ErrConflict
		}
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == bookingRequestConstraint {
			return bookings.Booking{}, bookings.ErrRetryRequestLookup
		}
		return bookings.Booking{}, fmt.Errorf("insert booking: %w", err)
	}
	booking.State = bookings.State(state)
	return booking, nil
}

func (tx *bookingCreationTransaction) AppendBookingCreatedEvent(ctx context.Context, event bookings.BookingCreatedEvent) error {
	details, err := json.Marshal(struct {
		ResourceID string    `json:"resource_id"`
		RequestID  string    `json:"request_id"`
		StartAt    time.Time `json:"start_at"`
		EndAt      time.Time `json:"end_at"`
		Purpose    string    `json:"purpose"`
	}{
		ResourceID: event.Booking.ResourceID,
		RequestID:  event.Booking.RequestID,
		StartAt:    event.Booking.StartAt,
		EndAt:      event.Booking.EndAt,
		Purpose:    event.Booking.Purpose,
	})
	if err != nil {
		return fmt.Errorf("encode booking activity details: %w", err)
	}
	if _, err := tx.tx.Exec(ctx, `
		INSERT INTO activity_events (
			id, actor_account_id, action, occurred_at, booking_id, details
		) VALUES (gen_random_uuid(), $1, 'booking.created', $2, $3, $4::jsonb)`,
		event.ActorAccountID, event.OccurredAt, event.Booking.ID, details); err != nil {
		return fmt.Errorf("append booking activity event: %w", err)
	}
	return nil
}

func (tx *bookingCreationTransaction) Commit(ctx context.Context) error {
	if err := tx.tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit booking transaction: %w", err)
	}
	return nil
}

func (tx *bookingCreationTransaction) Rollback(ctx context.Context) error {
	if err := tx.tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback booking transaction: %w", err)
	}
	return nil
}

func (s *Store) ListResourceBookingViews(ctx context.Context, resourceID string, dayStart, dayEnd time.Time, limit, offset int64) ([]BookingView, error) {
	if limit < 1 || offset < 0 || !dayEnd.After(dayStart) {
		return nil, fmt.Errorf("invalid booking schedule query")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id::text, b.resource_id::text, r.code, r.name,
		       a.id::text, a.display_name, b.start_at, b.end_at, b.purpose, b.state
		FROM bookings b
		JOIN resources r ON r.id = b.resource_id
		JOIN accounts a ON a.id = b.owner_account_id
		WHERE b.resource_id = $1 AND b.start_at < $3 AND b.end_at > $2
		ORDER BY b.start_at ASC, b.id ASC
		LIMIT $4 OFFSET $5`, resourceID, dayStart, dayEnd, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list resource bookings")
	}
	defer rows.Close()

	views := make([]BookingView, 0)
	for rows.Next() {
		var view BookingView
		if err := rows.Scan(
			&view.ID, &view.ResourceID, &view.ResourceCode, &view.ResourceName,
			&view.OwnerAccountID, &view.OwnerDisplayName, &view.StartAt, &view.EndAt,
			&view.Purpose, &view.State,
		); err != nil {
			return nil, fmt.Errorf("read resource bookings")
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read resource bookings")
	}
	return views, nil
}

func (s *Store) BookingViewByID(ctx context.Context, bookingID string) (BookingView, bool, error) {
	var view BookingView
	err := s.pool.QueryRow(ctx, `
		SELECT b.id::text, b.resource_id::text, r.code, r.name,
		       a.id::text, a.display_name, b.start_at, b.end_at, b.purpose, b.state
		FROM bookings b
		JOIN resources r ON r.id = b.resource_id
		JOIN accounts a ON a.id = b.owner_account_id
		WHERE b.id = $1`, bookingID).Scan(
		&view.ID, &view.ResourceID, &view.ResourceCode, &view.ResourceName,
		&view.OwnerAccountID, &view.OwnerDisplayName, &view.StartAt, &view.EndAt,
		&view.Purpose, &view.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingView{}, false, nil
	}
	if err != nil {
		return BookingView{}, false, fmt.Errorf("load booking")
	}
	return view, true, nil
}

var _ bookings.Repository = (*Store)(nil)
var _ bookings.Transaction = (*bookingCreationTransaction)(nil)
