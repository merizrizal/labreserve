package database

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"labreserve.local/labreserve/internal/resources"
)

var errResourceStateAcknowledgementLost = errors.New("simulated Resource COMMIT acknowledgement loss")

type resourceStateCommitMode uint8

const (
	resourceStateCommitNormally resourceStateCommitMode = iota
	resourceStateCommitDurablyThenLoseAcknowledgement
	resourceStateRollbackThenLoseAcknowledgement
)

type resourceStateCommitObservation struct {
	beginCalls           atomic.Int32
	mutationCalls        int
	resource             resources.Resource
	changed              bool
	commitCalls          int
	databaseCommitCalls  int
	databaseCommitErr    error
	returnedCommitErr    error
	rolledBackBeforeAck  bool
	rollbackBeforeAckErr error
}

type resourceStateCommitRepository struct {
	base        resources.StateRepository
	observation *resourceStateCommitObservation
	mode        resourceStateCommitMode
}

func (repository *resourceStateCommitRepository) BeginResourceStateChange(
	ctx context.Context,
) (resources.StateTransaction, error) {
	repository.observation.beginCalls.Add(1)
	transaction, err := repository.base.BeginResourceStateChange(ctx)
	if err != nil {
		return nil, err
	}
	return &resourceStateCommitTransaction{
		StateTransaction: transaction,
		observation:      repository.observation,
		mode:             repository.mode,
	}, nil
}

type resourceStateCommitTransaction struct {
	resources.StateTransaction
	observation *resourceStateCommitObservation
	mode        resourceStateCommitMode
}

func (transaction *resourceStateCommitTransaction) SetResourceState(
	ctx context.Context,
	actorID, resourceID string,
	active bool,
	occurredAt time.Time,
) (resources.Resource, bool, error) {
	resource, changed, err := transaction.StateTransaction.SetResourceState(ctx, actorID, resourceID, active, occurredAt)
	if err == nil {
		transaction.observation.mutationCalls++
		transaction.observation.resource = resource
		transaction.observation.changed = changed
	}
	return resource, changed, err
}

func (transaction *resourceStateCommitTransaction) Commit(ctx context.Context) error {
	observation := transaction.observation
	observation.commitCalls++
	switch transaction.mode {
	case resourceStateCommitNormally:
		observation.databaseCommitCalls++
		observation.databaseCommitErr = transaction.StateTransaction.Commit(ctx)
		observation.returnedCommitErr = observation.databaseCommitErr
	case resourceStateCommitDurablyThenLoseAcknowledgement:
		observation.databaseCommitCalls++
		observation.databaseCommitErr = transaction.StateTransaction.Commit(ctx)
		if observation.databaseCommitErr != nil {
			observation.returnedCommitErr = observation.databaseCommitErr
		} else {
			observation.returnedCommitErr = errResourceStateAcknowledgementLost
		}
	case resourceStateRollbackThenLoseAcknowledgement:
		observation.rollbackBeforeAckErr = transaction.StateTransaction.Rollback(ctx)
		observation.rolledBackBeforeAck = observation.rollbackBeforeAckErr == nil
		observation.returnedCommitErr = errResourceStateAcknowledgementLost
	default:
		observation.returnedCommitErr = errors.New("unknown Resource state commit test mode")
	}
	return observation.returnedCommitErr
}

