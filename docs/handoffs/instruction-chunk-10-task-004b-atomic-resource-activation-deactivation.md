# LabReserve Task 4B — Prepared Instruction for Chunk 10

- **Instruction state when prepared:** `PREPARED_NOT_AUTHORIZED`
- **Source handoff:** [`task-004b-chunk-9-to-10.md`](task-004b-chunk-9-to-10.md)
- **Controlling ladder:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, §9, Chunk 10 — **Retention, replay, migration and actual restart integration**.
- **Execution:** Not authorized by this instruction. Do not begin work unless a later user authorization separately names Chunk 10 and its execution mode.

## 1. Authority and entry gates

The controlling plan's Chunk 10 entry dependency is “9 accepted,” plus preservation of Chunk 2's guarded additive-upgrade evidence, unchanged migration 001–005 fingerprints, and safe disposable-project ownership.

Before editing, the future executor must:

1. Confirm explicit Chunk 10 authorization and acceptance of Chunk 9 under the controlling plan. Chunk 9's `CONTINUABLE` handoff is evidence, not acceptance or authorization.
2. Use `chunked-implementation` in **SINGLE-CHUNK** mode only if explicitly authorized; execute Chunk 10 and stop before Chunk 11 or any later task.
3. Read current repository instructions, the complete approved Task 4B, the complete controlling plan including its ending, this instruction, and the complete Chunk 9→10 handoff. Re-read relevant PRD/architecture and Chunk 2 migration-upgrade evidence.
4. Verify branch, HEAD, ancestry/upstream, staged/unstaged/untracked state, source fingerprints, all migration 001–006 fingerprints, and all prior Chunk 0–9 test/evidence files. Preserve unrelated work, including the two pre-existing documentation updates recorded by the Chunk 9 handoff. Do not rely on its candidate identity if the checkout has changed.
5. Confirm the test target is an owned disposable `labreserve_test` database; fixture/migration setup uses `labreserve_migrator` only for allowed setup/inspection, while concurrent/application operations use actual `labreserve_app`. Do not read/print credentials or access persistent/demo databases. Recheck Compose project ownership; preserve all retained projects/volumes unless separately authorized.
6. Confirm the Chunk 9 verification/plan discrepancy below has a controlling resolution before running the disputed full-gate check. Do not modify the authoritative plan to remove the conflict.

The user previously clarified that the prepared Chunk 9 instruction must not invent an independent intermediate review gate. Follow the controlling plan's actual entry requirements; do not add an independent-review prerequisite where the plan does not specify one. Eventual independent final-delivery review and human acceptance remain separate requirements.

## 2. Chunk 10 objective and authorized scope

Implement **only** the plan's Chunk 10 retention/replay/migration/restart integration work (V7–V9 and the remaining lifecycle portions of V2/V6). Do not change production state-service/transaction behavior. If evidence indicates a defect that would require production changes, stop and obtain scope/authority clarification rather than expanding this chunk. No new feature/API is authorized.

Plan-proposed paths, subject to current discovery and exact plan scope:

- New `internal/database/resource_state_retention_integration_test.go`.
- New `internal/database/resource_state_lifecycle_integration_test.go`.
- Minimal `scripts/verify.sh` wiring for separate state restart fixture/replay phases.
- Accurate `README.md` notes for the accepted internal state service, additive upgrade order, compatibility/recovery limitations, and explicit Task 4C/intervention/history deferrals.
- Only if needed, bounded extension to the existing `internal/web/resource_state_session_integration_test.go` for existing read paths; no Resource-management route or browser workflow.

Preserve accepted creation/edit, booking/replay, owner-cancellation, authorization, lock order, runtime privilege, schema, Activity, error-safety, and raw-text behavior. Do not modify migrations 001–006 or migration history. No reset, persistent-target upgrade, automatic down-migration, deployment, new dependency/infrastructure, Task 4C route/UI, cross-owner cancellation, Activity-history API, or later chunk is in scope.

## 3. Required Chunk 10 evidence from the controlling plan

