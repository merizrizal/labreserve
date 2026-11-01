# Task 4B — Chunk 7 → Chunk 8 Evidence Handoff

**Status: `CONTINUABLE`.** This records Chunk 7 only. Chunk 8 has not started and is **not authorized** by this handoff.

**Paired instruction:** [`instruction-chunk-8-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-8-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

## 1. Task, source, candidate, and authority

- **Task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`, tracked at current HEAD; SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, tracked at current HEAD; SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`. Chunk 8 is §9, “Concurrent desired states, descriptive edits and rollback waiters.”
- **Migration 006:** `internal/database/migrations/006_resource_state.sql`, unchanged; SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.
- **Current branch / candidate observed before writing these documents:** `feat/004b-resource-activation-deactivation`, HEAD `18c91ce4d5ca798fe69aac6e0d53b16d899c759e`, equal to `origin/feat/004b-resource-activation-deactivation`; working tree and index clean. HEAD's parent and Chunk 7 implementation base are `fbb291ecaed352895475d5204a02c0cf2a11fd24`. The Task 4B initial implementation base recorded in the plan history is older; for Chunk 8, revalidate from this current HEAD.
- **Current candidate delta from Chunk 7 base `fbb291e…`:** two code/test paths and two previously supplied handoff/instruction documents. `rtk git diff fbb291ecaed352895475d5204a02c0cf2a11fd24..HEAD --name-status` observed:
  - modified `internal/database/resource_state_booking_concurrency_integration_test.go` (SHA-256 `e7d671f6d587e3f6a72b93b9f66b0005573dd086e01eb7c3e1f7a46f2d68082d`);
  - added `internal/database/resource_state_cancellation_concurrency_integration_test.go` (SHA-256 `96e5640d1c5036ef422ca803c664560de8116cba26768131532bcae282ac431f`);
  - added `docs/handoffs/task-004b-chunk-6-to-7.md` (SHA-256 `7a374364e0b051ba446424fc82aa58e1d67a53b27fa3d2e5afbf803d8e440ea8`);
  - added `docs/handoffs/instruction-chunk-7-task-004b-atomic-resource-activation-deactivation.md` (SHA-256 `67cb19fafb639ac48cc4711bcdedf45cfb8bcb0749722b29a436fa047b275853`).
- **Candidate history discrepancy:** a prior session reported commit `5204917845b0721d085c1a3ac638ebd43a69a19e` at HEAD. The current observed branch instead points to `18c91ce…`, with the same subject and parent but a different tree. The old commit object remains inspectable; `rtk git diff 5204917845b0721d085c1a3ac638ebd43a69a19e..HEAD` adds only the two handoff/instruction files above. The Chunk 7 source/test hashes match the previously tested bytes. The reason for this HEAD change is unknown; do not infer an actor or rewrite history. This is `NONMATERIAL_CONFIRMED` for implementation code, but current identity is `18c91ce…`.
- **Missing earlier handoff artifact:** the prior session reported creating `docs/handoffs/task-004b-chunk-7-to-8.md` with SHA-256 `88f20df3bd7d4c26f3b474a9dfd93ecaf989f533690511605c29d0cc6da7846b`. Fresh inspection found that path absent and untracked/uncommitted at the current candidate. No explanation for its absence is established. This user-authorized handoff recreates it using the current candidate evidence; it does not claim the earlier file is still present.
- **Preflight provenance:** the Chunk 7 execution session reported a fresh `PREFLIGHT_PASS` at base `fbb291e…`. This handoff session independently rechecked current branch/status/history, task/plan/migration fingerprints, relevant committed paths and code hashes. It did **not** run an implementation preflight or authorize Chunk 8. A fresh Chunk 8 executor must perform its own preflight after separate authorization.
- **Authorization:** the earlier user explicitly authorized SINGLE-CHUNK Chunk 7. The current user explicitly authorizes these two Chunk 7→8 documents only. Chunk 8 implementation authorization is **not granted**.

## 2. Completed Chunk 7 — observed scope

Chunk 7 was **Owner cancellation versus deactivation**, test-only. It modified the existing race-test wrapper and added `internal/database/resource_state_cancellation_concurrency_integration_test.go`. No production service, migration, schema, route, dependency, or runtime privilege changed.

The helper delegates the real `HasBlockingBookings` query and offers an `afterOccupied` gate only when the real query returned occupied, while the Resource transaction remains open. This fills a concrete gap: the pre-existing `beforeSet`/`beforeCommit` gates cannot hold an occupied rejection decision because the service returns before either hook. The cancellation decorator wraps the real `CancellationTransaction`; the race helpers require `current_user = 'labreserve_app'`. `testPool` refuses any database other than `labreserve_test`.

Five new test functions were discovered and run in the prior execution session:

- `TestResourceStateCancellationRaceCancellationCommitsBeforeDeactivation` (two subcases: no remaining blocker; second blocker remains);
- `TestResourceStateCancellationRaceDeactivationRejectsBeforeCancellationAndRequiresResubmission`;
- `TestResourceStateCancellationRaceWaitCrossesExactStart`;
- `TestResourceStateCancellationRaceActivityFailureRollsBackBeforeDeactivation`;
- `TestResourceStateCancellationRaceCoordinatorCannotCancelForeignConfirmedOrCancelledBooking`.

They cover cancellation-first deactivation (with/without another blocker), occupied-deactivation rejection before cancellation plus a separate explicit resubmission, cancellation waiting across exact `start_at`, cancellation Activity failure/rollback while deactivation waits, and the Coordinator's inability to cancel another owner's Confirmed or Cancelled Booking. Assertions include exact persisted Resource/Booking/request/Activity snapshots and metadata, not just returned errors.

## 3. Verification evidence and limits

The following results were recorded by the prior Chunk 7 execution session and are **reported historical evidence**, not checks rerun while authoring this handoff. The tested Go source hashes above match the current candidate; the current HEAD change from the previously reported `520491…` candidate adds only handoff documents.

| Command/check reported by prior session | Reported outcome and limit |
| --- | --- |
| `rtk docker compose --project-name labreserve-task004b-c7-fbb291ec --file compose.yaml --profile verify build verify-go` | **PASS**; verification image rebuilt from the Chunk 7 candidate. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -list '^TestResourceStateCancellationRace' ./internal/database` | **PASS**; discovered the five new test functions above. Discovery only. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -list '^(TestOwnerCancellationUsesFreshTimeAfterResourceLockWait|TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp|TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied|TestOwnerCancellationAllowsOnlyEngineerAndCoordinatorOwnersAndInactiveResource|TestOwnerCancellationRetainsBookingReleasesIntervalAndPreservesReplay|TestOwnerCancellationWaitsForResourceBeforeBookingLock|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence)$' ./internal/database` | **PASS**; discovered all seven names. Discovery only. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateCancellationRace' ./internal/database` | **PASS**; all five new functions and both subcases executed; no skips. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestOwnerCancellationUsesFreshTimeAfterResourceLockWait|TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp|TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied|TestOwnerCancellationAllowsOnlyEngineerAndCoordinatorOwnersAndInactiveResource|TestOwnerCancellationRetainsBookingReleasesIntervalAndPreservesReplay)$' ./internal/database` | **PASS**; all five cancellation regressions executed without skips. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestOwnerCancellationWaitsForResourceBeforeBookingLock|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence)$' ./internal/database` | **PASS**; both additional lock-order/no-op regressions executed without skips. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateBookingRace' ./internal/database` | **PASS**; all five existing Chunk 6 schedules passed after the shared helper change, without skips. |
| Same Compose prefix, `run --rm --no-deps verify-go go test -run '^$' ./...` | **PASS**, compile-only; all packages compiled and no tests were selected. This is not behavioral evidence. |
| Same Compose prefix, `run --rm --no-deps verify-go sh -ec 'go build -o /tmp/labreserve-task004b-chunk7-check ./cmd/labreserve && test -s /tmp/labreserve-task004b-chunk7-check'` | **PASS**; build and artifact check in the same ephemeral container. |
| Same Compose prefix, `run --rm --no-deps verify-go gofmt -l internal/database/resource_state_booking_concurrency_integration_test.go internal/database/resource_state_cancellation_concurrency_integration_test.go` | **PASS**; no output. `rtk git diff --check` also **PASS**. |
| `make verify`, browser/Playwright, lifecycle/restart and delivery-wide verification | **NOT_RUN**, reserved for Chunk 11. No full Task 4B verification is claimed. |

