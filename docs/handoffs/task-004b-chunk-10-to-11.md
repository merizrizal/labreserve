# Task 4B — Chunk 10 → Chunk 11 Evidence Handoff

**Chunk 10 status: `CONTINUABLE`.** Chunk 10 exit conditions passed on the verified working-tree candidate, subsequently committed as recorded in §8. Chunk 11 has not started and remains separately unauthorized.

**Paired instruction:** [`instruction-chunk-11-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-11-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

## 1. Authority and plan clarification

- At Chunk 10 authorization, the user explicitly accepted Chunk 9 for Chunk 10's entry dependency, authorized **Task 4B Chunk 10 only** in **SINGLE-CHUNK** mode, and clarified that Chunk 10 must run the plan's `F` (`make verify`) using only verified disposable resources. This resolved the prepared Chunk 10 instruction's earlier plan-vs-Chunk-9 `F` discrepancy for this chunk only. The later commit authorization is recorded in §8; neither authorization permits Chunk 11, integration, or deployment.
- The user separately authorized preparing this Chunk 10→11 handoff and a Chunk 11 instruction marked `PREPARED_NOT_AUTHORIZED` after Chunk 10 exit conditions. Those artifacts do not authorize Chunk 11.
- **PREFLIGHT_PASS** was recorded before implementation. The controlling contract and plan were tracked/committed, their full contents and endings were inspected, task/plan fingerprints matched, and no unresolved contradiction prevented Chunk 10. At that pre-implementation check, the implementation base and working-tree delta were rechecked; no new commit had been made.

## 2. Candidate and preserved work

- **Branch:** `feat/004b-resource-activation-deactivation`.
- **Implementation base at Chunk 10 preflight:** `b6c1fcc7a55fdc7b3542fc0ce9068a864eeddb5c`; **tree:** `8ebacb5c3993cc665e9700c655a320d160d48553`. Upstream `origin/feat/004b-resource-activation-deactivation` matched that base. During Chunk 10 execution there were no staged changes, commit, push, rebase, merge, or reset.
- **Candidate at handoff drafting:** base HEAD plus the unstaged and untracked working-tree delta below. The checkout was deliberately left dirty; no candidate commit/tree existed at that time.
- **Task SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Controlling plan SHA-256:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- **Immutable migrations 001–006 SHA-256:**
  - 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`
  - 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`
  - 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`
  - 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`
  - 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`
  - 006 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`
- Task and plan are unchanged. Migrations 001–006 are unchanged. No application production code, schema, migration, runtime privilege, API, route, dependency, or credential file was edited.
- **Chunk 10 implementation/documentation paths:**
  - Added `internal/database/resource_state_retention_integration_test.go`.
  - Added `internal/database/resource_state_lifecycle_integration_test.go`.
  - Extended `internal/web/resource_state_session_integration_test.go` only for the existing Resource-list/session read path.
  - Extended `scripts/verify.sh` with an isolated project-name override, state restart phases, and seeded Resource/Booking identity preservation assertions.
  - Updated `README.md` with Task 4B internal-service behavior, migration-006 upgrade and 005 compatibility/recovery constraints, verification scope, and Task 4C deferral.
- **Chunk 10 working-copy SHA-256:** `README.md` `6de43ec3571ceff6758fa62b9c630459a2ef427a3767c13c40107e6bdd92f68c`; `scripts/verify.sh` `a8bd92a8e599b4aa3a71c8ab124aa82e16145e90a5bde8a8c9a64bec7c573edc`; `internal/database/resource_state_lifecycle_integration_test.go` `2b2bfbdcdce5304e76978a18c827e467bcc5f652abfab5fb2ab08fe04f6304a4`; `internal/database/resource_state_retention_integration_test.go` `bc8d7cc9a6a73be447b787a3e7d181f4072859d171b6ed0ea56ba77439fb23b0`; `internal/web/resource_state_session_integration_test.go` `c622ad2638c027561298807a156b24c61e1af0b54fd7ff9473c39265f325c0c6`.
- Prior Chunk 9 changes were preserved. Its source/test files (`internal/database/resource_commit_rejection_integration_test.go`, `internal/resources/state_test.go`, and `internal/database/resource_state_commit_integration_test.go`) are in commit `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb`. Its handoff/instruction documents remain part of the final docs commit: `docs/handoffs/task-004b-chunk-8-to-9.md`, `docs/handoffs/instruction-chunk-9-task-004b-atomic-resource-activation-deactivation.md`, `docs/handoffs/task-004b-chunk-9-to-10.md`, and `docs/handoffs/instruction-chunk-10-task-004b-atomic-resource-activation-deactivation.md`. Preserve the source commits and these documents.

## 3. Chunk 10 evidence delivered

1. `TestResourceStateRetentionAcrossTransitionsAndInactiveReplay` snapshots raw legacy Unicode/HTML Resource text, creation identity, prior Resource Activity, complete Booking/request/cancellation rows, and their Activity history. It exercises activation with retained future Confirmed and Cancelled records, matching-state no-op with UPDATE/Activity failure detectors, blocked deactivation with unchanged snapshots, allowed deactivation after Booking end/cancellation, inactive replay for a Past Confirmed request and a Cancelled request without extra Booking/event, changed-data request-ID reuse precedence, and the same request UUID under a different owner.
2. `TestResourceStateRestartFixture`/`TestResourceStateRestartReplay` are wired into `scripts/verify.sh` around real PostgreSQL **and application** restarts. The fixture commits an activation, creates and cancels Bookings through accepted services, deactivates a seeded Resource, inserts a scoped test session, repeats seed, and snapshots all seeded-Resource Booking histories. After re-seeding/restart, fresh runtime connections verify state/event/time pairs, Resource identity, retained Booking/request/cancellation/history, inactive replay, matching-state no-op with UPDATE/Activity detectors, and a real opposite-state transition.
3. The browser/session test resolves the retained Coordinator session with the runtime role and requests `/resources` from the actual restarted verification app; it verifies the seeded Resource is presented Inactive. No Resource-management route or browser workflow was added.
4. The verification script's `LABRESERVE_VERIFY_PROJECT` override is restricted to the `labreserve-verify` prefix and safe lowercase/digit/hyphen characters. It checks seeded Resource identity/creation time and pre-existing seeded Resource Booking/request/cancellation/Activity JSON across the full lifecycle. The ordinary default remains `labreserve-verify`; the successful run used a verified unused name.
5. README describes `resources.StateService.SetState`, stored Coordinator authority, post-lock all-owner Confirmed `end_at > operation_time` deactivation blocking, no-op/event semantics, unchanged retained Bookings, 006's narrow guarded capability, application-supplied trusted actor/time limits, explicit migration ordering, old-binary behavior, immutable 005 compatibility, non-destructive recovery constraints, and Task 4C/deferred boundaries.

**Engineering decision:** E2E creates many test Bookings on `K8S-01`; the first full-gate attempt proved it is not a clean deactivation target. The lifecycle fixture now uses seeded `DEMO-01`, records and compares its pre-existing Booking histories, and places deactivation 60 days after the controlled clock so all known E2E fixture Bookings are past. This preserves their data and keeps the separate 4A `NET-01` lifecycle untouched. The script independently compares pre-existing DEMO-01 Booking/history JSON across restart/reseed.

## 4. Verification actually executed

All database operations were restricted by test helpers to `labreserve_test`; service operations used `labreserve_app`, asserted directly in the restart test. Migration/setup/inspection used the disposable test database only. No `.env` contents or credentials were read or logged.

### Focused Chunk 10 checks

- **PASS:** uncached `TestResourceStateRetentionAcrossTransitionsAndInactiveReplay` on the disposable Chunk 10 project.
- **PASS:** explicit `TestResourceStateRestartFixture`, actual PostgreSQL restart, re-seed, actual verification-app startup/restart, retained-session HTTP request, and `TestResourceStateRestartReplay`; each flagged fixture/replay ran explicitly rather than relying on a skip in `go test ./...`.
- **PASS:** migration discovery and uncached serialized selectors found and passed `TestCheckSchemaRequiresExactMigrationLedger` (missing/unknown/mismatched cases), `TestResourceCapabilityMigrationPreservesVersion004Data`, `TestResourceStateMigrationPreservesVersion005Data`, and `TestResourceStateMigrationRejectsIncompatible005Ledger`.
- **PASS:** `go test -run '^$' ./...` compiled all Go packages; `gofmt -l` produced no output; `bash -n scripts/verify.sh` and `git diff --check` passed.

### Required repository gate — user-authorized category F

Final candidate verification ran successfully:

```sh
rtk env LABRESERVE_VERIFY_PROJECT=labreserve-verify-task004b-c10-b6c1fcc make verify
```

Observed final result: `Repository verification passed.` The complete run rebuilt the current Go/app/seed/migration/E2E images, applied fresh migrations and seed, ran all Go unit/real-PostgreSQL/HTTP packages successfully, passed **8/8 Playwright tests**, ran existing 4A Resource and 3B cancellation fixture phases, ran the new state fixture, repeated seed, restarted PostgreSQL and the app, checked retained state/sessions/history, then explicitly ran the existing Resource/cancellation replay phases plus the new web-session and state-replay phases. No required lifecycle phase was left only in the environment-gated skip path. The script's seeded-Resource and existing DEMO-01 Booking/history comparisons passed.

The full Go suite's ordinary environment-gated restart test invocations may skip when flags are absent; the script then ran each required fixture/replay explicitly with its flag. No skipped invocation was treated as lifecycle evidence.

## 5. Failed attempts, repairs, and finite budget

- A compile-only command initially named the wrong similarly named Compose project and used a stale image; it selected no tests. Inspection found it had created only an empty volume/network and no service container. Those exact accidental resources were removed; no retained DB or project was touched.
- Focused test iterations caught and repaired: a missing `pgxpool` test import, PostgreSQL's inability to infer uncast JSON fixture parameter types, and failure to register an extra cross-owner Booking in cleanup. Each issue was repaired and the focused retention selector passed.
- The first full `make verify` attempt failed closed because 28 E2E Bookings made K8S-01 non-pristine. The project was automatically cleaned by that run's own exit trap. The fixture was moved to DEMO-01 with full pre-existing-history snapshots and a post-E2E deactivation time; full verification then passed. A final additional opposite-transition Activity assertion was followed by another full `make verify`, which also passed.
- These were distinct in-scope issues, each repaired within the plan's two-failure-per-issue budget. No failing check was suppressed or weakened.

## 6. Environment, effects, and cleanup

- Full verification used only the previously absent project `labreserve-verify-task004b-c10-b6c1fcc` and its disposable `labreserve_test` volume. `scripts/verify.sh` removed only that project’s containers, network and volume on exit; it is absent after the run.
- The pre-existing default `labreserve-verify` exited container/volume was inspected and preserved. Chunk 8 and Chunk 9 projects/volumes were not stopped, reset, or removed. The separately retained focused Chunk 10 project `labreserve-task004b-c10-b6c1fcc` remains running and was not cleaned. The accidental empty `labreserve-task004b-c10` project created by the wrong-name compile invocation was removed after confirming it had no service containers.
- `make verify` rebuilt shared local `:verify` image tags; existing retained containers/volumes were not restarted or reset by those image builds. This is an observed local image-tag side effect.
- During Chunk 10 verification, no demo/persistent database was connected to, migrated, reset, or cleaned; no credentials were modified and no deployment occurred. The later user-requested source commits are recorded in §8.

## 7. Remaining Chunk 11 entry and stop condition

Chunk 11 is the plan's delivery-wide verification/documentation/candidate audit, not a continuation authorized by this handoff. A fresh session must revalidate current status, base and source fingerprints; read the full Task 4B, controlling plan including its end, this handoff, prepared instruction and current repository guidance; preserve all listed dirty work; and obtain explicit Chunk 11 authorization before executing it. The category-F clarification above authorized Chunk 10's `make verify` only and must not be treated as Chunk 11 authority.

Independent fresh-session review and human acceptance/integration remain outstanding. Do not make further commits, merge, deploy, begin Task 4C, or start Chunk 11 from this handoff. Stop here.

## 8. Implementation candidate commit identity (recorded after verification)

After the Chunk 10 verification, the user authorized committing the implementation/test changes in chunk order, with `docs/handoffs` last. The Chunk 9 test commit is `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb` (tree `c2e23f290edd0e32a888a9937e9f45655b2e9338`). The Chunk 10 implementation/test commit is `b59c8acd2bb1813513d8dbcce1430b5052b7739a` (tree `54a25d599009ef671a502b0f3549547143efaaef`). **Implementation candidate commit:** `b59c8acd2bb1813513d8dbcce1430b5052b7739a`; it includes the Chunk 9 and Chunk 10 source/test commits. The handoff/instruction documents are intentionally committed afterward; their final documentation commit is not part of this implementation-candidate hash. No implementation files changed after the successful full verification.
