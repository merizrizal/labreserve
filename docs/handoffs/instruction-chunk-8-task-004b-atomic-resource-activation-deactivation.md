# LabReserve Task 4B — Prepared Instruction for Chunk 8

**Instruction state: `PREPARED_NOT_AUTHORIZED`.**

**Prepared from:** [`task-004b-chunk-7-to-8.md`](task-004b-chunk-7-to-8.md).

**Target:** A fresh implementation session/subagent if supported and separately authorized. Do not claim session isolation unless the harness provides it.

## Authorization and execution mode

- Use the `chunked-implementation` skill in **SINGLE-CHUNK** mode only after verifying a separate, current, explicit user authorization for **Task 4B Chunk 8** (or a verified coordinator's already-authorized continuous-loop instruction; no such loop authorization is recorded here).
- The user's authorization to create this evidence handoff and prepared instruction does **not** authorize Chunk 8 implementation. Chunk 7 authorization, `CONTINUABLE`, a commit, or this document also does not authorize it.
- Do not edit code until authorization, current source freshness, candidate integrity, and the applicable implementation preflight are established. Report `PREFLIGHT_PASS` only if actually performed and all required gates pass.
- Execute exactly Chunk 8, then stop before Chunk 9. No continuous-loop dispatch. Do not block Chunk 8 solely on independent review of non-final Chunk 7 or Chunk 6; report their review status accurately and preserve the eventual review obligation.

## 1. Governing sources and recorded candidate

- **Approved task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`, current recorded SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, current recorded SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`; exact Chunk 8 section is §9, “Concurrent desired states, descriptive edits and rollback waiters.”
- **Latest evidence handoff:** `docs/handoffs/task-004b-chunk-7-to-8.md` (read through its ending).
- **Latest prior implementation instruction:** `docs/handoffs/instruction-chunk-7-task-004b-atomic-resource-activation-deactivation.md`.
- **Relevant earlier handoffs:** `docs/handoffs/task-004b-chunk-6-to-7.md` and `docs/handoffs/task-004b-chunk-5-to-6.md`; consult earlier Task 4B handoffs for any retained correction or risk referenced by those documents.
- **Recorded migration 006:** `internal/database/migrations/006_resource_state.sql`, SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`; Chunk 8 must not change migrations.
- **Recorded current branch/HEAD:** `feat/004b-resource-activation-deactivation` at `18c91ce4d5ca798fe69aac6e0d53b16d899c759e`, equal to upstream; parent/Chunk 7 base `fbb291ecaed352895475d5204a02c0cf2a11fd24`. The worktree was clean before these two new documents. Recheck all values; historical values are not current authority.
- **Code identities at handoff:** race helper `internal/database/resource_state_booking_concurrency_integration_test.go`, SHA-256 `e7d671f6d587e3f6a72b93b9f66b0005573dd086e01eb7c3e1f7a46f2d68082d`; Chunk 7 test `internal/database/resource_state_cancellation_concurrency_integration_test.go`, SHA-256 `96e5640d1c5036ef422ca803c664560de8116cba26768131532bcae282ac431f`. The latest observed candidate differed from a previously reported commit by only two already-read handoff documents; see the paired evidence handoff. Recompute hashes and review the actual diff before relying on this classification.
- **Preflight history:** Chunk 7 execution reported a fresh `PREFLIGHT_PASS` against base `fbb291e…`. It is not a Chunk 8 preflight. The handoff session rechecked source identities and found no requirements/plan drift; a fresh worker must revalidate.

## 2. Mandatory current-state/source gate — before editing

1. Read repository `AGENTS.md` and applicable session instructions, the complete Task 4B contract, the complete controlling plan through its ending, and relevant architecture/accepted cancellation and Resource-edit contracts. Confirm the task/plan are tracked and committed and still authoritative.
2. Verify separate explicit Chunk 8 authorization and SINGLE-CHUNK mode. If absent, stop without code edits; this prepared instruction cannot supply it.
3. Inspect current branch, HEAD, upstream, ancestry, full staged/unstaged/untracked state, and commit contents. Recompute full task/plan/migration fingerprints and inspect all changes since the recorded base, including the complete contents of untracked files.
4. Read the paired Chunk 7→8 evidence handoff and Chunk 7 instruction completely. Read earlier handoffs where they contain a correction, retry-budget history, environment ownership, or retained risk. Compare their identities to the current tree; distinguish earlier reported results from fresh inspection.
5. Reconfirm the exact Chunk 8 plan scope and boundaries: test file plus bounded test-helper work only. The user-provided Chunk 7 handoff and instruction were tracked at recorded HEAD; preserve them. Do not restore/overwrite absent artifacts without checking their current state.
6. Inspect the current state service/adapter and helper implementations, state/edit integration tests, update/activity failure helpers, migration/version files and test-pool setup. Verify that names and expected selectors below still match current code. The recorded candidate has an existing post-occupancy barrier from Chunk 7 and prior equal-time serial-transition coverage; do not assume either is an adequate gate for a new schedule without inspecting it.
7. Revalidate the Compose project/containers/volumes and ownership before DB work. The recorded Chunk 7 `labreserve-task004b-c7-fbb291ec` project has a healthy `verify-db` and retained `verify-data` volume; a separate Chunk 5 project was also active. Treat these as current-session observations only. Do not stop, reset, delete, or reuse retained data until exact project and `labreserve_test` ownership are confirmed. Prefer a fresh isolated project if appropriate and safe.
8. Confirm actual runtime operations use `labreserve_app`, and the test URL guard still refuses databases other than `labreserve_test`. Do not read/log `.env` contents or touch persistent/demo databases.
9. Classify source/candidate drift as nonmaterial confirmed, material requiring affected-gate revalidation, or unverified. Stop if authorization, task/plan authority, scope, database isolation, or required evidence cannot be established. Do not rely on prior test results alone.

## 3. Exact Chunk 8 objective and scope

**Plan title:** Chunk 8 — **Concurrent desired states, descriptive edits and rollback waiters**.

**Plan traceability:** remaining V5; V2/V6/V7; PRD FR-023–027; AC-028/043–044 internal facets; preserve accepted Task 4A text/no-op behavior.

**Objective:** Add deterministic real-PostgreSQL tests proving desired-state serialization, opposite transitions in each initial state and lock order, serialization with descriptive editing, and correct waiter behavior after a state mutation or required Activity failure. Preserve service, database, lock, isolation, privilege, event, and text semantics.

**Plan-proposed paths:**

- New `internal/database/resource_state_concurrency_integration_test.go`.
- Only bounded test-helper extensions/refactoring if required; the plan proposes `internal/database/resource_state_test_helpers_test.go`, but inspect the current layout first. Do not move or generalize helpers without a concrete Chunk 8 need.
- A narrowly scoped extension to an existing **test-only** update-failure helper is allowed only if needed for the required schedules.

No production service/SQL/migration/schema/privilege/route/dependency/README change is authorized in Chunk 8. If a reproduced defect requires one, stop and request the appropriate new authority instead of expanding this test-only task.

## 4. Actual code and test seams to revalidate

Observed in the recorded candidate; confirm still true:

- `internal/resources/state.go`: `StateService.SetState` resolves stored Coordinator authority, locks/reads Resource, checks desired-state equality, obtains time for actual transition, performs a separate occupancy query for deactivation, calls the guarded state capability, then commits. No-op returns without `SetResourceState` or a timestamp read.
- `internal/resources/service.go`: `Service.Edit(ctx, Actor, EditRequest)` uses the existing Resource transaction/lock and canonical descriptive no-op behavior. Preserve code immutability and accepted text rules.
- `internal/database/resource_state.go`: state and edit operations serialize through the shared Resource `FOR UPDATE` protocol; runtime service calls must continue through actual `labreserve_app`.
- `internal/database/resource_state_booking_concurrency_integration_test.go`: same-package `bookingRaceGroup`, `bookingRaceGate`, `resourceStateRaceRepository`/transaction, `requireRuntimeTransaction`, `waitForBookingRaceLockWait`, `logBookingRaceWait`, `resourceStateRaceExpectation` and persisted-snapshot helpers are available. Existing state wrapper fields include `started`, `resourceLocked`, `beforeSet`, `afterOccupied`, and `beforeCommit`.
- `internal/database/resource_concurrency_integration_test.go`: existing edit-race helpers and tests include `startResourceEdit`, `receiveResourceEdit`, `TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime`, `TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp`, and `TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait`.
- `internal/database/resource_mutations_integration_test.go`: existing `installResourceUpdateFailureTrigger` rejects every Resource update for one fixture; `installResourceActivityFailureTrigger` permits only known Resource actions and injects a narrowly scoped Activity failure.
- `internal/database/resource_concurrency_integration_test.go`: `installResourceNoOpUpdateFailureTrigger` is edit-specific: it raises when `name` and `description` remain equal. A valid state-only update also leaves those columns equal, so this helper would incorrectly fail a real activation/deactivation if reused unchanged.
- `internal/database/resource_state_integration_test.go`: `TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence`, occupancy and post-lock time cases, plus `TestResourceStateOppositeTransitionsAtEqualOperationTime` already cover serial equal-clock opposite changes. The latter is not concurrency evidence for Chunk 8.
- `resourceCreationSetup` calls `Migrate` on its test database; `testPool` skips if the requested URL is absent and refuses any database other than `labreserve_test`. Selected integration tests must not silently skip. The state-race decorators verify `current_user = 'labreserve_app'`; use admin access only for isolated fixtures, triggers, snapshots and PostgreSQL wait observation.

### Gate and detector cautions

- `resourceStateRaceTransaction.Commit` currently validates a changed transition when `beforeCommit` is configured and errors if the operation was a no-op (no recorded expected transition). A no-op as the leading lock holder in an opposite-state schedule cannot be forced with that gate unchanged. Add a narrow test-only gate after the actual Resource lock or otherwise safely before no-op transaction completion, and assert that no mutation/event was attempted.
- A `resourceLocked` notification proves the lock was acquired but does not pause the operation; it does not by itself force the competing operation's order. Use an actual bounded gate and observe the wait edge before release.
- Do not reuse `installResourceNoOpUpdateFailureTrigger` unchanged: it would also reject state transitions that preserve name/description. Build a fixture-scoped state-aware detector that permits an actual `active` change but rejects every UPDATE whose `OLD.active = NEW.active` during a state operation; this catches redundant no-op UPDATEs even if they rewrite only `updated_at`. An equally strong DB-level observation is acceptable. Timestamp equality alone is insufficient.
- `installResourceUpdateFailureTrigger` rejects the leader and any waiter update for that Resource. For rollback-waiter tests, design/scoped triggers so a legitimate waiter transition is not also sabotaged; alternatively make and verify a valid no-op waiter outcome after rollback. Preserve complete fixture cleanup.
- Existing edit-race tests use independent service calls and external Resource-lock holders, but may not provide a gate after the leading edit's mutation and before commit. Add a small decorator around the actual `EditTransaction` if needed; do not mock edit/state logic or claim order from unordered starts.

## 5. Required schedule matrix and invariants

Use real service calls through independent PostgreSQL transactions, controlled clocks, bounded gates, backend PIDs, observed PostgreSQL wait edges, and persisted snapshots. Force every leader/waiter schedule explicitly; arbitrary sleeps, missing timeouts, or simultaneous goroutine starts alone do not prove order.

### A. Repeated same desired state in both directions

1. Start Inactive. Gate the first actual activation after lock acquisition/at a safe pre-mutation or pre-commit boundary; start a second desired-Active request and observe it wait on the identified Resource holder. Release the leader. Require one actual transition/event and a waiting authorized no-op with no Resource UPDATE, `updated_at` rewrite, or Activity attempt.
2. Start eligible Active with no blocking Booking. Force the corresponding two desired-Inactive operations. Require one deactivation/event and a waiting authorized no-op with no UPDATE/time rewrite/event.
3. Prove the no-op UPDATE detector lets the leader's actual state change pass but would reject a redundant waiter UPDATE. Assert exact clock call counts and before/after Resource/Activity snapshots. A no-op must not become a database call that self-assigns fields.

### B. Opposing desired states, both initial states and both lock orders

Force and assert all four cases, starting from fixtures with no occupancy that would invalidate an actual deactivation:

| Initial state | First lock holder | Waiting request | Expected serialized result |
| --- | --- | --- | --- |
| Inactive | Activate (real transition) | Deactivate | Both changed; activation event then deactivation event; final Inactive. |
| Inactive | Deactivate (no-op) | Activate | First unchanged/no event; second changed/one activation event; final Active. |
| Active | Deactivate (real transition) | Activate | Both changed; deactivation event then activation event; final Active. |
| Active | Activate (no-op) | Deactivate | First unchanged/no event; second changed/one deactivation event; final Inactive. |

- Repeat opposite real transitions with equal controlled operation microseconds so a timestamp comparison cannot suppress a real change. The existing serial equal-time test is useful regression coverage, not a substitute for these forced schedules.
- Capture an intermediate persisted state between real transitions or at the leader gate. Equal timestamps and random Activity IDs do not establish event causality; compare per-operation outcomes and exact before/after event details.
- For the two no-op-leading cases, use a no-op-safe gate; the current `beforeCommit` changed-transition assertion is incompatible without a narrowly scoped test-only refinement.

### C. State versus descriptive edit, both orders

- Force a Resource edit leader and a state waiter; then force the opposite order. Use actual `resources.Service.Edit` and `StateService.SetState`, identified transaction backend PIDs, gates and an observed Resource-lock wait.
- Verify each operation sees authoritative post-lock values, and each preserves the other's permitted fields: final active state, exact name/description, immutable code and `created_at`, expected `updated_at` from the last actual mutation, and one accurate event per actual change with correct before/after snapshots.
- Include raw retained edge whitespace/legacy text where applicable; a state transition must not canonicalize text. Preserve retained Booking/request/cancellation/history snapshots. Keep the operation times and final expectations explicit.
- Do not use an edit no-op if the schedule purports to prove an actual descriptive mutation. Existing edit tests remain regression evidence, not both-order state/edit evidence.

### D. Mutation and Activity failure with a waiting state command

- Separately force a leader's Resource mutation to fail and a leader's required state Activity append to fail while another independent state command waits on the Resource lock.
- Observe each waiter→leader edge before releasing the leader gate. Assert the leader returns safe operational failure and rolls back the complete state/event pair. The waiter then reads committed state again and uses fresh operation time; its legitimate no-op/transition/rejection must match that state, not the leader's uncommitted value.
- Verify Resource fields/timestamp, all state Activity details/counts and retained Booking/history before/after; ensure no partial event/state pair and correct clock counts. Scope test-only triggers to the fixture/action, remove them reliably, open all gates and join every goroutine/connection.
- Preserve the existing Chunk 6/7 creation/cancellation rollback evidence; if shared helpers are changed or moved, rerun their focused race selectors and record outcomes rather than assuming helper compatibility.

## 6. Explicit exclusions and safety invariants

- No Chunk 9 COMMIT-uncertainty tests, Chunk 10 migration/restart integration, or Chunk 11 full verification. No Chunk 4C UI/routes/browser behavior, other tasks, Coordinator cross-owner cancellation, history UI, deletion, rescheduling, or maintenance workflow.
- No production/service/SQL/schema/privilege change, migration, route, dependency, generic concurrency framework, production time/fault hook, or direct SQL substitute for service operations.
- Preserve explicit desired-state semantics, authorization before successful no-op, matching-state no-UPDATE/no-time/no-event behavior, fresh post-lock time for actual changes, separate occupancy query and exact `Confirmed AND end_at > operation_time` predicate for actual deactivation, Resource `FOR UPDATE` serialization, existing Account/replay order, and owner cancellation Resource→Booking order.
- Preserve atomic state/Activity behavior, safe operational errors, correct event actor/action/target/time/details, Resource identity and text, Booking/request/cancellation records, and all prior Activity. Do not introduce timestamp-based no-op logic or order transitions by event UUID/time.
- No persistent/demo DB, deployment, credential, reset, or destructive volume action. Do not stage/commit/push/merge unless separately authorized; do not stop/clean another worker's Compose project.
- If a required schedule exposes a production invariant defect, reproduce and record it. Stop if fixing it would exceed the test-only Chunk 8 scope or alter an approved product/security/lock invariant; request precise additional authority rather than silently broadening scope.

## 7. Verification obligations

Re-discover names after implementation; planned/new names are not current evidence. Run focused tests uncached, verbose and serialized against an isolated real PostgreSQL test database. Test operations use `labreserve_app`; fixture/migration/inspection operations use only the permitted isolated test setup.

For an independently verified `<project>` name and ownership, repository-root command forms are:

```sh
rtk docker compose --project-name <project> --file compose.yaml --profile verify build verify-go
rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceStateConcurrent' ./internal/database
rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateConcurrent' ./internal/database
rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database
rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)$' ./internal/database
```

The `TestResourceStateConcurrent...` names are proposed until added; select names that cover the full matrix, then confirm discovery matches actual execution. If a helper file/refactor changes Chunk 6/7 support, also rediscover and rerun `^TestResourceStateBookingRace` and `^TestResourceStateCancellationRace`, plus any affected existing cancellation/edit regressions. Run exact-start/occupancy or failure selectors only when affected; identify each selected case and any intentional skip.

Also run and record, from the rebuilt verification environment:

- `rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go gofmt -l <changed-go-files>`; format only changed paths and recheck.
- `rtk docker compose --project-name <project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -run '^$' ./...` compile-only (explicitly state no tests selected).
- Build the app and verify its artifact **in the same ephemeral container**, e.g. `rtk docker compose ... run --rm --no-deps verify-go sh -ec 'go build -o /tmp/labreserve-task004b-chunk8-check ./cmd/labreserve && test -s /tmp/labreserve-task004b-chunk8-check'`.
- `rtk git diff --check`, complete changed-file diff, full status including staged/untracked files, and relevant cumulative diff from the current Chunk 8 base. Read full contents of new files. Confirm only test paths changed and no earlier/user work was overwritten.
- Verify discovery, actual executed tests, `labreserve_app` role, each observed wait edge, final persisted snapshots, cleanup, no hidden skips, and failure-attempt counts. Compilation, selector discovery, or log messages alone are not behavioral evidence.
- Do **not** run `make verify`, browser/Playwright, migration/lifecycle/restart or delivery-wide verification in this non-final chunk; these are reserved for Chunk 11.

## 8. Retry budget and repair authority

- Controlling plan §7 sets **two failed validation attempts per issue**. Chunk 6 had one resolved JSONB snapshot key-order test failure; Chunk 7 had one resolved artifact-check invocation failure caused by checking `/tmp` across separate ephemeral containers. Those historical issues have no unresolved failures. Do not reset a count if the same issue recurs in a new session; new, unrelated Chunk 8 issues start with their own budget.
- Diagnose ordinary in-scope test fixture, gate, PostgreSQL wait observation, formatting, compile and verification problems autonomously. Apply the smallest test/helper-only repair and rerun affected checks.
- Stop after two failed attempts for the same issue, on a required PostgreSQL/runtime-role evidence failure, or if completion needs production behavior, migration, scope expansion, or a security/architecture decision. Do not weaken tests or convert an unrun/skipped case into PASS.

## 9. Environment, source, and prior-risk controls

- Reconfirm the actual Compose project labels, healthy DB, project volume, and whether another worker owns it before use. The prior Chunk 7 project and a separate Chunk 5 project were observed active with retained volumes; this instruction grants no authority to stop, delete, reset, or assume them empty. A unique disposable project is acceptable after verifying isolation.
- `resourceCreationSetup` executes `Migrate` against the test URL. Use it only after proving the target is the authorized isolated `labreserve_test`; do not run these tests against demo/persistent data. Preserve all retained volumes unless separately and explicitly authorized to remove the exact project.
- Keep migrator/admin fixture connections separate from app/service connections. Require the actual `labreserve_app` identity for all concurrent service transactions; do not substitute a superuser/migrator for operation evidence.
- Carry forward the Task 4B conditional Migration 005 checksum caution for any later persistent-target work; Chunk 8 itself has no migration scope and must not touch that boundary.
- No `.env` values, credentials, tokens or private connection strings in logs, tests, handoffs, or final report.

## 10. Mandatory post-work and handoff boundary

1. Inspect branch/HEAD/upstream, all staged/unstaged/untracked changes, and the complete current-chunk plus cumulative candidate diff. Re-read full new/modified test/helper files and check no unrelated work was touched.
2. Verify each same-state/opposite-state/edit/failure schedule is deterministically forced; report its actual PIDs, blocker edge/lock, release condition, result and persisted state/event snapshot. Verify gate/trigger/transaction/goroutine cleanup.
3. Run focused discovery and behavior tests, affected prior regressions, formatting, compile/build and diff checks. State every `PASS`, `FAIL`, `SKIP`, `NOT_RUN`, and `UNVERIFIED` honestly. Record validation attempts and repairs.
4. Assess functional, concurrency, data-integrity, authorization, test-isolation and rollback risks; distinguish Chunk 8 delta from cumulative chunks 0–7. No full delivery acceptance or independent review is implied.
5. SINGLE-CHUNK mode stops after Chunk 8. Do not start Chunk 9. Do not create a Chunk 8→9 handoff unless separately authorized after Chunk 8; this prepared instruction authorizes no next transfer.
6. Do not stage/commit/push/merge/deploy or perform destructive cleanup absent separate explicit authority. Return all prior and user changes intact.

## 11. Final report and hard stop

Report actual task/plan fingerprints, implementation base and current candidate/status, authorization/mode, changed paths/symbols, full forced-schedule outcomes, exact commands and discovery/results/skips, observed PostgreSQL wait evidence, retry counts/repairs, environment/cleanup, complete diff scope, risks, and work explicitly left for Chunks 9–11. State that Chunk 8→9 remains unauthorized unless new explicit authorization is separately supplied.

Finish with exactly one non-final executor status: `CONTINUABLE`, `BLOCKED`, or `INCOMPLETE`. Stop after Chunk 8. This document remains `PREPARED_NOT_AUTHORIZED` until the current Chunk 8 authorization and preflight gates are satisfied.

## Suggested skills

- `chunked-implementation` — SINGLE-CHUNK mode only after separate authorization.
- `specification-readiness-authority` — if authority/source/candidate drift needs assessment.
- `pre-read-discipline`, `pre-edit-discipline`, `post-edit-discipline`, and `rtk-command-prefix` — for bounded test work and verification.
- `handoff` — only if a later transfer is separately authorized.