func TestResourceStateCommitRejectionRollsBackAndWaiterReevaluates(t *testing.T) {
	for _, test := range []struct {
		name          string
		initialActive bool
		desiredActive bool
		action        string
	}{
		{name: "activation", initialActive: false, desiredActive: true, action: "resource.activated"},
		{name: "deactivation", initialActive: true, desiredActive: false, action: "resource.deactivated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, test.initialActive)
			leaderAt := fixture.operationAt
			waiterAt := leaderAt.Add(time.Minute)
			removeRejection := installResourceCommitRejectionTrigger(
				t, fixture.admin, fixture.actors.coordinator, test.action, "", fixture.resourceID,
			)
			group := newBookingRaceGroup(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			leaderGate := group.newGate()
			leaderStarted := make(chan int32, 1)
			leaderCalls, leaderBlockingChecks := &atomic.Int32{}, &atomic.Int32{}
			leaderBlockingTime := &atomic.Int64{}
			leaderClock := newAtomicBookingClockAt(leaderAt)
			leaderRaceRepository := &resourceStateRaceRepository{
				base: NewStore(fixture.app), started: leaderStarted,
				beforeCommit: leaderGate, stateCalls: leaderCalls,
				blockingChecks: leaderBlockingChecks, blockingTimeMicros: leaderBlockingTime,
			}
			leaderObservation := &resourceStateCommitObservation{}
			leaderService := resources.NewStateService(&resourceStateCommitRepository{
				base: leaderRaceRepository, observation: leaderObservation, mode: resourceStateCommitNormally,
			}, leaderClock.Now)
			actor := resources.Actor{AccountID: fixture.actors.coordinator}
			leader := startResourceStateRaceCall(group, ctx, leaderService, actor,
				resources.StateRequest{ResourceID: fixture.resourceID, Active: test.desiredActive})
			leaderPID := awaitBookingRacePID(t, ctx, leaderStarted, "state transaction that will receive deferred COMMIT rejection")
			if gatePID := leaderGate.await(t, ctx, "successful guarded state statement before actual COMMIT"); gatePID != leaderPID {
				t.Fatalf("leader gate PID=%d, transaction PID=%d", gatePID, leaderPID)
			}
			if leaderObservation.mutationCalls != 1 || !leaderObservation.changed ||
				leaderObservation.resource.Active != test.desiredActive ||
				!leaderObservation.resource.UpdatedAt.Equal(leaderAt) {
				t.Fatalf("guarded state statement did not return changed success before COMMIT: %+v", leaderObservation)
			}

			waiterGate := group.newGate()
			waiterStarted := make(chan int32, 1)
			waiterCalls, waiterBlockingChecks := &atomic.Int32{}, &atomic.Int32{}
			waiterBlockingTime := &atomic.Int64{}
			waiterClock := newAtomicBookingClockAt(waiterAt)
			waiterRaceRepository := &resourceStateRaceRepository{
				base: NewStore(fixture.app), started: waiterStarted,
				beforeSet: waiterGate, stateCalls: waiterCalls,
				blockingChecks: waiterBlockingChecks, blockingTimeMicros: waiterBlockingTime,
			}
			waiterService := resources.NewStateService(waiterRaceRepository, waiterClock.Now)
			waiter := startResourceStateRaceCall(group, ctx, waiterService, actor,
				resources.StateRequest{ResourceID: fixture.resourceID, Active: test.desiredActive})
			waiterPID := awaitBookingRacePID(t, ctx, waiterStarted, "state waiter behind deferred-COMMIT transaction")
			waitCtx, cancelWait := context.WithTimeout(ctx, 8*time.Second)
			wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, leaderPID)
			cancelWait()
			if err != nil {
				t.Fatalf("observe waiter blocked by deferred-COMMIT leader: %v", err)
			}
			logBookingRaceWait(t, "state waiter behind actual rejected COMMIT", wait)

			leaderGate.open()
			leaderOutcome := awaitResourceStateRaceCall(t, ctx, leader)
			if !errors.Is(leaderOutcome.err, resources.ErrOperational) ||
				leaderOutcome.result != (resources.StateResult{}) ||
				strings.Contains(leaderOutcome.err.Error(), resourceCommitRejectionMessage) {
				t.Fatalf("rejected COMMIT result=%+v error=%v; want safe operational failure without trigger diagnostic",
					leaderOutcome.result, leaderOutcome.err)
			}
			assertResourceCommitRejected(t, &resourceCommitRejectionObservation{
				mutationCalls: leaderObservation.mutationCalls,
				resource:      leaderObservation.resource,
				changed:       leaderObservation.changed,
				commitCalls:   leaderObservation.databaseCommitCalls,
				commitErr:     leaderObservation.databaseCommitErr,
			}, true, leaderOutcome.err)
			if leaderObservation.commitCalls != 1 || leaderObservation.databaseCommitCalls != 1 ||
				leaderObservation.returnedCommitErr == nil || leaderCalls.Load() != 1 || leaderClock.calls.Load() != 1 {
				t.Fatalf("leader retry/COMMIT observations: commit calls=%d database COMMITs=%d state calls=%d clock calls=%d observation=%+v",
					leaderObservation.commitCalls, leaderObservation.databaseCommitCalls, leaderCalls.Load(), leaderClock.calls.Load(), leaderObservation)
			}
			if !test.desiredActive && (leaderBlockingChecks.Load() != 1 || leaderBlockingTime.Load() != leaderAt.UnixMicro()) {
				t.Fatalf("deactivation COMMIT-rejection eligibility checks=%d at=%d; want one at %s",
					leaderBlockingChecks.Load(), leaderBlockingTime.Load(), leaderAt)
			}

			removeRejection()
			if gatePID := waiterGate.await(t, ctx, "waiter re-evaluation after rejected COMMIT"); gatePID != waiterPID {
				t.Fatalf("waiter mutation gate PID=%d, transaction PID=%d", gatePID, waiterPID)
			}
			fixture.assertPersistedResource(t, test.initialActive, fixture.createdAt,
				fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t)
			fixture.assertBookingsUnchanged(t)
			if waiterCalls.Load() != 1 || waiterClock.calls.Load() != 1 {
				t.Fatalf("waiter did not freshly evaluate committed state/time: state calls=%d clock calls=%d",
					waiterCalls.Load(), waiterClock.calls.Load())
			}
			if !test.desiredActive && (waiterBlockingChecks.Load() != 1 || waiterBlockingTime.Load() != waiterAt.UnixMicro()) {
				t.Fatalf("waiting deactivation eligibility checks=%d at=%d; want one post-lock check at %s",
					waiterBlockingChecks.Load(), waiterBlockingTime.Load(), waiterAt)
			}

			waiterGate.open()
			waiterOutcome := awaitResourceStateRaceCall(t, ctx, waiter)
			if waiterOutcome.err != nil || !waiterOutcome.result.Changed ||
				waiterOutcome.result.Resource.Active != test.desiredActive ||
				!waiterOutcome.result.Resource.UpdatedAt.Equal(waiterAt) {
				t.Fatalf("waiter after rejected COMMIT result=%+v error=%v", waiterOutcome.result, waiterOutcome.err)
			}
			fixture.assertPersistedResource(t, test.desiredActive, waiterAt,
				fixture.initialName, fixture.initialDescription)
			fixture.assertHistory(t, resourceStateConcurrencyEvent{
				actorID: fixture.actors.coordinator, action: test.action, occurredAt: waiterAt,
				before: test.initialActive, after: test.desiredActive,
			})
			fixture.assertBookingsUnchanged(t)
		})
	}
}

