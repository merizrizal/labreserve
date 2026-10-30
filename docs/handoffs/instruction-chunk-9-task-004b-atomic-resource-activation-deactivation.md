# LabReserve Task 4B — Prepared Instruction for Chunk 9

- **Instruction state:** `PREPARED_NOT_AUTHORIZED`
- **Prepared from:** [`task-004b-chunk-8-to-9.md`](task-004b-chunk-8-to-9.md)
- **Target:** A fresh authorized implementation session, if the harness supports it. Do not claim session isolation unless it exists.

## Authorization and execution mode

- Use `chunked-implementation` in **SINGLE-CHUNK** mode only after checking explicit current authorization for Task 4B Chunk 9 and the controlling plan's Chunk 8 acceptance dependency.
- **Do not edit implementation files before verifying separate Chunk 9 authorization and completing current source/candidate preflight.** The authorization for Chunk 8 and the request to prepare this document do not authorize Chunk 9.
- The prior handoff's `CONTINUABLE` status records Chunk 8 execution evidence; it is not independent review or acceptance. The current record says independent review of Chunks 6, 7, and 8 is outstanding. Resolve the plan's `Chunk 8 accepted` entry requirement through the repository's separate review/acceptance workflow before proceeding. Do not self-certify it.
- If authorized and all gates pass, execute exactly Chunk 9 and stop before Chunk 10. Do not enter continuous-loop mode or dispatch another worker.

## 1. Governing task and candidate identity

- **Approved contract:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`; last observed SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`. Recompute from current file bytes.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`; last observed SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`. Recompute from current bytes and read its complete Chunk 9 section and ending.
- **Latest handoff:** `docs/handoffs/task-004b-chunk-8-to-9.md`. Also inspect the Chunk 7→8 handoff/instruction when needed for carried retry and provenance corrections.
- **Recorded implementation base:** `18c91ce4d5ca798fe69aac6e0d53b16d899c759e`, branch `feat/004b-resource-activation-deactivation`, observed equal to upstream at Chunk 8 handoff. These are historical observations, not proof of the current checkout.
- **Last candidate code identities:** `internal/database/resource_state_booking_concurrency_integration_test.go` SHA-256 `237e0ecd01348b5cb1de1d6ad2591c59239d1fe38330d1a9dcdc5db694e284b0`; `internal/database/resource_state_concurrency_integration_test.go` SHA-256 `1a3a9ba6b0896ba8da823a0825f7331a5b8de5bd5227092867b1802e8a1ff992`.
- **Last recorded supporting migration:** `internal/database/migrations/006_resource_state.sql` SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.
- At Chunk 8 handoff, its code changes were uncommitted (one modified tracked test helper and one untracked new integration test). Two supplied Chunk 7→8 handoff/instruction documents were also untracked and must be preserved. The authorized Chunk 8→9 handoff pair is untracked as well. Inspect the actual full staged/unstaged/untracked candidate; do not assume these identities remain current.
- Prior execution reported a `PREFLIGHT_PASS` for Chunk 8 only. Perform a **fresh** authority/source/candidate/environment preflight for Chunk 9; do not reuse the earlier verdict as current approval.

## 2. Mandatory state and source-freshness gate — before edits

1. Re-read repository `AGENTS.md`, the complete Task 4B contract, the controlling plan, and especially Chunk 9, Task V6, the final verification contract and their endings.
2. Inspect current branch, HEAD, upstream, parent/base, complete staged/unstaged/untracked status, and relevant diff. Protect all prior and unrelated work. Do not stage, commit, push, merge, deploy, reset, or perform destructive cleanup.
3. Recompute the full current Task 4B and plan fingerprints; verify the relevant Migration 006 and immutable Migration 001–005 identities if applicable. Compare with this historical handoff and explain all drift before proceeding.
4. Read the Chunk 8→9 evidence handoff fully. Confirm current Chunk 8 files and required test evidence still exist; obtain and verify the separate independent review/acceptance required by the plan. A handoff status is not acceptance.
5. Inspect `internal/database/resource_commit_rejection_integration_test.go`, `internal/database/resource_state.go`, `internal/resources/state.go`, `internal/resources/state_test.go`, relevant existing commit tests, scoped failure helpers and current state race helpers. Reconfirm exact interfaces and transaction behavior; do not code from proposed names alone.
6. Reconfirm explicit Chunk 9 authorization, SINGLE-CHUNK mode, available retry budget, correct runtime/admin database URLs and ownership of the disposable verification project. Ensure tests target only isolated `labreserve_test`; `resourceCreationSetup` invokes `Migrate`.
7. Classify source/candidate drift as `NONMATERIAL_CONFIRMED`, `MATERIAL_REVALIDATION_REQUIRED`, or `UNVERIFIED_DRIFT`. Stop if authority, acceptance dependency, test role, database isolation, required evidence, or scope cannot be established.

## 3. Exact Chunk 9 objective and scope

**Controlling-plan title:** Chunk 9 — **Actual COMMIT rejection and ambiguous acknowledgement** (§9).

