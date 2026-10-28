package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/resources"
)

const resourceCommitRejectionMessage = "injected definite Resource COMMIT rejection"

func TestResourceCreationDefiniteCommitRejectionRollsBackResourceAndActivity(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	operationTime := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	observation := &resourceCommitRejectionObservation{}
	service := resources.NewService(&resourceCommitRejectionRepository{
		Repository: NewStore(appPool), observation: observation,
	}, func() time.Time { return operationTime })
	code := "Commit-rejection-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", "")
	removeRejection := installResourceCommitRejectionTrigger(
		t, adminPool, fixtures.coordinator, "resource.created", code, "",
	)

	_, serviceErr := service.Create(t.Context(), resources.Actor{AccountID: fixtures.coordinator}, resources.CreateRequest{
		Code: code, Name: "Must roll back", Description: "creation commit rejection", Active: false,
	})
	removeRejection()
	if !errors.Is(serviceErr, resources.ErrOperational) {
		t.Fatalf("definite creation COMMIT rejection returned %v, want operational failure", serviceErr)
	}
	assertResourceCommitRejected(t, observation, true, serviceErr)
	assertResourceCreationAbsent(t, adminPool, code)
}

func TestResourceEditDefiniteCommitRejectionRollsBackResourceAndActivity(t *testing.T) {
	adminPool, appPool, fixtures := resourceCreationSetup(t)
	operationTime := time.Date(2040, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	store := NewStore(appPool)
	actor := resources.Actor{AccountID: fixtures.coordinator}
	service := resources.NewService(store, func() time.Time { return operationTime })
	created, err := service.Create(t.Context(), actor, resources.CreateRequest{
		Code: "Edit-commit-rejection-" + strings.ReplaceAll(testDatabaseUUID(t, adminPool), "-", ""),
		Name: "Original name", Description: "Original description", Active: true,
	})
	if err != nil {
		t.Fatalf("create Resource for edit commit-rejection test: %v", err)
	}
	fixtures.trackResource(created.Resource.ID)
	before := created.Resource
	observation := &resourceCommitRejectionObservation{}
	observedService := resources.NewService(&resourceCommitRejectionRepository{
		Repository: store, observation: observation,
	}, func() time.Time { return operationTime })
	operationTime = operationTime.Add(time.Minute)
	removeRejection := installResourceCommitRejectionTrigger(
		t, adminPool, fixtures.coordinator, "resource.descriptive_edited", "", before.ID,
	)

	_, serviceErr := observedService.Edit(t.Context(), actor, resources.EditRequest{
		ResourceID: before.ID, Code: before.Code,
		Name: "Changed name", Description: "Changed description",
	})
	removeRejection()
	if !errors.Is(serviceErr, resources.ErrOperational) {
		t.Fatalf("definite edit COMMIT rejection returned %v, want operational failure", serviceErr)
	}
	assertResourceCommitRejected(t, observation, true, serviceErr)

	after, found, err := store.ResourceByID(t.Context(), before.ID)
	if err != nil || !found {
		t.Fatalf("read Resource after rejected edit COMMIT: found=%v error=%v", found, err)
	}
	var createdAt, updatedAt time.Time
	if err := adminPool.QueryRow(t.Context(), `
		SELECT created_at, updated_at FROM public.resources WHERE id = $1::uuid`, before.ID).Scan(
		&createdAt, &updatedAt); err != nil {
		t.Fatalf("read Resource timestamps after rejected edit COMMIT: %v", err)
	}
	if after.ID != before.ID || after.Code != before.Code || after.Name != before.Name ||
		after.Description != before.Description || after.Active != before.Active ||
		!createdAt.Equal(before.CreatedAt) || !updatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("definite edit COMMIT rejection persisted Resource mutation: before=%+v after=%+v created=%s updated=%s",
			before, after, createdAt, updatedAt)
	}
	events := readResourceCapabilityEvents(t, adminPool, before.ID)
	if len(events) != 1 || events[0].Action != "resource.created" {
		t.Fatalf("definite edit COMMIT rejection persisted a descriptive Activity event: %+v", events)
	}
}

type resourceCommitRejectionObservation struct {
	mutationCalls int
	resource      resources.Resource
	changed       bool
	commitCalls   int
	commitErr     error
}

type resourceCommitRejectionRepository struct {
	resources.Repository
	observation *resourceCommitRejectionObservation
}

func (r *resourceCommitRejectionRepository) BeginResourceCreation(ctx context.Context) (resources.CreationTransaction, error) {
	tx, err := r.Repository.BeginResourceCreation(ctx)
	if err != nil {
		return nil, err
	}
	return &observedResourceCreationTransaction{
		CreationTransaction: tx, observation: r.observation,
	}, nil
}

func (r *resourceCommitRejectionRepository) BeginResourceEdit(ctx context.Context) (resources.EditTransaction, error) {
	tx, err := r.Repository.BeginResourceEdit(ctx)
	if err != nil {
		return nil, err
	}
	return &observedResourceEditTransaction{
		EditTransaction: tx, observation: r.observation,
	}, nil
}

type observedResourceCreationTransaction struct {
	resources.CreationTransaction
	observation *resourceCommitRejectionObservation
}

