# Task 4B — Chunk 6 → Chunk 7 Evidence Handoff

**Status: `CONTINUABLE`.** Chunk 6 exit evidence is recorded below; Chunk 7 has not started.

**Scope:** Chunk 6 only. This handoff does not authorize Chunk 7.

**Paired instruction:** [`instruction-chunk-7-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-7-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

**Independent review:** Chunk 6 has not received independent review. Per the current user's instruction for this non-final chunk, that review is not a gate for preparing or later authorizing Chunk 7. This is not a claim of review or final Task 4B acceptance.

## 1. Task, sources, and candidate identity

- **Task:** [`docs/tasks/004b-atomic-resource-activation-deactivation.md`](../tasks/004b-atomic-resource-activation-deactivation.md), tracked at current `HEAD`; SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** [`docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`](../implementation-plan/004b-atomic-resource-activation-deactivation.md), tracked at current `HEAD`; SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- **Migration 006:** `internal/database/migrations/006_resource_state.sql`, SHA-256 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.
- **Branch/candidate observed during this handoff:** `feat/004b-resource-activation-deactivation`, `HEAD` `fbb291ecaed352895475d5204a02c0cf2a11fd24`; `origin/feat/004b-resource-activation-deactivation` points to the same commit; clean worktree before these two documents were authored. `HEAD^` is Chunk 5 candidate/base `ac5f2b8b510b5c77226cc0a0cc47d798cfe559fa`; `HEAD^^` is `19566eeea5ec33944fba3740bf00ff54f0450b8c`.
- **Chunk 6 implementation base:** `ac5f2b8b510b5c77226cc0a0cc47d798cfe559fa`. Current Chunk 7 candidate/base is the committed `fbb291ecaed352895475d5204a02c0cf2a11fd24`.
- Current `HEAD` adds three files relative to `ac5f2b8`: the Chunk 6 test file, the Chunk 5→6 evidence handoff, and the prepared Chunk 6 instruction. It changes no production source or migration. The current test-file SHA-256 is `6a12f7f73b754b26b16b802b36ad52fac593a5f9d45d761822ba1c1ee19e60d3`.
- The latest preceding handoff [`task-004b-chunk-5-to-6.md`](task-004b-chunk-5-to-6.md) is currently tracked at `HEAD` (SHA-256 `db28eafa6741ff9649ea0e33c976913cb837520a99d7866823a3fa9027ac1a54`). Its paired [`instruction-chunk-6-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-6-task-004b-atomic-resource-activation-deactivation.md) is also tracked (SHA-256 `9239717af31f24487d3385da47713add47210b4ede12eb146ca5008f889d1c62`). The relevant earlier handoff is [`task-004b-chunk-4-to-5.md`](task-004b-chunk-4-to-5.md).
- **Candidate discrepancy:** an earlier conversation response reported a local commit beginning `8f67be1`; current Git inspection instead observes `fbb291e…`, equal to upstream, with the three-file delta above and parent `ac5f2b8…`. The reason for the revision-identity difference was not established. Use the current full `HEAD` and inspected tree; do not rely on the earlier short hash. The task, plan, migration, and tested Chunk 6 source bytes match their recorded identities, so this is confirmed candidate-history drift without production/source drift.
- The task document's historical status says approved for repository incorporation, not implementation. Chunk 6's separate implementation authorization was supplied in the prior user request; this current user authorization covers creation of this handoff pair only, not Chunk 7.
- **Preflight:** the prior Chunk 6 implementation session reported `PREFLIGHT_PASS` against base `ac5f2b8…`. This handoff revalidated current task/plan/migration hashes, branch, ancestry, upstream, status, and relevant diff. It did not perform a Chunk 7 implementation preflight.
- **Source drift:** `NONMATERIAL_CONFIRMED` for continuation preparation: task/plan/migration fingerprints are unchanged; the current candidate contains the exact tested Chunk 6 test bytes plus the two expected prior handoff documents, with no production diff. The earlier Chunk 6 instruction's possibility that those two documents remained untracked is historical; current Git shows them committed.

## 2. Execution mode, authorization, and policy