**Traceability:** remaining V6, failure re-evaluation V5/V7; Task 4B FR-026–027, AC-012/044 state-atomicity facets.

**Objective:** Add deterministic test evidence that distinguishes actual PostgreSQL COMMIT rejection after a successful guarded state statement from an ambiguous/lost acknowledgement. Preserve safe `ErrOperational` behavior, atomic state/event persistence, no blind retry, and correct subsequent explicit desired-state behavior.

**Plan-proposed paths and bounded scope:**

- New `internal/database/resource_state_commit_integration_test.go`.
- Add/update applicable unit cases in `internal/resources/state_test.go`.
- Minimally extend the test-only `installResourceCommitRejectionTrigger` pattern in `internal/database/resource_commit_rejection_integration_test.go` only if the existing action/Resource scope cannot serve Chunk 9.
- Keep existing Resource creation/edit COMMIT rejection tests intact. No production SQL, migration, schema, privilege, route, dependency, README, or generic fault-infrastructure change is authorized. If a reproduced defect needs scope/security/architecture changes, stop and request authority.

### Required behavior and evidence

1. **Actual deferred COMMIT rejection, both directions:** fixture-scope a PostgreSQL `DEFERRABLE INITIALLY DEFERRED` rejection to actor, Resource and expected activation/deactivation action. Observe the real guarded statement return a changed Resource successfully, then observe the transaction's actual PostgreSQL COMMIT fail with the fixture's expected code/message. Verify service error is `resources.ErrOperational` and leaks no trigger/SQL diagnostic. Fresh independent connections must see the original Resource state/`updated_at`, no transition Activity event, and unchanged code/name/description/`created_at`, Booking/request/cancellation rows and prior history.
2. **Waiting operation around rejected COMMIT:** force another state or Booking operation to wait on the Resource lock before releasing the rejected leader. Observe waiter/blocker PIDs and the PostgreSQL blocking edge before release. Show lock release, committed-state re-evaluation and fresh-time behavior; never use ambiguous acknowledgement as proof of rollback.
3. **Lost acknowledgement after durable commit:** use a narrow test-only transaction seam that delegates the actual successful COMMIT, then returns a simulated acknowledgement error. Verify one service attempt and one seam Commit call, safe operational/unknown result, no automatic retry or false rollback assertion, and durable matching Resource/event pair on fresh connections.
4. **Noncommitted acknowledgement failure:** separately make the test seam roll back/noncommit before returning an acknowledgement-like error. Verify one attempt/Commit call, safe operational/unknown result, neither Resource mutation nor event persisted, and no false claim that a successful commit occurred.
5. **Later explicit desired-state operation:** after either uncertainty case, submit a new explicit desired-state request. It must be a true authorized no-op if the first mutation committed, or a newly eligible transition if it did not. Do not create or imply Booking request identity or an automatic exactly-once/retry protocol.
6. **Safe failures and cleanup:** cover the applicable begin/role/lock/query/mutation/context-cancellation/finalization paths specified by Chunk 9. Errors must not leak SQL/trigger/token/credential details or fabricate domain conflicts/success; rollback and locks are bounded and released. No-op finalization failure remains operational.

Use independent real service transactions through `labreserve_app`; admin/migrator access is restricted to isolated setup, deferred trigger installation, observation and cleanup. Record the actual backend PIDs, event/wait observations, transaction statement result, actual Commit call/error, and fresh persisted snapshots. An outer transaction rollback, immediate trigger failure, or synthetic wrapper error by itself is **not** evidence of an actual rejected COMMIT.

## 4. Required invariants and preserved Chunk 8 evidence

- No automatic retry, fabricated Booking/request identity, toggle semantics, last-request-wins guarantee, or false assertion of durable rollback after an ambiguous acknowledgement.
- State and Activity remain both-or-neither for known committed/rejected outcomes; state event action/actor/Resource/time/before-after details remain exact; operation timestamps and safe operational errors remain unchanged.
- Preserve current authorization, runtime-role, Resource-lock and lock-order invariants, 4A canonical text/edit behavior, Booking/cancellation/replay identity/history, and all Chunk 6–8 test evidence. Never weaken tests or delete history to normalize a result.
- Keep fault triggers/functions fixture-scoped, removed on every exit, and isolated from other test schedules. No persistent/demo database or credentials.
- Migration 005 compatibility remains conditional. No persistent target inspection/upgrade is authorized by this prepared instruction; preserve the Task 4B checksum warning and do not reset, rewrite, or down-migrate a ledger.

## 5. Explicit exclusions

- No Chunk 10 retention/migration/restart/lifecycle work or Chunk 11 full verification/documentation/review work; no Task 4C routes/UI/browser work or unrelated task.
- No production COMMIT protocol, network fault service, automatic retries, new request identity, generic elevated helper, production fault/time hook, migration, schema/privilege change, dependency, or deployment.
- Do not run `make verify`, browser/Playwright, migration/restart/lifecycle procedures in Chunk 9; the controlling plan reserves those for Chunk 11.
- Do not modify authoritative requirements/architecture to make tests pass. Do not stage/commit/push/merge/deploy or destructively clean any project/volume.