func (tx *observedResourceCreationTransaction) CreateResource(
	ctx context.Context, actorID string, input resources.CreateRequest, occurredAt time.Time,
) (resources.Resource, error) {
	resource, err := tx.CreationTransaction.CreateResource(ctx, actorID, input, occurredAt)
	if err == nil {
		tx.observation.mutationCalls++
		tx.observation.resource = resource
		tx.observation.changed = true
	}
	return resource, err
}

func (tx *observedResourceCreationTransaction) Commit(ctx context.Context) error {
	tx.observation.commitCalls++
	tx.observation.commitErr = tx.CreationTransaction.Commit(ctx)
	return tx.observation.commitErr
}

type observedResourceEditTransaction struct {
	resources.EditTransaction
	observation *resourceCommitRejectionObservation
}

func (tx *observedResourceEditTransaction) EditResource(
	ctx context.Context, actorID, resourceID, name, description string, occurredAt time.Time,
) (resources.Resource, bool, error) {
	resource, changed, err := tx.EditTransaction.EditResource(ctx, actorID, resourceID, name, description, occurredAt)
	if err == nil {
		tx.observation.mutationCalls++
		tx.observation.resource = resource
		tx.observation.changed = changed
	}
	return resource, changed, err
}

func (tx *observedResourceEditTransaction) Commit(ctx context.Context) error {
	tx.observation.commitCalls++
	tx.observation.commitErr = tx.EditTransaction.Commit(ctx)
	return tx.observation.commitErr
}

func assertResourceCommitRejected(
	t *testing.T, observation *resourceCommitRejectionObservation, wantChanged bool, serviceErr error,
) {
	t.Helper()
	if observation.mutationCalls != 1 || observation.resource.ID == "" || observation.changed != wantChanged {
		t.Fatalf("Resource mutation statement did not return success before COMMIT: observation=%+v", observation)
	}
	if observation.commitCalls != 1 || observation.commitErr == nil {
		t.Fatalf("observed COMMIT calls/error = %d/%v, want one rejected COMMIT", observation.commitCalls, observation.commitErr)
	}
	var postgresErr *pgconn.PgError
	if !errors.As(observation.commitErr, &postgresErr) || postgresErr.Code != "P0001" ||
		postgresErr.Message != resourceCommitRejectionMessage {
		t.Fatalf("COMMIT error = %v, want the fixture's definite PostgreSQL rejection", observation.commitErr)
	}
	if strings.Contains(serviceErr.Error(), resourceCommitRejectionMessage) {
		t.Fatalf("service leaked PostgreSQL COMMIT diagnostics: %v", serviceErr)
	}
}

func installResourceCommitRejectionTrigger(
	t *testing.T, admin *pgxpool.Pool, actorID, action, code, resourceID string,
) func() {
	t.Helper()
	var scope string
	switch action {
	case "resource.created":
		if code == "" || resourceID != "" {
			t.Fatalf("creation COMMIT rejection trigger requires only a Resource code scope")
		}
		scope = fmt.Sprintf("NEW.details ->> 'code' = %s", sqlLiteral(code))
	case "resource.descriptive_edited":
		if resourceID == "" || code != "" {
			t.Fatalf("edit COMMIT rejection trigger requires only a Resource ID scope")
		}
		scope = fmt.Sprintf("NEW.resource_id = %s::uuid", sqlLiteral(resourceID))
	default:
		t.Fatalf("unsupported Resource COMMIT rejection action %q", action)
	}
	scope = fmt.Sprintf("NEW.actor_account_id = %s::uuid AND NEW.action = %s AND %s",
		sqlLiteral(actorID), sqlLiteral(action), scope)

	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionName := "labreserve_test_resource_commit_reject_" + suffix
	triggerName := functionName
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION public.%s() RETURNS trigger LANGUAGE plpgsql AS $test$
		BEGIN
		    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = %s;
		END;
		$test$`, functionName, sqlLiteral(resourceCommitRejectionMessage))); err != nil {
		t.Fatalf("create deferred Resource COMMIT rejection function: %v", err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerName+" ON public.activity_events"); err != nil {
			t.Errorf("drop deferred Resource COMMIT rejection trigger: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS public."+functionName+"()"); err != nil {
			t.Errorf("drop deferred Resource COMMIT rejection function: %v", err)
		}
	}
	t.Cleanup(cleanup)
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE CONSTRAINT TRIGGER %s
		AFTER INSERT ON public.activity_events
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW WHEN (%s)
		EXECUTE FUNCTION public.%s()`, triggerName, scope, functionName)); err != nil {
		t.Fatalf("install deferred Resource COMMIT rejection trigger: %v", err)
	}
	var deferred, initiallyDeferred bool
	if err := admin.QueryRow(t.Context(), `
		SELECT tgdeferrable, tginitdeferred
		FROM pg_catalog.pg_trigger
		WHERE tgrelid = 'public.activity_events'::regclass AND tgname = $1`, triggerName).Scan(
		&deferred, &initiallyDeferred); err != nil {
		t.Fatalf("inspect deferred Resource COMMIT rejection trigger: %v", err)
	}
	if !deferred || !initiallyDeferred {
		t.Fatalf("test rejection trigger deferrable/initially deferred = %v/%v", deferred, initiallyDeferred)
	}
	return cleanup
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