- Completed mode: **single-chunk**, Chunk 6 only. The prior explicit Chunk 6 request did not authorize later work.
- The current user explicitly authorizes the two Chunk 6→7 handoff documents. **No Chunk 7 implementation authorization is granted by this request or these documents.**
- The controlling plan §7 sets the default failed-validation budget at **two failed validation attempts per issue**. Chunk 6 had one failed test invocation for a resolved JSONB empty-snapshot key-order expectation; the expected serialization was corrected and the complete race selector then passed. No Chunk 6 failure remains unresolved. Do not reset a repeated issue merely because a session changes.
- Chunk 6 used isolated Compose project `labreserve-task004b-c6-ac5f2b8b`. Current inspection found its `verify-db` container exited and its project-labelled `verify-data` volume retained. No volume was removed. Treat the volume as retained data; a future worker must independently revalidate project/volume ownership and the test database before reuse. Do not assume the standard `labreserve-verify` project is safe: historical handoffs differ on its status/ownership, and it was not revalidated here.
- No persistent/demo database, credentials, or `.env` contents were accessed. No migrations, tests, or services were run during this handoff authoring.

## 3. Completed Chunk 6 — observed scope

Chunk 6 — **Booking creation versus deactivation in both lock orders** — added only `internal/database/resource_state_booking_concurrency_integration_test.go` (1029 lines; full current file inspected). It adds five independent real-PostgreSQL race tests:

1. `TestResourceStateBookingRaceBookingCommitsBeforeDeactivation`
2. `TestResourceStateBookingRaceDeactivationCommitsBeforeBooking`
3. `TestResourceStateBookingRaceSameCoordinatorAccountActivityLockCompatibility`
4. `TestResourceStateBookingRaceFailedBookingRollsBackBeforeDeactivation`
5. `TestResourceStateBookingRaceFailedDeactivationRollsBackBeforeBooking`

The tests wrap the existing booking/state transactions with bounded gates, capture runtime backend PIDs and require `current_user = 'labreserve_app'`. They query PostgreSQL's blocking-PID/lock graph, verify the installed Account `FOR NO KEY UPDATE` and Resource `FOR UPDATE` primitives, and assert persisted Resource, Booking/request, and Activity outcomes. Scoped Activity-failure triggers and test operations have bounded cleanup. No production lock/service code, migration, route, or dependency changed.

The existing Task 2B regressions additionally cover the five-minute post-Resource-lock time check and retained replay/reuse before current Resource eligibility; those were run as part of Chunk 6 evidence below.

## 4. Verification actually executed

The following commands/results are from the prior Chunk 6 implementation session, not rerun while writing this handoff. Current source hashes and Git state were rechecked; the current test file has the same SHA-256 as the tested source. Compose output contained no skipped selected tests.

