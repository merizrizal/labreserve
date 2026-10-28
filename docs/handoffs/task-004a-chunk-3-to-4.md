# Task 4A handoff — Chunk 3 to Chunk 4

## Authority and current state

- **Task:** Task 4A — Atomic Resource creation and descriptive editing.
- **Execution mode:** Single-chunk; this session executed Chunk 3 only and stops here.
- **Preflight:** `PREFLIGHT_PASS` was reported for implementation base `a90d3fd37226b5a648153edf7d134ec605a78dc0`. At this session's start, HEAD and branch matched that base; governing sources were tracked/committed and unchanged. The prior untracked `internal/resources/service.go` and `internal/resources/service_test.go` were preserved.
- **Candidate:** Uncommitted working tree based on `a90d3fd37226b5a648153edf7d134ec605a78dc0`; no commit, push, merge, or deployment.
- **Resolved execution policy:** User-authorized single chunk; two failed validation runs per same current-chunk issue; disposable local PostgreSQL/Compose verification only. The user explicitly requested this Chunk 4 handoff.

## Controlling implementation ladder

Preserve the approved Task 4A ladder; do not create a competing plan:

1. Chunk 1 — Baseline and integration points: complete.
2. Chunk 2 — Command contracts and canonical validation: complete.
3. Chunk 3 — Guarded database capabilities and additive migration: complete here.
4. Chunk 4 — Atomic Resource creation: next.
5. Chunk 5 — Atomic descriptive editing.
6. Chunk 6 — Concurrency, authorization, and privilege verification.
7. Chunk 7 — Lifecycle, documentation, and delivery-wide verification.

This handoff authorizes no functionality beyond Chunk 4. Task 4B/4C, Resource activation/deactivation, browser/HTTP management, cross-owner cancellation, history browsing, and deletion remain excluded.

## Chunk 3 result

Added `005_resource_mutations.sql` with two operation-specific `SECURITY DEFINER` capabilities:

- `public.labreserve_create_resource` generates the Resource ID, validates canonical required/optional text and explicit initial state, checks the actor's stored Coordinator role, inserts the Resource, and appends one `resource.created` event in the same database statement/transaction.
- `public.labreserve_edit_resource` checks the actor, locks and reads the Resource, accepts only name/description/timestamp inputs, returns an unchanged result without UPDATE or Activity when canonical values match, and otherwise updates only descriptive fields plus `updated_at`. The event's before-values come from the locked persisted row; its after-values come from canonical inputs.

Both capabilities use a fixed `pg_catalog` search path, schema-qualified business objects, no dynamic SQL, and migration-role ownership. The migration requires the non-superuser `labreserve_migrator`, revokes PUBLIC execution, and grants only runtime execution. It grants no direct Resource table/column writes and changes no prior migration.

`requiredSchemaVersion` is now 005. The repository verification script expects five ledger entries. The existing isolated foundation-upgrade test now stops at migration 004; a separate transactional integration test exercises the additive 004-to-005 migration with existing Resource, Booking/request ID, Activity, and session data. The 001–004 migration files and their checksums remain untouched.

## Files changed in Chunk 3

- `internal/database/migrations/005_resource_mutations.sql` — new guarded capabilities and scoped grants.
- `internal/database/migrate.go` — required schema version 005.
- `internal/database/resource_mutations_integration_test.go` — real-PostgreSQL direct capability, actor/canonical guards, effective permission, immutability, no-op, Activity rollback, outer rollback, search-path-shadowing, uniqueness, and additive migration tests.
- `internal/database/booking_persistence_integration_test.go` — preserve its isolated schema migration test through migration 004; 005 is tested in the public-schema migration test.
- `scripts/verify.sh` — expected migration-ledger count updated from four to five.

Preserved prior untracked files:

- `internal/resources/service.go`
- `internal/resources/service_test.go`

The handoff itself is at `docs/handoffs/task-004a-chunk-3-to-4.md`.

## Verification actually run

