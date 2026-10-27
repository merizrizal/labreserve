package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	creationCoordinatorID = "11111111-1111-4111-8111-111111111111"
	creationEngineerID    = "22222222-2222-4222-8222-222222222222"
	creationResourceID    = "33333333-3333-4333-8333-333333333333"
)

func TestCreateUsesCanonicalValuesAndExplicitInitialState(t *testing.T) {
	now := time.Date(2040, time.January, 2, 3, 4, 5, 123456789, time.FixedZone("test", 7*60*60))
	for _, active := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "inactive"}[active], func(t *testing.T) {
			repository := &creationFakeRepository{exists: true, coordinator: true}
			service := NewService(repository, func() time.Time { return now })
			result, err := service.Create(context.Background(), Actor{AccountID: strings.ToUpper(creationCoordinatorID)}, CreateRequest{
				Code: "\u2003MiX/Code\t value\u00a0", Name: "\u00a0Network\tLab  west\u2003",
				Description: "\u2003line one\t internal\nline two\u00a0", Active: active,
			})
			if err != nil {
				t.Fatalf("create Resource: %v", err)
			}
			if result.Resource.ID != creationResourceID || result.Resource.Code != "MiX/Code\t value" ||
				result.Resource.Name != "Network\tLab  west" || result.Resource.Description != "line one\t internal\nline two" ||
				result.Resource.Active != active || !result.Resource.CreatedAt.Equal(now.UTC().Truncate(time.Microsecond)) ||
				!result.Resource.UpdatedAt.Equal(result.Resource.CreatedAt) {
				t.Fatalf("creation result = %+v", result.Resource)
			}
			if repository.tx.actorID != creationCoordinatorID || repository.tx.input.Code != result.Resource.Code ||
				repository.tx.input.Name != result.Resource.Name || repository.tx.input.Description != result.Resource.Description ||
				repository.tx.input.Active != active || !repository.tx.occurredAt.Equal(result.Resource.CreatedAt) {
				t.Fatalf("persistence values = actor %q request %+v time %s", repository.tx.actorID, repository.tx.input, repository.tx.occurredAt)
			}
			if !repository.tx.committed || repository.tx.createCalls != 1 || repository.tx.commitCalls != 1 ||
				!equalStrings(repository.tx.trace, []string{"authorize", "create", "commit"}) {
				t.Fatalf("transaction trace = %v, create/commit=%d/%d committed=%v", repository.tx.trace,
					repository.tx.createCalls, repository.tx.commitCalls, repository.tx.committed)
			}
		})
	}
}

func TestCreateRejectsUnauthenticatedAndNonCoordinatorActorsBeforeMutation(t *testing.T) {
	tests := []struct {
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
		{name: "engineer", actor: Actor{AccountID: creationEngineerID}, exists: true, want: ErrForbidden, wantBegins: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &creationFakeRepository{exists: test.exists, coordinator: test.coordinator}
			service := NewService(repository, time.Now)
			_, err := service.Create(context.Background(), test.actor, CreateRequest{Code: "Code", Name: "Name"})
			if !errors.Is(err, test.want) {
				t.Fatalf("Create error = %v, want %v", err, test.want)
			}
			if repository.beginCalls != test.wantBegins || (repository.tx != nil && repository.tx.createCalls != 0) {
				t.Fatalf("unauthorized creation reached mutation: begins=%d tx=%+v", repository.beginCalls, repository.tx)
			}
		})
	}
}

func TestCreateValidatesRequiredTextAfterCoordinatorAuthorization(t *testing.T) {
	for _, input := range []CreateRequest{
		{Name: "Name"},
		{Code: "\u2003\u00a0", Name: "Name"},
		{Code: "Code"},
		{Code: "Code", Name: "\u2003\u00a0"},
	} {
		repository := &creationFakeRepository{exists: true, coordinator: true}
		service := NewService(repository, time.Now)
		_, err := service.Create(context.Background(), Actor{AccountID: creationCoordinatorID}, input)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Create with invalid required fields %+v returned %v", input, err)
		}
		if repository.tx.createCalls != 0 || repository.tx.commitCalls != 0 {
			t.Fatalf("invalid input reached mutation/commit: %+v", repository.tx)
		}
	}
}

