# Task 4A handoff — Chunk 5 complete; Chunk 6 may continue

## Authority and candidate state

- **Task:** Task 4A — Atomic Resource Creation and Descriptive Editing.
- **Execution mode:** Single-chunk; this session executed Chunk 5 only and stops before Chunk 6.
- **Implementation base / HEAD:** `a90d3fd37226b5a648153edf7d134ec605a78dc0`.
- **Branch:** `feat/004a-resource-creation-editing`.
- **Approval/preflight:** The current user explicitly authorized continuation of approved Task 4A and identified the formal `PREFLIGHT_PASS` for this exact base. Task/architecture sources are tracked at the base and unmodified. Their historical draft-status wording is superseded for this execution by the explicit authorization and prior preflight evidence, as recorded in the preceding handoff.
- **Candidate:** Uncommitted; HEAD remains the base. Chunks 2–4 changes remain staged and preserved. Chunk 5 edits are unstaged in files that were already staged as additions, plus two new untracked test files and this handoff. No staging, commit, push, merge, or deployment was performed.
- **Resolved policy:** Single-chunk, explicit user authorization for this handoff, local disposable verification environment, two failed validation attempts for a new current-chunk issue. Historical Chunk 4 import-cycle failures remain **3/5**; Chunk 5 had one failed `make verify` attempt for a distinct timestamp-test expectation, repaired and verified. No budget was reset.

## Controlling implementation ladder

Continue the existing Task 4A ladder; do not create a competing plan:

1. Chunks 1–3 — contracts/canonical validation and guarded Migration 005 capabilities: complete before Chunk 4.
2. Chunk 4 — Atomic Resource creation: complete; prior handoff and verification preserved.
3. Chunk 5 — Atomic Resource descriptive editing: **complete and verified**.
4. Chunk 6 — Concurrency, authorization, and privilege verification: **next; not started**.
5. Chunk 7 — Lifecycle, documentation, and delivery-wide verification: later; not started.

Do not implement Task 4B/4C, Resource activation/deactivation, HTTP/browser management, coordinator cross-owner cancellation, history browsing, deletion, or any other excluded work.

## Chunk 5 implementation

- Added `Service.Edit` with explicit changed/no-op result and the existing typed outcomes. It authorizes from the trusted actor account ID and persisted Coordinator role before validation, Resource lookup, or successful no-op. It does not accept a role claim.
- Reused the existing Unicode-aware canonical text helpers: code assertion and name are required/trimmed; description is optional/trimmed and may be cleared. No arbitrary length limit, case rewrite, or Unicode normalization was added.
- The edit transaction uses READ COMMITTED and the established `public.labreserve_lock_booking_resource` helper. It reads code/name/description/state/identity/timestamps after locking, then compares the canonical immutable-code assertion against that authoritative record. Different and case-only code assertions return `ErrImmutableCode` without calling the mutation capability.
- The service invokes Migration 005's existing `public.labreserve_edit_resource` capability. No migration change, direct Resource UPDATE, second Activity path, or broader privilege was added. The capability rechecks Coordinator role, takes/reuses the Resource lock, derives before-values from the persisted row, suppresses no-op UPDATE/event, and couples actual descriptive UPDATE plus one `resource.descriptive_edited` event atomically.
- The operation time is captured after the Resource lock and canonicalized to UTC microseconds. Actual event time equals `updated_at`; no-op preserves all Resource fields and timestamp.
- Actual edits preserve Resource ID, exact code, active state, creation time, and retained Booking/request data. Existing `ResourceByID` and `ListResources` expose the edited text.

## Chunk 5 files changed

- `internal/resources/service.go` — editing contracts, combined repository interface, UUID validation reuse, and Coordinator-only edit use case.
- `internal/database/resource_creation.go` — READ COMMITTED edit transaction, shared Resource lock/read, guarded Migration 005 call, and typed error mapping.
- `internal/resources/service_creation_test.go` — creation fake now implements the combined repository contract.
- `internal/resources/service_test.go` — no-limit long-text edit canonicalization coverage.
- `internal/resources/service_edit_test.go` — new service tests for authorization ordering, canonical actual edit, immutable-code rejection, invalid/unknown outcomes, and no-op.
- `internal/database/resource_edit_integration_test.go` — new real-PostgreSQL tests for active/inactive edits, name/description/clear, immutable code, Engineer/invalid/unknown rejection, no-op UPDATE detector and timestamp/event preservation, Activity failure and outer-transaction rollback, event details/time/actor/target, Booking retention/replay, and existing Resource reads.
- `docs/handoffs/task-004a-chunk-5-to-6.md` — this handoff.

No authoritative PRD, baseline, architecture, task contract, Migration 005, verification script, or unrelated implementation file was modified in Chunk 5.

## Verification actually run

