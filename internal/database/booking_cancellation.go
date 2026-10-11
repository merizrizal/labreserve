package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"labreserve.local/labreserve/internal/bookings"
)

type bookingCancellationTransaction struct {
	tx pgx.Tx
}

func (s *Store) BeginBookingCancellation(ctx context.Context) (bookings.CancellationTransaction, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin booking cancellation: %w", err)
	}
	return &bookingCancellationTransaction{tx: tx}, nil
}

// FindBookingResource is an unlocked ownership-scoped read. The authoritative
// Booking row is re-read under lock only after the Resource lock is acquired.
func (tx *bookingCancellationTransaction) FindBookingResource(ctx context.Context, bookingID, ownerID string) (string, bool, error) {
	var resourceID string
	err := tx.tx.QueryRow(ctx, `
		SELECT resource_id::text FROM bookings
		WHERE id = $1 AND owner_account_id = $2`, bookingID, ownerID).Scan(&resourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve owned booking resource: %w", err)
	}
	return resourceID, true, nil
}

func (tx *bookingCancellationTransaction) LockResource(ctx context.Context, resourceID string) (bool, error) {
	var locked bool
	if err := tx.tx.QueryRow(ctx, "SELECT public.labreserve_lock_booking_resource($1)", resourceID).Scan(&locked); err != nil {
		return false, fmt.Errorf("lock cancellation resource: %w", err)
	}
	return locked, nil
}

func (tx *bookingCancellationTransaction) LockBooking(ctx context.Context, bookingID string) (bookings.Booking, bool, error) {
	var booking bookings.Booking
	var state string
	err := tx.tx.QueryRow(ctx, `
		SELECT id::text, resource_id::text, owner_account_id::text, request_id::text,
		       start_at, end_at, purpose, state, created_at,
		       cancelled_by_account_id::text, cancelled_at
		FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).Scan(
		&booking.ID, &booking.ResourceID, &booking.OwnerAccountID, &booking.RequestID,
		&booking.StartAt, &booking.EndAt, &booking.Purpose, &state, &booking.CreatedAt,
		&booking.CancelledByAccountID, &booking.CancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookings.Booking{}, false, nil
	}
	if err != nil {
		return bookings.Booking{}, false, fmt.Errorf("lock cancellation booking: %w", err)
	}
	booking.State = bookings.State(state)
	return booking, true, nil
}

func (tx *bookingCancellationTransaction) UpdateBookingCancellation(ctx context.Context, bookingID, actorID string, at time.Time) error {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE bookings
		SET state = 'cancelled', cancelled_by_account_id = $2, cancelled_at = $3
		WHERE id = $1 AND state = 'confirmed'`, bookingID, actorID, at)
	if err != nil {
		return fmt.Errorf("update booking cancellation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("update booking cancellation: confirmed row not found")
	}
	return nil
}

func (tx *bookingCancellationTransaction) AppendBookingCancelledEvent(ctx context.Context, event bookings.BookingCancelledEvent) error {
	details, err := json.Marshal(struct {
		ResourceID string    `json:"resource_id"`
		StartAt    time.Time `json:"start_at"`
		EndAt      time.Time `json:"end_at"`
	}{
		ResourceID: event.Booking.ResourceID,
		StartAt:    event.Booking.StartAt,
		EndAt:      event.Booking.EndAt,
	})
	if err != nil {
		return fmt.Errorf("encode booking cancellation details: %w", err)
	}
	if _, err := tx.tx.Exec(ctx, `
		INSERT INTO activity_events (
			id, actor_account_id, action, occurred_at, booking_id, details
		) VALUES (gen_random_uuid(), $1, 'booking.cancelled', $2, $3, $4::jsonb)`,
		event.ActorAccountID, event.OccurredAt, event.Booking.ID, details); err != nil {
		return fmt.Errorf("append booking cancellation event: %w", err)
	}
	return nil
}

func (tx *bookingCancellationTransaction) Commit(ctx context.Context) error {
	if err := tx.tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit booking cancellation: %w", err)
	}
	return nil
}

func (tx *bookingCancellationTransaction) Rollback(ctx context.Context) error {
	if err := tx.tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback booking cancellation: %w", err)
	}
	return nil
}

var _ bookings.CancellationRepository = (*Store)(nil)
var _ bookings.CancellationTransaction = (*bookingCancellationTransaction)(nil)
