package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"labreserve.local/labreserve/internal/bookings"
	"labreserve.local/labreserve/internal/resources"
)

func TestResourceStateBookingRaceBookingCommitsBeforeDeactivation(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)

	group := newBookingRaceGroup(t)
	creatorGate := group.newGate()
	creatorStarted := make(chan int32, 1)
	creatorClock := newAtomicBookingClockAt(now)
	creatorService := bookings.NewService(&bookingRaceRepository{
		base: NewStore(app), started: creatorStarted, beforeCommit: creatorGate,
	}, creatorClock.Now)
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, time.Hour, time.Hour)
	creatorCtx, cancelCreator := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCreator()
	creator := startBookingRaceCall(group, creatorCtx, creatorService,
		bookings.Actor{AccountID: fixtures.engineer}, request)
	creatorPID := awaitBookingRacePID(t, creatorCtx, creatorStarted, "booking creator transaction")
	if gatePID := creatorGate.await(t, creatorCtx, "booking creator before COMMIT"); gatePID != creatorPID {
		t.Fatalf("creator gate backend PID=%d, transaction PID=%d", gatePID, creatorPID)
	}

	deactivationClock := newAtomicBookingClockAt(now)
	stateStarted := make(chan int32, 1)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted,
	}, deactivationClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation transaction")
	waitCtx, cancelWait := context.WithTimeout(stateCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, statePID, creatorPID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe deactivation waiting for uncommitted Booking creator: %v", err)
	}
	logBookingRaceWait(t, "deactivation waits for booking creation", wait)

	creatorGate.open()
	creatorOutcome := awaitBookingRaceCall(t, creatorCtx, creator)
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	if creatorOutcome.err != nil || creatorOutcome.result.Replayed || creatorOutcome.result.Booking.ID == "" {
		t.Fatalf("leading booking creation result=%+v error=%v", creatorOutcome.result, creatorOutcome.err)
	}
	if !errors.Is(stateOutcome.err, resources.ErrResourceOccupied) || stateOutcome.result != (resources.StateResult{}) {
		t.Fatalf("waiting deactivation result=%+v error=%v, want occupied after creator commit", stateOutcome.result, stateOutcome.err)
	}
	if creatorClock.calls.Load() != 1 || deactivationClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: creator=%d deactivation=%d, want one each", creatorClock.calls.Load(), deactivationClock.calls.Load())
	}
	assertBookingCreationRacePersisted(t, admin, fixtures.engineer, request, creatorOutcome.result.Booking.ID, true, now)
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("blocked deactivation changed Resource/history: before=%s %s after=%s %s", beforeResource, beforeEvents, afterResource, afterEvents)
	}
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 1, 1)
	*bookingIDs = append(*bookingIDs, creatorOutcome.result.Booking.ID)
	assertBookingRaceResource(t, admin, resourceID, true, createdAt, createdAt)
}

func TestResourceStateBookingRaceDeactivationCommitsBeforeBooking(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	stateClock := newAtomicBookingClockAt(now)
	group := newBookingRaceGroup(t)
	stateCommitGate := group.newGate()
	stateStarted := make(chan int32, 1)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted, beforeCommit: stateCommitGate,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "deactivation transaction")
	if gatePID := stateCommitGate.await(t, stateCtx, "deactivation after mutation and Activity"); gatePID != statePID {
		t.Fatalf("deactivation gate backend PID=%d, transaction PID=%d", gatePID, statePID)
	}

	creatorClock := newAtomicBookingClockAt(now)
	creatorStarted := make(chan int32, 1)
	creatorService := bookings.NewService(&bookingRaceRepository{
		base: NewStore(app), started: creatorStarted,
	}, creatorClock.Now)
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, time.Hour, time.Hour)
	creatorCtx, cancelCreator := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCreator()
	creator := startBookingRaceCall(group, creatorCtx, creatorService,
		bookings.Actor{AccountID: fixtures.engineer}, request)
	creatorPID := awaitBookingRacePID(t, creatorCtx, creatorStarted, "booking creator transaction")
	waitCtx, cancelWait := context.WithTimeout(creatorCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, creatorPID, statePID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe creator waiting for uncommitted deactivation: %v", err)
	}
	logBookingRaceWait(t, "booking creation waits for deactivation", wait)

	stateCommitGate.open()
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	creatorOutcome := awaitBookingRaceCall(t, creatorCtx, creator)
	if stateOutcome.err != nil || !stateOutcome.result.Changed || stateOutcome.result.Resource.Active {
		t.Fatalf("leading deactivation result=%+v error=%v", stateOutcome.result, stateOutcome.err)
	}
	if !errors.Is(creatorOutcome.err, bookings.ErrInactiveResource) || creatorOutcome.result != (bookings.CreateResult{}) {
		t.Fatalf("waiting booking result=%+v error=%v, want inactive Resource rejection", creatorOutcome.result, creatorOutcome.err)
	}
	if stateClock.calls.Load() != 1 || creatorClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: deactivation=%d creator=%d, want one each", stateClock.calls.Load(), creatorClock.calls.Load())
	}
	assertBookingRaceResource(t, admin, resourceID, false, createdAt, now)
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", now, true, false)
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 0, 0)
	if snapshot := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); snapshot != `{"events": [], "bookings": []}` {
		t.Fatalf("rejected creation left a persisted Booking/request binding or event: %s", snapshot)
	}
	_ = bookingIDs
}