| Exact command/check | Observed result | Limits |
| --- | --- | --- |
| `rtk docker compose --project-name labreserve-task004b-c6-ac5f2b8b --file compose.yaml --profile verify build verify-go` | **PASS**; rebuilt the verification image from the candidate. | Does not itself prove test behavior. |
| `rtk docker compose --project-name labreserve-task004b-c6-ac5f2b8b --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceStateBookingRace' ./internal/database` | **PASS**; discovered exactly the five test names listed above. | Discovery only. |
| Same Compose prefix, ending `go test -list '^(TestBookingCreationValidatesTimeAfterResourceLockWait|TestBookingCreationReplaySurvivesPastAndCancelledInactiveRecords|TestBookingCreationChangedDataWinsBeforeResourceEligibility|TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability)$' ./internal/database` | **PASS**; all four names were discovered. | Discovery only. |
| Same Compose prefix, ending `go test -count=1 -v -p 1 -run '^TestResourceStateBookingRace' ./internal/database` | **PASS**; all five race tests executed without skips. Per-schedule wait evidence is recorded below. | `wait_event_type`/`wait_event` fields were empty in emitted logs; actual `pg_blocking_pids` edges and ungranted `transactionid/ShareLock` records were present and asserted. |
| Same Compose prefix, ending `go test -count=1 -v -p 1 -run '^(TestBookingCreationValidatesTimeAfterResourceLockWait|TestBookingCreationReplaySurvivesPastAndCancelledInactiveRecords|TestBookingCreationChangedDataWinsBeforeResourceEligibility|TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability)$' ./internal/database` | **PASS**; all four existing regression tests executed without skips. | Does not replace the full delivery gate. |
| `rtk docker compose --project-name labreserve-task004b-c6-ac5f2b8b --file compose.yaml --profile verify run --rm --no-deps verify-go sh -ec 'go test -run "^$" ./... && go build -o /tmp/labreserve ./cmd/labreserve && test -s /tmp/labreserve && test -z "$(gofmt -l internal/database/resource_state_booking_concurrency_integration_test.go)"'` | **PASS**; all packages compiled with no tests selected, application binary built, changed file formatted. | Compile-only is not behavior evidence. |
| `rtk git diff --check`; `rtk git diff --no-index --check /dev/null internal/database/resource_state_booking_concurrency_integration_test.go` | **PASS/no diagnostics** for whitespace. | The no-index command returns the normal “files differ” status for an untracked file; no whitespace diagnostic was emitted. |
| Current `rtk git status --porcelain=v2 --branch`, `rtk git show --stat HEAD`, and SHA-256 checks | **PASS** for identity/scope audit: branch equals upstream; clean before the two authorized docs were written; current HEAD delta from `ac5f2b8…` is the test and the two Chunk 5→6 docs only. | This handoff pair will appear as two new untracked documents; do not stage them. |
| `rtk docker compose ... ps --all` and project-labelled volume listing for `labreserve-task004b-c6-ac5f2b8` | **PASS** observation: verification DB container is exited; named `verify-data` volume remains. | No DB state or volume contents were inspected. |
| `rtk make verify`, browser/Playwright, lifecycle/restart, and delivery-wide migration verification | **NOT_RUN**, as required for Chunk 6. | Reserved by the plan for Chunk 11. |

### Forced schedule evidence

The successful verbose race run reported these actual PostgreSQL wait edges. Each edge was observed before releasing the named leader; the test uses `pg_blocking_pids` plus an ungranted lock record, not timeout absence alone.

| Schedule | Observed edge | Persisted/result evidence after release |
| --- | --- | --- |
| Booking creator held inserted Booking and `booking.created` before commit; deactivation waited. | waiter PID **2619** → creator PID **2620**, `transactionid/ShareLock`. | Creator committed one Booking and one creation event. Deactivation returned `ErrResourceOccupied`; Resource/history snapshot stayed unchanged and no deactivation event was persisted. |
| Deactivation held state mutation and Activity before commit; new creator waited. | waiter PID **2623** → state PID **2624**, `transactionid/ShareLock`. | State transition/event committed; creator returned `ErrInactiveResource`. Booking/request snapshot and counts remained empty; persisted Resource was Inactive. |
| Same Coordinator was state actor and booking creator; creator acquired Account lock then waited for state-held Resource. | creator waiter PID **2627** → state PID **2628**, `transactionid/ShareLock`; the same edge remained after the state Activity reached its pre-commit barrier. Installed lock functions were inspected for Account `FOR NO KEY UPDATE` and Resource `FOR UPDATE`. | State event completed while creator still waited; after state commit creator returned inactive. No Booking or request binding persisted; exactly the deactivation state/event remained. |
| Booking insert was held before an injected `booking.created` Activity failure; deactivation waited. | waiter PID **2632** → creator PID **2631**, `transactionid/ShareLock`. | Booking transaction failed operationally and rolled back; waiting deactivation succeeded. No Booking/request/event from the failed attempt remained; deactivation state/event persisted. |
| Deactivation was held before its guarded mutation with a scoped Activity failure; valid creator waited. | waiter PID **2635** → state PID **2636**, `transactionid/ShareLock`. | Deactivation failed operationally and rolled back; creator succeeded. Resource remained Active, with one Booking and one creation event and no partial deactivation. |

The existing regression run separately passed: `TestBookingCreationValidatesTimeAfterResourceLockWait` (fresh clock after Resource wait), `TestBookingCreationReplaySurvivesPastAndCancelledInactiveRecords` (retained Confirmed/Cancelled replay before inactive eligibility), `TestBookingCreationChangedDataWinsBeforeResourceEligibility` (changed-key reuse before Resource eligibility/lock), and `TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability`.