func TestResourceStateContextCancellationReleasesResourceLock(t *testing.T) {
	fixture := newResourceStateConcurrencyFixture(t, true)
	held := holdBookingResourceLock(t, fixture.admin, fixture.resourceID)
	group := newBookingRaceGroup(t)
	started := make(chan int32, 1)
	stateCalls := &atomic.Int32{}
	clockCalls := &atomic.Int32{}
	clock := func() time.Time {
		clockCalls.Add(1)
		return fixture.operationAt
	}
	service := resources.NewStateService(&resourceStateRaceRepository{
		base: NewStore(fixture.app), started: started, stateCalls: stateCalls,
	}, clock)
	callCtx, cancelCall := context.WithCancel(t.Context())
	defer cancelCall()
	call := startResourceStateRaceCall(group, callCtx, service,
		resources.Actor{AccountID: fixture.actors.coordinator},
		resources.StateRequest{ResourceID: fixture.resourceID, Active: false})
	waiterPID := awaitBookingRacePID(t, t.Context(), started, "context-cancellable Resource state transaction")
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 8*time.Second)
	wait, err := waitForBookingRaceLockWait(waitCtx, fixture.admin, waiterPID, held.pid)
	cancelWait()
	if err != nil {
		t.Fatalf("observe runtime transaction blocked on Resource before cancellation: %v", err)
	}
	logBookingRaceWait(t, "context-cancelled Resource state operation", wait)

	cancelCall()
	outcome := awaitResourceStateRaceCall(t, t.Context(), call)
	if !errors.Is(outcome.err, resources.ErrOperational) || outcome.result != (resources.StateResult{}) ||
		strings.Contains(outcome.err.Error(), "context canceled") {
		t.Fatalf("cancelled state operation result=%+v error=%v; want safe operational failure", outcome.result, outcome.err)
	}
	if stateCalls.Load() != 0 || clockCalls.Load() != 0 {
		t.Fatalf("cancelled lock waiter reached eligibility/mutation: state calls=%d clock calls=%d", stateCalls.Load(), clockCalls.Load())
	}
	fixture.assertPersistedResource(t, true, fixture.createdAt, fixture.initialName, fixture.initialDescription)
	fixture.assertHistory(t)
	fixture.assertBookingsUnchanged(t)
	if err := held.release(); err != nil {
		t.Fatalf("release fixture Resource lock after cancelled waiter: %v", err)
	}

	laterAt := fixture.operationAt.Add(time.Minute)
	laterClock := newAtomicBookingClockAt(laterAt)
	laterService := resources.NewStateService(NewStore(fixture.app), laterClock.Now)
	later, err := laterService.SetState(t.Context(), resources.Actor{AccountID: fixture.actors.coordinator}, resources.StateRequest{
		ResourceID: fixture.resourceID, Active: false,
	})
	if err != nil || !later.Changed || later.Resource.Active || !later.Resource.UpdatedAt.Equal(laterAt) || laterClock.calls.Load() != 1 {
		t.Fatalf("explicit operation after cancelled waiter result=%+v error=%v clock calls=%d",
			later, err, laterClock.calls.Load())
	}
	fixture.assertPersistedResource(t, false, laterAt, fixture.initialName, fixture.initialDescription)
	fixture.assertHistory(t, resourceStateConcurrencyEvent{
		actorID: fixture.actors.coordinator, action: "resource.deactivated", occurredAt: laterAt,
		before: true, after: false,
	})
	fixture.assertBookingsUnchanged(t)
}