func TestResourceStateBookingRaceSameCoordinatorAccountActivityLockCompatibility(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	assertBookingRaceLockModes(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	group := newBookingRaceGroup(t)
	stateSetGate := group.newGate()
	stateCommitGate := group.newGate()
	stateStarted := make(chan int32, 1)
	stateResourceLocked := make(chan int32, 1)
	stateClock := newAtomicBookingClockAt(now)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted, resourceLocked: stateResourceLocked,
		beforeSet: stateSetGate, beforeCommit: stateCommitGate,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "state transaction")
	if lockedPID := awaitBookingRacePID(t, stateCtx, stateResourceLocked, "state Resource lock acquisition"); lockedPID != statePID {
		t.Fatalf("state Resource lock backend PID=%d, transaction PID=%d", lockedPID, statePID)
	}
	if setPID := stateSetGate.await(t, stateCtx, "state capability call after Resource lock"); setPID != statePID {
		t.Fatalf("state capability gate backend PID=%d, transaction PID=%d", setPID, statePID)
	}

	creatorStarted := make(chan int32, 1)
	accountLocked := make(chan int32, 1)
	creatorClock := newAtomicBookingClockAt(now)
	creatorService := bookings.NewService(&bookingRaceRepository{
		base: NewStore(app), started: creatorStarted, accountLocked: accountLocked,
	}, creatorClock.Now)
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, time.Hour, time.Hour)
	creatorCtx, cancelCreator := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCreator()
	creator := startBookingRaceCall(group, creatorCtx, creatorService,
		bookings.Actor{AccountID: fixtures.coordinator}, request)
	creatorPID := awaitBookingRacePID(t, creatorCtx, creatorStarted, "same-Coordinator booking transaction")
	accountPID := awaitBookingRacePID(t, creatorCtx, accountLocked, "creator Account FOR NO KEY UPDATE acquisition")
	if creatorPID != accountPID {
		t.Fatalf("creator transaction PID=%d differs from Account lock PID=%d", creatorPID, accountPID)
	}
	waitCtx, cancelWait := context.WithTimeout(creatorCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, creatorPID, statePID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe same-Account creator waiting on state-held Resource: %v", err)
	}
	logBookingRaceWait(t, "same Coordinator holds Account lock then waits for Resource", wait)

	stateSetGate.open()
	if gatePID := stateCommitGate.await(t, stateCtx, "state Activity inserted while creator waits"); gatePID != statePID {
		t.Fatalf("state commit gate backend PID=%d, transaction PID=%d", gatePID, statePID)
	}
	waitCtx, cancelWait = context.WithTimeout(creatorCtx, 5*time.Second)
	stillWaiting, err := waitForBookingRaceLockWait(waitCtx, admin, creatorPID, statePID)
	cancelWait()
	if err != nil {
		t.Fatalf("creator stopped waiting before state Activity transaction committed: %v", err)
	}
	logBookingRaceWait(t, "state Activity completed while creator still waits", stillWaiting)

	stateCommitGate.open()
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	creatorOutcome := awaitBookingRaceCall(t, creatorCtx, creator)
	if stateOutcome.err != nil || !stateOutcome.result.Changed || stateOutcome.result.Resource.Active {
		t.Fatalf("state transition with same-Account creator waiting: result=%+v error=%v", stateOutcome.result, stateOutcome.err)
	}
	if !errors.Is(creatorOutcome.err, bookings.ErrInactiveResource) || creatorOutcome.result != (bookings.CreateResult{}) {
		t.Fatalf("same-Account creator final outcome=%+v error=%v, want inactive rejection", creatorOutcome.result, creatorOutcome.err)
	}
	if stateClock.calls.Load() != 1 || creatorClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: state=%d creator=%d, want one each", stateClock.calls.Load(), creatorClock.calls.Load())
	}
	assertBookingRaceResource(t, admin, resourceID, false, createdAt, now)
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", now, true, false)
	assertBookingRequestCounts(t, admin, fixtures.coordinator, request.RequestID, 0, 0)
	if snapshot := bookingRaceRequestSnapshot(t, admin, fixtures.coordinator, request.RequestID); snapshot != `{"events": [], "bookings": []}` {
		t.Fatalf("same-Account inactive rejection left a request binding: %s", snapshot)
	}
	_ = bookingIDs
}