## 6. Verification obligations

After verifying exact discovery names, run focused uncached serialized tests in the isolated Compose project. Use the verified project only if ownership/isolation is current; otherwise create a uniquely named disposable project and document it. Rebuild `verify-go` from current source first.

Planned command forms, subject to fresh discovery:

```sh
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify build verify-go
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceStateCommit' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStateCommit' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState' ./internal/resources
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResource(Creation|Edit)DefiniteCommitRejection' ./internal/database
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go gofmt -l <changed-go-files>
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go go test -run '^$' ./...
rtk docker compose --project-name <verified-project> --file compose.yaml --profile verify run --rm --no-deps verify-go sh -ec 'go build -o /tmp/labreserve-task4b-chunk9-check ./cmd/labreserve && test -s /tmp/labreserve-task4b-chunk9-check'
rtk git status --short --untracked-files=all
rtk git diff --check
```

These are **planned forms**, not Chunk 9 results. Inspect discovery and skips; `go test -run '^$' ./...` is compile-only, not behavior evidence. Do not claim any command passed until it actually runs. Run affected Chunk 8/creation/edit regressions if helpers change. `G`, `C`, focused `D/U`, and `R` are required by plan §8; full `F` is reserved for Chunk 11.

## 7. Validation and repair policy

- Budget: **two failed validation attempts per issue**, from repository policy and controlling plan §7; preserve across sessions.
- Resolved attempts already used: prior Chunk 6 JSONB snapshot-key assertion (1); prior Chunk 7 cross-container artifact check (1); Chunk 8 missing test-helper argument compile failure (1); Chunk 8 PostgreSQL `$4` type-inference failure (1); Chunk 8 login-shell `gofmt` invocation failure (1). None is unresolved or repeated. Treat a recurrence of the same issue as using its prior budget, not a new issue. New, unrelated Chunk 9 failures start their own budget.
- Diagnose and repair ordinary fixture, trigger, COMMIT observation, compilation, formatting and cleanup problems autonomously within scope. Reproduce review findings before changing code. Stop after the second failed validation attempt for the same issue or for any required runtime-role/database-isolation failure.

## 8. Environment, migration, and prior-risk controls

- Last observed project `labreserve-task004b-c8-18c91ce4` had a healthy PostgreSQL 17.7 `verify-db`; the project/volume was deliberately retained. A future executor must recheck ownership and health; do not assume it is empty or stop/remove it if another worker may own it.
- Tests use only the guarded `labreserve_test` database; actual service calls require `labreserve_app`. `resourceCreationSetup` invokes `Migrate`; do not invoke it against persistent/demo data.
- Do not read or print `.env`, passwords, connection strings, tokens or private data. No persistent Migration 005 compatibility is established by disposable tests.
- The earlier Chunk 7→8 artifacts were untracked at the latest candidate and preserved. Inspect and preserve all supplied/untracked files at fresh preflight; do not infer ownership from filename.

## 9. Mandatory post-work

- Inspect current branch/HEAD/upstream, full staged/unstaged/untracked status, exact base, source fingerprints and the complete relevant cumulative diff, including every untracked file.
- Verify actual rejected COMMIT separately from both acknowledgement simulations; assert successful guarded statement, actual Commit call/error, retry count, and fresh persisted pair/no-pair per case.
- Run formatting, compile-only all-package tests, same-container app build/artifact check, exact discovery and focused behavior tests; state all `PASS`, `FAIL`, `SKIP`, `NOT_RUN`, and `UNVERIFIED` explicitly.
- Verify scoped trigger cleanup and joined goroutines/connections. Confirm no production/schema/privilege/route/dependency changes or unrelated work was touched.
- Assess data-integrity, concurrency, authorization, error-leak, test-isolation and rollback risks. Report Chunk 9 delta separately from cumulative Chunks 0–8. Independent review remains a separate fresh session.

## 10. Handoff policy and stop rule

- SINGLE-CHUNK only. Stop after Chunk 9; do not begin Chunk 10 or another task.
- Prepare a Chunk 9→10 evidence handoff and a separate Chunk 10 instruction only if that transfer is separately authorized after Chunk 9. Do not infer handoff permission from this instruction.
- If blocked or incomplete, report `BLOCKED` or `INCOMPLETE` and request the exact required evidence/authority; do not claim continuation readiness or perform a dependent speculative change.
- Do not stage/commit/push/merge/deploy, access persistent systems, or perform destructive cleanup.

## 11. Final report

Report current task/plan fingerprints, implementation base and candidate/tree status, explicit authorization and acceptance/review dependency, exact changed paths, actual COMMIT versus acknowledgement evidence, discovered/executed tests and skips, commands/results, failed-attempt counts, wait PIDs/edges, persisted snapshots, cleanup, complete diff scope, risks/limitations, handoff policy and remaining Chunk 10–11 work. Finish with exactly one non-final status: `CONTINUABLE`, `BLOCKED`, or `INCOMPLETE`.