func TestResourceStateAmbiguousAcknowledgementAndLaterExplicitOperation(t *testing.T) {
	for _, test := range []struct {
		name                 string
		initialActive        bool
		desiredActive        bool
		mode                 resourceStateCommitMode
		committed            bool
		laterExpectedChanged bool
		laterDesiredActive   bool
		firstAction          string
		laterAction          string
	}{
		{
			name:          "durable commit followed by lost acknowledgement",
			initialActive: false, desiredActive: true,
			mode: resourceStateCommitDurablyThenLoseAcknowledgement, committed: true,
			laterExpectedChanged: false, laterDesiredActive: true,
			firstAction: "resource.activated",
		},
		{
			name:          "rollback followed by acknowledgement-like error",
			initialActive: true, desiredActive: false,
			mode: resourceStateRollbackThenLoseAcknowledgement, committed: false,
			laterExpectedChanged: true, laterDesiredActive: false,
			firstAction: "resource.deactivated", laterAction: "resource.deactivated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newResourceStateConcurrencyFixture(t, test.initialActive)
			operationAt := fixture.operationAt
			started := make(chan int32, 1)
			stateCalls := &atomic.Int32{}
			raceRepository := &resourceStateRaceRepository{
				base: NewStore(fixture.app), started: started, stateCalls: stateCalls,
			}
			observation := &resourceStateCommitObservation{}
			service := resources.NewStateService(&resourceStateCommitRepository{
				base: raceRepository, observation: observation, mode: test.mode,
			}, func() time.Time { return operationAt })
			actor := resources.Actor{AccountID: fixture.actors.coordinator}
			result, serviceErr := service.SetState(t.Context(), actor, resources.StateRequest{
				ResourceID: fixture.resourceID, Active: test.desiredActive,
			})
			pid := awaitBookingRacePID(t, t.Context(), started, "acknowledgement-simulation runtime transaction")
			t.Logf("acknowledgement simulation used labreserve_app transaction pid=%d", pid)
			if !errors.Is(serviceErr, resources.ErrOperational) || result != (resources.StateResult{}) ||
				strings.Contains(serviceErr.Error(), errResourceStateAcknowledgementLost.Error()) {
				t.Fatalf("acknowledgement error result=%+v error=%v; want sanitized operational failure", result, serviceErr)
			}
			if observation.beginCalls.Load() != 1 || observation.mutationCalls != 1 || !observation.changed ||
				observation.resource.Active != test.desiredActive || !observation.resource.UpdatedAt.Equal(operationAt) ||
				observation.commitCalls != 1 || stateCalls.Load() != 1 {
				t.Fatalf("acknowledgement simulation retried or missed successful statement: state calls=%d observation=%+v",
					stateCalls.Load(), observation)
			}

			if test.committed {
				if observation.databaseCommitCalls != 1 || observation.databaseCommitErr != nil ||
					!errors.Is(observation.returnedCommitErr, errResourceStateAcknowledgementLost) ||
					observation.rolledBackBeforeAck {
					t.Fatalf("lost-acknowledgement seam did not delegate a successful durable COMMIT: %+v", observation)
				}
				fixture.assertPersistedResource(t, test.desiredActive, operationAt,
					fixture.initialName, fixture.initialDescription)
				fixture.assertHistory(t, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: test.firstAction, occurredAt: operationAt,
					before: test.initialActive, after: test.desiredActive,
				})
			} else {
				if observation.databaseCommitCalls != 0 || !observation.rolledBackBeforeAck ||
					observation.rollbackBeforeAckErr != nil ||
					!errors.Is(observation.returnedCommitErr, errResourceStateAcknowledgementLost) {
					t.Fatalf("noncommitted acknowledgement-like path did not roll back before returning: %+v", observation)
				}
				fixture.assertPersistedResource(t, test.initialActive, fixture.createdAt,
					fixture.initialName, fixture.initialDescription)
				fixture.assertHistory(t)
			}
			fixture.assertBookingsUnchanged(t)

			laterAt := operationAt.Add(time.Minute)
			laterClock := newAtomicBookingClockAt(laterAt)
			laterStateCalls := &atomic.Int32{}
			laterStarted := make(chan int32, 1)
			laterRepository := &resourceStateRaceRepository{
				base: NewStore(fixture.app), started: laterStarted, stateCalls: laterStateCalls,
			}
			laterService := resources.NewStateService(laterRepository, laterClock.Now)
			laterResult, laterErr := laterService.SetState(t.Context(), actor, resources.StateRequest{
				ResourceID: fixture.resourceID, Active: test.laterDesiredActive,
			})
			laterPID := awaitBookingRacePID(t, t.Context(), laterStarted, "later explicit desired-state transaction")
			t.Logf("later explicit operation used labreserve_app transaction pid=%d", laterPID)
			if laterErr != nil || laterResult.Changed != test.laterExpectedChanged ||
				laterResult.Resource.Active != test.laterDesiredActive {
				t.Fatalf("later explicit desired-state result=%+v error=%v", laterResult, laterErr)
			}
			if test.laterExpectedChanged {
				if laterStateCalls.Load() != 1 || laterClock.calls.Load() != 1 ||
					!laterResult.Resource.UpdatedAt.Equal(laterAt) || test.laterAction == "" {
					t.Fatalf("fresh explicit transition calls/time/action=%d/%d/%s action=%q",
						laterStateCalls.Load(), laterClock.calls.Load(), laterResult.Resource.UpdatedAt, test.laterAction)
				}
				fixture.assertPersistedResource(t, test.laterDesiredActive, laterAt,
					fixture.initialName, fixture.initialDescription)
				fixture.assertHistory(t, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: test.laterAction, occurredAt: laterAt,
					before: test.initialActive, after: test.laterDesiredActive,
				})
			} else {
				if laterStateCalls.Load() != 0 || laterClock.calls.Load() != 0 ||
					!laterResult.Resource.UpdatedAt.Equal(operationAt) {
					t.Fatalf("explicit matching-state request was not a genuine no-op: state calls=%d clock calls=%d result=%+v",
						laterStateCalls.Load(), laterClock.calls.Load(), laterResult)
				}
				fixture.assertPersistedResource(t, test.desiredActive, operationAt,
					fixture.initialName, fixture.initialDescription)
				fixture.assertHistory(t, resourceStateConcurrencyEvent{
					actorID: fixture.actors.coordinator, action: test.firstAction, occurredAt: operationAt,
					before: test.initialActive, after: test.desiredActive,
				})
			}
			fixture.assertBookingsUnchanged(t)
		})
	}
}