func TestResourceStateBookingRaceFailedBookingRollsBackBeforeDeactivation(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, time.Hour, time.Hour)
	removeFailure := installBookingRaceActivityFailure(t, admin, fixtures.engineer, resourceID, request.RequestID)
	group := newBookingRaceGroup(t)
	appendGate := group.newGate()
	creatorStarted := make(chan int32, 1)
	creatorClock := newAtomicBookingClockAt(now)
	creatorService := bookings.NewService(&bookingRaceRepository{
		base: NewStore(app), started: creatorStarted, beforeAppend: appendGate, bookingIDs: bookingIDs,
	}, creatorClock.Now)
	creatorCtx, cancelCreator := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCreator()
	creator := startBookingRaceCall(group, creatorCtx, creatorService,
		bookings.Actor{AccountID: fixtures.engineer}, request)
	creatorPID := awaitBookingRacePID(t, creatorCtx, creatorStarted, "failing booking transaction")
	if gatePID := appendGate.await(t, creatorCtx, "Booking inserted before failing Activity append"); gatePID != creatorPID {
		t.Fatalf("pre-append gate backend PID=%d, transaction PID=%d", gatePID, creatorPID)
	}

	stateClock := newAtomicBookingClockAt(now)
	stateStarted := make(chan int32, 1)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "waiting deactivation transaction")
	waitCtx, cancelWait := context.WithTimeout(stateCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, statePID, creatorPID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe deactivation waiting for the uncommitted Booking insert: %v", err)
	}
	logBookingRaceWait(t, "deactivation waits for Booking whose Activity will fail", wait)

	appendGate.open()
	creatorOutcome := awaitBookingRaceCall(t, creatorCtx, creator)
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	removeFailure()
	if !errors.Is(creatorOutcome.err, bookings.ErrOperational) || strings.Contains(creatorOutcome.err.Error(), "injected booking Activity failure") {
		t.Fatalf("failed creator error=%v, want safe operational failure", creatorOutcome.err)
	}
	if stateOutcome.err != nil || !stateOutcome.result.Changed || stateOutcome.result.Resource.Active {
		t.Fatalf("deactivation after creator rollback: result=%+v error=%v", stateOutcome.result, stateOutcome.err)
	}
	if creatorClock.calls.Load() != 1 || stateClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: creator=%d deactivation=%d, want one each", creatorClock.calls.Load(), stateClock.calls.Load())
	}
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 0, 0)
	if snapshot := bookingRaceRequestSnapshot(t, admin, fixtures.engineer, request.RequestID); snapshot != `{"events": [], "bookings": []}` {
		t.Fatalf("failed creator retained a Booking/request identity or event: %s", snapshot)
	}
	assertBookingRaceResource(t, admin, resourceID, false, createdAt, now)
	assertResourceStateEvent(t, admin, resourceID, fixtures.coordinator, "resource.deactivated", now, true, false)
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource == beforeResource || afterEvents == beforeEvents {
		t.Fatalf("successful waiting deactivation not reflected in Resource/history: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
	_ = bookingIDs
}

func TestResourceStateBookingRaceFailedDeactivationRollsBackBeforeBooking(t *testing.T) {
	admin, app, fixtures := resourceCreationSetup(t)
	bookingIDs := registerResourceStateBookingCleanup(t, admin)
	now := bookingCreationNow()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertBookingRaceResource(t, admin, fixtures, now, true)
	beforeResource, beforeEvents := resourceCapabilitySnapshot(t, admin, resourceID)
	removeFailure := installBookingRaceResourceStateFailure(t, admin, fixtures.coordinator, resourceID)
	group := newBookingRaceGroup(t)
	stateSetGate := group.newGate()
	stateStarted := make(chan int32, 1)
	stateClock := newAtomicBookingClockAt(now)
	stateService := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(app), started: stateStarted, beforeSet: stateSetGate,
	}, stateClock.Now)
	stateCtx, cancelState := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelState()
	deactivation := startResourceStateRaceCall(group, stateCtx, stateService,
		resources.Actor{AccountID: fixtures.coordinator}, resources.StateRequest{ResourceID: resourceID, Active: false})
	statePID := awaitBookingRacePID(t, stateCtx, stateStarted, "failing deactivation transaction")
	if gatePID := stateSetGate.await(t, stateCtx, "state lock held before guarded mutation"); gatePID != statePID {
		t.Fatalf("pre-mutation gate backend PID=%d, transaction PID=%d", gatePID, statePID)
	}

	request := bookingCreationRequest(resourceID, testDatabaseUUID(t, admin), now, time.Hour, time.Hour)
	creatorStarted := make(chan int32, 1)
	creatorClock := newAtomicBookingClockAt(now)
	creatorService := bookings.NewService(&bookingRaceRepository{
		base: NewStore(app), started: creatorStarted,
	}, creatorClock.Now)
	creatorCtx, cancelCreator := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelCreator()
	creator := startBookingRaceCall(group, creatorCtx, creatorService,
		bookings.Actor{AccountID: fixtures.engineer}, request)
	creatorPID := awaitBookingRacePID(t, creatorCtx, creatorStarted, "waiting Booking transaction")
	waitCtx, cancelWait := context.WithTimeout(creatorCtx, 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, admin, creatorPID, statePID)
	cancelWait()
	if err != nil {
		t.Fatalf("observe Booking creation waiting on failed deactivation: %v", err)
	}
	logBookingRaceWait(t, "Booking creation waits for deactivation whose Activity will fail", wait)

	stateSetGate.open()
	stateOutcome := awaitResourceStateRaceCall(t, stateCtx, deactivation)
	creatorOutcome := awaitBookingRaceCall(t, creatorCtx, creator)
	removeFailure()
	if !errors.Is(stateOutcome.err, resources.ErrOperational) || strings.Contains(stateOutcome.err.Error(), "injected resource state Activity failure") {
		t.Fatalf("failed deactivation error=%v, want safe operational failure", stateOutcome.err)
	}
	if creatorOutcome.err != nil || creatorOutcome.result.Replayed || creatorOutcome.result.Booking.ID == "" {
		t.Fatalf("Booking after deactivation rollback: result=%+v error=%v", creatorOutcome.result, creatorOutcome.err)
	}
	if creatorClock.calls.Load() != 1 || stateClock.calls.Load() != 1 {
		t.Fatalf("post-lock clock calls: state=%d creator=%d, want one each", stateClock.calls.Load(), creatorClock.calls.Load())
	}
	if afterResource, afterEvents := resourceCapabilitySnapshot(t, admin, resourceID); afterResource != beforeResource || afterEvents != beforeEvents {
		t.Fatalf("failed deactivation persisted partial state/history: before=%s %s after=%s %s",
			beforeResource, beforeEvents, afterResource, afterEvents)
	}
	assertBookingRaceResource(t, admin, resourceID, true, createdAt, createdAt)
	assertBookingCreationRacePersisted(t, admin, fixtures.engineer, request, creatorOutcome.result.Booking.ID, true, now)
	assertBookingRequestCounts(t, admin, fixtures.engineer, request.RequestID, 1, 1)
	*bookingIDs = append(*bookingIDs, creatorOutcome.result.Booking.ID)
}

