# LabReserve Task 4B — Prepared Instruction for Chunk 6

**Instruction state: `PREPARED_NOT_AUTHORIZED`.**

**Prepared from:** [`task-004b-chunk-5-to-6.md`](task-004b-chunk-5-to-6.md).

**Target:** A fresh implementation session, only after separate explicit Chunk 6 authorization. This document is not authorization.

## Authorization and execution mode

- Use the `chunked-implementation` workflow only after confirming that the current user explicitly authorized **Task 4B Chunk 6**. Chunk 5 authorization, its commit, and this instruction do not authorize Chunk 6.
- Execute exactly Chunk 6 and stop before Chunk 7. Do not run a continuous loop or dispatch another chunk.
- The controlling plan records execution mode as `UNSELECTED`. Resolve mode from the fresh authorization and current execution policy before editing; do not assume continuous-loop authority. If execution mode remains unresolved, ask before code changes.
- Do not make Chunk 5's outstanding independent review a gate for Chunk 6. Do verify the committed Chunk 5 code/evidence and current candidate; if a concrete defect or material concern is found, reproduce and report it rather than relying on a review-status label.
- No stage, commit, push, merge, deployment, production access, credential change, or destructive cleanup is authorized by this prepared instruction.

## 1. Governing sources and candidate identity to revalidate

- Approved contract: `docs/tasks/004b-atomic-resource-activation-deactivation.md`.
- Sole controlling ladder: `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, **Chunk 6 — Booking creation versus deactivation in both lock orders**.
- Latest evidence handoff: [`task-004b-chunk-5-to-6.md`](task-004b-chunk-5-to-6.md).
- Relevant earlier handoff: `docs/handoffs/task-004b-chunk-4-to-5.md` for retained Chunk 4 behavior and environment cautions.
- At handoff time, task SHA-256 was `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`; plan SHA-256 was `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`. Recompute both; do not trust these historical values if the files differ.
- Recorded implementation candidate at handoff: branch `feat/004b-resource-activation-deactivation`, `HEAD` `ac5f2b8b510b5c77226cc0a0cc47d798cfe559fa`, base/parent `19566eeea5ec33944fba3740bf00ff54f0450b8c`. Reinspect branch, `HEAD`, upstream, full status, commit ancestry, and the entire candidate diff, including untracked files. The two new handoff documents may still be untracked; preserve them and any other user work.
- Migration 006 was reported unchanged at the Chunk 5 candidate (SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`). Verify current status before any DB work.

## 2. Mandatory fresh pre-edit gate

Before editing implementation or tests:

1. Read the repository `AGENTS.md` completely and follow the task's specification-readiness and verification rules.
2. Re-read the complete Task 4B contract, relevant PRD and architecture, and the entire controlling plan, including its ending. Inspect the complete Chunk 6 section and its referenced V4, V5, and V7 obligations.
3. Re-read the Chunk 5→6 handoff through its ending and inspect the Chunk 4→5 handoff where necessary. Confirm that the current candidate contains the reported Chunk 5 tests and accepted Chunk 4 implementation.
4. Recompute full task/plan fingerprints, verify they remain tracked and committed, and record current branch/HEAD/base/upstream plus `git status --short --untracked-files=all`, staged and unstaged diffs, and relevant untracked contents.
5. Compare actual source/candidate changes to the handoff. Classify drift as nonmaterial with evidence, material requiring affected-gate revalidation, or unverified. A matching task hash alone does not validate architecture, candidate, environment, or authorization.
6. Re-discover exact test names and current Go/Compose commands. Confirm an isolated Compose project is owned by this execution, the test helper accepts the exact `labreserve_test` database, and ordinary operations use the actual `labreserve_app` login. Do not infer ownership or database identity from a project name.
7. Establish the relevant baseline where practical. Report `PREFLIGHT_PASS` only after all applicable gates pass. Otherwise stop before edits and report `PREFLIGHT_BLOCKED` or `INCOMPLETE` with the specific evidence needed.

## 3. Exact Chunk 6 objective and authorized scope

**Plan title:** Chunk 6 — **Booking creation versus deactivation in both lock orders**.

**Plan traceability:** V4 and the creation portions of V5/V7; FR-009/015/024–027; AC-005–006/010/021–023/033 preservation.

**Objective:** Add deterministic real-PostgreSQL evidence for booking creation versus deactivation in both forced Resource-lock orders, same-Coordinator-Account lock compatibility, the existing post-lock booking time check, retained request replay/reuse precedence, and the two leading-operation rollback schedules. Preserve the accepted services and lock protocol.

