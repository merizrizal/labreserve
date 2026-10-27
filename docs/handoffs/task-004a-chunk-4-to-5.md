# Task 4A handoff — Chunk 4 complete; Chunk 5 may continue

## Authority and candidate state

- **Task:** Task 4A — Atomic Resource Creation and Descriptive Editing.
- **Execution mode:** Single-chunk recovery; this session repaired and verified Chunk 4 only. Chunk 5 was not started.
- **Implementation base:** `a90d3fd37226b5a648153edf7d134ec605a78dc0`.
- **Branch/HEAD:** `feat/004a-resource-creation-editing` at the approved base; no commit was made.
- **Authority/freshness:** The current user explicitly authorized the recovery and two additional failed validation attempts. HEAD, branch, tracked authoritative specifications, and approval remain applicable; specification files are unchanged. The historical draft wording in Task 4A/ADR 004 is superseded for this implementation by the user's explicit approval and prior preflight confirmation for this exact base.
- **Candidate:** Uncommitted working tree. All Chunk 2/3 work was preserved. Chunk 4 is `CONTINUABLE`; this does **not** mean Task 4A is `READY_FOR_REVIEW`.
- **Validation budget:** Three failures for the original compile issue were reported before recovery. The authorized cumulative cap is five. Recovery consumed **zero additional failed attempts**; cumulative count is **3/5**. No counter was reset or extended.

## Controlling ladder

Use the existing Task 4A ladder; do not create a competing plan:

1. Chunks 1–3 — complete before this session.
2. Chunk 4 — Atomic Resource creation — **complete and verified**.
3. Chunk 5 — Atomic descriptive editing — next; not started here.
4. Chunk 6 — concurrency, authorization, and privilege verification.
5. Chunk 7 — lifecycle, documentation, and delivery-wide verification.

No Task 4B/4C, existing-Resource activation/deactivation, HTTP/browser management, cross-owner cancellation, Activity-history UI, or deletion was added.

## Import-cycle diagnosis and repair

- **Exact file:** `internal/database/resource_creation_integration_test.go`.
- **Declared package:** `package database`.
- **Cause:** It retained an import of `labreserve.local/labreserve/internal/auth` after the session-helper code that had used it was removed. `internal/auth/service.go` imports `internal/database`; importing `auth` from a same-package `database` test therefore formed the test-time cycle `database -> auth -> database`. `rg` confirmed the auth path appeared only in the unused import.
- **Same-package dependencies:** The integration test uses unexported database-test helpers/types including `testPool`, `newResourceCapabilityFixtures`, `resourceCapabilityFixtures`, `testDatabaseUUID`, and `installResourceActivityFailureTrigger`.
- **External-package option:** Converting the entire file to `database_test` would make those helpers inaccessible, requiring duplication or exporting test helpers. That is not the smaller correction.
- **Correction:** Removed only the stale `internal/auth` import. The test remains a same-package database integration test, retains all PostgreSQL coverage, and adds no production dependency, abstraction, or exported helper.
- **Package-boundary result:** `resources` does not import `database`; the adapter continues to import the resource contract. No production package dependency changed.

## Chunk 4 implementation present

Current implementation (from the preceding Chunk 4 session):

- `internal/resources/service.go` — creation service takes the trusted application `Actor.AccountID` and no caller-supplied role claim; checks the stored Coordinator role in the transaction; canonicalizes required code/name and optional description; preserves explicit Active/Inactive state; captures UTC microsecond operation time; returns typed outcomes; commits once and does not retry an ambiguous commit.
- `internal/database/resource_creation.go` — `Store` adapter opens one READ COMMITTED transaction and calls only `public.labreserve_create_resource`. It maps only SQLSTATE `23505` for `resources_code_case_insensitive_uq` to duplicate code and does not append a second Activity event.
- `internal/resources/service_creation_test.go` — unit coverage for canonical values, both initial states, authorization, required values, duplicate mapping, and no blind retry after commit acknowledgement failure.
- `internal/database/resource_creation_integration_test.go` — real-PostgreSQL service coverage for coordinator creation (both states), Engineer rejection without mutation, invalid required code/name, canonical persistence and empty description, case-insensitive duplicate handling, exactly one accurate creation Activity, Activity-trigger rollback, and visibility through `ResourceByID` and `ListResources`.
- Existing `internal/database/migrations/005_resource_mutations.sql` remains the guarded atomic write boundary. It generates identity, explicitly persists the chosen state and timestamps, and couples exactly one creation Activity event to the Resource insert. No Go-side Activity insert or direct Resource write privilege was added.

## Verification actually run

Commands were run in the requested order after the import correction:

1. `rtk gofmt -w internal/database/resource_creation_integration_test.go` plus `rtk gofmt -d` for the changed Go implementation/test files, and `rtk git diff --check` — passed; formatter diff and whitespace check were clean.
2. `rtk go test ./internal/database` — passed/compiled on the host. Host `LABRESERVE_TEST_DATABASE_URL` and `LABRESERVE_TEST_APP_DATABASE_URL` are unset, so this command alone did not execute PostgreSQL integration tests.
3. `rtk go test ./internal/resources` — passed; 28 tests.
4. `rtk go test ./...` — passed; 103 tests across 8 packages. The host lacks database test URLs, so PostgreSQL integration evidence comes from the next command, not this host run.
5. `rtk make verify` — passed. The isolated verification stack applied migration 005 and ran the Go suite with real PostgreSQL as migrator/runtime roles, including `internal/database` (package passed), the application, Playwright (8/8 passed), and database/application restart checks. The script ended with `Repository verification passed.`

