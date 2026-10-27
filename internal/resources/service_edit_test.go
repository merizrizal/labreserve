package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEditUsesAuthorizedLockedResourceAndCanonicalValues(t *testing.T) {
	createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	repository := newEditFakeRepository(Resource{
		ID: creationResourceID, Code: "MiX-Code", Name: "Original", Description: "Old description",
		Active: false, CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
	operationTime := updatedAt.Add(time.Hour).In(time.FixedZone("test", 7*60*60))
	service := NewService(repository, func() time.Time {
		if repository.tx == nil || !repository.tx.locked {
			t.Fatal("authoritative time was captured before locking the Resource")
		}
		return operationTime
	})

	result, err := service.Edit(context.Background(), Actor{AccountID: strings.ToUpper(creationCoordinatorID)}, EditRequest{
		ResourceID: strings.ToUpper(creationResourceID), Code: "\u2003MiX-Code\u00a0",
		Name: "\u00a0Renamed\tLab\u2003", Description: "\u2003New\ntext\u00a0",
	})
	if err != nil {
		t.Fatalf("edit Resource: %v", err)
	}
	wantUpdatedAt := operationTime.UTC().Truncate(time.Microsecond)
	if !result.Changed || result.Resource.ID != creationResourceID || result.Resource.Code != "MiX-Code" ||
		result.Resource.Name != "Renamed\tLab" || result.Resource.Description != "New\ntext" || result.Resource.Active ||
		!result.Resource.CreatedAt.Equal(createdAt) || !result.Resource.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("edit result = %+v", result)
	}
	if repository.tx.actorID != creationCoordinatorID || repository.tx.resourceID != creationResourceID ||
		repository.tx.name != result.Resource.Name || repository.tx.description != result.Resource.Description ||
		!repository.tx.occurredAt.Equal(wantUpdatedAt) || repository.tx.lockCalls != 1 || repository.tx.editCalls != 1 ||
		!repository.tx.committed || !equalStrings(repository.tx.trace, []string{"authorize", "lock", "edit", "commit"}) {
		t.Fatalf("edit transaction = %+v", repository.tx)
	}
}

func TestEditAuthorizesBeforeValidationOrNoOp(t *testing.T) {
	for _, test := range []struct {
		name        string
		actor       Actor
		exists      bool
		coordinator bool
		want        error
		wantBegins  int
	}{
		{name: "missing actor", want: ErrUnauthenticated},
		{name: "malformed actor", actor: Actor{AccountID: "not-an-id"}, want: ErrUnauthenticated},
		{name: "unknown account", actor: Actor{AccountID: creationCoordinatorID}, want: ErrUnauthenticated, wantBegins: 1},
		{name: "engineer same-value request", actor: Actor{AccountID: creationEngineerID}, exists: true, want: ErrForbidden, wantBegins: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := newEditFakeRepository(Resource{
				ID: creationResourceID, Code: "Stored-Code", Name: "Same", Description: "Same description",
			})
			repository.exists, repository.coordinator = test.exists, test.coordinator
			service := NewService(repository, time.Now)
			_, err := service.Edit(context.Background(), test.actor, EditRequest{
				ResourceID: creationResourceID, Code: "Stored-Code", Name: "Same", Description: "Same description",
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Edit error = %v, want %v", err, test.want)
			}
			if repository.beginCalls != test.wantBegins || (repository.tx != nil &&
				(repository.tx.lockCalls != 0 || repository.tx.editCalls != 0 || repository.tx.commitCalls != 0)) {
				t.Fatalf("unauthorized edit reached Resource lookup/mutation: begins=%d tx=%+v", repository.beginCalls, repository.tx)
			}
		})
	}
}

func TestEditRejectsInvalidInputAndImmutableCodeAssertionsBeforeMutation(t *testing.T) {
	for _, input := range []EditRequest{
		{ResourceID: creationResourceID, Code: "Stored-Code", Name: "\u2003\u00a0"},
		{ResourceID: creationResourceID, Code: "Stored-Code", Name: "Name", Description: string([]byte{0xff})},
		{ResourceID: "not-a-resource-id", Code: "Stored-Code", Name: "Name"},
	} {
		repository := newEditFakeRepository(Resource{ID: creationResourceID, Code: "Stored-Code", Name: "Name"})
		service := NewService(repository, time.Now)
		_, err := service.Edit(context.Background(), Actor{AccountID: creationCoordinatorID}, input)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid edit %+v returned %v", input, err)
		}
		if repository.tx.lockCalls != 0 || repository.tx.editCalls != 0 || repository.tx.commitCalls != 0 {
			t.Fatalf("invalid input reached Resource mutation: %+v", repository.tx)
		}
	}

	for _, code := range []string{"Different-Code", "other-existing-code", "stored-code"} {
		repository := newEditFakeRepository(Resource{
			ID: creationResourceID, Code: "Stored-Code", Name: "Original", Description: "Original description",
		})
		service := NewService(repository, time.Now)
		_, err := service.Edit(context.Background(), Actor{AccountID: creationCoordinatorID}, EditRequest{
			ResourceID: creationResourceID, Code: code, Name: "Must not change", Description: "Must not change",
		})
		if !errors.Is(err, ErrImmutableCode) {
			t.Fatalf("code assertion %q returned %v, want immutable-code rejection", code, err)
		}
		if repository.tx.editCalls != 0 || repository.tx.commitCalls != 0 || repository.tx.resource.Name != "Original" {
			t.Fatalf("immutable-code rejection changed Resource: %+v", repository.tx)
		}
	}
}

