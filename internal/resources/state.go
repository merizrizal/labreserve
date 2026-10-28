package resources

import (
	"context"
	"errors"
	"time"
)

// ErrResourceOccupied identifies an actual deactivation blocked by Confirmed bookings.
var ErrResourceOccupied = errors.New("resource has blocking confirmed bookings")

// StateRequest names an explicit desired state. Active=false requests Inactive;
// it is not a toggle or an omitted state.
type StateRequest struct {
	ResourceID string
	Active     bool
}

// StateResult returns the retained Resource and whether a state transition occurred.
// Changed=false represents a successful unchanged desired-state submission.
type StateResult struct {
	Resource Resource
	Changed  bool
}

// StateRepository opens a transaction dedicated to explicit Resource state changes.
type StateRepository interface {
	BeginResourceStateChange(context.Context) (StateTransaction, error)
}

// StateTransaction provides the narrow collaborators needed for a state change.
type StateTransaction interface {
	ResourceTransaction
	LockResource(context.Context, string) (Resource, bool, error)
	HasBlockingBookings(context.Context, string, time.Time) (bool, error)
	SetResourceState(context.Context, string, string, bool, time.Time) (Resource, bool, error)
}

// StateService handles explicit desired-state commands for existing Resources.
type StateService struct {
	repository StateRepository
	now        func() time.Time
}

// NewStateService constructs a state service with an injectable operation clock.
func NewStateService(repository StateRepository, now func() time.Time) *StateService {
	if now == nil {
		now = time.Now
	}
	return &StateService{repository: repository, now: now}
}

// SetState applies an explicit desired state using stored Coordinator authority
// and the shared Resource lock. Activation is available in this slice;
// deactivation remains fail-closed until its occupancy contract is implemented.
func (s *StateService) SetState(ctx context.Context, actor Actor, request StateRequest) (StateResult, error) {
	if s == nil || s.repository == nil {
		return StateResult{}, operationalError("initialize resource state service")
	}
	actorID, actorOK := canonicalResourceActorID(actor.AccountID)
	if !actorOK {
		return StateResult{}, ErrUnauthenticated
	}

	tx, err := s.repository.BeginResourceStateChange(ctx)
	if err != nil {
		return StateResult{}, operationalError("begin resource state change")
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	exists, coordinator, err := tx.ResolveCoordinator(ctx, actorID)
	if err != nil {
		return StateResult{}, operationalError("resolve resource state actor")
	}
	if !exists {
		return StateResult{}, ErrUnauthenticated
	}
	if !coordinator {
		return StateResult{}, ErrForbidden
	}

	resourceID, resourceIDOK := canonicalResourceUUID(request.ResourceID)
	if !resourceIDOK {
		return StateResult{}, ErrInvalidInput
	}
	if !request.Active {
		return StateResult{}, operationalError("resource deactivation is not implemented")
	}

	locked, found, err := tx.LockResource(ctx, resourceID)
	if err != nil {
		return StateResult{}, operationalError("lock resource for state change")
	}
	if !found {
		return StateResult{}, ErrUnknownResource
	}
	if locked.Active == request.Active {
		if err := tx.Commit(ctx); err != nil {
			return StateResult{}, operationalError("commit resource state no-op")
		}
		return StateResult{Resource: locked}, nil
	}

	occurredAt := s.now().UTC().Truncate(time.Microsecond)
	resource, changed, err := tx.SetResourceState(ctx, actorID, resourceID, request.Active, occurredAt)
	if errors.Is(err, ErrUnknownResource) {
		return StateResult{}, ErrUnknownResource
	}
	if errors.Is(err, ErrForbidden) {
		return StateResult{}, ErrForbidden
	}
	if errors.Is(err, ErrResourceOccupied) {
		return StateResult{}, ErrResourceOccupied
	}
	if err != nil {
		return StateResult{}, operationalError("activate resource")
	}
	if !changed || !resource.Active {
		return StateResult{}, operationalError("guarded resource activation returned no transition")
	}
	if err := tx.Commit(ctx); err != nil {
		return StateResult{}, operationalError("commit resource activation")
	}
	return StateResult{Resource: resource, Changed: true}, nil
}