type bookingRaceOutcome struct {
	result bookings.CreateResult
	err    error
}

type resourceStateRaceOutcome struct {
	result resources.StateResult
	err    error
}

type bookingRaceCall struct {
	result chan bookingRaceOutcome
	done   chan struct{}
}

type resourceStateRaceCall struct {
	result chan resourceStateRaceOutcome
	done   chan struct{}
}

type bookingRaceGroup struct {
	t     *testing.T
	gates []*bookingRaceGate
	calls []<-chan struct{}
}

func newBookingRaceGroup(t *testing.T) *bookingRaceGroup {
	group := &bookingRaceGroup{t: t}
	t.Cleanup(group.cleanup)
	return group
}

func (group *bookingRaceGroup) newGate() *bookingRaceGate {
	gate := &bookingRaceGate{entered: make(chan int32, 1), release: make(chan struct{})}
	group.gates = append(group.gates, gate)
	return gate
}

func (group *bookingRaceGroup) track(done <-chan struct{}) {
	group.calls = append(group.calls, done)
}

func (group *bookingRaceGroup) cleanup() {
	for _, gate := range group.gates {
		gate.open()
	}
	for _, done := range group.calls {
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-done:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			group.t.Errorf("concurrency test operation did not join during cleanup")
		}
	}
}

type bookingRaceGate struct {
	entered chan int32
	release chan struct{}
	once    sync.Once
}

