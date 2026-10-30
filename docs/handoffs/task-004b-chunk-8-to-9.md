# Task 4B — Chunk 8 → Chunk 9 Evidence Handoff

**Status: `CONTINUABLE`.** This records Chunk 8 only. Chunk 9 has not started.

**Paired next-session instruction:** [`instruction-chunk-9-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-9-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

## 1. Task, source, candidate, and authority

- **Approved task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`, tracked at current HEAD; observed SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`; observed SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`. Chunk 8 is §9, “Concurrent desired states, descriptive edits and rollback waiters”; Chunk 9 is §9, “Actual COMMIT rejection and ambiguous acknowledgement.”
- **Migration 006:** `internal/database/migrations/006_resource_state.sql`, unchanged; SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.
- **Chunk 8 implementation base:** `18c91ce4d5ca798fe69aac6e0d53b16d899c759e`, branch `feat/004b-resource-activation-deactivation`; observed upstream `origin/feat/004b-resource-activation-deactivation` resolves to the same HEAD. No commit was made. Current code identities: modified `internal/database/resource_state_booking_concurrency_integration_test.go` SHA-256 `237e0ecd01348b5cb1de1d6ad2591c59239d1fe38330d1a9dcdc5db694e284b0`; new `internal/database/resource_state_concurrency_integration_test.go` SHA-256 `1a3a9ba6b0896ba8da823a0825f7331a5b8de5bd5227092867b1802e8a1ff992`.
- **Working-tree state at handoff:** one tracked modified test helper, no staged changes; untracked new Chunk 8 test file and four handoff/instruction documents. Two of those documents—`task-004b-chunk-7-to-8.md` (SHA-256 `c8a569c86b32b1d86fc0bd322c2e289b6f94608b75f9c349b2f34ed76652cdea`) and `instruction-chunk-8-task-004b-atomic-resource-activation-deactivation.md` (SHA-256 `c6d6629db06525fb02882faa65cbf0b687668f2c21f105026bad626344a16115`)—were already untracked at preflight and were preserved unmodified. The other two are this authorized handoff pair. `git diff --cached` is empty.
- **Preflight:** this execution revalidated the task/plan/migration identities, current candidate and working tree, full task and plan, Chunk 8 instruction, repository constraints, existing state/edit/database test seams, and isolated PostgreSQL setup. `PREFLIGHT_PASS` for Chunk 8 only. The task and plan are tracked; the supplied prior handoff/instruction are untracked. The Chunk 7→8 handoff had reported the handoff path absent at its earlier inspection; this preflight observed it present but untracked. The reason for that artifact-state change is unknown. This is a nonmaterial documentation-status discrepancy, not task/plan authority drift. No independent review was performed.
- **Authorization provenance:** the current user explicitly authorized Task 4B Chunk 8 only in SINGLE-CHUNK mode and the Chunk 8→9 evidence handoff plus prepared Chunk 9 instruction. This does not authorize Chunk 9.

## 2. Execution mode, environment, and retry policy

- **Mode/scope:** SINGLE-CHUNK; Chunk 8 only. No Chunk 9, 10, or 11 implementation or verification was started.
- **Disposable PostgreSQL:** uniquely named Compose project `labreserve-task004b-c8-18c91ce4`; its `verify-db` was observed healthy as PostgreSQL 17.7 with the project-labelled volume retained. Verification tests used only `labreserve_test`; concurrency decorators assert the actual operation transaction's `current_user` is `labreserve_app`. Admin/migrator use remained limited to migration setup, isolated fixture/trigger management and inspection. `resourceCreationSetup` invokes `Migrate` from tests; no separate migration command was run.
- **Cleanup:** test teardown completed without cleanup errors. A read-only post-run catalog/data query returned `resource_fixtures=0`, `state_test_functions=0`, `state_test_triggers=0`. The disposable Compose DB/volume was intentionally left running/retained; no down-volume/reset/destructive cleanup was done.
- **Failure budget:** two failed validation attempts per issue. Three separate Chunk 8 invocation/validation issues each had one failed attempt and were repaired: (1) compile-only test build caught a missing `t` argument; (2) PostgreSQL fixture insertion rejected an untyped `$4` parameter (`42P18`), fixed with an explicit boolean cast; (3) `gofmt` was not found under a login-shell invocation, corrected by invoking it directly in the verification container. No issue recurred and no failure remains unresolved. Carried forward: one resolved Chunk 6 JSONB snapshot-key assertion and one resolved Chunk 7 artifact check across separate ephemeral containers; neither recurred. Do not reset history if the same issue recurs.
- **Verification boundary:** `make verify`, browser/Playwright, lifecycle/restart, migration-upgrade and delivery-wide checks were **NOT_RUN**, reserved for Chunk 11. Independent review of Chunks 6, 7, and 8 remains outstanding; no acceptance/review completion is claimed.

## 3. Completed Chunk 8 — observed work

Only test paths changed:

1. Extended `resourceStateRaceRepository` / `resourceStateRaceTransaction` in `internal/database/resource_state_booking_concurrency_integration_test.go` with optional post-lock gating, state-capability call counts, and observed occupancy-check count/time. These seams delegate to the actual transaction methods and do not change production behavior.
2. Added `internal/database/resource_state_concurrency_integration_test.go` with fixture-scoped real-PostgreSQL service schedules, backend PID and blocking-edge observations, controlled clocks, persisted Resource/Activity/Booking/history assertions, and scoped failure/no-op triggers.

The new tests force and assert:

- Repeated desired Active from Inactive and repeated desired Inactive from eligible Active: one changed leader and event; blocked waiter returns a true no-op with no guarded state call, clock read, UPDATE, timestamp rewrite, or extra event. A state-aware database trigger rejects any Resource UPDATE that leaves `active` unchanged while allowing an actual transition.
- Both initial states and both lock orders for opposing desired states, including no-op leaders. Actual opposite transitions use equal controlled operation microseconds; intermediate persisted states/events and service outcomes establish causal order. No timestamp or random event-ID ordering is used as proof.
- Actual `resources.Service.Edit` versus `StateService.SetState` in both orders. Gates capture the first committed operation before the second mutation; checks preserve raw legacy text, exact edited text, immutable code/`created_at`, final active state/`updated_at`, and one accurate event per real change.
- Resource UPDATE failure (an `AFTER UPDATE` trigger raises after the attempted state row update) and Activity INSERT failure after the state update, each with a blocked state waiter. Both leaders return safe `ErrOperational`, their transactions roll back, no partial state/event pair remains, and the waiter re-reads committed Active state and commits a fresh-timestamp deactivation.
- A waiter blocked behind activation while a future Confirmed Booking crosses its end boundary: clock/occupancy are untouched while blocked; the test advances the controlled clock to exact `end_at`, then observes one post-lock occupancy check using that exact time and a valid deactivation under the existing `end_at > operation_time` rule.

All schedules retain and compare Resource creation history and complete Booking/request/cancellation Activity snapshots. Standard state/edit fixtures contain past Confirmed and future Cancelled Bookings; the time-boundary fixture also retains a future Confirmed Booking.

## 4. Verification actually executed

All commands were run from repository root with `rtk`, project `labreserve-task004b-c8-18c91ce4`, rebuilt `verify-go`, and the isolated database unless stated otherwise. No selected test reported a skip.

| Command/check | Observed result |
| --- | --- |
| `rtk docker compose --project-name labreserve-task004b-c8-18c91ce4 --file compose.yaml --profile verify build verify-go` | **PASS**, rebuilt from current source. |
| `... run --rm --no-deps verify-go go test -list '^(TestResourceStateConcurrent.*|TestResourceStateAndDescriptiveEditSerializeBothOrders|TestResourceStateFailureRollbacksWakeWaitersForFreshStateAndTime|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestResourceStateOppositeTransitionsAtEqualOperationTime|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database` | **PASS** discovery; found all five new functions, both existing state regressions and all three named edit regressions. |
| Same prefix, `go test -count=1 -v -p 1 -run '^(TestResourceStateConcurrent.*|TestResourceStateAndDescriptiveEditSerializeBothOrders|TestResourceStateFailureRollbacksWakeWaitersForFreshStateAndTime|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestResourceStateOppositeTransitionsAtEqualOperationTime|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database` | **PASS**; all selected tests/subtests executed uncached and serialized. |
| Same prefix, `go test -list '^TestResourceState(Booking|Cancellation)Race' ./internal/database` and `go test -count=1 -v -p 1 -run '^TestResourceState(Booking|Cancellation)Race' ./internal/database` | **PASS**; discovered and reran all five Chunk 6 Booking race functions and all five Chunk 7 cancellation race functions after extending their shared state wrapper. No skips. |
| Same prefix, `gofmt -l internal/database/resource_state_booking_concurrency_integration_test.go internal/database/resource_state_concurrency_integration_test.go` | **PASS**, no output. Host `rtk gofmt -w` was applied only to those changed Go files. |
| Same prefix, `go test -run '^$' ./...` | **PASS**, all repository packages compiled; **no tests selected**. |
| Same prefix, `sh -ec 'go build -o /tmp/labreserve-task4b-chunk8-check ./cmd/labreserve && test -s /tmp/labreserve-task4b-chunk8-check'` | **PASS**, build and artifact check ran inside one ephemeral container. |
| `rtk git diff --check`; `rtk git diff --cached --check`; untracked-file `rtk git diff --no-index --check /dev/null internal/database/resource_state_concurrency_integration_test.go` | **PASS**; no whitespace diagnostics. The no-index command returned 1 because the file is an untracked addition, not because of whitespace errors. |
| Read-only post-run fixture/function/trigger query | **PASS** cleanup evidence: all three counts were zero. |
| `make verify`, browser/Playwright, migration/restart/lifecycle and full Task 4B delivery gate | **NOT_RUN**, outside Chunk 8. |

### Observed PostgreSQL wait edges and serialized outcomes

Every listed edge was observed through PostgreSQL blocking PID/lock data before releasing the relevant gate. Each was `transactionid/ShareLock`; `wait_event_type`/`wait_event` were empty, so no more specific wait-event claim is made.

| Schedule (latest uncached run) | Waiter → blocker | Verified order/result |
| --- | --- | --- |
| Repeated desired Active | 3680 → 3681 | Active leader committed once; waiter no-op, no timestamp/time/capability rewrite. |
| Repeated desired Inactive | 3684 → 3685 | Inactive leader committed once; waiter no-op, no timestamp/time/capability rewrite. |
| Inactive: activate, then deactivate | 3688 → 3689 | Both changed at equal controlled time; intermediate Active/event snapshot, final Inactive and both exact events. |
| Inactive: deactivate no-op, then activate | 3692 → 3693 | No-op leader; waiter changed once; final Active and one transition event. |
| Active: deactivate, then activate | 3697 → 3696 | Both changed at equal controlled time; intermediate Inactive/event snapshot, final Active and both exact events. |
| Active: activate no-op, then deactivate | 3700 → 3701 | No-op leader; waiter changed once; final Inactive and one transition event. |
| Descriptive edit, then state | 3704 → 3705 | Edit pair committed first; state gate observed edited text while still Active; state then deactivated. |
| State, then descriptive edit | 3709 → 3708 | State pair committed first; edit gate observed Inactive with old text; edit then preserved Inactive. |
| Resource UPDATE fails | 3712 → 3713 | Leader rolled back; waiter observed original Active row/history, then deactivated at its own time. |
| Activity append fails | 3716 → 3717 | Uncommitted Resource update/event statement rolled back; waiter observed original Active row/history, then deactivated at its own time. |
| Booking-end time advances while waiter blocks | 3720 → 3721 | No pre-lock time/occupancy call; waiter used exact end time after lock, occupancy check was nonblocking, then deactivated. |

The tests additionally asserted final persisted rows, `updated_at`, actor/action/target/time and before/after Activity details, unchanged original creation event, and byte-equivalent retained Booking/request/cancellation snapshots.

## 5. Diff, preservation, and risk audit

- Final pre-handoff `git status --short --untracked-files=all` showed only the one scoped tracked helper edit, the new test file, and the two pre-existing untracked Chunk 7→8 documents. The two new authorized handoff documents are also untracked. No staged changes, production/service/SQL/migration/route/dependency/README changes, or unrelated file edits were present.
- The tracked helper diff and the full 1,132-line new Go test file were inspected. The helper preserves existing delegates and adds optional test-only gates/counters. New test code uses independent service transactions, fixed time, bounded gates, actual runtime-role assertions, scoped triggers, exact snapshots, and registered cleanup.
- The failed `sh -lc` formatting attempt was corrected by direct Compose invocation; final gofmt/build checks pass. `make verify` and independent review remain outstanding and are not implied by this handoff.
- No production behavior, authorization/privilege boundary, migration, data model, or approved lock/isolation invariant changed. No persistent/demo database was used. The plan's conditional Migration 005 checksum warning remains applicable to any separately authorized persistent-target work.
- **Review status:** independent review of Chunk 6 and Chunk 7 remains outstanding from prior handoff; independent review of Chunk 8 has not occurred. A fresh review session and required acceptance are separate from this implementation evidence.

## 6. Exact next boundary — Chunk 9, not authorized

The controlling plan's next title is **Chunk 9 — Actual COMMIT rejection and ambiguous acknowledgement**. The proposed scope is a new `internal/database/resource_state_commit_integration_test.go`, state-service tests in `internal/resources/state_test.go`, and only a minimal test-helper extension in `internal/database/resource_commit_rejection_integration_test.go` if needed.

The next authorized executor must separately prove, in both transition directions, that a guarded state mutation succeeds but a real deferred PostgreSQL COMMIT rejection yields safe `ErrOperational` and no persisted Resource/event pair; separately model a successful durable commit with lost acknowledgement and a noncommitted failure path, with exactly one service attempt/Commit seam call and no blind retry or false rollback claim. Add the planned waiting-operation/re-evaluation evidence and safe error/cleanup checks. Use the exact Chunk 9 plan and Task 4B V6 contract; do not substitute an outer rollback or synthetic post-hoc error for actual rejected COMMIT evidence.

**Chunk 9 authorization is not granted by this handoff or its paired instruction.** The plan lists Chunk 8 acceptance as a dependency. A future executor must verify separate Chunk 9 authorization, current Chunk 8 acceptance/review status, and all source/candidate/environment gates before edits. Do not start Chunk 10/11, run `make verify`, or touch persistent data as part of Chunk 9.

## Suggested skills

- `chunked-implementation` — SINGLE-CHUNK only after separate Chunk 9 authorization and current preflight.
- `specification-readiness-authority` — if task/plan/candidate/approval drift requires assessment.
- `pre-read-discipline`, `pre-edit-discipline`, `post-edit-discipline`, `rtk-command-prefix` — for any later authorized implementation.
- `handoff` — only for a separately authorized later transfer.

## 7. Current-candidate verification addendum

This addendum records a later independent verification pass against the repository candidate current at that session. It supplements, and does not replace, the historical execution evidence above.

### Candidate and source revalidation

- **Current candidate:** branch `feat/004b-resource-activation-deactivation`, HEAD `b6c1fcc7a55fdc7b3542fc0ce9068a864eeddb5c`, equal to `origin/feat/004b-resource-activation-deactivation`; tree was clean before this addendum. The user-provided expected candidate `18c91ce4d5ca798fe69aac6e0d53b16d899c759e` is an ancestor, not current HEAD.
- **Investigated drift:** `2a884d50fd973f5ab20bbb5d4c21806e890e4995` adds the Chunk 8 concurrency test/helper work, Chunk 8→9 handoff, and prepared Chunk 8 instruction; `b6c1fcc7a55fdc7b3542fc0ce9068a864eeddb5c` adds the Chunk 7→8 handoff and prepared Chunk 9 instruction. The complete `18c91ce…..b6c1fcc…` delta is six paths only: two test files and four handoff/instruction documents. No production service, migration, schema, privilege, route, dependency, or application configuration changed. The two commits and their file contents were inspected; the Chunk 8 implementation was already present and committed, so this verification session made no implementation-code edits.
- **Fingerprints rechecked:** Task SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`; controlling plan SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`; Migration 006 SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`. Current test-file SHA-256s remain `237e0ecd01348b5cb1de1d6ad2591c59239d1fe38330d1a9dcdc5db694e284b0` (shared race helper) and `1a3a9ba6b0896ba8da823a0825f7331a5b8de5bd5227092867b1802e8a1ff992` (Chunk 8 test).
- The approved task, sole plan, complete Chunk 7→8 handoff, prepared Chunk 8 instruction, and relevant PRD/baseline/architecture and accepted Resource-edit/cancellation contracts were re-read. The tracked task/plan remain authoritative and unchanged. Fresh current-candidate preflight passed; prior Chunk 7/8 preflight claims were not reused as current evidence.

### Environment and verification

- A new isolated Compose project `labreserve-task004b-c8-check-b6c1fcc` was created for this verification; `verify-db` was healthy, PostgreSQL `17.0.7`, database `labreserve_test`. The pre-existing active `labreserve-task004b-c8-18c91ce4` project and its volume were left untouched. The new project's test volume is intentionally retained; no destructive cleanup was run.
- `testPool` refuses any database other than `labreserve_test`; `resourceCreationSetup` runs migrations only against that test URL. Every race service transaction passed `requireRuntimeTransaction`'s `current_user = 'labreserve_app'` assertion. Fixture/admin access was limited to setup, inspection, scoped triggers, and cleanup.
- After tests, a read-only query reported `labreserve_test|postgres|0|0|0`: zero `State-Concurrent-*` Resource fixtures, zero matching state test triggers, and zero matching state test functions. Test commands used `--rm`; no fixture cleanup error was reported.
- **PASS, discovery:** `go test -list` discovered all ten selected main database test functions (five Chunk 8 functions, two existing state regressions, and three Resource-edit regressions), all five Chunk 6 booking race functions, all five Chunk 7 cancellation race functions, and eleven `internal/resources` state-service test functions. No selected behavior test skipped.
- **PASS, behavior:** the uncached serialized database selector passed all new Chunk 8 schedules, the existing equal-clock/no-op regressions, and all three named Resource-edit regressions. The uncached serialized `^TestResourceState(Booking|Cancellation)Race` rerun passed all ten Chunk 6/7 functions after the shared race-helper change. The `internal/resources` `^TestResourceState` unit suite passed.
- **PASS:** changed-file `gofmt -l` produced no output; `go test -run '^$' ./...` compiled every Go package and selected no tests; app build plus artifact check succeeded inside one ephemeral container; `git diff --check` passed for the candidate delta.
- `make verify`, Playwright, migration/restart/lifecycle, and delivery-wide checks were **NOT_RUN**, as required for this chunk. Independent review of Chunks 6–8 remains outstanding.

The exact executed test/build commands used project `labreserve-task004b-c8-check-b6c1fcc` and repository-root paths:

```sh
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify build verify-go
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify up --wait -d verify-db
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResourceStateConcurrent.*|TestResourceStateAndDescriptiveEditSerializeBothOrders|TestResourceStateFailureRollbacksWakeWaitersForFreshStateAndTime|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestResourceStateOppositeTransitionsAtEqualOperationTime|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState(Booking|Cancellation)Race' ./internal/database
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState' ./internal/resources
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateConcurrent.*|TestResourceStateAndDescriptiveEditSerializeBothOrders|TestResourceStateFailureRollbacksWakeWaitersForFreshStateAndTime|TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestResourceStateOppositeTransitionsAtEqualOperationTime|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceState(Booking|Cancellation)Race' ./internal/database
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go gofmt -l internal/database/resource_state_booking_concurrency_integration_test.go internal/database/resource_state_concurrency_integration_test.go
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -run '^$' ./...
rtk docker compose --project-name labreserve-task004b-c8-check-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go sh -ec 'go build -o /tmp/labreserve-task4b-chunk8-check ./cmd/labreserve && test -s /tmp/labreserve-task4b-chunk8-check'
rtk git diff --check 18c91ce4d5ca798fe69aac6e0d53b16d899c759e..HEAD
```

### Forced schedules and observed persisted results

All actual service operations below used independent transactions through `labreserve_app`. The tests observed each waiter→leader edge with PostgreSQL `pg_blocking_pids` and an ungranted `pg_locks` row before releasing a gate. Every edge was `transactionid/ShareLock`; `wait_event_type` and `wait_event` were empty. `T0 = 2040-04-05T06:07:08.123456Z`; `T1 = T0 + 1 minute`; the fixture `created_at = 2040-04-04T06:07:08.123456Z`. Each fixture retained raw name `"\u2003Legacy concurrent lab\t"`, description `"  Raw\tlegacy  description\n"`, immutable code/creation time, one past Confirmed Booking, one future Cancelled Booking, and their request/history snapshots.

| Forced order | PostgreSQL waiter → blocker | Gate release and actual outcomes | Persisted state/time and Activity evidence |
|---|---|---|---|
| Repeated Active, initial Inactive | 290 → 291 | Leader Active transition was held after mutation/event; waiter blocked on Resource. Release, leader committed, waiter resumed as authorized no-op. | Final Active, `updated_at=T0`; one `resource.activated` (`false→true`) transition event; no waiter UPDATE, timestamp rewrite, clock/capability call, or event. |
| Repeated Inactive, initial Active | 294 → 295 | Leader Inactive transition held; waiter blocked, then leader committed before waiter resumed as no-op. | Final Inactive, `updated_at=T0`; one `resource.deactivated` (`true→false`) event; no waiter UPDATE/time/event attempt. |
| Inactive: Activate → Deactivate | 298 → 299 | Leader activation held after state/event; waiter blocked. Release and commit activation; intermediate Active/event snapshot taken before waiter gate opened; waiter then changed state. | Both changed at identical `T0`; final Inactive, `updated_at=T0`; events `false→true`, then `true→false`, exactly two state events. |
| Inactive: Deactivate no-op → Activate | 303 → 302 | No-op leader gated immediately after Resource lock; waiter edge observed. Release; leader committed unchanged before waiter reached its guarded mutation gate. | Leader unchanged/no event; waiter activation changed state. Final Active, `updated_at=T0`; one `resource.activated` event. |
| Active: Deactivate → Activate | 306 → 307 | Leader deactivation held; waiter blocked. Release/commit deactivation; intermediate Inactive/event snapshot observed before waiter transition. | Both changed at identical `T0`; final Active, `updated_at=T0`; events `true→false`, then `false→true`, exactly two state events. |
| Active: Activate no-op → Deactivate | 317 → 318 | No-op leader gated after Resource lock; waiter blocked. Release; leader committed no-op before waiter transition. | Leader unchanged/no event; waiter deactivation changed state. Final Inactive, `updated_at=T0`; one `resource.deactivated` event. |
| Descriptive edit → state | 321 → 322 | Edit leader held after edit+event; state waiter blocked. Release and commit edit; waiter gate observed updated text while still Active; release waiter to deactivate. | Final Inactive, `updated_at=T1`; edited name `Edited concurrent lab`, description `Edited  descriptive text`; one descriptive event with exact raw-before/canonical-after values, then one `resource.deactivated` (`true→false`) at `T1`. |
| State → descriptive edit | 326 → 325 | State leader held after deactivation+event; edit waiter blocked. Release and commit state; edit gate observed Inactive with original text; release edit. | Final Inactive, `updated_at=T1`; same exact edited values; one deactivation event at `T0`, then one descriptive event with exact before/after values at `T1`. |
| Resource mutation failure → waiting deactivation | 330 → 331 | Leader held after Resource lock/eligibility; waiter edge observed. Release leader into scoped `AFTER UPDATE` injected failure; leader returned safe `ErrOperational` and rolled back before waiter proceeded. | Waiter re-read original Active/creation timestamp and committed deactivation at its own `T1`; final Inactive, `updated_at=T1`; only waiter's one deactivation event persisted; no partial leader event/state. |
| Activity failure → waiting deactivation | 334 → 335 | Leader held before guarded mutation; waiter edge observed. Release leader; scoped Activity INSERT failure followed uncommitted state change; leader returned safe `ErrOperational` and rollback completed before waiter proceeded. | Same fresh waiter result: final Inactive, `updated_at=T1`, only waiter's deactivation event; leader's Resource/event pair absent. |
| Activation → time-sensitive deactivation waiter | 338 → 339 | Deactivation waiter blocked behind activation leader. While blocked, controlled time advanced from `end_at−1µs` to exact `end_at = 2040-04-05T09:07:08.123456Z`; leader released and committed before waiter performed its sole occupancy query. | Leader activation at `T0`, waiter deactivation allowed at exact end under `end_at > operation_time`; final Inactive, `updated_at=end_at`, two exact state events. Future Confirmed Booking/request/history remained unchanged. |

The state-aware UPDATE detector allowed actual active-bit transitions but raised on every fixture Resource UPDATE with `OLD.active = NEW.active`, including timestamp-only rewrites. Assertions verified exact state/event outcomes and counts, original `resource.created` event unchanged, and complete Booking/request/cancellation/history snapshots unchanged after each schedule. State events were attributed to the fixture Coordinator, targeted the Resource with no Booking target, and carried the tested before/after active values; descriptive events carried the exact text snapshots. Including the retained creation event, schedules with one mutation ended with two Resource events, and schedules with two mutations ended with three; no-op leaders added none. For equal-time opposite transitions, captured intermediate persisted snapshots and service outcomes—not timestamps or event IDs—established order.

### Regression, failure-budget, and continuation state

- All selectors above passed; no selected test was skipped. No validation failed in this current-candidate verification session. The previously recorded Chunk 8 issues (one missing helper argument, one PostgreSQL parameter-type inference issue, one `gofmt` invocation issue), plus the earlier Chunk 6/7 corrected invocations, remain historical and are not reset; none recurred.
- Independent review remains outstanding. Chunk 9 COMMIT/acknowledgement work, Chunk 10 lifecycle work, and Chunk 11 `make verify` remain unstarted/unverified and require their own authorization/gates. The prepared Chunk 9 instruction remains `PREPARED_NOT_AUTHORIZED`.
- The named handoff and instruction already existed at this candidate; this session preserved their prior contents and appended this evidence rather than replacing them. The actual session's final repository state includes these documentation edits only; no implementation code was changed.

**Chunk 8 status: `CONTINUABLE`.**

## 8. Later authorization and candidate commit identity

This handoff's verification addendum records the earlier state in which Chunk 9 had not yet been authorized; that statement was accurate when written. The user later accepted Chunk 8 for Chunk 9's entry dependency and authorized Chunk 9, then accepted Chunk 9 for Chunk 10's entry dependency and authorized Chunk 10. The current Chunk 10→11 handoff contains the latest verification and authority state.

The Chunk 9 source/test commit is `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb`; the Chunk 10 implementation/test candidate is `b59c8acd2bb1813513d8dbcce1430b5052b7739a` (tree `54a25d599009ef671a502b0f3549547143efaaef`). Documentation/handoffs are committed separately afterward.
