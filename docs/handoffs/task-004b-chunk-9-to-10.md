# LabReserve Task 4B — Chunk 9 → Chunk 10 Evidence Handoff

**Chunk 9 status: `CONTINUABLE`.** Chunk 9 is complete on this working-tree candidate. Chunk 10 has not started and is **not authorized**.

**Paired instruction:** [`instruction-chunk-10-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-10-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

## 1. Authority and Chunk 8 acceptance

- The user explicitly accepted Chunk 8 for satisfying Chunk 9's entry dependency after an independent assessment reported `ACCEPTANCE_RECOMMENDED`, no blocking findings, and one non-blocking documentation finding F1. The user clarified that F1 overstates an intermediate independent-review/acceptance requirement and directed us to follow the controlling plan. No authoritative source was edited to resolve it.
- The user separately authorized **Task 4B Chunk 9 only**, in **SINGLE-CHUNK** mode, and authorized this handoff plus the prepared Chunk 10 instruction. This does not accept Chunk 9, authorize Chunk 10, permit integration, or waive the eventual separate fresh-session final-delivery review/human acceptance.
- The controlling Chunk 9 dependency (“8 accepted”) is satisfied by the explicit user acceptance above. The prepared Chunk 9 instruction's additional review gate was not treated as controlling.

## 2. Candidate, source fingerprints, and preserved work

- **Branch:** `feat/004b-resource-activation-deactivation`.
- **Implementation base / HEAD:** `b6c1fcc7a55fdc7b3542fc0ce9068a864eeddb5c`; **upstream:** same revision; **HEAD tree:** `8ebacb5c3993cc665e9700c655a320d160d48553`. No commit, push, rebase, or merge was performed.
- **Candidate:** HEAD plus the working-tree delta listed below. The two expected candidate/source revisions agreed at preflight; no source drift was found.
- **Task SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Controlling plan SHA-256:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- **Migration SHA-256s:** 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`; 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`; 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`; 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`; 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`; 006 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.
- Task and plan are tracked and committed at HEAD. Relevant PRD and architecture sections, repository instructions, Task 4B, the complete plan, Chunk 8→9 handoff/instruction, and current test/transaction seams were inspected. The independent Chunk 8 findings were the user-provided decision in this session; no separate review report was found in the repository search.
- **Chunk 9 implementation files:**
  - Modified `internal/database/resource_commit_rejection_integration_test.go` — SHA-256 `4b8f7f26fdf3a722491da819ba500b4e2d8809dc5b4dd0bd255557df1d25ff68`.
  - Added `internal/database/resource_state_commit_integration_test.go` — SHA-256 `755d20c20e92c0ba9a41bb2af3356d4bdd65887c399efcf89f509a03c72a7e83`.
  - Modified `internal/resources/state_test.go` — SHA-256 `116eeadeb0b909de8fb84d950d397b34570f550fb66836dc5df9f200a8de7ade`.
- The two pre-existing unstaged documentation updates (`task-004b-chunk-8-to-9.md` and `instruction-chunk-9-task-004b-atomic-resource-activation-deactivation.md`) were preserved unmodified. No staged changes. No production code, migrations, schema, runtime privileges, routes, dependencies, application configuration, or unrelated files were changed.

## 3. Completed Chunk 9 scope

Added only test coverage and the narrow existing deferred-trigger helper extension:

1. `installResourceCommitRejectionTrigger` now supports `resource.activated` and `resource.deactivated`, scoped to the exact actor, action, and Resource UUID. Its creation/edit behavior is unchanged.
2. Real PostgreSQL service tests exercise deferred COMMIT rejection in both state directions, observe the actual guarded statement success and actual failing COMMIT, and force an independent waiter to block and re-evaluate after rollback.
3. Test-only transaction wrappers separately model a durable real commit followed by a lost acknowledgement and a rollback/noncommit followed by an acknowledgement-like error. They count service transaction/mutation/commit calls and do not add a production retry or outcome protocol.
4. A real-PostgreSQL context-cancellation test observes a `labreserve_app` Resource-lock waiter, cancels it, verifies safe operational failure/no mutation, then proves a later explicit state operation can acquire the Resource and commit.
5. A Resource-state unit regression verifies a commit-acknowledgement error stays sanitized and is not retried. Existing begin/role/lock/query/mutation/no-op-finalization unit cases remain in the executed state suite.

## 4. Actual COMMIT rejection, waiters, and persistence

The new deferred trigger is `DEFERRABLE INITIALLY DEFERRED` and raises PostgreSQL `P0001` with the fixture diagnostic only when the exact state event is committed. The tests observe a changed result from `SetResourceState`, then the actual `pgx` transaction `Commit()` call returning that PostgreSQL error. The application returns `resources.ErrOperational` without leaking the trigger diagnostic. This is actual COMMIT rejection, not an outer rollback or a synthetic acknowledgement error.

Fixture times: `T0 = 2040-04-05T06:07:08.123456Z`; waiter time `T1 = T0 + 1 minute`. Both tests use independent service transactions through `labreserve_app`; the separate admin pool reads committed state/history after transaction resolution.

| Direction | Observed PostgreSQL wait (waiter → rejected leader) | Rejected leader result | Re-evaluated waiter result and durable outcome |
|---|---|---|---|
| Activation, initially Inactive | PID `2181 → 2182`, `transactionid/ShareLock`; wait-event fields empty | Guarded state statement returned Active/changed at T0; actual deferred COMMIT returned `P0001`; no Resource transition or activation event persisted | Trigger removed after confirmed rejection; waiting explicit Active request re-read Inactive, sampled T1, and committed one `false→true` event. Final Active, `updated_at=T1`. |
| Deactivation, initially Active | PID `2185 → 2186`, `transactionid/ShareLock`; wait-event fields empty | Guarded state statement returned Inactive/changed at T0; actual deferred COMMIT returned `P0001`; no Resource transition or deactivation event persisted | Trigger removed after confirmed rejection; waiting explicit Inactive request re-read Active, performed its own post-lock occupancy check at T1, and committed one `true→false` event. Final Inactive, `updated_at=T1`. |

Before releasing each waiter’s mutation gate, assertions confirmed the committed Resource remained at its original state/timestamp and only its original creation event remained. After the waiter committed, assertions checked the exact state/event actor/action/target/time/before-after details and unchanged Resource identity/text/creation time, all fixture Bookings, request data, and prior history. The deactivation fixtures contain only past Confirmed and future Cancelled Bookings, so the explicit later transition is eligible.

## 5. Ambiguous-acknowledgement simulations and later explicit requests

These are deliberately distinct test-seam outcomes; neither is described as a network fault or exactly-once protocol.

| Simulation | Underlying transaction action | Service result / call evidence | Fresh persisted evidence | Later explicit request |
|---|---|---|---|---|
| Durable commit, lost acknowledgement | Wrapper delegates the real PostgreSQL COMMIT successfully, then returns a synthetic acknowledgement error | Safe `ErrOperational`, zero success result, exactly one begin/mutation/Commit-seam call; no automatic retry | Inactive→Active Resource and its matching activation event both persist at T0 | Explicit Active request is an authorized no-op: unchanged timestamp/history, no clock or guarded mutation call. |
| Noncommitted, acknowledgement-like error | Wrapper explicitly rolls back the transaction before returning the same synthetic error; underlying COMMIT is not called | Safe `ErrOperational`, zero success result, exactly one begin/mutation/Commit-seam call; no automatic retry | Original Active Resource and creation-only history persist; no deactivation event | Explicit Inactive request at T1 performs a new eligible transition and persists exactly one deactivation event. |

For both cases, assertions also compare retained Booking/request/history snapshots. The service-facing error does not identify which result occurred; only the test's controlled seam and independent persisted reads distinguish them. No failed desired state is retried automatically.

## 6. Cancellation, cleanup, and environment evidence

- Context-cancellation waiter: PID `2189 → 2187` (`transactionid/ShareLock`; wait-event fields empty). Cancellation while blocked returned safe operational failure; no clock, guarded mutation, Resource update, or Activity event occurred. After releasing the test-held lock, a separately issued explicit deactivation succeeded at T1, proving no transaction/lock was leaked.
- **Environment:** isolated Compose project `labreserve-task004b-c9-b6c1fcc`; `verify-db` healthy, PostgreSQL `17.7`, database `labreserve_test`. Read-only query returned `labreserve_test|postgres|17.7|0|0|0`: zero deferred-commit test functions, zero matching triggers, and zero `State-Concurrent-*` fixture Resources after tests. `testPool` enforces the `labreserve_test` database. `TestResourceStatePrivilegeBoundary` passed its explicit `current_user = 'labreserve_app'` assertion; the race wrapper additionally queried `current_user` for each concurrent runtime transaction.
- Migrations/fixtures/trigger setup and inspection were confined to this disposable `labreserve_test` database. No persistent/demo database was accessed. `.env` contents and credentials were not read or printed.
- The two earlier retained Chunk 8 Compose projects/volumes were untouched. The new Chunk 9 test project and its volume remain running/retained; no `down`, volume deletion, reset, or destructive cleanup occurred.

## 7. Verification executed

All shell commands below were run from repository root and routed through `rtk`. The successful final behavior selector was uncached and serialized (`-count=1 -p 1`); it reported no skips.

### Environment and pre-edit baseline

```sh
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify build verify-go
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify up --wait -d verify-db
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResourceStateActivationFailureRollsBackAndNoOpDoesNoWrites|TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry|TestResourceStatePrivilegeBoundary|TestResourceCreationDefiniteCommitRejectionRollsBackResourceAndActivity|TestResourceEditDefiniteCommitRejectionRollsBackResourceAndActivity)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateActivationFailureRollsBackAndNoOpDoesNoWrites|TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry|TestResourceStatePrivilegeBoundary|TestResourceCreationDefiniteCommitRejectionRollsBackResourceAndActivity|TestResourceEditDefiniteCommitRejectionRollsBackResourceAndActivity)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
```

All passed. This established existing activation and deactivation Activity-failure evidence, actual runtime identity, and creation/edit COMMIT-rejection behavior before code edits.

### Current-candidate discovery and focused execution

```sh
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify build verify-go
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState(CommitRejectionRollsBackAndWaiterReevaluates|ContextCancellationReleasesResourceLock|AmbiguousAcknowledgementAndLaterExplicitOperation)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState' ./internal/resources
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResource(Creation|Edit)DefiniteCommitRejectionRollsBackResourceAndActivity$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateCommitRejectionRollsBackAndWaiterReevaluates|TestResourceStateContextCancellationReleasesResourceLock|TestResourceStateAmbiguousAcknowledgementAndLaterExplicitOperation|TestResourceStateActivationFailureRollsBackAndNoOpDoesNoWrites|TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry|TestResourceCreationDefiniteCommitRejectionRollsBackResourceAndActivity|TestResourceEditDefiniteCommitRejectionRollsBackResourceAndActivity)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
```

All three new database test functions and all 12 Resource-state unit test functions were discovered. Both new COMMIT directions, the real cancellation waiter, both acknowledgement paths, both-direction Activity rollback regressions, and existing creation/edit COMMIT-rejection regressions passed without skips.

### Formatting, compile, build, and diff

```sh
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go gofmt -l internal/database/resource_commit_rejection_integration_test.go internal/database/resource_state_commit_integration_test.go internal/resources/state_test.go
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -run '^$' ./...
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go sh -ec 'go build -o /tmp/labreserve-task4b-chunk9-check ./cmd/labreserve && test -s /tmp/labreserve-task4b-chunk9-check'
rtk git diff --check
rtk git diff --cached --check
rtk git diff --no-index --check /dev/null internal/database/resource_state_commit_integration_test.go
```

Formatting produced no output; compile-only passed across all Go packages (no tests selected); application build and same-container nonempty artifact assertion passed. The no-index diff check produced no whitespace diagnostics; its expected status 1 reflects the untracked addition.

Read-only cleanup/identity query:

```sh
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify exec -T verify-db psql -X -qAt -U postgres -d labreserve_test -c "SELECT current_database(), current_user, current_setting('server_version'), (SELECT count(*) FROM pg_catalog.pg_proc WHERE proname LIKE 'labreserve_test_resource_commit_reject_%'), (SELECT count(*) FROM pg_catalog.pg_trigger WHERE tgname LIKE 'labreserve_test_resource_commit_reject_%' AND NOT tgisinternal), (SELECT count(*) FROM public.resources WHERE code LIKE 'State-Concurrent-%');"
```

### Failed validation attempt and repair

One initial post-edit discovery command ran before rebuilding the Go verification image and found no new tests. No behavior test was run/claimed from that stale image. Rebuilding `verify-go` from the current source repaired the invocation; the repeated discovery listed all three new tests and subsequent focused runs passed. This is **one failed invocation for the stale-image issue**, within the two-attempt budget.

Preserved historical issue-level validation counts from prior handoffs/instruction: Chunk 6 JSONB snapshot-key assertion (one failed attempt, repaired); Chunk 7 cross-container artifact check (one, repaired); Chunk 8 missing test-helper argument (one, repaired), PostgreSQL `$4` type inference (one, repaired), and login-shell `gofmt` invocation (one, repaired). None recurred; no validation budget was reset. No behavior test failed in this Chunk 9 run.

## 8. Diff, risk, and rollback assessment

- The Chunk 9 delta is limited to the two existing approved test files and the new approved test file above. The complete working-tree review also included the two pre-existing documentation updates; they were preserved. No staged changes or unrelated edits were present.
- **Functional/data risk:** no production behavior or transaction protocol changed. Test risks are trigger/gate cleanup and correct attribution of actual versus simulated commit outcomes; observed persisted snapshots and zero remaining fixture trigger/function/resource counts passed.
- **Integration risk:** the existing deferred trigger helper still passes creation/edit COMMIT regressions; state operation wrappers delegate to the actual state service transaction; actual runtime transactions assert `labreserve_app`.
- **Validation gap:** no transport/network-level acknowledgement-loss fault was introduced; the plan calls for a test-only transaction seam. `make verify`, Playwright, delivery-wide lifecycle/restart, and Chunk 10 migration/replay work were **NOT_RUN** as required for this chunk. These remain later-plan work, not claimed passes.
- **Rollback:** if needed, revert only this chunk's three test-file changes, preserving the two prior documentation updates. Do not stop/remove the retained Compose projects/volumes or clean the working tree.

## 9. Chunk 10 entry prerequisites and open verification-plan discrepancy

Before Chunk 10 implementation: obtain separate Chunk 9 acceptance and explicit Chunk 10 authorization; rerun complete source/base/worktree/fingerprint preflight; confirm Chunk 9 tests/files remain present; retain the Chunk 8 acceptance and preserve all unrelated dirty work; verify immutable migrations 001–005 and Migration 006 identity; use only an owned isolated disposable `labreserve_test` with actual runtime operations through `labreserve_app`. The plan's Chunk 10 dependencies also require Chunk 2's guarded additive-upgrade evidence to remain available and safe disposable-project ownership. No persistent/demo DB access or destructive cleanup is authorized.

Chunk 10 is the plan's **Retention, replay, migration and actual restart integration** slice. Its precise file/scope/test requirements are in §9 Chunk 10 of the controlling plan; this handoff does not start them.

**Verification discrepancy to resolve before Chunk 10 execution:** controlling plan §9 Chunk 10's focused-verification line says to run `F` (`make verify`), while the current user instruction describes `make verify`, Playwright, and delivery-wide lifecycle verification as reserved for Chunk 11. Chunk 9 obeyed the direct restriction and did not run them. The prepared Chunk 10 instruction records this mismatch and does not decide or run either option; obtain a controlling clarification before executing the disputed check. Do not edit the authoritative plan to resolve it.

**Chunk 10 remains `PREPARED_NOT_AUTHORIZED`; Chunk 9 is not yet reported as accepted.**

## 10. Final focused re-verification after handoff drafting

To resolve the earlier truncated-output uncertainty, the focused behavior selector and Resource-state unit suite were rerun after authoring the handoff pair. Both commands completed with exit status 0; no tests were skipped.

```sh
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateCommitRejectionRollsBackAndWaiterReevaluates|TestResourceStateContextCancellationReleasesResourceLock|TestResourceStateAmbiguousAcknowledgementAndLaterExplicitOperation|TestResourceStateActivationFailureRollsBackAndNoOpDoesNoWrites|TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry|TestResourceCreationDefiniteCommitRejectionRollsBackResourceAndActivity|TestResourceEditDefiniteCommitRejectionRollsBackResourceAndActivity)$' ./internal/database
rtk docker compose --project-name labreserve-task004b-c9-b6c1fcc --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
```

Observed database result: **PASS**, 7 top-level selected functions including all cases/subtests; actual waiter→rejected-leader PIDs activation `3014→3013`, deactivation `3017→3018`; context-cancelled waiter→lock-holder `3022→3019`. Durable-commit/lost-ack and rollback/ack-like scenarios, each followed by explicit operation, both passed. Creation/edit COMMIT regressions and both-direction Activity-failure regressions passed. Observed unit result: **PASS**, all 12 top-level Resource-state test functions (including their table subtests), including the new sanitized/no-retry acknowledgement case.

## 11. Subsequent authorization and candidate commit identity

This handoff records the state before the later Chunk 10 authorization; its original statements that Chunk 10 had not started and was unauthorized were accurate at handoff creation. The user subsequently accepted Chunk 9 for Chunk 10's entry dependency and authorized Chunk 10. Chunk 10 completed; see `task-004b-chunk-10-to-11.md` for its verification and safety evidence.

Chunk 9 source/test changes were committed as `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb` (tree `c2e23f290edd0e32a888a9937e9f45655b2e9338`). The subsequent Chunk 10 source/test commit is `b59c8acd2bb1813513d8dbcce1430b5052b7739a` (tree `54a25d599009ef671a502b0f3549547143efaaef`). Handoff/instruction documents are committed separately afterward; the Chunk 10 commit is the implementation-candidate hash, not the final documentation commit.