func (gate *bookingRaceGate) pause(ctx context.Context, pid int32) error {
	select {
	case gate.entered <- pid:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-gate.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (gate *bookingRaceGate) await(t *testing.T, ctx context.Context, operation string) int32 {
	t.Helper()
	select {
	case pid := <-gate.entered:
		return pid
	case <-ctx.Done():
		t.Fatalf("wait for %s gate: %v", operation, ctx.Err())
		return 0
	}
}

func (gate *bookingRaceGate) open() {
	gate.once.Do(func() { close(gate.release) })
}

type bookingRaceRepository struct {
	base          bookings.Repository
	bookingIDs    *[]string
	started       chan<- int32
	accountLocked chan<- int32
	beforeAppend  *bookingRaceGate
	beforeCommit  *bookingRaceGate
}

func (repository *bookingRaceRepository) BeginBookingCreation(ctx context.Context) (bookings.Transaction, error) {
	transaction, err := repository.base.BeginBookingCreation(ctx)
	if err != nil {
		return nil, err
	}
	concrete, ok := transaction.(*bookingCreationTransaction)
	if !ok {
		_ = transaction.Rollback(context.Background())
		return nil, fmt.Errorf("unexpected booking transaction type %T", transaction)
	}
	pid, err := requireRuntimeTransaction(ctx, concrete.tx)
	if err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	if err := sendBookingRacePID(ctx, repository.started, pid); err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	return &bookingRaceTransaction{
		Transaction: transaction, raw: concrete, pid: pid,
		accountLocked: repository.accountLocked, beforeAppend: repository.beforeAppend,
		beforeCommit: repository.beforeCommit, bookingIDs: repository.bookingIDs,
	}, nil
}

type bookingRaceTransaction struct {
	bookings.Transaction
	raw           *bookingCreationTransaction
	pid           int32
	bookingID     string
	accountLocked chan<- int32
	beforeAppend  *bookingRaceGate
	beforeCommit  *bookingRaceGate
	bookingIDs    *[]string
}

func (tx *bookingRaceTransaction) LockAccount(ctx context.Context, accountID string) (bool, error) {
	locked, err := tx.Transaction.LockAccount(ctx, accountID)
	if err != nil || !locked {
		return locked, err
	}
	if err := sendBookingRacePID(ctx, tx.accountLocked, tx.pid); err != nil {
		return false, err
	}
	return true, nil
}

func (tx *bookingRaceTransaction) AppendBookingCreatedEvent(ctx context.Context, event bookings.BookingCreatedEvent) error {
	tx.bookingID = event.Booking.ID
	if tx.bookingIDs != nil {
		*tx.bookingIDs = append(*tx.bookingIDs, event.Booking.ID)
	}
	if tx.beforeAppend != nil {
		var inserted bool
		if err := tx.raw.tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM public.bookings WHERE id = $1::uuid AND request_id = $2::uuid
			               AND resource_id = $3::uuid AND owner_account_id = $4::uuid)`,
			event.Booking.ID, event.Booking.RequestID, event.Booking.ResourceID, event.Booking.OwnerAccountID).Scan(&inserted); err != nil {
			return fmt.Errorf("verify inserted Booking before Activity barrier: %w", err)
		}
		if !inserted {
			return fmt.Errorf("Booking insert is not visible before Activity barrier")
		}
		if err := tx.beforeAppend.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.Transaction.AppendBookingCreatedEvent(ctx, event)
}

func (tx *bookingRaceTransaction) Commit(ctx context.Context) error {
	if tx.beforeCommit != nil {
		var eventCount int
		if err := tx.raw.tx.QueryRow(ctx, `
			SELECT count(*) FROM public.activity_events
			WHERE booking_id = $1::uuid AND action = 'booking.created'`,
			tx.bookingID).Scan(&eventCount); err != nil {
			return fmt.Errorf("verify Booking Activity before commit barrier: %w", err)
		}
		if eventCount != 1 {
			return fmt.Errorf("Booking Activity count before commit = %d, want one", eventCount)
		}
		if err := tx.beforeCommit.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.Transaction.Commit(ctx)
}

type resourceStateRaceRepository struct {
	base           resources.StateRepository
	started        chan<- int32
	resourceLocked chan<- int32
	beforeSet      *bookingRaceGate
	beforeCommit   *bookingRaceGate
}

func (repository *resourceStateRaceRepository) BeginResourceStateChange(ctx context.Context) (resources.StateTransaction, error) {
	transaction, err := repository.base.BeginResourceStateChange(ctx)
	if err != nil {
		return nil, err
	}
	concrete, ok := transaction.(*resourceStateTransaction)
	if !ok {
		_ = transaction.Rollback(context.Background())
		return nil, fmt.Errorf("unexpected Resource state transaction type %T", transaction)
	}
	pid, err := requireRuntimeTransaction(ctx, concrete.resourceCreationTransaction.tx)
	if err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	if err := sendBookingRacePID(ctx, repository.started, pid); err != nil {
		_ = transaction.Rollback(context.Background())
		return nil, err
	}
	return &resourceStateRaceTransaction{
		StateTransaction: transaction, raw: concrete, pid: pid,
		resourceLocked: repository.resourceLocked, beforeSet: repository.beforeSet,
		beforeCommit: repository.beforeCommit,
	}, nil
}

type resourceStateRaceTransaction struct {
	resources.StateTransaction
	raw            *resourceStateTransaction
	pid            int32
	resourceLocked chan<- int32
	beforeSet      *bookingRaceGate
	beforeCommit   *bookingRaceGate
	expected       *resourceStateRaceExpectation
}

type resourceStateRaceExpectation struct {
	actorID    string
	resourceID string
	active     bool
	occurredAt time.Time
}

func (tx *resourceStateRaceTransaction) LockResource(ctx context.Context, resourceID string) (resources.Resource, bool, error) {
	resource, found, err := tx.StateTransaction.LockResource(ctx, resourceID)
	if err == nil && found {
		if err := sendBookingRacePID(ctx, tx.resourceLocked, tx.pid); err != nil {
			return resources.Resource{}, false, err
		}
	}
	return resource, found, err
}

func (tx *resourceStateRaceTransaction) SetResourceState(
	ctx context.Context,
	actorID, resourceID string,
	active bool,
	occurredAt time.Time,
) (resources.Resource, bool, error) {
	if tx.beforeSet != nil {
		if err := tx.beforeSet.pause(ctx, tx.pid); err != nil {
			return resources.Resource{}, false, err
		}
	}
	resource, changed, err := tx.StateTransaction.SetResourceState(ctx, actorID, resourceID, active, occurredAt)
	if err == nil && changed {
		tx.expected = &resourceStateRaceExpectation{
			actorID: actorID, resourceID: resourceID, active: active, occurredAt: occurredAt,
		}
	}
	return resource, changed, err
}

func (tx *resourceStateRaceTransaction) Commit(ctx context.Context) error {
	if tx.beforeCommit != nil {
		if err := tx.assertPendingStateTransition(ctx); err != nil {
			return err
		}
		if err := tx.beforeCommit.pause(ctx, tx.pid); err != nil {
			return err
		}
	}
	return tx.StateTransaction.Commit(ctx)
}

func (tx *resourceStateRaceTransaction) assertPendingStateTransition(ctx context.Context) error {
	if tx.expected == nil {
		return fmt.Errorf("state transaction reached commit barrier without a changed transition")
	}
	action := "resource.deactivated"
	if tx.expected.active {
		action = "resource.activated"
	}
	var active, beforeActive, afterActive bool
	var updatedAt time.Time
	var eventCount int
	err := tx.raw.resourceCreationTransaction.tx.QueryRow(ctx, `
		SELECT resource.active, resource.updated_at,
		       (SELECT count(*) FROM public.activity_events AS event
		        WHERE event.resource_id = resource.id AND event.actor_account_id = $2::uuid
		          AND event.action = $3 AND event.occurred_at = $4::timestamptz AND event.booking_id IS NULL),
	       (SELECT (event.details->'before'->>'active')::boolean FROM public.activity_events AS event
	        WHERE event.resource_id = resource.id AND event.actor_account_id = $2::uuid
	          AND event.action = $3 AND event.occurred_at = $4::timestamptz AND event.booking_id IS NULL LIMIT 1),
	       (SELECT (event.details->'after'->>'active')::boolean FROM public.activity_events AS event
	        WHERE event.resource_id = resource.id AND event.actor_account_id = $2::uuid
	          AND event.action = $3 AND event.occurred_at = $4::timestamptz AND event.booking_id IS NULL LIMIT 1)
		FROM public.resources AS resource WHERE resource.id = $1::uuid`,
		tx.expected.resourceID, tx.expected.actorID, action, tx.expected.occurredAt).Scan(
		&active, &updatedAt, &eventCount, &beforeActive, &afterActive)
	if err != nil {
		return fmt.Errorf("inspect uncommitted state transition before commit barrier: %w", err)
	}
	if active != tx.expected.active || !updatedAt.Equal(tx.expected.occurredAt) || eventCount != 1 ||
		beforeActive == tx.expected.active || afterActive != tx.expected.active {
		return fmt.Errorf("uncommitted state/event pair is incomplete before commit: active=%v updated_at=%s event_count=%d before=%v after=%v",
			active, updatedAt, eventCount, beforeActive, afterActive)
	}
	return nil
}

func startBookingRaceCall(
	group *bookingRaceGroup,
	ctx context.Context,
	service *bookings.Service,
	actor bookings.Actor,
	request bookings.CreateRequest,
) *bookingRaceCall {
	call := &bookingRaceCall{result: make(chan bookingRaceOutcome, 1), done: make(chan struct{})}
	group.track(call.done)
	go func() {
		defer close(call.done)
		result, err := service.Create(ctx, actor, request)
		call.result <- bookingRaceOutcome{result: result, err: err}
	}()
	return call
}

func startResourceStateRaceCall(
	group *bookingRaceGroup,
	ctx context.Context,
	service *resources.StateService,
	actor resources.Actor,
	request resources.StateRequest,
) *resourceStateRaceCall {
	call := &resourceStateRaceCall{result: make(chan resourceStateRaceOutcome, 1), done: make(chan struct{})}
	group.track(call.done)
	go func() {
		defer close(call.done)
		result, err := service.SetState(ctx, actor, request)
		call.result <- resourceStateRaceOutcome{result: result, err: err}
	}()
	return call
}

func awaitBookingRaceCall(t *testing.T, ctx context.Context, call *bookingRaceCall) bookingRaceOutcome {
	t.Helper()
	select {
	case <-call.done:
		return <-call.result
	case <-ctx.Done():
		t.Fatalf("booking operation did not finish: %v", ctx.Err())
		return bookingRaceOutcome{}
	}
}

func awaitResourceStateRaceCall(t *testing.T, ctx context.Context, call *resourceStateRaceCall) resourceStateRaceOutcome {
	t.Helper()
	select {
	case <-call.done:
		return <-call.result
	case <-ctx.Done():
		t.Fatalf("Resource state operation did not finish: %v", ctx.Err())
		return resourceStateRaceOutcome{}
	}
}

func awaitBookingRacePID(t *testing.T, ctx context.Context, source <-chan int32, operation string) int32 {
	t.Helper()
	select {
	case pid := <-source:
		return pid
	case <-ctx.Done():
		t.Fatalf("%s did not start: %v", operation, ctx.Err())
		return 0
	}
}

func sendBookingRacePID(ctx context.Context, destination chan<- int32, pid int32) error {
	if destination == nil {
		return nil
	}
	select {
	case destination <- pid:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func requireRuntimeTransaction(ctx context.Context, transaction pgx.Tx) (int32, error) {
	var currentUser string
	var pid int32
	if err := transaction.QueryRow(ctx, "SELECT current_user, pg_backend_pid()").Scan(&currentUser, &pid); err != nil {
		return 0, fmt.Errorf("identify runtime transaction: %w", err)
	}
	if currentUser != "labreserve_app" {
		return 0, fmt.Errorf("concurrency evidence must use labreserve_app, got %q", currentUser)
	}
	return pid, nil
}

type bookingRaceLockWait struct {
	waiterPID     int32
	blockerPID    int32
	lockType      string
	lockMode      string
	relation      string
	waitEventType string
	waitEvent     string
}

func waitForBookingRaceLockWait(ctx context.Context, admin *pgxpool.Pool, waiterPID, blockerPID int32) (bookingRaceLockWait, error) {
	const query = `
		SELECT activity.pid, blocker.pid, waiting.locktype, waiting.mode,
		       COALESCE(waiting.relation::regclass::text, ''),
		       COALESCE(activity.wait_event_type, ''), COALESCE(activity.wait_event, '')
		FROM pg_stat_activity AS activity
		CROSS JOIN LATERAL unnest(pg_blocking_pids(activity.pid)) AS blocker(pid)
		JOIN LATERAL (
			SELECT locktype, mode, relation FROM pg_locks
			WHERE pid = activity.pid AND NOT granted
			ORDER BY CASE WHEN locktype = 'transactionid' THEN 0 ELSE 1 END
			LIMIT 1
		) AS waiting ON TRUE
		WHERE activity.pid = $1 AND blocker.pid = $2
		LIMIT 1`
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var wait bookingRaceLockWait
		err := admin.QueryRow(ctx, query, waiterPID, blockerPID).Scan(
			&wait.waiterPID, &wait.blockerPID, &wait.lockType, &wait.lockMode,
			&wait.relation, &wait.waitEventType, &wait.waitEvent)
		if err == nil {
			return wait, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return bookingRaceLockWait{}, fmt.Errorf("query PostgreSQL blocking edge waiter=%d blocker=%d: %w", waiterPID, blockerPID, err)
		}
		select {
		case <-ctx.Done():
			return bookingRaceLockWait{}, fmt.Errorf("no PostgreSQL blocking edge waiter=%d blocker=%d before deadline: %w", waiterPID, blockerPID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func logBookingRaceWait(t *testing.T, description string, wait bookingRaceLockWait) {
	t.Helper()
	if wait.waiterPID == 0 || wait.blockerPID == 0 || wait.lockType == "" || wait.lockMode == "" {
		t.Fatalf("incomplete PostgreSQL lock-wait observation for %s: %+v", description, wait)
	}
	t.Logf("observed PostgreSQL wait (%s): waiter pid=%d blocker pid=%d lock=%s/%s relation=%q event=%s/%s",
		description, wait.waiterPID, wait.blockerPID, wait.lockType, wait.lockMode,
		wait.relation, wait.waitEventType, wait.waitEvent)
}

func assertBookingRaceLockModes(t *testing.T, admin *pgxpool.Pool) {
	t.Helper()
	var accountDefinition, resourceDefinition string
	if err := admin.QueryRow(t.Context(), `
		SELECT pg_get_functiondef('public.labreserve_lock_booking_owner(uuid)'::regprocedure),
		       pg_get_functiondef('public.labreserve_lock_booking_resource(uuid)'::regprocedure)`).Scan(
		&accountDefinition, &resourceDefinition); err != nil {
		t.Fatalf("inspect installed PostgreSQL booking lock capabilities: %v", err)
	}
	if !strings.Contains(strings.ToUpper(accountDefinition), "FOR NO KEY UPDATE") ||
		!strings.Contains(strings.ToUpper(resourceDefinition), "FOR UPDATE") {
		t.Fatalf("installed lock modes changed: owner definition=%q Resource definition=%q", accountDefinition, resourceDefinition)
	}
	t.Log("verified installed PostgreSQL order primitives: Account FOR NO KEY UPDATE; Resource FOR UPDATE")
}

func newAtomicBookingClockAt(value time.Time) *atomicBookingClock {
	clock := &atomicBookingClock{}
	clock.Set(value)
	return clock
}

func insertBookingRaceResource(t *testing.T, admin *pgxpool.Pool, fixtures *resourceCapabilityFixtures, now time.Time, active bool) string {
	t.Helper()
	createdAt := now.Add(-24 * time.Hour)
	resourceID := insertResourceCapabilityFixture(t, admin,
		"Booking-Race-"+strings.ReplaceAll(testDatabaseUUID(t, admin), "-", ""),
		"Booking race lab", "retained description ", active, createdAt)
	fixtures.trackResource(resourceID)
	return resourceID
}

func assertBookingRaceResource(t *testing.T, admin *pgxpool.Pool, resourceID string, active bool, createdAt, updatedAt time.Time) {
	t.Helper()
	var code, name, description string
	var persistedActive bool
	var persistedCreatedAt, persistedUpdatedAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT code, name, description, active, created_at, updated_at
		FROM public.resources WHERE id = $1::uuid`, resourceID).Scan(
		&code, &name, &description, &persistedActive, &persistedCreatedAt, &persistedUpdatedAt); err != nil {
		t.Fatalf("read persisted Booking-race Resource: %v", err)
	}
	if code == "" || name != "Booking race lab" || description != "retained description " ||
		persistedActive != active || !persistedCreatedAt.Equal(createdAt) || !persistedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("Booking-race Resource=%q/%q/%q active=%v created=%s updated=%s; want active=%v created=%s updated=%s",
			code, name, description, persistedActive, persistedCreatedAt, persistedUpdatedAt, active, createdAt, updatedAt)
	}
}

