# Task 4B — Chunk 4 → Chunk 5 Handoff

**Status: `CONTINUABLE` — the required Chunk 4 equal-operation-microsecond regression has been added and passed against real PostgreSQL.**

**Chunk 5 gate: closed.** The repaired Chunk 4 candidate received a separate repair review: **PASS**, with no blocking findings in the repair. Separate Chunk 5 authorization and a fresh preflight remain outstanding. This handoff does not authorize implementation.

## Authority and source identity

- Task: `docs/tasks/004b-atomic-resource-activation-deactivation.md` — SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`
- Sole controlling ladder: `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md` — SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`
- The task and plan are tracked, unchanged at this candidate, and present at `HEAD`.
- Branch: `feat/004b-resource-activation-deactivation`, tracking `origin/feat/004b-resource-activation-deactivation`.
- Chunk 4 implementation base and current `HEAD`: `19566eeea5ec33944fba3740bf00ff54f0450b8c` (also current upstream). Candidate implementation is this commit plus the dirty worktree below; it has no candidate commit SHA.
- Chunk 4’s prior implementation session reported its authorization/source/base checks revalidated before editing. This handoff session freshly rechecked the task/plan identities and relevant plan clauses. **No Chunk 5 preflight or authorization has occurred.**

## Candidate and protected worktree state

At the start of this update, `git status --short --untracked-files=all` showed only unstaged changes: the Chunk 4 implementation files below, the modified Chunk 3→4 handoff, and three untracked handoff/instruction files (the Chunk 4 instruction, this handoff, and the Chunk 5 instruction). No staged changes were present. The separately authorized Chunk 4 recovery added the equal-time regression to the database integration test; this documentation update changes only the two requested handoff documents. Preserve all other paths.

Chunk 4 candidate files and current SHA-256:

- `internal/resources/state.go` — `029b2839fd978a57fbe6cc80b437d34b51dfa5c60447a5705c1e9f8f1ca2c3bf`
- `internal/resources/state_test.go` — `437753a8cc852c8399dd059c347a9f69e00b5b99a291d159289936a17c8229ec`
- `internal/database/resource_state.go` — `347f0997f7e582ec4aa9cd4770358330bfb6abebe2f2e4b5dd2f7ce8e36b7311`
- `internal/database/resource_state_integration_test.go` — `ce27290722f9864fd7ea87ff382141c78b18f151304120f9f4c17d00ad73fbf6`
- `internal/database/migrations/006_resource_state.sql` — `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f` (unchanged; not in the dirty paths)

Other existing work that must remain untouched:

- Modified `docs/handoffs/task-004b-chunk-3-to-4.md` — SHA-256 `612cc96d6255af08faee6847270844db715718d62d2ff2e4398040ddd64f2e51`.
- Untracked `docs/handoffs/instruction-chunk-4-task-004b-atomic-resource-activation-deactivation.md` — SHA-256 `2d6de0c4b4b860267fd0a318b981696d2e39ffbd97b864ed7dd31c0554c9be50`.
- The requested Chunk 5 instruction and this handoff were already present and untracked before this update; both are being updated in place.

Do not stage or commit these documents or any other work. No credentials or `.env` files were changed. Do not reset, clean, overwrite, push, merge, deploy, or perform database/volume cleanup.

## Chunk 4 completed work (as observed)

The implementation removes fail-closed deactivation stubs, checks confirmed Booking occupancy after Resource authorization/lock using a fresh UTC-microsecond operation time and strict `end_at > operation_time`, maps the guarded occupancy rejection to `ErrResourceOccupied`, and keeps matching-state no-ops free of clock, occupancy, Resource update, and Activity work. The Chunk 4 SQL migration was not edited.

Added/extended coverage includes authorization and no-op behavior, strict end boundaries and occupancy scope, creation precedence on inactive Resources, lock-wait timing, retained Booking/history, adapter mapping, transaction/Activity failure rollback, and explicit retry. Current test names include:

- Unit: `TestResourceStateDeactivationUsesPostLockOccupancyAndSharedTime`, `TestResourceStateInactiveNoOpCommitsWithoutClockOrOccupancy`, and related authorization/failure tests.
- PostgreSQL state: `TestResourceStateDesiredStateNoOpsAndInactiveCreationPrecedence`, `TestResourceStateDeactivationOccupancyAndEndBoundary`, `TestResourceStateDeactivationUsesFreshTimeAfterResourceLockWait`, `TestResourceStateDeactivationActivityFailureOuterRollbackAndRetry`, and related service/capability tests.

## Verification evidence from Chunk 4 implementation and recovery

The implementation-session results below were reported then and were **not rerun** during the recovery or this documentation update. Recovery results follow; they were **not rerun** during this documentation update:

- `go test ./internal/resources` — PASS (65 package tests).
- Focused `^TestResourceState` unit selector in `./internal/resources` — PASS (26 tests).
- Focused PostgreSQL Resource-state/capability selectors in the isolated `labreserve-verify` Compose project — PASS (13 top-level tests); the deactivation lock-wait selector was also rerun after its final timing adjustment and passed.
- Five existing Booking creation/cancellation regression selectors — PASS.
- Database package compile-only check (`go test -run '^$' ./internal/database`) — PASS; no behavior tests were selected by that command.
- Docker application/test image build — PASS.
- `gofmt` check and `git diff --check` — PASS in the implementation session. The earlier handoff-authoring pass also ran `git diff --check` successfully.
- Recovery session: `TestResourceStateOppositeTransitionsAtEqualOperationTime` — PASS against real PostgreSQL. It holds one UTC-microsecond operation time constant, verifies both Inactive→Active and Active→Inactive return changed results, checks the committed intermediate state, then verifies final state and the creation/activation/deactivation Activity records, including actor, action, timestamps, and before/after state details.
- Recovery session: focused Resource-state tests, selected Booking/cancellation regressions, all-package compile-only check, application build, formatting, and diff checks — PASS. The recovery summary did not preserve every expanded shell invocation; this handoff does not reconstruct commands that were not retained.
- Separate repair-review session: **PASS** for the repaired dirty candidate based on `19566eeea5ec33944fba3740bf00ff54f0450b8c`; no blocking findings in the repair. The review confirmed B1 resolved: `internal/database/resource_state.go` maps only the deliberate Coordinator rejection to `ErrForbidden`, and other `42501` failures remain operational; `internal/database/resource_state_integration_test.go` verifies safe error text, unchanged Resource/Activity snapshots, and preservation of the genuine authorization rejection. Reviewed adapter/test SHA-256 prefixes `347f0997f7e5` and `ce27290722f9` match the full hashes recorded above.
- Independent verification reported with that review: all 17 Resource-state PostgreSQL tests passed without skips; 65 Resource tests/subtests passed; all-package compilation, focused `go vet`, formatting, and diff checks passed; fault-trigger cleanup was confirmed. These review-session results were not rerun during this documentation update. Full browser/lifecycle verification was not rerun.
- No test or validation failure was reported. `make verify`, browser/Playwright tests, and delivery-wide lifecycle checks were **not run**; full verification belongs to Chunk 11. No persistent target was accessed.

The implementation-session summary did not preserve the complete expanded shell invocations for every Compose selector, the five regressions, or image build. This handoff records those reported selectors/results rather than guessing command lines; none were rerun in this documentation session.

Recovery used the isolated Compose project `labreserve-recovery-c4-19566ee`; it was stopped and its named volume retained. The separate `labreserve-verify` project was observed running with a named volume and was left untouched; its ownership was not established. Do not infer either volume is disposable. Reconfirm exact project ownership and test-database identity before any later DB operation.

## Recovered Chunk 4 verification gap

The earlier version of this handoff correctly recorded that the plan’s Chunk 4 ordered tasks require “opposite real transitions at equal operation microsecond; no timestamp-based false no-op.” A separately authorized **Chunk 4-only recovery** has now added `TestResourceStateOppositeTransitionsAtEqualOperationTime` to `internal/database/resource_state_integration_test.go`. This was a missing test-evidence item, not a reproduced production defect; the recovery changed no production code or migration.

The regression uses an isolated real PostgreSQL fixture and one fixed UTC microsecond for both service operations. It starts Inactive, commits Inactive→Active and verifies the intermediate persisted state, then commits Active→Inactive at the same operation time. It asserts both results are changed, checks final persisted Inactive state, and verifies the retained creation event plus one activation and one deactivation event with the expected Coordinator, action, timestamp, and before/after state details. The test passed against real PostgreSQL. The recovery’s focused Resource-state tests, selected Booking/cancellation regressions, compile-only check, build, formatting, and diff checks also passed as recorded above. Migration 006 remains unchanged.

### Current Chunk 4 status and next gate

The previously missing plan-required regression is now covered with direct database evidence. The recovery session stopped after Chunk 4; no Chunk 5 work was started. `CONTINUABLE` records this non-final chunk’s completed recovery evidence.

The separate repair review is recorded as **PASS**, with no blocking findings in the repair. This review outcome does not authorize Chunk 5. Chunk 5 still requires separate explicit user authorization and a fresh specification preflight. Do not combine any further Chunk 4 repair/review work with Chunk 5 absent the appropriate authorization.

`make verify`, browser/Playwright tests, and delivery-wide lifecycle checks remain **not run** and are reserved for Chunk 11. No persistent target was accessed.

## Next planned chunk (not authorized)

**Chunk 5 — Effective runtime privileges and trusted-session boundary.** The plan proposes test-only additions at:

- `internal/database/resource_state_privilege_integration_test.go`
- `internal/web/resource_state_session_integration_test.go`

The plan requires actual effective runtime privilege evidence and trusted session→internal state-service evidence, while preserving existing read/cancellation behavior and adding no management HTTP endpoint. The prepared instruction is at `docs/handoffs/instruction-chunk-5-task-004b-atomic-resource-activation-deactivation.md`; it remains `PREPARED_NOT_AUTHORIZED` and records the recovered regression and repair-review PASS. Separate current user authorization and base-specific preflight are still required; this handoff is not authorization.

## Risk and stop condition

- **Functional/security risk:** the equal-time opposite-transition regression now passes against real PostgreSQL; the repair review passed with no blocking findings in the repair. Future privilege tests must use actual `labreserve_app`, preserve narrow grants, and not claim a cached Actor authenticates a logged-out session.
- **Integration/environment risk:** the recovery project `labreserve-recovery-c4-19566ee` was stopped with its volume retained. The separate `labreserve-verify` project was observed running with a named volume and left untouched; its ownership was not established. Preserve both environments and verify exact ownership/database identity before any later DB operation; do not touch persistent/demo targets or use destructive cleanup.
- **Verification gap:** the Chunk 4 equal-microsecond gap is closed. `make verify`, browser/Playwright, and delivery-wide lifecycle verification remain unrun and are reserved for Chunk 11.
- **Independent review:** separate repair review reported **PASS**; no blocking findings in the repair. This handoff update itself is not a separate fresh-session review.
- **Rollback:** no changes from this handoff session need rollback. Any future repair must preserve pre-existing dirty work and remain isolated to the separately authorized current chunk.

**Stop here.** Do not execute Chunk 5 or any later chunk. Do not stage, commit, push, merge, deploy, access persistent targets, or clean up the verification environment.
