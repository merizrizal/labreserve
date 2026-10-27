# Task 4A — Chunk 6 handoff to Chunk 7

## State and authority

- Approved scope executed: **Chunk 6 — Concurrency, Authorization, and Privilege Verification only**. No Chunk 7 work was started.
- Implementation base / current `HEAD`: `a90d3fd37226b5a648153edf7d134ec605a78dc0` (`feat/004a-resource-creation-editing`). The supplied prior `PREFLIGHT_PASS` and committed Task 4A contract were confirmed; relevant PRD, architecture, and accepted Task 2A–3C contracts were inspected.
- Candidate is an **uncommitted working tree** on that same HEAD. No commit, push, merge, deployment, or credential change was made.
- Existing staged and unstaged work was preserved. Chunk 6 added/extended tests and this handoff only; it did not change production code or migrations.

## Chunk 6 evidence

Added deterministic real-PostgreSQL concurrency coverage in `internal/database/resource_concurrency_integration_test.go`:

- Concurrent creation with identical, case-variant, and Unicode-whitespace-variant codes; observed PostgreSQL lock waits use the existing recursive blocker-chain helper. Exactly one Resource and creation event commits; persisted fields and actor/event data match the winner.
- A scoped Activity-insert failure for the first creator rolls back its Resource and lets the blocked canonical-code waiter succeed.
- Concurrent descriptive edits serialize on the Resource lock. Tests inspect post-lock clock reads, timestamps, locked before/after snapshots, retained fields, and final persisted values. Concurrent canonical-equivalent edits produce one mutation and one no-op, with a failure trigger detecting any no-op UPDATE.
- Resource edits coordinate with Booking creation and owner cancellation/replay on the same Resource; tests verify eligibility, Booking identity/state/history, and event counts remain intact.

Extended authorization regressions:

- `internal/database/resource_creation_integration_test.go` verifies an unknown account cannot create a Resource and leaves no Resource/event.
- `internal/database/resource_edit_integration_test.go` verifies unknown actors, Engineer no-op/code-change attempts, invalid text, and immutable code attempts do not mutate Resource state or create success events.
- `internal/web/foundation_integration_test.go` resolves the stored Coordinator identity from a trusted session and verifies its actor is recorded; a revoked session cannot create a Resource. The Account model has no disabled-account state, so no new inactive-account semantics were invented.

Existing migration/database tests were inspected and run as part of the real-PostgreSQL suite. They verify guarded capability use, effective runtime-role privileges and denied direct writes, actor/field guards, Activity coupling/rollback, and migration preservation. The database checks use the migration/admin pool only for setup/inspection and the actual restricted runtime pool for use cases. These do not claim that PostgreSQL independently authenticates a supplied actor ID.

## Verification executed

- Focused disposable PostgreSQL run (project `labreserve-task4a-chunk6`): `rtk docker compose --project-name labreserve-task4a-chunk6 --file compose.yaml --profile verify up --wait -d verify-db`, `... build verify-go`, then `... run --rm verify-migrate` and `... run --rm verify-seed`. Targeted real-DB tests passed:
  - Database: `rtk docker compose --project-name labreserve-task4a-chunk6 --file compose.yaml --profile verify run --rm verify-go go test -p 1 -run '^(TestConcurrentResourceCreationUsesCanonicalCodeUniqueness|TestResourceCreationActivityFailureReleasesCanonicalCodeForWaiter|TestConcurrentResourceEditsUseLockedBeforeValuesAndPostLockTime|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp|TestResourceEditsRemainCompatibleWithBookingCreation|TestResourceEditsRemainCompatibleWithOwnerCancellationAndReplay|TestResourceCreationRejectsEngineerInvalidInputAndCaseInsensitiveDuplicates|TestResourceEditingRejectsUnauthorizedInvalidAndImmutableChanges)$' ./internal/database`.
  - Web: `rtk docker compose --project-name labreserve-task4a-chunk6 --file compose.yaml --profile verify run --rm verify-go go test -p 1 -run '^TestFoundationHTTPAuthenticationAndResourceBrowsing$' ./internal/web`.
  Both returned `ok`; the test volume/project was removed with `... down --volumes --remove-orphans`.