### PostgreSQL wait evidence reported by the execution session

The prior session reported observing these edges before releasing the corresponding gate/lock. Each was `transactionid/ShareLock`; `wait_event_type` and `wait_event` were empty, so no specific wait-event value is claimed.

| Schedule | Observed waiter → blocker | Reported result after release |
| --- | --- | --- |
| Deactivation waited for owner cancellation to commit; no second blocker | **1563 → 1562** | Cancellation retained its Booking and one cancellation event; deactivation then observed Cancelled and committed Inactive with one correct state event. |
| Deactivation waited for cancellation; second Confirmed future-ending Booking remained | **1566 → 1567** | First Booking cancellation committed; deactivation returned `ErrResourceOccupied`; Resource and history stayed unchanged. |
| Cancellation waited behind deactivation's real occupied-query decision | **1570 → 1571** | First deactivation returned `ErrResourceOccupied`; owner cancellation then committed. Only a separate explicit request later deactivated the Resource. |
| Cancellation waited on an independent Resource lock until exact Booking start | **1575 → 1572** | Cancellation returned `ErrCancellationStarted`; Booking/request/history remained unchanged and still blocked deactivation. |
| Deactivation waited for a cancellation whose Activity append failed | **1578 → 1579** | Cancellation rolled back with safe `ErrOperational`; deactivation re-read Confirmed state and returned `ErrResourceOccupied`; no partial history persisted. |