**Plan-proposed files:**

- New `internal/database/resource_state_booking_concurrency_integration_test.go`.
- Optional `internal/database/resource_state_test_helpers_test.go` only for narrowly scoped, reusable real-transaction holders/waiters, barriers, PID/lock observation, and snapshots. Add helpers only when needed by Chunk 6; do not implement Chunk 7 support speculatively.

**Existing seams to inspect before editing:**

- `internal/bookings/service.go`: `Service.Create` and the existing `minimumStartLead` (`5*time.Minute`). Current order is Account lock → separate retained request replay/reuse lookup → Resource lock for a new key → authoritative clock read → Active/input checks → Booking insert → `booking.created` append → transaction commit.
- `internal/database/bookings.go`: `Store.BeginBookingCreation`, `bookingCreationTransaction`, `LockAccount`, `FindBookingByRequest`, `LockResource`, `InsertBooking`, `AppendBookingCreatedEvent`, and commit/rollback. Its lock helpers use the existing scoped DB functions; do not replace their lock order.
- `internal/resources/state.go`: `StateService.SetState` resolves stored Coordinator authority, locks Resource, returns matching-state no-op before clock/occupancy, obtains operation time after the Resource lock, checks deactivation occupancy, calls the guarded state/event capability, and commits.
- `internal/database/resource_state.go` and `internal/database/migrations/006_resource_state.sql`: actual state transaction/capability; do not alter migration history or production lock behavior in this test-only chunk.
- `internal/database/booking_creation_integration_test.go`: `holdBookingResourceLock`, `holdBookingAccountLock`, `waitForBlockedBookingCalls`, `atomicBookingClock`, creation fixtures, and request/event count helpers.
- `internal/database/resource_state_integration_test.go` and `resource_state_capability_integration_test.go`: state fixtures, scoped failure-trigger/snapshot patterns, and the existing `TestResourceStateCapabilityWaitsForResourceAndSeesCommittedBooking`.
- `internal/database/resource_concurrency_integration_test.go` and `internal/database/resource_mutations_integration_test.go` for established lock-observation and fault-fixture conventions.

The plan explicitly limits this chunk to tests and says **no production locking change**. Test wrappers may pause an actual service transaction at a controlled seam (for example, immediately before forwarding `Commit`, or before forwarding the event/state call) while the actual runtime transaction owns its PostgreSQL locks. Do not substitute a fake repository, process-local mutex, arbitrary sleep, or shared enclosing transaction for the contested service operations.

## 4. Required schedules and invariants

Implement and verify each schedule independently. Identify the leader and waiter before releasing a barrier; an unordered race or a timeout-free run is not proof of a forced lock order.

### A. Booking creation wins; deactivation waits

- Use a valid new request on an Active Resource. Let the actual creation service acquire Account/Resource locks, insert the Booking and `booking.created` event, then pause before the actual transaction commit.
- Capture the creator backend PID. Start deactivation on an independent runtime transaction and observe its PostgreSQL wait on the creator/Resource lock using `pg_blocking_pids` or the repository's equivalent bounded lock graph.
- Release only the named creator. After it commits, the waiting deactivation must complete its post-lock occupancy check against the committed future-ending Confirmed Booking and return `resources.ErrResourceOccupied`.
- Assert exactly one retained Booking and one `booking.created` event; Resource remains Active with unchanged state timestamp; no `resource.deactivated` event; Booking/history snapshot is intact.

### B. Deactivation wins; valid creation waits

- Start with an eligible Active Resource and a valid new request with its own unused request key. Pause the actual state-service transaction after its guarded inactive mutation and `resource.deactivated` event have executed but before commit.
- Start the actual creation service on a separate runtime transaction; observe its wait on the state holder's Resource lock and identify the blocker.
- Commit the named deactivation. The creator must then observe Inactive and return `bookings.ErrInactiveResource`.
- Assert one deactivation event and no Booking, no `booking.created` event, and no successful request-key binding. Resource and Activity must reflect only the committed deactivation.

### C. Same Coordinator Account during state-history insertion

- Use the same stored Coordinator Account as the creation actor and state actor. Have the state service acquire Resource, then pause a test wrapper after `LockResource` and before forwarding the guarded state call. Start the creator with that same Account; observe it hold Account `FOR NO KEY UPDATE` and wait on Resource.
- Only after that wait is observed, release the state wrapper so the actual state capability performs the transition and writes its Activity event while the creator still owns Account and waits on Resource. Pause before commit if needed to capture the overlap, then commit the named state holder.
- Observe and record actual PostgreSQL lock modes/order and the wait edge, not merely absence of a timeout. Prove the Activity foreign-key/role-read path completes without a Resource→Account inversion/deadlock. Release only after the intended wait is observed.
- Assert the serial result and exact Resource/Booking/Activity history. Keep the same Coordinator identity in the fixture; an unrelated actor/account would not cover this invariant.