- `rtk go test ./...` — **115 passed in 8 packages** on the host. PostgreSQL-dependent tests skip when `LABRESERVE_TEST_DATABASE_URL` / `LABRESERVE_TEST_APP_DATABASE_URL` are absent; `internal/database/foundation_integration_test.go` and `internal/web/foundation_integration_test.go` contain the explicit `t.Skip` guards. This host result is not represented as PostgreSQL evidence.
- `rtk gofmt -l internal/database/resource_concurrency_integration_test.go internal/database/resource_creation_integration_test.go internal/database/resource_edit_integration_test.go internal/web/foundation_integration_test.go` and `rtk git diff --check` — passed (no output).
- Final `rtk make verify` — **passed**. The repository gate ran Go packages against isolated real PostgreSQL, started the app with the restricted runtime role, passed all **8 Playwright tests**, and completed restart/cancellation persistence checks. Earlier full runs also passed; only the latest successful run is relied on for completion.
- `make verify` also re-ran existing migration-ledger, effective-privilege, atomic Activity, and prior Delivery 1–3 regression tests. No browser-management workflow was introduced.

## Review, decisions, and limitations

- The creation-race wait helper recursively follows PostgreSQL blocker chains, so the second creator waiting on the first transaction's uncommitted unique key is observed even though its immediate blocker differs from the held Activity-table lock.
- Edit timestamps are obtained after acquiring the shared Resource lock; canonical no-op requests avoid UPDATE/event work. Existing Booking lock ordering and cancellation/creation semantics were not changed.
- The current account model has no account-disable flag; unknown account and revoked-session behavior were tested. Account deactivation is not asserted as an existing product state.
- The full V6 Resource read/restart lifecycle remains for the controlling ladder's later work. Task 4B/4C and all other deferred behavior remain untouched.
- One initial host compile attempt for the new tests failed and was repaired (1 failed attempt for that issue; one remains under the two-attempt budget). An intermediate verification capture was inconclusive; the final real-PostgreSQL and repository gates passed. Historical retry records remain unchanged: Chunk 4 import-cycle failures 3/5; Chunk 5 precision failure 1/2.

## Working-tree inventory

The following earlier staged work remains staged and preserved:

- Added: `docs/handoffs/task-004a-chunk-3-to-4.md`, `docs/handoffs/task-004a-chunk-4-to-5.md`, `internal/database/migrations/005_resource_mutations.sql`, `internal/database/resource_mutations_integration_test.go`.
- Modified: `internal/database/booking_persistence_integration_test.go`, `internal/database/migrate.go`, `scripts/verify.sh`.
- Added with subsequent unstaged edits: `internal/database/resource_creation.go`, `internal/database/resource_creation_integration_test.go`, `internal/resources/service.go`, `internal/resources/service_creation_test.go`, `internal/resources/service_test.go`.

Other pre-existing untracked work remains untracked: `docs/handoffs/task-004a-chunk-5-to-6.md`, `internal/database/resource_edit_integration_test.go`, and `internal/resources/service_edit_test.go`.

Chunk 6 worktree changes are `internal/database/resource_creation_integration_test.go`, `internal/database/resource_edit_integration_test.go`, and `internal/web/foundation_integration_test.go`; new `internal/database/resource_concurrency_integration_test.go`; and this handoff. No Chunk 6 file was staged.

## Next boundary

Chunk 6 verification is complete. Continue only with the separately authorized next ladder chunk. Do not infer Chunk 7 scope beyond its controlling contract; do not treat this handoff as final Task 4A acceptance or independent review.

**CONTINUABLE**