### Failed attempt and repair

One initial `^TestResourceStateBookingRace` invocation had three test failures, all the same test-only issue: PostgreSQL `jsonb_build_object(... )::text` serializes the empty snapshot keys as `{"events": [], "bookings": []}`, while three assertions expected the reverse key order. The assertions were corrected to the observed canonical output. The full five-test selector was rerun and passed. **Budget accounting:** one failed validation attempt for this resolved issue, zero unresolved Chunk 6 failures.

## 5. Diff, preservation, and source audit

- The entire current Chunk 6 test file and both preceding handoff files were read. Current committed delta from Chunk 5 base `ac5f2b8…` contains three files: `internal/database/resource_state_booking_concurrency_integration_test.go`, `docs/handoffs/task-004b-chunk-5-to-6.md`, and `docs/handoffs/instruction-chunk-6-task-004b-atomic-resource-activation-deactivation.md`.
- No production source or migration changed. Migration 006 hash is unchanged. Task and plan are tracked and committed with matching current hashes.
- At this handoff's start the repository was clean and branch HEAD equaled upstream. The only files this handoff authoring adds are this evidence file and its paired instruction; they remain untracked/unstaged. No other work is removed or rewritten.
- Chunk 6 preserves the accepted Account → replay → Resource booking-creation order and state Resource lock protocol. Its PostgreSQL evidence does not prove Task 4B as a whole or substitute for an independent review.

## 6. Risks, gaps, and decisions

- **No reproduced production defect or Level 2 decision** remains from Chunk 6. A future production lock, authority, or semantic change is outside Chunk 7's test-only scope and requires stopping/escalating under the contract.
- **Wait observation limit:** emitted `wait_event_type` and `wait_event` were blank; `pg_blocking_pids` and the actual ungranted transaction lock established the wait edges. Preserve that limitation rather than claiming a specific `pg_stat_activity.wait_event` value.
- **Verification limits:** no full `make verify`, browser/Playwright, migration-upgrade, or lifecycle/restart checks ran; the plan reserves full delivery verification for Chunk 11.
- **Review:** Chunk 6 independent review is not recorded. It is explicitly not a gate to preparing/authorizing a non-final next chunk under the current user instruction; later review/Task 4B acceptance remain outstanding.
- **Environment:** Chunk 6's unique verification database container is stopped and the named volume is retained. Do not delete it, inspect secrets, or assume any other worker's Compose project is available.
- **Rollback:** no code rollback is needed. Preserve current committed candidate and retained test volume. The two documents created now are authorized handoff artifacts only.

## 7. Exact next boundary

The controlling plan's next section is **Chunk 7 — Owner cancellation versus deactivation** (V5 cancellation schedules/V7 preservation; FR-019–025/027; AC-008–010/025/036–039). It is test-only and proposes `internal/database/resource_state_cancellation_concurrency_integration_test.go`, with a narrowly scoped helper extension only if necessary.

Next-session prerequisites and exact schedules are in the paired prepared instruction. In brief: force cancellation-before-deactivation (including a second remaining blocker), deactivation rejection-before-cancellation followed by explicit resubmission, cancellation waiting across exact start, and cancellation-Activity rollback with a waiting deactivation. Preserve owner-only cancellation, Resource → Booking locking, fresh cancellation time, and actual PostgreSQL wait evidence.

The next planned chunk is **not authorized by this handoff**. Obtain fresh explicit Chunk 7 authorization and perform its own base/source/environment preflight. Do not begin Chunk 8 or any later work.

## Suggested skills

- `chunked-implementation` — SINGLE-CHUNK mode only, after separate Chunk 7 authorization and preflight.
- `specification-readiness-authority` — if source, candidate, or authority differs at preflight.
- `pre-read-discipline`, `pre-edit-discipline`, `post-edit-discipline`, and `rtk-command-prefix` — for the authorized implementation session.
- `handoff` — only for a later separately authorized transfer.

**Chunk 6 status: `CONTINUABLE`.**
