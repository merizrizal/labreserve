# Task 4A — Chunk 7 lifecycle, documentation, and delivery-wide audit

## Candidate and scope

- Implementation base / current `HEAD`: `a90d3fd37226b5a648153edf7d134ec605a78dc0` on `feat/004a-resource-creation-editing`.
- Candidate remains uncommitted; index is empty. Tracked changes: `README.md`, `internal/database/booking_persistence_integration_test.go`, `internal/database/migrate.go`, `internal/web/foundation_integration_test.go`, `scripts/verify.sh`. Untracked Task 4A work: `docs/handoffs/task-004a-chunk-3-to-4.md`, `task-004a-chunk-4-to-5.md`, `task-004a-chunk-5-to-6.md`, `task-004a-chunk-6-to-7.md`, this handoff; `internal/database/migrations/005_resource_mutations.sql`; `internal/database/resource_concurrency_integration_test.go`, `resource_creation.go`, `resource_creation_integration_test.go`, `resource_edit_integration_test.go`, `resource_lifecycle_integration_test.go`, `resource_mutations_integration_test.go`; and `internal/resources/service.go`, `service_creation_test.go`, `service_edit_test.go`, `service_test.go`.
- No staged work was present or changed. No commit, push, merge, deployment, credential modification, or Task 4B/4C work was performed. The verification script used only its isolated disposable Compose project; its cleanup trap removed that project's volume.

## Chunk 7 work

- Added `internal/database/resource_lifecycle_integration_test.go` and extended `scripts/verify.sh` for actual PostgreSQL/application restarts, repeat seeding after application creation and seeded-Resource editing, retained Resource/Booking identities and histories, post-restart no-op and duplicate behavior, Booking creation/cancellation replay, and runtime capability checks.
- Updated `README.md` for the internal-only Task 4A service, migration 005, explicit-upgrade/recovery ordering, guarded runtime capabilities, the existing cancellation limitation, verification coverage, and still-deferred management browser/state workflows.
- Final V1–V9 audit tightened evidence assertions: direct-capability rejections compare persisted Resource/Activity snapshots; duplicate creation is checked against retained Booking/history; the existing Resource list asserts Inactive state; and descriptive edit retries successfully after removing an Activity-failure trigger. The updated tests passed the final real-PostgreSQL repository gate.

## V1–V9 evidence audit

| Criterion | Candidate coverage / evidence state |
| --- | --- |
| **V1 Trusted authorization** | Service tests cover missing/unknown actors, both Engineers, authorization before unchanged edits, and stored-role resolution; database tests call guarded capabilities with Engineer/unknown IDs and verify unchanged Resource/Activity snapshots; web integration resolves the Coordinator from a trusted session and exercises revoked-session rejection. Direct calls only check stored role and do not independently authenticate a supplied Coordinator ID; README states this boundary. Real-PostgreSQL tests passed in the final gate. |
| **V2 Canonicalization and validation** | Unit and PostgreSQL tests cover Unicode edge whitespace, casing/internal whitespace, empty descriptions, composed/decomposed text, invalid UTF-8 at the service boundary, long text, edit clearing/required name, both states, and invalid direct-capability inputs. Existing list assertions include the Inactive state. Real-PostgreSQL tests passed in the final gate. |
| **V3 Duplicate code/concurrency** | Service and guarded-capability tests cover exact/case variants; PostgreSQL concurrency tests cover identical, case, Unicode-whitespace variants, and an Activity failure releasing a blocked contender. Existing index uniqueness tests remain. Duplicate-service coverage snapshots a retained Booking and its Activity. All passed in the final real-PostgreSQL gate. |
| **V4 Immutable code/no-op** | Service rejects changed, existing, and case-only code assertions before mutation. Runtime UPDATE denial is tested. Scoped UPDATE and Activity-failure triggers detect no-op writes/events; concurrent equivalent edits test one mutation and one unchanged result. |
| **V5 Activity/time/atomicity** | Creation/edit event detail, locked before-values, timestamps, rollback, outer transaction rollback, lock serialization, Booking creation/cancellation compatibility, and commit-acknowledgement non-retry are covered. The direct edit Activity-failure retry assertion passed in the final real-PostgreSQL gate. The commit-acknowledgement case is a service transaction test seam; no transport-level ambiguous-commit integration fault was available, and no outcome is described as definite rollback. |
| **V6 Identity/history/read paths** | Real PostgreSQL tests cover Active/Inactive service creation and list state, booking eligibility, future/in-use/past/Cancelled retained Bookings, replay, and unchanged Booking/event snapshots. Web/PostgreSQL tests cover service-created HTML-like text in existing list, schedule/detail, and My Bookings views with contextual escaping. The duplicate-Booking snapshot and inactive-list assertions passed in the final gate. |
| **V7 Runtime privileges** | Actual `labreserve_app` login tests inspect Resource table/column/function/schema/role ACLs, SECURITY DEFINER owner/search path/PUBLIC and other grants, shadow objects, denied direct writes/DDL/alter/role assumption, and preserved Booking cancellation-column and Activity boundaries. Existing foundation/Booking tests retain broader regression checks. |
| **V8 Migration/seed/restart** | Migration-ledger tests cover missing, unknown, and checksum-mismatched versions; migration-004 data (Resource, Booking request identity, Activity, session) is preserved by the migration simulation; repeat seed preserves state/history. The final successful `make verify` exercised actual PostgreSQL and application restarts on the isolated volume, including Resource replay/capability checks. |
| **V9 Repository regression gate** | The final `rtk make verify` passed on the current candidate, including Go/real-PostgreSQL suites, all 8 Playwright tests, and PostgreSQL/application restart plus replay checks. No database/browser suite was silently skipped. |

