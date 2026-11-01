# LabReserve Task 4B — Prepared Instruction for Chunk 7

**Instruction state: `PREPARED_NOT_AUTHORIZED`.**

**Prepared from:** [`task-004b-chunk-6-to-7.md`](task-004b-chunk-6-to-7.md).

**Target:** A fresh implementation session if available and separately authorized. This document does not authorize Chunk 7.

## Authorization and execution mode

- Use `chunked-implementation` in **SINGLE-CHUNK** mode only after verifying that a fresh user explicitly authorizes **Task 4B Chunk 7**. The current request authorized these handoff documents only. Chunk 6 authorization, its commit, and this instruction do not authorize Chunk 7.
- Execute exactly Chunk 7 and stop before Chunk 8. Do not enter continuous-loop mode or dispatch another chunk.
- Do **not** gate Chunk 7 solely on the independent review of completed non-final Chunk 6. That review remains outstanding, but the current user explicitly says it is not a progression gate. Do not describe it as completed or accepted.
- The task document's historical “approved for repository incorporation; not approved for implementation” status is not overridden by this instruction. Require fresh implementation authorization and current preflight.
- No stage, commit, push, merge, deployment, production access, credential changes, or destructive cleanup is authorized by this prepared instruction.

## 1. Governing task and candidate identity