func TestEditReturnsUnknownAndSuccessfulNoOpOutcomes(t *testing.T) {
	t.Run("unknown resource", func(t *testing.T) {
		repository := newEditFakeRepository(Resource{ID: creationResourceID, Code: "Stored-Code"})
		repository.found = false
		service := NewService(repository, time.Now)
		_, err := service.Edit(context.Background(), Actor{AccountID: creationCoordinatorID}, EditRequest{
			ResourceID: creationResourceID, Code: "Stored-Code", Name: "Name",
		})
		if !errors.Is(err, ErrUnknownResource) || repository.tx.editCalls != 0 || repository.tx.commitCalls != 0 {
			t.Fatalf("unknown Resource result: error=%v transaction=%+v", err, repository.tx)
		}
	})

	t.Run("unchanged canonical values", func(t *testing.T) {
		createdAt := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
		updatedAt := createdAt.Add(time.Minute)
		repository := newEditFakeRepository(Resource{
			ID: creationResourceID, Code: "Stored-Code", Name: "Same", Description: "Same description",
			Active: true, CreatedAt: createdAt, UpdatedAt: updatedAt,
		})
		service := NewService(repository, func() time.Time { return updatedAt.Add(time.Hour) })
		result, err := service.Edit(context.Background(), Actor{AccountID: creationCoordinatorID}, EditRequest{
			ResourceID: creationResourceID, Code: "\u2003Stored-Code\u00a0",
			Name: "\u00a0Same\u2003", Description: "\u2003Same description\u00a0",
		})
		if err != nil {
			t.Fatalf("unchanged edit: %v", err)
		}
		if result.Changed || !result.Resource.UpdatedAt.Equal(updatedAt) || repository.tx.editCalls != 1 ||
			!repository.tx.committed || !equalStrings(repository.tx.trace, []string{"authorize", "lock", "edit", "commit"}) {
			t.Fatalf("unchanged edit result=%+v transaction=%+v", result, repository.tx)
		}
	})
}

type editFakeRepository struct {
	exists      bool
	coordinator bool
	found       bool
	resource    Resource
	lockErr     error
	editErr     error
	commitErr   error
	beginCalls  int
	tx          *editFakeTransaction
}

func newEditFakeRepository(resource Resource) *editFakeRepository {
	return &editFakeRepository{exists: true, coordinator: true, found: true, resource: resource}
}

func (r *editFakeRepository) BeginResourceCreation(context.Context) (CreationTransaction, error) {
	return nil, errors.New("unexpected Resource creation transaction")
}

func (r *editFakeRepository) BeginResourceEdit(context.Context) (EditTransaction, error) {
	r.beginCalls++
	r.tx = &editFakeTransaction{repository: r, resource: r.resource, found: r.found}
	return r.tx, nil
}

type editFakeTransaction struct {
	repository  *editFakeRepository
	trace       []string
	resource    Resource
	found       bool
	locked      bool
	actorID     string
	resourceID  string
	name        string
	description string
	occurredAt  time.Time
	lockCalls   int
	editCalls   int
	commitCalls int
	committed   bool
}

func (tx *editFakeTransaction) ResolveCoordinator(_ context.Context, actorID string) (bool, bool, error) {
	tx.trace = append(tx.trace, "authorize")
	tx.actorID = actorID
	return tx.repository.exists, tx.repository.coordinator, nil
}

func (tx *editFakeTransaction) LockResource(_ context.Context, resourceID string) (Resource, bool, error) {
	tx.trace = append(tx.trace, "lock")
	tx.lockCalls++
	tx.locked, tx.resourceID = true, resourceID
	if tx.repository.lockErr != nil {
		return Resource{}, false, tx.repository.lockErr
	}
	return tx.resource, tx.found, nil
}

func (tx *editFakeTransaction) EditResource(_ context.Context, actorID, resourceID, name, description string, occurredAt time.Time) (Resource, bool, error) {
	tx.trace = append(tx.trace, "edit")
	tx.editCalls++
	tx.actorID, tx.resourceID = actorID, resourceID
	tx.name, tx.description, tx.occurredAt = name, description, occurredAt
	if tx.repository.editErr != nil {
		return Resource{}, false, tx.repository.editErr
	}
	changed := tx.resource.Name != name || tx.resource.Description != description
	if changed {
		tx.resource.Name, tx.resource.Description, tx.resource.UpdatedAt = name, description, occurredAt
	}
	return tx.resource, changed, nil
}

func (tx *editFakeTransaction) Commit(context.Context) error {
	tx.trace = append(tx.trace, "commit")
	tx.commitCalls++
	if tx.repository.commitErr != nil {
		return tx.repository.commitErr
	}
	tx.committed = true
	return nil
}

func (tx *editFakeTransaction) Rollback(context.Context) error {
	if !tx.committed {
		tx.trace = append(tx.trace, "rollback")
	}
	return nil
}