func TestCreateMapsDuplicateAndDoesNotRetryUnknownCommitOutcome(t *testing.T) {
	t.Run("duplicate code", func(t *testing.T) {
		repository := &creationFakeRepository{
			exists: true, coordinator: true, createErr: ErrDuplicateCode,
		}
		service := NewService(repository, time.Now)
		_, err := service.Create(context.Background(), Actor{AccountID: creationCoordinatorID}, CreateRequest{Code: "Code", Name: "Name"})
		if !errors.Is(err, ErrDuplicateCode) || repository.tx.commitCalls != 0 {
			t.Fatalf("duplicate creation error=%v commit calls=%d", err, repository.tx.commitCalls)
		}
	})

	t.Run("commit acknowledgement failure is not retried", func(t *testing.T) {
		commitErr := errors.New("simulated lost commit acknowledgement")
		repository := &creationFakeRepository{exists: true, coordinator: true, commitErr: commitErr}
		service := NewService(repository, time.Now)
		_, err := service.Create(context.Background(), Actor{AccountID: creationCoordinatorID}, CreateRequest{Code: "Code", Name: "Name"})
		if !errors.Is(err, ErrOperational) || repository.tx.createCalls != 1 || repository.tx.commitCalls != 1 ||
			strings.Contains(err.Error(), commitErr.Error()) {
			t.Fatalf("commit failure = %v; create/commit calls=%d/%d", err, repository.tx.createCalls, repository.tx.commitCalls)
		}
	})
}

type creationFakeRepository struct {
	exists      bool
	coordinator bool
	createErr   error
	commitErr   error
	beginCalls  int
	tx          *creationFakeTransaction
}

func (r *creationFakeRepository) BeginResourceCreation(context.Context) (CreationTransaction, error) {
	r.beginCalls++
	r.tx = &creationFakeTransaction{repository: r}
	return r.tx, nil
}

func (r *creationFakeRepository) BeginResourceEdit(context.Context) (EditTransaction, error) {
	return nil, errors.New("unexpected Resource edit transaction")
}

type creationFakeTransaction struct {
	repository  *creationFakeRepository
	trace       []string
	actorID     string
	input       CreateRequest
	occurredAt  time.Time
	createCalls int
	commitCalls int
	committed   bool
}

func (tx *creationFakeTransaction) ResolveCoordinator(_ context.Context, actorID string) (bool, bool, error) {
	tx.trace = append(tx.trace, "authorize")
	tx.actorID = actorID
	return tx.repository.exists, tx.repository.coordinator, nil
}

func (tx *creationFakeTransaction) CreateResource(_ context.Context, actorID string, input CreateRequest, occurredAt time.Time) (Resource, error) {
	tx.trace = append(tx.trace, "create")
	tx.createCalls++
	tx.actorID, tx.input, tx.occurredAt = actorID, input, occurredAt
	if tx.repository.createErr != nil {
		return Resource{}, tx.repository.createErr
	}
	return Resource{
		ID: creationResourceID, Code: input.Code, Name: input.Name, Description: input.Description,
		Active: input.Active, CreatedAt: occurredAt, UpdatedAt: occurredAt,
	}, nil
}

func (tx *creationFakeTransaction) Commit(context.Context) error {
	tx.trace = append(tx.trace, "commit")
	tx.commitCalls++
	if tx.repository.commitErr != nil {
		return tx.repository.commitErr
	}
	tx.committed = true
	return nil
}

func (tx *creationFakeTransaction) Rollback(context.Context) error {
	if !tx.committed {
		tx.trace = append(tx.trace, "rollback")
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