- Snapshot Resource identity/non-state fields and raw UTF-8 legacy text, all retained Bookings/request/cancellation data, and prior Activity across activation, allowed/blocked deactivation, no-op, and rollback. Keep existing read paths safe and accurate.
- Exercise retained replay after deactivation: identically replay the original Cancelled and past Confirmed Booking request identities without new Booking/event or occupancy; verify changed-data reuse precedence and per-owner request scope. Preserve ordinary conflict/adjacency/different-Resource/time/purpose/cancellation behavior.
- Add distinct lifecycle fixtures for committed activation and deactivation, including a Coordinator-deactivated seeded Resource; retain exact operation times/events and relevant Booking/history snapshots.
- Wire fixture and replay phases around actual PostgreSQL **and application** restarts, with fresh runtime connections, repeat seeding, preserved sessions/identities/history, post-restart no-op/opposite-state operation, and existing 4A/3B lifecycle assertions still meaningful. A pool reconnect alone is not restart evidence. Every phase must actually run; an environment-skipped test is not evidence.
- Re-run fresh install, compatible 005→006 additive upgrade, repeat migration, exact-ledger and preserved 004→005 tests in disposable databases only. Preserve source hashes 001–005; do not inspect or alter persistent targets.
- Update README with actual behavior/commands/results and non-destructive upgrade/recovery constraints. Do not claim browser management acceptance (Task 4C remains deferred).

## 4. Verification discrepancy — unresolved at preparation, later resolved

The controlling plan §9 Chunk 10 focused-verification line includes `F` (`make verify`). The current Chunk 9 authorization explicitly said not to run `make verify`, Playwright, or delivery-wide lifecycle verification because those are reserved for Chunk 11. Chunk 9 obeyed that current-chunk restriction and recorded the conflicting plan text in its handoff.

**Subsequent resolution:** the user later authorized Chunk 10 only and clarified that its plan-required `F` (`make verify`) must run against confirmed disposable resources. The final command and passing result are recorded in `task-004b-chunk-10-to-11.md`. The authoritative plan was not changed. The remainder of this section records the preparation-time state, not an unresolved current blocker. Before Chunk 10 execution, obtain a controlling clarification on whether Chunk 10 must follow its plan's `F` command or defer the full gate to Chunk 11. Do not silently change the plan, omit a plan-required check, or run the disputed command before the decision. Independent Chunk 10 checks that do not prejudge this choice may proceed only after separate Chunk 10 authorization and all other entry gates pass.

Planned focused checks should follow Chunk 10's exact selectors and command forms in plan §9 after current test discovery. The plan lists retention and migration selectors, formatting, compile-only all-package checks, application build, shell syntax when `scripts/verify.sh` changes, complete diff review, and the full verification command subject to the clarification above. Do not report planned commands as executed results.

## 5. Environment, verification, and preservation rules

- The Chunk 9 evidence project `labreserve-task004b-c9-b6c1fcc` and its `labreserve_test` volume were intentionally retained. Verify ownership/health before reuse; do not stop, reset, or remove it. Other retained Chunk 8 projects were left untouched.
- Never connect tests/migrations to demo or persistent databases. Confirm database names via guarded test helpers and actual transaction identity (`current_user = labreserve_app`) without logging connection strings or secrets.
- `resourceCreationSetup` invokes `Migrate`; use it only against disposable `labreserve_test`.
- Preserve the complete working-tree state, including the two pre-existing documentation updates and all Chunk 9 evidence. Do not stage, commit, push, merge, rebase, deploy, or perform destructive cleanup.
- Failed validation budget: two failed attempts per issue; preserve Chunk 6–9 history recorded by prior handoffs. Do not reset counts across sessions. Repair only in-scope defects and rerun affected checks.
- Discovery and actual execution are separate. Record PASS, FAIL, SKIP, NOT_RUN, and UNVERIFIED honestly, including restart flags/phases and cleanup.

## 6. Stop condition

Stop after Chunk 10. Do not begin Chunk 11. Report actual base/candidate and source fingerprints, exact changed paths, migration and restart/replay evidence, commands/discovery/skips, failures/repairs, environment/cleanup status, complete diff, risks/rollback, unresolved `F` clarification if any, and precise remaining Chunk 11 entry requirements. At preparation, this instruction was `PREPARED_NOT_AUTHORIZED`. The user later authorized Chunk 10 and its full disposable verification; Chunk 10 is complete as recorded in the paired handoff. This historical instruction does not authorize Chunk 11.

## 7. Subsequent candidate identity

Chunk 9 implementation/test commit: `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb`. Chunk 10 implementation/test candidate: `b59c8acd2bb1813513d8dbcce1430b5052b7739a` (tree `54a25d599009ef671a502b0f3549547143efaaef`). Handoff documents are committed separately after the source/test commits.