### D. Lock-wait crossing the booking minimum-lead boundary

- Preserve Task 2B fresh-time behavior using the actual creation service, an Active Resource, `atomicBookingClock`, and a controlled Resource lock holder.
- Construct a request at the five-minute minimum lead using the initial clock. Hold Resource, start creation, observe its PostgreSQL wait, then advance the injected clock just beyond the request's minimum-lead eligibility before releasing the lock.
- Require post-lock validation to return `bookings.ErrInvalidInput`; assert no Booking/event/request binding and that the clock was not read for eligibility before Resource acquisition. Use controlled microsecond values rather than a wall-clock sleep.

### E. Replay/reuse precedes Resource eligibility and lock

- With an Inactive Resource locked by an independent transaction, submit an identical retained request replay and a same-owner/same-key changed-data reuse request through the actual service.
- Both calls must finish while the Resource remains locked: identical data returns the original retained Booking (`Replayed=true`), changed data returns `bookings.ErrRequestIDReuse`. Neither may wait for Resource eligibility or create an additional Booking/event. Verify persisted snapshots/counts before releasing the lock.
- Retain existing coverage such as `TestBookingCreationChangedDataWinsBeforeResourceEligibility`; do not change replay, ownership, or idempotency semantics.

### F. Leading Booking creation fails; waiting deactivation re-evaluates

- Arrange a narrowly scoped `booking.created` Activity failure for the chosen Resource/actor/request. Pause the actual creation transaction after Booking insert but before it attempts the event append; start and observe the named deactivation waiter; then allow the event attempt to fail and the creation transaction to roll back.
- The waiting deactivation must acquire Resource after rollback, see no new Confirmed Booking, and be allowed to commit if otherwise eligible.
- Assert the failed request left no Booking, request key, or event; exactly the permitted deactivation state/event persists; no partial Booking or Activity history remains.

### G. Leading deactivation fails; waiting valid creation re-evaluates

- Arrange a state-event failure scoped to the target Resource, Coordinator, and `resource.deactivated` action. Pause the state service after it owns Resource and before its actual guarded mutation/event attempt; start and observe the valid creation waiter; then allow the guarded call to fail and the state transaction to roll back.
- The waiting creator must proceed against the still-Active Resource and may commit the valid Booking and its creation event.
- Assert no deactivation state/time/event persisted, exactly one new Booking and one creation event persisted, and no false inactive rejection or partial state update occurred.

### Shared safety and evidence rules

- Use independent real runtime transactions/connections for every contested operation. `LABRESERVE_TEST_DATABASE_URL` is for permitted migration/fixture/inspection operations; `LABRESERVE_TEST_APP_DATABASE_URL` must resolve to actual `current_user = 'labreserve_app'` for service operations.
- Use finite contexts, deterministic channels/barriers, actual backend PIDs, observed PostgreSQL wait edges, and explicit release/rollback cleanup. Every goroutine must be joined; every held connection/transaction and scoped trigger must be released/removed on success and failure paths.
- For each schedule, assert returned domain result **and** persisted Resource fields/timestamps, complete relevant Booking/request state, Activity actions/count/details, and unchanged prior history. Do not claim rollback from an error alone.
- Do not add a new locking primitive or change actor authority, replay precedence, the five-minute rule, occupancy predicate, state semantics, route/API, migration, dependency, or production code. If a reproduced production defect requires one of those changes, stop and report the evidence and Level 2 decision needed instead of silently broadening this test-only chunk.

## 5. Explicit exclusions

- Do not begin Chunk 7 cancellation races, Chunk 8 desired-state/edit races, Chunk 9 commit uncertainty, Chunk 10 migration/lifecycle work, or Chunk 11 full verification.
- No production lock/schema/privilege/route changes; no broad refactor or speculative shared test framework.
- No weakening existing Task 2B replay, time, overlap, cancellation, identity, or Activity assertions.
- No `make verify`, Playwright/browser run, lifecycle/restart run, persistent target, demo reset, or production credential access. Full delivery verification is reserved for Chunk 11.
- Do not stage or commit unless separately requested. Never push, merge, deploy, or run destructive volume cleanup without independent explicit authorization.

