package bookings

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	minimumStartLead = 5 * time.Minute
	maximumStartLead = 30 * 24 * time.Hour
	minimumDuration  = 30 * time.Minute
	maximumDuration  = 8 * time.Hour
)

var (
	ErrInvalidInput       = errors.New("invalid booking input")
	ErrUnknownResource    = errors.New("unknown resource")
	ErrInactiveResource   = errors.New("inactive resource")
	ErrConflict           = errors.New("booking conflict")
	ErrRequestIDReuse     = errors.New("request identifier reused with different data")
	ErrOperational        = errors.New("booking service unavailable")
	ErrRetryRequestLookup = errors.New("request identity requires a fresh lookup")
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Actor struct {
	AccountID string
}

type CreateRequest struct {
	ResourceID string
	RequestID  string
	StartAt    time.Time
	EndAt      time.Time
	Purpose    string
}

type State string

const (
	StateConfirmed State = "confirmed"
	StateCancelled State = "cancelled"
)

type Booking struct {
	ID             string
	ResourceID     string
	OwnerAccountID string
	RequestID      string
	StartAt        time.Time
	EndAt          time.Time
	Purpose        string
	State          State
	CreatedAt      time.Time
}

type Resource struct {
	ID     string
	Active bool
}

type BookingCreatedEvent struct {
	ActorAccountID string
	Booking        Booking
	OccurredAt     time.Time
}

type CreateResult struct {
	Booking  Booking
	Replayed bool
}

type Repository interface {
	BeginBookingCreation(context.Context) (Transaction, error)
}

type Transaction interface {
	LockAccount(context.Context, string) (bool, error)
	FindBookingByRequest(context.Context, string, string) (Booking, bool, error)
	LockResource(context.Context, string) (Resource, bool, error)
	InsertBooking(context.Context, Booking) (Booking, error)
	AppendBookingCreatedEvent(context.Context, BookingCreatedEvent) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

// Create reserves a resource for the authenticated actor. Owner identity is
// never accepted as part of the request data.
func (s *Service) Create(ctx context.Context, actor Actor, input CreateRequest) (CreateResult, error) {
	if s == nil || s.repository == nil {
		return CreateResult{}, operationalError("initialize booking service")
	}
	ownerID, ownerOK := canonicalUUID(actor.AccountID)
	resourceID, resourceOK := canonicalUUID(input.ResourceID)
	requestID, requestOK := canonicalUUID(input.RequestID)
	if !ownerOK || !resourceOK || !requestOK {
		return CreateResult{}, ErrInvalidInput
	}

	request := CreateRequest{
		ResourceID: resourceID,
		RequestID:  requestID,
		StartAt:    canonicalInstant(input.StartAt),
		EndAt:      canonicalInstant(input.EndAt),
		Purpose:    strings.TrimSpace(input.Purpose),
	}

	// An unexpected request-key uniqueness violation means another writer did
	// not follow Account serialization. Roll back, then resolve it only through
	// a new transaction and the ordinary locked replay lookup.
	for attempt := 0; attempt < 2; attempt++ {
		result, err := s.createOnce(ctx, ownerID, request)
		if !errors.Is(err, ErrRetryRequestLookup) {
			return result, err
		}
	}
	return CreateResult{}, operationalError("refresh request identity")
}

func (s *Service) createOnce(ctx context.Context, ownerID string, request CreateRequest) (CreateResult, error) {
	tx, err := s.repository.BeginBookingCreation(ctx)
	if err != nil {
		return CreateResult{}, operationalError("begin booking transaction")
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	locked, err := tx.LockAccount(ctx, ownerID)
	if err != nil {
		return CreateResult{}, operationalError("lock booking owner")
	}
	if !locked {
		return CreateResult{}, operationalError("resolve authenticated booking owner")
	}

	// This lookup is intentionally a separate statement after the Account lock
	// so READ COMMITTED observes a preceding creator's committed Booking.
	existing, found, err := tx.FindBookingByRequest(ctx, ownerID, request.RequestID)
	if err != nil {
		return CreateResult{}, operationalError("look up booking request")
	}
	if found {
		if !sameCanonicalRequest(existing, request) {
			return CreateResult{}, ErrRequestIDReuse
		}
		return CreateResult{Booking: existing, Replayed: true}, nil
	}

	resource, found, err := tx.LockResource(ctx, request.ResourceID)
	if err != nil {
		return CreateResult{}, operationalError("lock booking resource")
	}
	if !found {
		return CreateResult{}, ErrUnknownResource
	}

	// Read the authoritative clock only after both required row locks. A lock
	// wait must not make the final horizon check use stale request-arrival time.
	now := canonicalInstant(s.now())
	if !resource.Active {
		return CreateResult{}, ErrInactiveResource
	}
	if err := validateNewBooking(request, now); err != nil {
		return CreateResult{}, err
	}

	booking := Booking{
		ResourceID: request.ResourceID, OwnerAccountID: ownerID,
		RequestID: request.RequestID, StartAt: request.StartAt,
		EndAt: request.EndAt, Purpose: request.Purpose,
		State: StateConfirmed, CreatedAt: now,
	}
	created, err := tx.InsertBooking(ctx, booking)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return CreateResult{}, ErrConflict
		}
		if errors.Is(err, ErrRetryRequestLookup) {
			return CreateResult{}, ErrRetryRequestLookup
		}
		return CreateResult{}, operationalError("insert booking")
	}
	if err := tx.AppendBookingCreatedEvent(ctx, BookingCreatedEvent{
		ActorAccountID: ownerID, Booking: created, OccurredAt: now,
	}); err != nil {
		return CreateResult{}, operationalError("append booking activity event")
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, operationalError("commit booking transaction")
	}
	return CreateResult{Booking: created}, nil
}

func canonicalUUID(value string) (string, bool) {
	if !uuidPattern.MatchString(value) {
		return "", false
	}
	return strings.ToLower(value), true
}

func canonicalInstant(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func sameCanonicalRequest(existing Booking, request CreateRequest) bool {
	existingResource, ok := canonicalUUID(existing.ResourceID)
	if !ok || existingResource != request.ResourceID {
		return false
	}
	return canonicalInstant(existing.StartAt).Equal(canonicalInstant(request.StartAt)) &&
		canonicalInstant(existing.EndAt).Equal(canonicalInstant(request.EndAt)) &&
		existing.Purpose == strings.TrimSpace(request.Purpose)
}

func validateNewBooking(request CreateRequest, now time.Time) error {
	if !utf8.ValidString(request.Purpose) || utf8.RuneCountInString(request.Purpose) < 1 || utf8.RuneCountInString(request.Purpose) > 200 {
		return ErrInvalidInput
	}
	if request.StartAt.Before(now.Add(minimumStartLead)) || request.StartAt.After(now.Add(maximumStartLead)) {
		return ErrInvalidInput
	}
	if !request.EndAt.After(request.StartAt) {
		return ErrInvalidInput
	}
	duration := request.EndAt.Sub(request.StartAt)
	if duration < minimumDuration || duration > maximumDuration {
		return ErrInvalidInput
	}
	return nil
}

func operationalError(operation string) error {
	return fmt.Errorf("%w: %s", ErrOperational, operation)
}
