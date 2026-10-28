package resources

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResourceStateContractExplicitDesiredValue(t *testing.T) {
	active := StateRequest{ResourceID: creationResourceID, Active: true}
	inactive := StateRequest{ResourceID: creationResourceID, Active: false}
	if !active.Active || inactive.Active {
		t.Fatalf("desired states did not preserve explicit Active values: active=%+v inactive=%+v", active, inactive)
	}

	changed := StateResult{Resource: Resource{ID: creationResourceID, Active: true}, Changed: true}
	unchanged := StateResult{Resource: Resource{ID: creationResourceID, Active: false}, Changed: false}
	if !changed.Changed || !changed.Resource.Active || unchanged.Changed || unchanged.Resource.Active {
		t.Fatalf("changed/no-op results do not distinguish desired-state outcomes: changed=%+v unchanged=%+v", changed, unchanged)
	}
	if errors.Is(ErrResourceOccupied, ErrOperational) {
		t.Fatal("occupancy rejection must remain distinct from an operational failure")
	}
}

func TestResourceStateActivationUsesAuthorizedLockedResourceAndFreshTime(t *testing.T) {
	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	previousUpdatedAt := createdAt.Add(time.Minute)
	repository := newStateFakeRepository(Resource{
		ID: creationResourceID, Code: "MiX-Code", Name: "  Legacy Name  ", Description: "<b>legacy</b>",
		Active: false, CreatedAt: createdAt, UpdatedAt: previousUpdatedAt,
	})
	operationTime := previousUpdatedAt.Add(time.Hour).In(time.FixedZone("test", 7*60*60)).Add(789 * time.Nanosecond)
	clockCalls := 0
	service := NewStateService(repository, func() time.Time {
		clockCalls++
		if repository.tx == nil || !repository.tx.locked || repository.tx.stateCalls != 0 {
			t.Fatal("activation operation time was captured before the Resource lock or more than once")
		}
		return operationTime
	})

	result, err := service.SetState(context.Background(), Actor{AccountID: strings.ToUpper(creationCoordinatorID)}, StateRequest{
		ResourceID: strings.ToUpper(creationResourceID), Active: true,
	})
	if err != nil {
		t.Fatalf("activate Resource: %v", err)
	}
	wantUpdatedAt := operationTime.UTC().Truncate(time.Microsecond)
	if !result.Changed || !result.Resource.Active || result.Resource.ID != creationResourceID ||
		result.Resource.Code != "MiX-Code" || result.Resource.Name != "  Legacy Name  " ||
		result.Resource.Description != "<b>legacy</b>" || !result.Resource.CreatedAt.Equal(createdAt) ||
		!result.Resource.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("activation result = %+v", result)
	}
	if repository.tx.actorID != creationCoordinatorID || repository.tx.resourceID != creationResourceID ||
		!repository.tx.desiredActive || !repository.tx.occurredAt.Equal(wantUpdatedAt) ||
		repository.tx.lockCalls != 1 || repository.tx.stateCalls != 1 || repository.tx.occupancyCalls != 0 ||
		clockCalls != 1 || !repository.tx.committed ||
		!reflect.DeepEqual(repository.tx.trace, []string{"authorize", "lock", "state", "commit"}) {
		t.Fatalf("activation transaction = %+v; clock calls=%d", repository.tx, clockCalls)
	}
}

func TestResourceStateAuthorizesBeforeValidationAndNoOp(t *testing.T) {
	for _, test := range []struct {
		name        string
		actor       Actor
		exists      bool
		coordinator bool
		resourceID  string
		found       bool
		want        error
		begins      int
		locks       int
	}{
		{name: "missing actor", want: ErrUnauthenticated},
		{name: "malformed actor", actor: Actor{AccountID: "not-an-id"}, want: ErrUnauthenticated},
		{name: "unknown account", actor: Actor{AccountID: creationCoordinatorID}, want: ErrUnauthenticated, begins: 1},
		{name: "Engineer cannot use matching-state no-op", actor: Actor{AccountID: creationEngineerID}, exists: true, want: ErrForbidden, begins: 1},
		{name: "Engineer rejected before malformed target", actor: Actor{AccountID: creationEngineerID}, exists: true, resourceID: "bad-id", want: ErrForbidden, begins: 1},
		{name: "Coordinator malformed target", actor: Actor{AccountID: creationCoordinatorID}, exists: true, coordinator: true, resourceID: "bad-id", want: ErrInvalidInput, begins: 1},
		{name: "unknown Resource", actor: Actor{AccountID: creationCoordinatorID}, exists: true, coordinator: true, resourceID: creationResourceID, found: false, want: ErrUnknownResource, begins: 1, locks: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newStateFakeRepository(Resource{ID: creationResourceID, Active: true})
			repository.exists, repository.coordinator, repository.found = test.exists, test.coordinator, test.found
			service := NewStateService(repository, time.Now)
			_, err := service.SetState(context.Background(), test.actor, StateRequest{ResourceID: test.resourceID, Active: true})
			if !errors.Is(err, test.want) {
				t.Fatalf("SetState error = %v, want %v", err, test.want)
			}
			if repository.beginCalls != test.begins {
				t.Fatalf("transaction begins = %d, want %d", repository.beginCalls, test.begins)
			}
			if repository.tx != nil && (repository.tx.lockCalls != test.locks || repository.tx.stateCalls != 0 || repository.tx.commitCalls != 0) {
				t.Fatalf("unauthorized/invalid/unknown request reached mutation or commit: %+v", repository.tx)
			}
		})
	}
}