- `rtk go test ./internal/resources` — passed; 16 tests reported (baseline and post-change).
- `rtk make verify` before Chunk 3 edits — passed on the repository's isolated disposable verification stack.
- Focused PostgreSQL capability tests — passed after fixing a fixture's ambiguous `$4` timestamp type with an explicit `timestamptz` cast.
- `rtk go test -count=1 ./internal/database` in the isolated Compose PostgreSQL environment — passed. This includes the new tests and existing migration-ledger tests. The pre-fix run exposed the old custom-schema test colliding with the new `public` functions; that test was correctly narrowed to migrations 001–004, with 005 covered separately.
- `rtk make verify` after the changes — exited successfully. The repository script ran its PostgreSQL, Go, browser, and restart checks on its isolated verification project and cleaned that project on exit.
- `rtk gofmt -w` on changed Go files; `rtk gofmt -l` returned no files.
- `rtk sh -n scripts/verify.sh` — passed.
- `rtk git diff --check` — passed.

Distinct failed attempts and repairs: (1) test-fixture cleanup referenced an undefined `t`; retained the test handle; (2) migration fixture timestamp parameter inference failed with SQLSTATE `42P08`; added explicit casts; (3) legacy custom-schema migration test collided with globally qualified migration-005 functions; limited that test to 001–004 and added a dedicated 004-to-005 preservation test. Each issue was repaired and its affected validation rerun successfully.

## Security and integrity boundary

The runtime role has no direct Resource INSERT, any-column UPDATE, DELETE, or TRUNCATE privilege. Integration tests inspect effective table/column privileges and attempt the forbidden operations. The allowed function grants are checked for non-superuser `labreserve_migrator` ownership, `search_path=pg_catalog`, no PUBLIC/other-role/grant-option execution, no runtime membership in the owner role, and no runtime CREATE on `public`. Runtime calls are tested with caller-controlled temporary shadow tables/search path.

The functions check the **stored role** for the supplied actor ID; the shared runtime database credential does not independently authenticate an end user. Trusted application/session identity remains an application responsibility. Function inputs do not include caller-authored edit before-values, an Activity skip flag, Resource ID/code/state mutation for editing, or arbitrary SQL. The caller-supplied operation timestamp is for the trusted service clock boundary; it is not end-user-authenticated by PostgreSQL.

## Risk and remaining verification gaps

- **Functional:** SQL capabilities are tested directly under the actual runtime role, but the Resource application service does not yet call them. Chunk 4 must wire trusted actor resolution, explicit transaction ownership, operation time, result/error mapping, and commit behavior.
- **Integration:** Existing booking, cancellation, session, and Activity grants were not broadened. Concurrent Resource creation races and wider lock-order/concurrency coverage remain assigned to Chunk 6.
- **Validation:** Chunk 3 is complete; this does not establish Task 4A V1–V9 completion or `READY_FOR_REVIEW`. Service-to-session authorization, end-to-end Resource creation behavior, descriptive service behavior, and the later lifecycle/restart evidence remain outstanding.
- **Rollback:** Migration 005 is append-only and has no down migration. Do not edit migrations 001–004, delete migration history, reset the demo volume, or attempt a destructive rollback. A failed future migration recovery must use the approved non-destructive operational path.

## Exact next instructions

For the next fresh session, execute **Chunk 4 — Atomic Resource creation only**:

1. Re-read this handoff and the approved Task 4A creation contract; preserve the controlling ladder and both prior untracked `internal/resources` files.
2. Implement the Coordinator-authorized service path for creation using the existing command contract and the guarded `public.labreserve_create_resource` capability. Keep identity/role derived from the trusted application session, not caller-selected role fields.
3. Own one explicit transaction and one commit; pass canonical values, an explicit Active/Inactive choice, and the established UTC microsecond operation timestamp. Do not grant or use direct Resource INSERT. Map only the established case-insensitive code-index violation to duplicate-code; do not convert other failures into success.
4. Add focused service and real-PostgreSQL coverage for Coordinator/Engineer outcomes, both initial states, validation/duplicate behavior, event content/count, Activity-write rollback, and existing read/booking compatibility required by Chunk 4.
5. Run focused verification, inspect the complete diff, assess risks, and stop before Chunk 5. Do not implement descriptive editing, Task 4B/4C, or any HTTP/browser management.

**Chunk 3 status: CONTINUABLE.**
