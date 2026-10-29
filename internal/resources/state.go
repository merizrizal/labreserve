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

// SetState applies an explicit desired state using stored Coordinator authority,
// the shared Resource lock, and the guarded state capability.
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
	if !request.Active {
		occupied, err := tx.HasBlockingBookings(ctx, resourceID, occurredAt)
		if err != nil {
			return StateResult{}, operationalError("check resource deactivation occupancy")
		}
		if occupied {
			return StateResult{}, ErrResourceOccupied
		}
	}

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
		return StateResult{}, operationalError("change resource state")
	}
	if !changed || resource.Active != request.Active {
		return StateResult{}, operationalError("guarded resource state change returned no transition")
	}
	if err := tx.Commit(ctx); err != nil {
		return StateResult{}, operationalError("commit resource state change")
	}
	return StateResult{Resource: resource, Changed: true}, nil
}