func TestResourceStateActiveNoOpCommitsWithoutClockOrMutation(t *testing.T) {
	updatedAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	repository := newStateFakeRepository(Resource{
		ID: creationResourceID, Code: "Noop", Name: "Active Lab", Active: true,
		CreatedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt,
	})
	clockCalls := 0
	service := NewStateService(repository, func() time.Time {
		clockCalls++
		return updatedAt.Add(time.Hour)
	})

	result, err := service.SetState(context.Background(), Actor{AccountID: creationCoordinatorID}, StateRequest{
		ResourceID: creationResourceID, Active: true,
	})
	if err != nil || result.Changed || !result.Resource.Active || !result.Resource.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("active no-op result=%+v error=%v", result, err)
	}
	if repository.tx.lockCalls != 1 || repository.tx.stateCalls != 0 || repository.tx.occupancyCalls != 0 ||
		repository.tx.commitCalls != 1 || !repository.tx.committed || clockCalls != 0 ||
		!reflect.DeepEqual(repository.tx.trace, []string{"authorize", "lock", "commit"}) {
		t.Fatalf("active no-op performed mutation/time/occupancy work: tx=%+v clock calls=%d", repository.tx, clockCalls)
	}
}

func TestResourceStateActivationRepositoryFailuresAreOperational(t *testing.T) {
	for _, test := range []struct {
		name        string
		configure   func(*stateFakeRepository)
		wantBegins  int
		wantLocks   int
		wantStates  int
		wantCommits int
	}{
		{name: "begin", configure: func(r *stateFakeRepository) { r.beginErr = errors.New("database unavailable") }, wantBegins: 1},
		{name: "role lookup", configure: func(r *stateFakeRepository) { r.resolveErr = errors.New("database unavailable") }, wantBegins: 1},
		{name: "resource lock", configure: func(r *stateFakeRepository) { r.lockErr = errors.New("database unavailable") }, wantBegins: 1, wantLocks: 1},
		{name: "guarded mutation", configure: func(r *stateFakeRepository) { r.stateErr = errors.New("database unavailable") }, wantBegins: 1, wantLocks: 1, wantStates: 1},
		{name: "transition commit", configure: func(r *stateFakeRepository) { r.commitErr = errors.New("commit outcome unavailable") }, wantBegins: 1, wantLocks: 1, wantStates: 1, wantCommits: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newStateFakeRepository(Resource{ID: creationResourceID, Active: false})
			test.configure(repository)
			service := NewStateService(repository, func() time.Time { return time.Now() })
			_, err := service.SetState(context.Background(), Actor{AccountID: creationCoordinatorID}, StateRequest{
				ResourceID: creationResourceID, Active: true,
			})
			if !errors.Is(err, ErrOperational) {
				t.Fatalf("SetState error = %v, want ErrOperational", err)
			}
			if repository.beginCalls != test.wantBegins {
				t.Fatalf("begin calls = %d, want %d", repository.beginCalls, test.wantBegins)
			}
			if repository.tx != nil {
				if repository.tx.lockCalls != test.wantLocks || repository.tx.stateCalls != test.wantStates || repository.tx.commitCalls != test.wantCommits {
					t.Fatalf("repository failure retried or reached unexpected operations: %+v", repository.tx)
				}
				if !repository.tx.rolledBack {
					t.Fatalf("failed transaction was not rolled back: %+v", repository.tx)
				}
			}
		})
	}
}

func TestResourceStateNoOpCommitFailureIsOperational(t *testing.T) {
	repository := newStateFakeRepository(Resource{ID: creationResourceID, Active: true})
	repository.commitErr = errors.New("commit acknowledgement unavailable")
	service := NewStateService(repository, time.Now)
	result, err := service.SetState(context.Background(), Actor{AccountID: creationCoordinatorID}, StateRequest{
		ResourceID: creationResourceID, Active: true,
	})
	if !errors.Is(err, ErrOperational) || result != (StateResult{}) || repository.tx.commitCalls != 1 ||
		repository.tx.stateCalls != 0 || !repository.tx.rolledBack {
		t.Fatalf("failed no-op finalization result=%+v error=%v transaction=%+v", result, err, repository.tx)
	}
}