- Baseline `rtk go test ./internal/resources` — passed, 28 tests reported.
- Baseline `rtk make verify` — passed before Chunk 5 edits; real PostgreSQL, existing Go suites, Playwright 8/8, and existing restart checks ran.
- `rtk gofmt -w` on changed Go files and `rtk git diff --check` — passed after implementation and again after test updates.
- Final `rtk go test ./internal/resources` — passed, 39 tests reported.
- Host `rtk go test ./internal/database` — package passed/compiled, but database integration tests skipped because `LABRESERVE_TEST_DATABASE_URL` is not configured. `testPool` explicitly skips in that case; this was not treated as PostgreSQL evidence.
- Final host `rtk go test ./...` — passed, 114 tests across 8 packages; host-only run does not establish PostgreSQL behavior.
- Final `rtk make verify` — passed. Its isolated disposable stack applied Migration 005 and ran the Go suite against real PostgreSQL with restricted runtime/migrator roles, all database integration tests, Playwright **8/8**, and existing app/database restart checks. Output ended `Repository verification passed.`
- The first post-edit `rtk make verify` failed only because the new test expected nanosecond input time while the approved service contract stores UTC microseconds. The test expectation was corrected to truncate to microseconds; focused tests and three subsequent full `make verify` runs passed. This used **1/2** failed attempts for that Chunk 5 issue.

## Required-behavior evidence in this chunk

- Coordinator name-only edit, description edit, and description clear: real PostgreSQL integration test.
- Engineer rejection, invalid name, different/existing/case-only code assertions, unknown Resource, and unauthorized inaccessible/no-op request: service unit and PostgreSQL tests; rejected cases preserve persisted state/history.
- Active and Inactive Resource descriptive changes preserve active state, identity, code, and creation time.
- Canonical no-op is exercised through the service while PostgreSQL UPDATE and Activity failure triggers are installed; succeeds without UPDATE, timestamp rewrite, or event.
- Actual edit produces exactly one matching Resource Activity event with correct actor, target, action, UTC timestamp, and authoritative before/after values. Injected Activity failure rolls back the Resource update.
- A confirmed Booking, its request ID, owner, interval, canonical purpose, state and creation identity remain unchanged; original creation replay returns the same Booking after editing.
- The new `TestResourceEditingOuterRollbackRestoresResourceAndActivity` invokes the guarded edit inside an outer runtime transaction, verifies the uncommitted event is visible there, rolls back, and confirms persisted Resource values/timestamp and the original creation event remain unchanged. Existing Migration 005 integration coverage, run by `make verify`, additionally checks creation outer rollback, atomic Activity failure, direct runtime Resource-write denial, guarded capability ownership/search path/EXECUTE, and effective privileges.

## Remaining Task 4A verification gaps

These are not Chunk 5 blockers; retain them for their assigned later chunks and do not claim Task 4A `READY_FOR_REVIEW`:

- **Chunk 6:** independent concurrent Resource creation; competing/identical edits and no-op serialization; fresh clock and authoritative before-values after a deterministic lock wait; edit interactions with booking creation and owner cancellation; remaining Resource-specific trusted-session/service seam and broader authorization/privilege checks. Preserve the agreed Resource → Booking and Account → Resource protocols; do not add activation/deactivation.
- **Broader V6:** current edit test retains and replays a Confirmed Booking. Extend as assigned to cover relevant Cancelled/past/in-use histories, wider read-path visibility, and Active/Inactive Resource booking eligibility without changing Booking behavior.
- **Chunk 7:** Resource-specific seed/restart durability, migration/lifecycle and README evidence; complete Task 4A V1–V9 coverage audit and final full-diff verification. Existing `make verify` restart checks are not Resource-specific restart evidence.
- Existing code-only runtime privilege restrictions and Activity-coupling tests passed; reassess the required effective-permission matrix in Chunk 6 rather than weakening prior assertions.

## Risk assessment

- **Functional:** Normal authorized edits, canonical no-op, immutable code, event detail/atomicity, and Booking retention passed unit and real-PostgreSQL evidence.
- **Concurrency:** The implementation holds the approved Resource lock through the guarded operation, but adversarial lock-wait/competing-edit and cross-operation concurrency tests remain for Chunk 6.
- **Security:** Stored Coordinator role is checked in the service and again in the guarded capability. Database role validation does not authenticate a supplied account ID; the existing trusted application/session boundary remains authoritative. No broader privilege was granted.
- **Integration:** No HTTP/browser Resource-management behavior was added; it remains explicitly out of scope for Task 4A.
- **Rollback:** Do not reset or clean the repository. Preserve all staged Chunk 2–4 work. If only Chunk 5 needs reverting, use its unstaged diff/new-file inventory as the boundary; do not discard the index or prior handoffs.

## Exact next instructions

For the next authorized session, execute **Chunk 6 — Concurrency, authorization, and privilege verification only**:

1. Read this complete handoff and the complete Task 4A contract; verify the branch/base and inventory without resetting, cleaning, staging, or overwriting any existing work.
2. Reuse the existing Resource service, Migration 005 operations, lock helpers, fixtures, and real-PostgreSQL verification infrastructure. Do not create a competing ladder or modify approved requirements.
3. Address the Task 4A Chunk 6 responsibilities and the verification gaps listed above with deterministic independent database operations. Keep activation/deactivation, browser workflows, lifecycle/documentation, and Chunk 7 out of scope.
4. Run focused checks and `rtk make verify`, inspect the complete candidate diff/inventory, record actual outcomes and remaining Task 4A gaps, and create the next handoff only if the Chunk 6 execution instruction authorizes/requires it.
5. Stop without beginning Chunk 7.

**Chunk 5 status: CONTINUABLE.** This does not mean Task 4A is `READY_FOR_REVIEW`.