The five existing Chunk 6 schedules were also reported passing after the helper change. Observed edges: **2084→2083**, **2088→2087**, **2092→2091** (twice while state Activity remained uncommitted), **2095→2096**, **2099→2100**; all were `transactionid/ShareLock`, with empty wait-event fields. Installed Account `FOR NO KEY UPDATE` and Resource `FOR UPDATE` primitives were rechecked.

### Failed attempt and repair

One prior validation invocation returned 1 because it built `/tmp/labreserve-task004b-chunk7-check` in one `docker compose run --rm` container and checked the path from a second ephemeral container. The build itself succeeded; the artifact was not shared between containers. The command was corrected to build and check in one `sh -ec` invocation, which passed. No test failed. Budget history: **one failed validation invocation for this resolved artifact-check issue**; do not reset if the same issue recurs. Chunk 6's preceding handoff separately records one corrected JSONB snapshot key-order test attempt. No unresolved Chunk 6/7 validation failure is reported.

This handoff-authoring session ran read-only Git/hash/source and Docker inventory checks only. It did not run tests, builds, or migrations; integration test setup calls `Migrate`, and this handoff task does not run migration/test work. Direct current observations: the Chunk 7 Compose `verify-db` is healthy and its project-labelled `verify-data` volume remains; the separate `labreserve-task004b-c5-19566ee` `verify-db` is also healthy. Neither was stopped or cleaned up. `.env` is ignored and untouched; its contents were not read.

## 4. Preservation, risks, and review state

- Current HEAD is the committed Chunk 7 candidate plus the two previously supplied Chunk 6→7 documents. No staged, unstaged, or unrelated working-tree change was present before writing this pair. These two new handoff documents are the only files this task is authorized to add.
- Migration 006 and all production state/cancellation code remain unchanged. The code-only hashes matching the prior tested source establish that the current commit-identity drift did not change the verified implementation bytes.
- **Independent review:** Chunk 6 independent review remains outstanding, and Chunk 7 helper/test review is also not recorded. The user explicitly said not to gate a non-final chunk on the completed chunk's independent review. Neither is claimed as accepted; both remain eventual review obligations.
- **Risk/evidence limits:** Chunk 7 evidence covers the enumerated owner-cancellation/deactivation schedules, not all Task 4B concurrency or acceptance properties. The observed PostgreSQL blocker edges prove waits; blank wait-event fields limit event-specific claims. Persistent Migration 005 compatibility was not inspected or certified by this test work. No browser, lifecycle, or full delivery gate is claimed.
- **Environment safety:** retain the active Chunk 7 and Chunk 5 test projects/volumes unless a later authorized executor verifies exact ownership and has a scoped reason. Never infer that retained `labreserve_test` data is empty or resettable. No cleanup command was run here.
- **Rollback:** these handoff files are factual records, not a reason to revert code or remove earlier handoffs, `.env`, active projects, or volumes. Preserve all existing work.

## 5. Exact next boundary — Chunk 8, not authorized