func assertBookingCreationRacePersisted(
	t *testing.T,
	admin *pgxpool.Pool,
	ownerID string,
	request bookings.CreateRequest,
	resultID string,
	want bool,
	createdAt time.Time,
) {
	t.Helper()
	if !want {
		assertBookingRequestCounts(t, admin, ownerID, request.RequestID, 0, 0)
		return
	}
	var id, resourceID, persistedOwner, requestID, state, purpose string
	var startAt, endAt, persistedCreatedAt time.Time
	var eventActor, action, eventBookingID, detailResourceID, detailRequestID, detailPurpose string
	var eventAt time.Time
	if err := admin.QueryRow(t.Context(), `
		SELECT booking.id::text, booking.resource_id::text, booking.owner_account_id::text,
		       booking.request_id::text, booking.state, booking.purpose, booking.start_at,
		       booking.end_at, booking.created_at, event.actor_account_id::text, event.action,
		       event.booking_id::text, event.occurred_at, event.details->>'resource_id',
		       event.details->>'request_id', event.details->>'purpose'
		FROM public.bookings AS booking
		JOIN public.activity_events AS event ON event.booking_id = booking.id AND event.action = 'booking.created'
		WHERE booking.owner_account_id = $1::uuid AND booking.request_id = $2::uuid`,
		ownerID, request.RequestID).Scan(
		&id, &resourceID, &persistedOwner, &requestID, &state, &purpose, &startAt, &endAt,
		&persistedCreatedAt, &eventActor, &action, &eventBookingID, &eventAt,
		&detailResourceID, &detailRequestID, &detailPurpose); err != nil {
		t.Fatalf("read persisted Booking and creation Activity: %v", err)
	}
	if id != resultID || resourceID != request.ResourceID || persistedOwner != ownerID || requestID != request.RequestID ||
		state != string(bookings.StateConfirmed) || purpose != request.Purpose || !startAt.Equal(request.StartAt) ||
		!endAt.Equal(request.EndAt) || !persistedCreatedAt.Equal(createdAt) || eventActor != ownerID ||
		action != "booking.created" || eventBookingID != id || !eventAt.Equal(createdAt) ||
		detailResourceID != request.ResourceID || detailRequestID != request.RequestID || detailPurpose != request.Purpose {
		t.Fatalf("persisted Booking/event mismatch: booking=%s/%s/%s/%s %s %q %s..%s created=%s event=%s/%s/%s at=%s details=%s/%s/%q",
			id, resourceID, persistedOwner, requestID, state, purpose, startAt, endAt, persistedCreatedAt,
			eventActor, action, eventBookingID, eventAt, detailResourceID, detailRequestID, detailPurpose)
	}
	assertBookingRequestCounts(t, admin, ownerID, request.RequestID, 1, 1)
}