## 6. Environment and verification obligations

Use only an isolated, locally owned disposable Compose project after confirming its identity is not another worker's active environment and the test helper's safety guard accepts `labreserve_test`. The previous Chunk 5 project `labreserve-task004b-c5-19566ee` was observed active during handoff authoring; do not reuse or stop it without reconfirming ownership. The standard `labreserve-verify` project had been observed stopped with a retained volume; do not assume the volume is disposable. Prefer a unique project identity for this execution if a fresh isolated project is needed. Do not print connection strings or credentials.

The controlling plan's section 8 documents the Compose verification shape and actual PostgreSQL/runtime roles. Revalidate current service names and project ownership first. For a confirmed project variable `<verified-project>`, discovery and focused invocations follow this form (new race selector is proposed until discovered):

```sh
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceStateBookingRace' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateBookingRace' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestBookingCreationValidatesTimeAfterResourceLockWait|TestBookingCreationChangedDataWinsBeforeResourceEligibility|TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability)$' ./internal/database
```

- The new race selector and subtest names are **proposed**, not discovered until the new tests exist. Run `go test -list` and report every discovered/selected test. Fail the evidence gate if the selector matches none or required tests skip due to missing DB configuration.
- The three existing names above are plan-listed regression tests; re-discover them in the current candidate before relying on that selector.
- Build/rebuild only the required verification image(s) for the current candidate. Use the plan's isolated service setup as applicable; do not run `make verify` or an unrelated browser suite.
- Run the plan's compile-only check (`go test -run '^$' ./...`) and application build in the verified container, plus repository-convention formatting (`gofmt -l` on changed Go files), `git diff --check`, and complete diff review. Use `-count=1 -p 1` for PostgreSQL tests. A compile-only check proves no behavior.
- Record actual commands, project identity (not credentials), runtime `current_user`, discovered tests, skips, every forced wait/order, final snapshots/counts, failures, cleanup, and limitations. Mark anything not run `NOT_RUN`/`UNVERIFIED`.
- Do not run full `make verify`; it and browser/lifecycle verification belong to Chunk 11.

## 7. Validation budget and repair policy

- Default budget: **two failed validation attempts for the same current-chunk issue**, from `chunked-implementation`'s `execution-policy.md`. Keep this finite budget fixed before testing.
- The Chunk 5 handoff has no unresolved failure reported but does not preserve an exact count of all transient attempts. Establish the Chunk 6 issue-level baseline at preflight; do not reset a repeated failure because the session changed.
- Diagnose ordinary fixture, barrier, lock-observation, compilation, formatting, and test failures autonomously if they remain within Chunk 6 and preserve the accepted architecture. Make the smallest test-only repair and rerun affected checks.
- Stop with `BLOCKED` when the budget is exhausted, required infrastructure/evidence is unavailable, a required check remains failing, or correct completion requires a production locking/authority change, scope expansion, or another Level 2 decision. Do not weaken tests to manufacture a pass.

## 8. Mandatory post-work and handoff boundary

Before reporting:

1. Inspect branch/HEAD/upstream and complete staged/unstaged/untracked state; read the full relevant diff against the implementation base and inspect all new helper/test files.
2. Confirm only Chunk 6 tests/helpers changed; preserve committed Chunk 4/5 code and the two Chunk 5→6 documents. Remove no unrelated or retained environment work.
3. Verify every required schedule ran independently with actual PostgreSQL waiting evidence, actual `labreserve_app` service operations, exact persistent outcomes, and trigger/transaction/connection cleanup. Report any schedule not covered.
4. Run the required focused tests, discovery, compile/build, formatting, and diff checks; state exact outcomes and skips. Assess lock-cycle, data-integrity, privilege, and regression risks and any rollback needed.
5. The plan requires a Chunk 6 evidence handoff with each forced schedule and actual wait evidence, not an aggregate result. Create only the handoff artifact(s) allowed by the fresh user's authorization and resolved execution policy. Do not treat documentation as authorization to continue.
6. Report base/candidate, source fingerprints, files/symbols changed, actual commands and results, failed-attempt counts, schedule-by-schedule evidence, environment cleanup, limitations, and outstanding decisions. Finish with exactly one non-final executor status: `CONTINUABLE`, `BLOCKED`, or `INCOMPLETE`.

**Hard stop:** after Chunk 6's required tests/evidence and report, stop. Do not execute Chunk 7 or claim final Task 4B acceptance. Obtain separate authorization for any later chunk.