Historical retry records remain unchanged: Chunk 4 import-cycle failures 3/5; Chunk 5 precision failure 1/2; Chunk 6 compile failure was repaired.

## Verification in this session

- `TMPDIR=/home/meriz/.cache/labreserve-tmp rtk go test ./internal/resources ./internal/database` — passed (41 tests reported by RTK; database integration tests are skipped in this host invocation without the test database URLs).
- `TMPDIR=/home/meriz/.cache/labreserve-tmp rtk go test ./...` — passed, 115 tests in 8 packages; this host run is supplemental, not the PostgreSQL/browser evidence.
- `TMPDIR=/home/meriz/.cache/labreserve-tmp rtk go vet ./...` — passed.
- Go formatting, `rtk sh -n scripts/verify.sh`, and `rtk git diff --check` — passed. The repository gate also checked formatting across `cmd` and `internal`.
- `TMPDIR=/home/meriz/.cache/labreserve-tmp rtk make verify` — **passed on the current candidate**. All Go packages, including `internal/database` with real PostgreSQL, passed; the app started with the restricted runtime role; Playwright passed **8/8**; Resource and owner-cancellation fixtures survived actual PostgreSQL/application restarts; post-restart Resource capability/no-op/duplicate checks and Booking creation/cancellation replay checks passed. Script ended `Repository verification passed.`

## Final review and remaining boundaries

`/tmp` was cleared before the final run; `df -h /tmp` showed 32 GB available. No unrelated archive was moved or deleted. I inspected the complete tracked diff, all Task 4A untracked implementation/test files and handoffs, including the final V1–V9 audit. The index remains empty; all candidate files are uncommitted on the stated base. `git diff --check` is clean. No unrelated changes were removed, and no Task 4B/4C functionality or scaffolding was added.

The only remaining limitations are the approved scope boundaries and the absence of a separate fresh-session independent review. Database role checks do not independently authenticate an actor ID supplied via arbitrary runtime SQL; the trusted application/session boundary remains authoritative. A transport-level ambiguous-commit fault was unavailable; the service maps commit acknowledgement failure to an operational result and does not retry. No independent review, final human acceptance, commit, merge, or deployment is claimed.

## Meaningful engineering choices retained

- PostgreSQL creation-race observation follows recursive blocker chains because unique-key waiters may not directly block on the Activity lock.
- Edit time is sampled only after the Resource lock; canonical no-ops avoid Resource UPDATE and Activity. Stored actor role is checked in the service and again in guarded SQL, while session identity remains trusted application input.
- Account deactivation semantics were not invented because the model has no disabled state. Existing cancellation-column privilege limitations were documented, not broadened or redesigned.
- Task 4A remains an internal service/persistence capability. Resource-management browser workflows and existing-Resource activation/deactivation remain deferred.

**Current terminal status: READY_FOR_REVIEW** — the approved Task 4A scope and V1–V9 verification are complete on this candidate. Independent review must occur in a separate fresh agent session; human final acceptance and integration remain outstanding.