func bookingRaceRequestSnapshot(t *testing.T, admin *pgxpool.Pool, ownerID, requestID string) string {
	t.Helper()
	var snapshot string
	if err := admin.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'bookings', COALESCE((
				SELECT jsonb_agg(to_jsonb(booking) ORDER BY booking.id)
				FROM public.bookings AS booking
				WHERE booking.owner_account_id = $1::uuid AND booking.request_id = $2::uuid
			), '[]'::jsonb),
			'events', COALESCE((
				SELECT jsonb_agg(to_jsonb(event) ORDER BY event.id)
				FROM public.activity_events AS event
				JOIN public.bookings AS booking ON booking.id = event.booking_id
				WHERE booking.owner_account_id = $1::uuid AND booking.request_id = $2::uuid
			), '[]'::jsonb)
		)::text`, ownerID, requestID).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot Booking request identity and Activity: %v", err)
	}
	return snapshot
}

func installBookingRaceActivityFailure(
	t *testing.T,
	admin *pgxpool.Pool,
	actorID, resourceID, requestID string,
) func() {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionID := pgx.Identifier{"public", "labreserve_c6_booking_event_fail_" + suffix}.Sanitize()
	triggerID := pgx.Identifier{"labreserve_c6_booking_event_fail_" + suffix}.Sanitize()
	sqlLiteral := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.actor_account_id = %s::uuid AND NEW.action = 'booking.created'
			   AND NEW.details->>'resource_id' = %s AND NEW.details->>'request_id' = %s THEN
				RAISE EXCEPTION 'injected booking Activity failure';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionID, sqlLiteral(actorID), sqlLiteral(resourceID), sqlLiteral(requestID))); err != nil {
		t.Fatalf("create scoped Booking Activity failure trigger function: %v", err)
	}
	cleanup := installBookingRaceTriggerCleanup(t, admin, triggerID, functionID)
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON public.activity_events
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerID, functionID)); err != nil {
		t.Fatalf("install scoped Booking Activity failure trigger: %v", err)
	}
	return cleanup
}

func installBookingRaceResourceStateFailure(t *testing.T, admin *pgxpool.Pool, actorID, resourceID string) func() {
	t.Helper()
	suffix := strings.ReplaceAll(testDatabaseUUID(t, admin), "-", "")
	functionID := pgx.Identifier{"public", "labreserve_c6_state_event_fail_" + suffix}.Sanitize()
	triggerID := pgx.Identifier{"labreserve_c6_state_event_fail_" + suffix}.Sanitize()
	sqlLiteral := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.actor_account_id = %s::uuid AND NEW.resource_id = %s::uuid
			   AND NEW.action = 'resource.deactivated' THEN
				RAISE EXCEPTION 'injected resource state Activity failure';
			END IF;
			RETURN NEW;
		END;
		$body$`, functionID, sqlLiteral(actorID), sqlLiteral(resourceID))); err != nil {
		t.Fatalf("create scoped Resource state Activity failure trigger function: %v", err)
	}
	cleanup := installBookingRaceTriggerCleanup(t, admin, triggerID, functionID)
	if _, err := admin.Exec(t.Context(), fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON public.activity_events
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerID, functionID)); err != nil {
		t.Fatalf("install scoped Resource state Activity failure trigger: %v", err)
	}
	return cleanup
}

func installBookingRaceTriggerCleanup(t *testing.T, admin *pgxpool.Pool, triggerID, functionID string) func() {
	t.Helper()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := admin.Exec(ctx, "DROP TRIGGER IF EXISTS "+triggerID+" ON public.activity_events"); err != nil {
				t.Errorf("drop concurrency-test Activity trigger: %v", err)
			}
			if _, err := admin.Exec(ctx, "DROP FUNCTION IF EXISTS "+functionID+"()"); err != nil {
				t.Errorf("drop concurrency-test Activity trigger function: %v", err)
			}
		})
	}
	t.Cleanup(cleanup)
	return cleanup
}