The controlling plan's next title is **Chunk 8 — Concurrent desired states, descriptive edits and rollback waiters**. Its bounded scope is a new `internal/database/resource_state_concurrency_integration_test.go` and only bounded test-helper additions/refactoring if needed (the plan proposes `internal/database/resource_state_test_helpers_test.go`). It is test-only: do not change production services/SQL/migrations/routes/dependencies.

The next authorized executor must force and separately assert:

1. **Repeated desired Active:** start Inactive; one transition/event, then a waiting Active submission is a genuine no-op with no Resource UPDATE, timestamp rewrite, or event attempt.
2. **Repeated desired Inactive:** start eligible Active; one transition/event, then a waiting Inactive submission is a genuine no-op with no UPDATE/time rewrite/event.
3. **Opposing desired states:** both forced lock orders from both initial states. Starting Inactive, activation-first then deactivation produces two actual transitions/events and final Inactive; deactivation-no-op-first then activation produces one transition/event and final Active. Starting Active, deactivation-first then activation produces two transitions/events and final Active; activation-no-op-first then deactivation produces one transition/event and final Inactive. Repeat opposite transitions with equal operation-clock microseconds; use intermediate persisted snapshots, not timestamps/random event IDs as causal ordering.
4. **State/edit serialization:** force state and actual `resources.Service.Edit` in both orders; verify each preserves the other operation's permitted fields, exact descriptive event before/after values, immutable code/`created_at`, raw retained text and Booking/history.
5. **Rollback waiters:** separately force a leading Resource state mutation failure and a leading state Activity failure while another state command waits. Observe the PostgreSQL blocker edge, release the leader, prove rollback of the complete state/event pair, and prove the waiter re-evaluates the committed Resource and clock rather than trusting uncommitted values. Assert exact final state, timestamps, events, and snapshots.

Use independent actual `labreserve_app` transactions, backend PIDs, explicit bounded gates and observed PostgreSQL wait edges (`pg_blocking_pids` plus ungranted `pg_locks`), exact persisted snapshots, and reliable cleanup. Existing `resource_state_booking_concurrency_integration_test.go` provides `bookingRaceGroup`, `bookingRaceGate`, `requireRuntimeTransaction`, `resourceStateRaceRepository`/transaction wrappers, and wait-edge helpers; inspect their current implementation before reuse/refactor. Existing edit-race tests and update/activity failure helpers are in `resource_concurrency_integration_test.go` and `resource_mutations_integration_test.go`. `resourceCreationSetup` runs the idempotent migration setup; `testPool` requires `labreserve_test`, and state/cancellation wrappers verify runtime role. Do not use mocks, a process mutex, sleeps, a shared transaction, or a migrator connection as operation evidence.

Important test-seam cautions from current code:

- `resourceStateRaceTransaction.Commit` currently calls `assertPendingStateTransition` when its `beforeCommit` gate is installed; it errors if no transition was recorded. A leading no-op in an opposing-order schedule therefore cannot be held with that gate unchanged. Add a narrow test-only post-lock/no-op gate or refine its assertions; do not confuse observed-but-not-held locks with a forced order.
- `installResourceNoOpUpdateFailureTrigger` in `resource_concurrency_integration_test.go` is edit-specific: it fires when `name` and `description` are unchanged. A legitimate state-only UPDATE also preserves both, so do **not** reuse it unchanged for state transitions. Use a fixture-scoped state-aware detector that permits an actual `active` change but rejects every UPDATE whose `OLD.active = NEW.active` during a state operation; this also catches timestamp-only rewrites. An equally strong database-level observation is acceptable.
- The general `installResourceUpdateFailureTrigger` rejects every update to the fixture Resource. Ensure it cannot also sabotage a rollback waiter that should transition; scope the injected failure or choose/assert a valid waiter no-op after rollback.

## Suggested skills

- `chunked-implementation` — **SINGLE-CHUNK** only after separate Chunk 8 authorization and current preflight.
- `specification-readiness-authority` — if task/plan/candidate/authority differs at preflight.
- `pre-read-discipline`, `pre-edit-discipline`, `post-edit-discipline`, `rtk-command-prefix` — for the authorized test-only implementation.
- `handoff` — only for a later separately authorized transfer.

**Chunk 7 status: `CONTINUABLE`.**