No failed validations occurred in this recovery. The three earlier import-cycle failures remain counted; total is 3/5. The two newly authorized failed attempts remain unused.

## Required Chunk 4 behavior evidence

Observed through the new service/PostgreSQL tests executed by `make verify`:

- Coordinator service calls create Active and Inactive Resources; `false` remains Inactive.
- Stored Engineer role is rejected before persistence; rejected creation leaves no Resource or success event.
- Empty/Unicode-whitespace-only required code/name reject; surrounding Unicode whitespace is trimmed, internal text/casing/composition is preserved, and empty description is accepted.
- A case-insensitive duplicate maps to the typed duplicate result; the original Resource remains unchanged and no duplicate event is added.
- Successful create persists generated identity, canonical values, explicit initial state, and operation timestamps. Exactly one `resource.created` Activity event has the Coordinator actor, Resource target only, timestamp equal to Resource timestamps, and matching code/name/description/active details.
- Injected Activity insertion failure returns operational failure and leaves no Resource or successful creation event.
- The created Resource is visible through existing Resource read-by-ID and list boundaries.
- Existing Migration 005 PostgreSQL tests, run by `make verify`, retain direct runtime privilege-denial checks, function ownership/search-path/EXECUTE checks, event failure rollback, and additive migration coverage. No new direct Resource write privilege was introduced.

## Changed-file inventory and diff review

The full working-tree inventory at review was:

- Modified tracked files from Chunk 3: `internal/database/booking_persistence_integration_test.go`, `internal/database/migrate.go`, `scripts/verify.sh`.
- Untracked Chunk 2 file preserved: `internal/resources/service_test.go`.
- Untracked Chunk 3 files preserved: `internal/database/migrations/005_resource_mutations.sql`, `internal/database/resource_mutations_integration_test.go`, `docs/handoffs/task-004a-chunk-3-to-4.md`.
- Chunk 4 files: `internal/resources/service.go` (the existing untracked Chunk 2 contract file extended with creation service logic), `internal/database/resource_creation.go`, `internal/database/resource_creation_integration_test.go`, `internal/resources/service_creation_test.go`.
- Handoff: `docs/handoffs/task-004a-chunk-4-to-5.md`.

The tracked diff was reviewed in full: migration version moves to 005; verification ledger expectations move from four to five; the isolated foundation migration test is bounded through 004 and the separate migration-005 test preserves prior data. New files were read/reviewed, including the migration, guarded-capability tests, service, adapter, and creation tests. The recovery changed only the import list in `internal/database/resource_creation_integration_test.go`. No unrelated working-tree changes were discarded or overwritten. No Chunk 5 behavior is present.

## Remaining Task 4A verification and risks

Chunk 4 completion does not establish complete Task 4A acceptance. Later obligations include:

- Chunk 5 descriptive-edit service behavior, immutable-code rejection, locked before-values, no-op-without-UPDATE, serialization, timestamp, and rollback coverage.
- Chunk 6 independent concurrent creation and event-failure-waiter behavior, plus broader authorization/session seam and privilege verification.
- Task 4A V6 compatibility checks for active/inactive Resource booking behavior and retained Booking/request/history compatibility, as assigned by the controlling ladder.
- Task 4A V8 Resource creation/edit retention through reseeding and actual app/database restart; the current `make verify` restart checks cover the existing lifecycle, not Resource-specific restart fixtures.
- Final README/implementation notes, complete Task 4A V1–V9 coverage, final full-diff review, and separate independent review.

**Functional risk:** ordinary creation, duplicate rejection, explicit state, Activity details/count, and rollback passed real-PostgreSQL verification. No Resource creation replay protocol was added.

**Integration risk:** no Resource HTTP/browser route is in scope. The service relies on the accepted trusted application actor boundary; it accepts only account identity, while Coordinator authority is loaded from stored role and rechecked by SQL.

**Validation gap:** host-only `go test` commands skip DB integration without URLs; `make verify` supplied actual PostgreSQL evidence and passed. Resource-specific restart and the Task 4A obligations listed above remain for their assigned chunks.

**Rollback note:** no commit was made. Preserve the entire current working tree. Do not revert migration 005 or reset any database.

## Exact next instructions

For the next fresh session, execute **Chunk 5 — Atomic descriptive editing only**:

1. Read this handoff and the authoritative Task 4A/ADR 004 documents; confirm the same base and preserve every current working-tree file.
2. Implement only the Coordinator-authorized descriptive-edit service and adapter integration using Migration 005's guarded edit capability.
3. Add focused service and real-PostgreSQL tests for locked authoritative before-values, immutable code rejection, no-op without UPDATE/event, changed edit event details, both resource states, rollback, and retained Resource/Booking data as required by Chunk 5's contract.
4. Run focused tests and `make verify`, inspect all tracked/untracked changes, assess risks, and update the handoff with actual evidence.
5. Stop before Chunk 6. Do not implement activation/deactivation or HTTP/browser workflows.

**Chunk 4 status: CONTINUABLE.**
