package resources

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrUnauthenticated = errors.New("resource operation requires an authenticated actor")
	ErrForbidden       = errors.New("resource operation requires coordinator authority")
	ErrInvalidInput    = errors.New("invalid resource input")
	ErrUnknownResource = errors.New("unknown resource")
	ErrImmutableCode   = errors.New("resource code is immutable")
	ErrDuplicateCode   = errors.New("resource code already exists")
	ErrOperational     = errors.New("resource service unavailable")
)

type Actor struct {
	AccountID string
}

type Resource struct {
	ID          string
	Code        string
	Name        string
	Description string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateRequest struct {
	Code        string
	Name        string
	Description string
	// Active is an explicit creation choice; false represents an Inactive Resource.
	Active bool
}

type EditRequest struct {
	ResourceID string
	// Code is an assertion of the immutable existing code, not a mutable field.
	Code        string
	Name        string
	Description string
}

type CreateResult struct {
	Resource Resource
}

type EditResult struct {
	Resource Resource
	// Changed distinguishes a persisted edit from a successful unchanged no-op.
	Changed bool
}

type createValues struct {
	Code        string
	Name        string
	Description string
	Active      bool
}

type editValues struct {
	ResourceID  string
	Code        string
	Name        string
	Description string
}

func canonicalizeCreate(input CreateRequest) (createValues, error) {
	code, err := canonicalRequiredText(input.Code)
	if err != nil {
		return createValues{}, err
	}
	name, err := canonicalRequiredText(input.Name)
	if err != nil {
		return createValues{}, err
	}
	description, err := canonicalOptionalText(input.Description)
	if err != nil {
		return createValues{}, err
	}
	return createValues{
		Code: code, Name: name, Description: description, Active: input.Active,
	}, nil
}

func canonicalizeEdit(input EditRequest) (editValues, error) {
	code, err := canonicalRequiredText(input.Code)
	if err != nil {
		return editValues{}, err
	}
	name, err := canonicalRequiredText(input.Name)
	if err != nil {
		return editValues{}, err
	}
	description, err := canonicalOptionalText(input.Description)
	if err != nil {
		return editValues{}, err
	}
	return editValues{
		ResourceID:  input.ResourceID,
		Code:        code,
		Name:        name,
		Description: description,
	}, nil
}

func canonicalRequiredText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", ErrInvalidInput
	}
	canonical := strings.TrimSpace(value)
	if canonical == "" {
		return "", ErrInvalidInput
	}
	return canonical, nil
}

func canonicalOptionalText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", ErrInvalidInput
	}
	return strings.TrimSpace(value), nil
}

var resourceActorUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type ResourceTransaction interface {
	ResolveCoordinator(context.Context, string) (exists bool, coordinator bool, err error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type CreationRepository interface {
	BeginResourceCreation(context.Context) (CreationTransaction, error)
}

type CreationTransaction interface {
	ResourceTransaction
	CreateResource(context.Context, string, CreateRequest, time.Time) (Resource, error)
}

type EditingRepository interface {
	BeginResourceEdit(context.Context) (EditTransaction, error)
}

type EditTransaction interface {
	ResourceTransaction
	LockResource(context.Context, string) (Resource, bool, error)
	EditResource(context.Context, string, string, string, string, time.Time) (Resource, bool, error)
}

type Repository interface {
	CreationRepository
	EditingRepository
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

// Create uses only the trusted application actor identity; role claims are
// resolved from persisted account authority and rechecked by the guarded SQL capability.
func (s *Service) Create(ctx context.Context, actor Actor, input CreateRequest) (CreateResult, error) {
	if s == nil || s.repository == nil {
		return CreateResult{}, operationalError("initialize resource service")
	}
	actorID, actorOK := canonicalResourceActorID(actor.AccountID)
	if !actorOK {
		return CreateResult{}, ErrUnauthenticated
	}

	tx, err := s.repository.BeginResourceCreation(ctx)
	if err != nil {
		return CreateResult{}, operationalError("begin resource creation")
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	exists, coordinator, err := tx.ResolveCoordinator(ctx, actorID)
	if err != nil {
		return CreateResult{}, operationalError("resolve resource actor")
	}
	if !exists {
		return CreateResult{}, ErrUnauthenticated
	}
	if !coordinator {
		return CreateResult{}, ErrForbidden
	}

	values, err := canonicalizeCreate(input)
	if err != nil {
		return CreateResult{}, err
	}
	canonicalInput := CreateRequest{
		Code: values.Code, Name: values.Name, Description: values.Description, Active: values.Active,
	}
	occurredAt := s.now().UTC().Truncate(time.Microsecond)
	resource, err := tx.CreateResource(ctx, actorID, canonicalInput, occurredAt)
	if errors.Is(err, ErrDuplicateCode) {
		return CreateResult{}, ErrDuplicateCode
	}
	if errors.Is(err, ErrForbidden) {
		return CreateResult{}, ErrForbidden
	}
	if err != nil {
		return CreateResult{}, operationalError("create resource")
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, operationalError("commit resource creation")
	}
	return CreateResult{Resource: resource}, nil
}

func (s *Service) Edit(ctx context.Context, actor Actor, input EditRequest) (EditResult, error) {
	if s == nil || s.repository == nil {
		return EditResult{}, operationalError("initialize resource service")
	}
	actorID, actorOK := canonicalResourceActorID(actor.AccountID)
	if !actorOK {
		return EditResult{}, ErrUnauthenticated
	}

	tx, err := s.repository.BeginResourceEdit(ctx)
	if err != nil {
		return EditResult{}, operationalError("begin resource edit")
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	exists, coordinator, err := tx.ResolveCoordinator(ctx, actorID)
	if err != nil {
		return EditResult{}, operationalError("resolve resource actor")
	}
	if !exists {
		return EditResult{}, ErrUnauthenticated
	}
	if !coordinator {
		return EditResult{}, ErrForbidden
	}

	values, err := canonicalizeEdit(input)
	if err != nil {
		return EditResult{}, err
	}
	resourceID, resourceIDOK := canonicalResourceUUID(values.ResourceID)
	if !resourceIDOK {
		return EditResult{}, ErrInvalidInput
	}
	locked, found, err := tx.LockResource(ctx, resourceID)
	if err != nil {
		return EditResult{}, operationalError("lock resource for edit")
	}
	if !found {
		return EditResult{}, ErrUnknownResource
	}
	if values.Code != locked.Code {
		return EditResult{}, ErrImmutableCode
	}

	occurredAt := s.now().UTC().Truncate(time.Microsecond)
	resource, changed, err := tx.EditResource(ctx, actorID, resourceID, values.Name, values.Description, occurredAt)
	if errors.Is(err, ErrUnknownResource) {
		return EditResult{}, ErrUnknownResource
	}
	if errors.Is(err, ErrForbidden) {
		return EditResult{}, ErrForbidden
	}
	if err != nil {
		return EditResult{}, operationalError("edit resource")
	}
	if err := tx.Commit(ctx); err != nil {
		return EditResult{}, operationalError("commit resource edit")
	}
	return EditResult{Resource: resource, Changed: changed}, nil
}

func canonicalResourceActorID(value string) (string, bool) {
	return canonicalResourceUUID(value)
}

func canonicalResourceUUID(value string) (string, bool) {
	if !resourceActorUUIDPattern.MatchString(value) {
		return "", false
	}
	return strings.ToLower(value), true
}

func operationalError(operation string) error {
	return fmt.Errorf("%w: %s", ErrOperational, operation)
}