func TestResourceStateDeactivationFailsClosedInActivationSlice(t *testing.T) {
	repository := newStateFakeRepository(Resource{ID: creationResourceID, Active: true})
	clockCalls := 0
	service := NewStateService(repository, func() time.Time {
		clockCalls++
		return time.Now()
	})
	result, err := service.SetState(context.Background(), Actor{AccountID: creationCoordinatorID}, StateRequest{
		ResourceID: creationResourceID, Active: false,
	})
	if !errors.Is(err, ErrOperational) || !strings.Contains(err.Error(), "not implemented") || result != (StateResult{}) {
		t.Fatalf("deactivation result=%+v error=%v, want fail-closed operational result", result, err)
	}
	if repository.tx.lockCalls != 0 || repository.tx.stateCalls != 0 || repository.tx.occupancyCalls != 0 ||
		repository.tx.commitCalls != 0 || repository.tx.resolveCalls != 1 || clockCalls != 0 || !repository.tx.rolledBack {
		t.Fatalf("deactivation reached incomplete state work: tx=%+v clock calls=%d", repository.tx, clockCalls)
	}
}

type stateFakeRepository struct {
	exists      bool
	coordinator bool
	found       bool
	resource    Resource
	beginErr    error
	resolveErr  error
	lockErr     error
	stateErr    error
	commitErr   error
	beginCalls  int
	tx          *stateFakeTransaction
}

func newStateFakeRepository(resource Resource) *stateFakeRepository {
	return &stateFakeRepository{exists: true, coordinator: true, found: true, resource: resource}
}

func (r *stateFakeRepository) BeginResourceStateChange(context.Context) (StateTransaction, error) {
	r.beginCalls++
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	r.tx = &stateFakeTransaction{repository: r, resource: r.resource, found: r.found}
	return r.tx, nil
}

type stateFakeTransaction struct {
	repository     *stateFakeRepository
	trace          []string
	resource       Resource
	found          bool
	locked         bool
	actorID        string
	resourceID     string
	desiredActive  bool
	occurredAt     time.Time
	resolveCalls   int
	lockCalls      int
	stateCalls     int
	occupancyCalls int
	commitCalls    int
	committed      bool
	rolledBack     bool
}

func (tx *stateFakeTransaction) ResolveCoordinator(_ context.Context, actorID string) (bool, bool, error) {
	tx.trace = append(tx.trace, "authorize")
	tx.resolveCalls++
	tx.actorID = actorID
	if tx.repository.resolveErr != nil {
		return false, false, tx.repository.resolveErr
	}
	return tx.repository.exists, tx.repository.coordinator, nil
}

func (tx *stateFakeTransaction) LockResource(_ context.Context, resourceID string) (Resource, bool, error) {
	tx.trace = append(tx.trace, "lock")
	tx.lockCalls++
	tx.resourceID = resourceID
	if tx.repository.lockErr != nil {
		return Resource{}, false, tx.repository.lockErr
	}
	tx.locked = true
	return tx.resource, tx.found, nil
}

func (tx *stateFakeTransaction) HasBlockingBookings(context.Context, string, time.Time) (bool, error) {
	tx.trace = append(tx.trace, "occupancy")
	tx.occupancyCalls++
	return false, errors.New("unexpected occupancy check")
}

func (tx *stateFakeTransaction) SetResourceState(_ context.Context, actorID, resourceID string, active bool, occurredAt time.Time) (Resource, bool, error) {
	tx.trace = append(tx.trace, "state")
	tx.stateCalls++
	tx.actorID, tx.resourceID, tx.desiredActive, tx.occurredAt = actorID, resourceID, active, occurredAt
	if tx.repository.stateErr != nil {
		return Resource{}, false, tx.repository.stateErr
	}
	if !active {
		return Resource{}, false, errors.New("unexpected deactivation")
	}
	tx.resource.Active = true
	tx.resource.UpdatedAt = occurredAt
	return tx.resource, true, nil
}

func (tx *stateFakeTransaction) Commit(context.Context) error {
	tx.trace = append(tx.trace, "commit")
	tx.commitCalls++
	if tx.repository.commitErr != nil {
		return tx.repository.commitErr
	}
	tx.committed = true
	return nil
}

func (tx *stateFakeTransaction) Rollback(context.Context) error {
	if !tx.committed {
		tx.trace = append(tx.trace, "rollback")
		tx.rolledBack = true
	}
	return nil
}