- **Approved contract:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`; current observed SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`; current observed SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`; read the whole file and exact Chunk 7 section before editing.
- **Latest evidence handoff:** `docs/handoffs/task-004b-chunk-6-to-7.md`.
- **Latest prior handoff/instruction:** `docs/handoffs/task-004b-chunk-5-to-6.md` and `docs/handoffs/instruction-chunk-6-task-004b-atomic-resource-activation-deactivation.md`.
- **Earlier continuing context:** `docs/handoffs/task-004b-chunk-4-to-5.md`; consult only for retained obligations or environment/source discrepancies.
- **Candidate recorded by this handoff:** branch `feat/004b-resource-activation-deactivation`; `HEAD` `fbb291ecaed352895475d5204a02c0cf2a11fd24`, equal to `origin/feat/004b-resource-activation-deactivation`; parent/base for Chunk 7 `fbb291ecaed352895475d5204a02c0cf2a11fd24`; Chunk 6's base was `ac5f2b8b510b5c77226cc0a0cc47d798cfe559fa`. The recorded task/plan/migration fingerprints are task `43e5644…`, plan `27a0bd…`, Migration 006 `84330d7…`. Recompute full hashes and all identities; these are historical values, not a substitute for preflight.
- At this handoff, worktree was clean before the two new handoff docs. `HEAD` contains the Chunk 6 test file and the two Chunk 5→6 documents; no production source or migration changed in that delta. The test file SHA-256 is `6a12f7f73b754b26b16b802b36ad52fac593a5f9d45d761822ba1c1ee19e60d3`.
- An earlier conversation response reported a commit beginning `8f67be1`, whereas current Git observes `fbb291e…` at upstream. The reason is not established. Reinspect actual history and tree; do not use the earlier short hash. Task/plan/migration and tested Chunk 6 source bytes currently match the recorded hashes; current Chunk 7 preflight must still classify any new drift.
- **Prior Chunk 6 evidence:** `CONTINUABLE`; its `PREFLIGHT_PASS` was for Chunk 6 and is not Chunk 7 preflight. Verify the committed test/evidence yourself. The current user says not to wait for Chunk 6 independent review.
- **Current validation policy:** plan §7 sets two failed validation attempts per issue. Chunk 6 had one resolved JSONB snapshot-order test failure and no unresolved issue; see its linked handoff. Do not reset a repeated issue's count merely because the session changes.

## 2. Mandatory state/source preflight — before edits

1. Read repository `AGENTS.md`, applicable session instructions, the complete Task 4B contract, relevant PRD/architecture, the whole controlling plan, and the latest handoff/instruction through their endings. Read Task 3B and architecture 002 for the cancellation contract; inspect Chunk 7 and its V5/V7 references.
2. Recheck explicit Chunk 7 implementation authorization and SINGLE-CHUNK mode. The current permission to write handoffs is not execution authority.
3. Inspect branch, `HEAD`, upstream, ancestry, complete status, staged/unstaged diffs, and all untracked files. Protect user work. Read the full relevant candidate delta, including the Chunk 6 race file and these handoffs; untracked content is not shown by ordinary `git diff`.
4. Recompute current full Task/plan/migration hashes and verify authoritative files are tracked and committed. Confirm the task/plan content still matches their authority and the actual Chunk 7 section remains unchanged in scope.
5. Verify Chunk 6's source/test file is present and inspect its relevant wrappers. Confirm the reported Chunk 6 wait schedules/outcomes against actual current code/evidence; review is not a gate, but a concrete defect must be reproduced and reported, not ignored.
6. Re-discover current cancellation/state test names and Compose commands. Verify the test helper accepts only `labreserve_test`, and ensure service operations use actual `current_user = 'labreserve_app'`.
7. Recheck Docker/Compose project/container/volume ownership and activity before any DB operation. A prior read-only check found `labreserve-task004b-c6-ac5f2b8b`'s DB container stopped with its `verify-data` volume retained. This does not prove that volume is appropriate for reuse. Historical reports about `labreserve-verify` conflict; do not assume it is available or owned. Prefer a unique disposable project after verifying it is isolated. Never print connection strings or credentials.
8. Establish the relevant cancellation/state baseline where practical. Record exact test discovery and skips. Report `PREFLIGHT_PASS` only if all applicable gates pass; otherwise stop before edits with the precise `PREFLIGHT_BLOCKED` or `INCOMPLETE` evidence.

## 3. Exact Chunk 7 objective and scope

**Plan title:** Chunk 7 — **Owner cancellation versus deactivation**.

**Plan traceability:** V5 cancellation schedules/V7 preservation; FR-019–025/027; AC-008–010/025/036–039.

**Objective:** Add deterministic real-PostgreSQL evidence for owner cancellation versus Resource deactivation in both forced lock orders, strict cancellation time after lock wait, owner-only authority, remaining blockers, explicit deactivation resubmission, and cancellation rollback while deactivation waits. Preserve existing services and lock protocols.

**Plan-proposed change:** new `internal/database/resource_state_cancellation_concurrency_integration_test.go`. A narrowly scoped test-helper extension is allowed only if a concrete barrier/diagnostic needed by this chunk cannot be composed from Chunk 6 helpers. No production changes, schema/migration, route, dependency, or generalized test framework.

**Actual existing seams to inspect and preserve:**

- `internal/bookings/cancellation.go`: `CancellationService.Cancel` performs the preliminary ownership-scoped Resource lookup without locking Booking; then Resource lock → Booking lock/re-read → ownership/state decision → fresh time for a Confirmed transition → state/metadata update → `booking.cancelled` event → one commit. A Cancelled owned Booking is a no-time-read/no-event no-op; inactive Resource does not prevent owner cancellation.
- `internal/database/booking_cancellation.go`: `bookingCancellationTransaction` implements the same transaction with Resource `FOR UPDATE` before Booking `FOR UPDATE`; the preliminary query is not authority for final Booking state.
- `internal/resources/state.go`: `StateService.SetState` resolves stored Coordinator authority without Account coordination locking, locks Resource, honors matching-state no-op, captures operation time after Resource wait, runs `HasBlockingBookings` with `state='confirmed' AND end_at > operationTime`, then uses the guarded state/event capability and commits.
- `internal/database/resource_state.go`: `HasBlockingBookings` is a real separate query; occupancy rejection happens before `SetResourceState` and before state `Commit`.
- `internal/database/resource_state_booking_concurrency_integration_test.go`: package-level `bookingRaceGroup`, `bookingRaceGate`, `waitForBookingRaceLockWait`, `logBookingRaceWait`, `requireRuntimeTransaction`, `resourceStateRaceRepository`, and `resourceStateRaceTransaction` are available to same-package tests. Existing state wrapper gates `beforeSet` and `beforeCommit` do **not** pause after an occupied `HasBlockingBookings` result; do not mistake a pre-mutation gate for proof the deactivation decision is held. If needed, add a small test-only decorator that forwards the real occupancy query, then pauses on its returned `occupied=true` while the real Resource lock is still held. For cancellation, use a narrow wrapper over the real `CancellationTransaction` to expose runtime PID and pause after the actual update/event before commit, or immediately before the actual event append for the failure schedule.
- `internal/database/resource_creation_integration_test.go`: `resourceCreationSetup`; `resource_state_capability_integration_test.go`: `registerResourceStateBookingCleanup`, Booking snapshots, and `assertResourceStateEvent`; `resource_mutations_integration_test.go`: Resource/history snapshots; `booking_creation_integration_test.go`: `holdBookingResourceLock`, `holdBookingRowLock`, `waitForBlockedBookingCalls`, and `atomicBookingClock`.
- Existing cancellation cases in `internal/database/booking_cancellation_integration_test.go`: `TestOwnerCancellationRetainsBookingReleasesIntervalAndPreservesReplay`, `TestOwnerCancellationAllowsOnlyEngineerAndCoordinatorOwnersAndInactiveResource`, `TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp`, `TestOwnerCancellationUsesFreshTimeAfterResourceLockWait`, and `TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied`.

Use same-package wrappers around the actual services and actual PostgreSQL transactions. Do not replace cancellation/state logic with mocks, direct test SQL, a process-local mutex, or a shared enclosing transaction for the contested operations.

## 4. Required schedules and invariants

Force leaders explicitly. Capture service backend PIDs, observe PostgreSQL blocker edges with `pg_blocking_pids`/`pg_locks`, and only then release the identified gate. A timeout-free unordered start is not a forced-order result.

### A. Cancellation commits before deactivation

- Start with a future Confirmed Booking; call the actual owner cancellation service and gate it after its state update and `booking.cancelled` event, before forwarding the actual commit.
- Start deactivation independently and observe it wait on the cancellation holder's Resource lock. Commit cancellation, then require the deactivation's post-lock separate occupancy query to observe Cancelled and permit deactivation if no other Confirmed future-ending Booking remains.
- Assert cancellation metadata and original Booking/request/creation event are retained, exactly one cancellation event exists, and the Resource transition/event is exact.
- In a separate subcase include a second future-ending Confirmed Booking; cancellation of the first must not mask the remaining blocker. Deactivation must return `ErrResourceOccupied`, retain Active state and history, and append no deactivation event.

### B. Deactivation rejects before cancellation; explicit retry only

- Start with a future Confirmed Booking. Let the real state service lock Resource and execute its real occupancy query. A test-only wrapper may pause **after** the query returned occupied and before returning that result to `StateService`, while the Resource lock remains held.
- Start the owner's actual cancellation service and observe it blocked on that Resource. Release the deactivation decision; it must return `ErrResourceOccupied` and release its transaction. Cancellation may then independently succeed while the booking remains future.
- Do not automatically retry the rejected state operation. Explicitly submit a new deactivation request after cancellation completes; it may now transition. Verify each response and exact persisted state, cancellation metadata, request/creation/cancellation/state Activity counts/details, and retained prior history.

### C. Cancellation wait crosses exact start

- Hold Resource on an independent connection, start actual owner cancellation of a future Confirmed Booking, and observe its PostgreSQL wait. Advance the injected clock to exactly `StartAt` before releasing the Resource; the service must return `ErrCancellationStarted` after acquiring the required locks and reading fresh time.
- Assert Booking remains Confirmed with null/unmodified cancellation metadata; no cancellation event exists; its creation/request identity and original interval remain retained and still block deactivation.
- Verify the owner-only boundary: a Coordinator using this service cannot cancel another owner's Confirmed Booking or obtain a successful no-op for another owner's already-Cancelled Booking. Reuse the existing regression and add direct evidence only as needed; no cross-owner authorization is introduced.

### D. Cancellation Activity fails while deactivation waits

- Install an Activity-failure trigger narrowly scoped to the chosen Booking/cancellation action. Gate the actual cancellation after its uncommitted Booking update but before forwarding the event append; start deactivation and observe its wait on the cancellation Resource holder.
- Release the cancellation gate; the trigger must fail the append and the actual cancellation transaction must roll back. The waiting deactivation must re-read committed state and still see the future Confirmed Booking, returning `ErrResourceOccupied`.
- Assert the Booking remains Confirmed, cancellation actor/time stay unchanged, creation/request identity and original creation Activity remain intact, no cancellation or deactivation event persists, and Resource state/timestamp/history are unchanged. Remove the trigger on all exits; join operations and release locks.

### Shared safety rules

- Cancellation remains owner-only for both Engineers and Coordinators; a Coordinator can cancel only their own Booking through this service. Never implement cross-owner cancellation/reason behavior or let a caller-supplied account ID grant authority.
- Preserve cancellation eligibility `fresh now < booking.start_at`; exactly at or after start rejects for Confirmed. Already-Cancelled owned repeats remain successful no-ops without a clock read/event; unauthorized repeats remain inaccessible.
- Preserve global lock order: booking creation Account `FOR NO KEY UPDATE` → replay lookup → Resource; cancellation Resource → Booking; state role read (no Account coordination lock) → Resource. Do not introduce Resource → Account coordination locking or lock Booking before Resource.
- Deactivation occupancy remains all-owner `Confirmed AND end_at > fresh operation time`, in a separate post-Resource-lock query. Cancellation failure/rollback leaves the Confirmed future-ending row blocking; cancellation commit changes that same retained row to Cancelled.
- Use actual `labreserve_app` service connections, independent transactions, finite contexts, barriers, real backend PIDs and observed wait edges. Assert domain outcomes and persisted Resource fields/timestamps, full Booking/request/cancellation metadata, Activity counts/details, and unchanged prior history. An error alone is not rollback evidence.
- No arbitrary sleeps, altered clock rule, changed SQL/lock protocol, route/API, production test seam, added migration, or dependency.

## 5. Explicit exclusions

- Do not begin Chunk 8 desired-state/edit races, Chunk 9 commit uncertainty, Chunk 10 migration/lifecycle work, or Chunk 11 full verification.
- No production changes to cancellation/state services, adapters, schema, privileges, lock order, routes, handlers, or dependency graph. If evidence shows a defect that requires such a change, reproduce it and stop for the appropriate decision; do not silently broaden this test-only chunk.
- No Coordinator cross-owner cancellation, reason workflow, Activity-history surface, cancellation semantic redesign, direct SQL as substitute for service behavior, or weakening of Task 3B/Task 2B tests.
- Do not run `make verify`, browser/Playwright, restart/lifecycle suites, persistent-target operations, demo reset, or destructive volume cleanup; full delivery verification is Chunk 11.
- No staging/commit/push/merge/deploy absent fresh explicit authorization.

## 6. Verification obligations

Re-discover all names before trusting selectors. On a verified isolated disposable project, use uncached serialized real-PostgreSQL tests (`-count=1 -p 1`) and actual runtime-role assertions. For a confirmed `<verified-project>`, the forms are:

```sh
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceStateCancellationRace' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateCancellationRace' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestOwnerCancellationUsesFreshTimeAfterResourceLockWait|TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp|TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied|TestOwnerCancellationAllowsOnlyEngineerAndCoordinatorOwnersAndInactiveResource|TestOwnerCancellationRetainsBookingReleasesIntervalAndPreservesReplay)$' ./internal/database
```

The new selector and subtests are proposed until implemented and discovered. The three first existing cancellation selectors are plan-listed; the ownership and replay regressions are added here because they directly protect the Chunk 7 authority/retention boundary. If the current candidate lacks any named test, adapt selectors based on actual discovery and document the evidence; do not invent test names as results.

Also run the changed-file `gofmt -l` check, `git diff --check`, `go test -run '^$' ./...` compile-only check, application build in the verified container, and full relevant diff/status review. Compile-only is not behavioral evidence. `testPool` in `internal/database/foundation_integration_test.go` skips if a URL is absent and refuses any database other than `labreserve_test`; selected PostgreSQL tests must execute rather than skip. The Compose service provides separate migrator/admin fixture and `labreserve_app` URLs; do not reveal them.

Record exact commands, discovery, selected/executed tests, skip status, project identity, actual runtime role, each leader/waiter PID and PostgreSQL wait edge, results/snapshots, trigger/connection/goroutine cleanup, and limitations. Mark unrun checks `NOT_RUN`/`UNVERIFIED`.

## 7. Validation budget and repair policy

- Use the controlling plan §7 default: **two failed validation attempts per issue**. The Chunk 6 JSONB key-order issue was resolved and has no outstanding attempts; do not reset any same issue if it recurs. Establish Chunk 7's issue-level baseline before testing.
- Diagnose ordinary fixture/barrier/lock-observation/compile/format issues autonomously when they remain within the authorized test-only scope. Make the smallest test/helper-only repair and rerun affected checks.
- Stop if the budget is exhausted, mandatory PostgreSQL evidence is unavailable, a required check remains failing, or correct completion would require production behavior/security/lock changes or scope expansion. Do not weaken tests to manufacture a pass.

## 8. Environment and retained-risk controls

- Use only a verified isolated disposable Compose project and confirm the test helper's `labreserve_test` guard. The previously used `labreserve-task004b-c6-ac5f2b8b` DB container is stopped and its named volume remains; this handoff does not authorize deleting or resetting it. The status/ownership of `labreserve-verify` and other workers' projects must be freshly checked because prior handoffs differ. Never stop or reuse an active project without confirming ownership.
- Tests may invoke the existing idempotent migration setup through `resourceCreationSetup`; that is test-fixture initialization on the disposable test DB, not permission to edit migration files or access a persistent database. No migration change is in scope.
- Use a unique project if ownership of a retained volume is unclear. Do not print secrets, read `.env` contents into output, delete volumes, or touch demo/persistent data.
- No known Chunk 6 production, migration, or privilege deviation is recorded. Full `make verify`, browser, lifecycle and independent review remain future delivery evidence, not Chunk 7 acceptance substitutes.

## 9. Mandatory post-work

1. Inspect branch/HEAD/upstream, full status, staged/unstaged/untracked state, and the complete relevant diff from the Chunk 7 implementation base. Read the complete new race file and any helper extensions.
2. Confirm only authorized test/helper paths changed; no production logic, migrations, routes, dependencies, or unrelated work changed. Preserve this handoff pair and all existing Chunk 5/6 evidence.
3. Verify every required ordering independently with actual runtime transactions, observed PostgreSQL waits, final persisted Booking/request/Resource/Activity outcomes, and cleanup on success/failure.
4. Run required discovery, focused tests, regression selectors, compile/build/format/diff checks; report skips and failures honestly. Assess lock-cycle, ownership, time-boundary, retained-data and cleanup risks.
5. Report any independent review state factually. Do not make Chunk 6 independent review a gate; do not claim Chunk 7 review or Task 4B acceptance.
6. No next handoff document is authorized automatically by single-chunk mode. Ask before creating one unless a later explicit instruction authorizes it.

## 10. Final report and hard stop

Report the exact implementation base/candidate, current source fingerprints, authorization/mode, test files/symbols changed, each forced wait/order and persisted result, discovered/executed tests/skips, exact commands, failure attempts/repairs, environment/cleanup, complete diff scope, risks and unverified checks.

Finish the authorized session after Chunk 7. Do not start Chunk 8 or any other chunk. Use exactly one non-final executor status: `CONTINUABLE`, `BLOCKED`, or `INCOMPLETE`. This prepared instruction itself remains `PREPARED_NOT_AUTHORIZED` until fresh explicit Chunk 7 authorization.

## Suggested skills

- `chunked-implementation` — SINGLE-CHUNK mode only after authorization.
- `specification-readiness-authority` — for current source/authority preflight.
- `pre-read-discipline`, `pre-edit-discipline`, `post-edit-discipline`, and `rtk-command-prefix` — for the authorized test change.
- `handoff` — only if separately authorized after Chunk 7.
