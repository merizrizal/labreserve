# LabReserve Task 4A review repair handoff — R4A-01 complete

## Candidate and scope

- **Repair:** R4A-01 — canonical-equivalent retained text incorrectly mutates Resources.
- **Mode:** Single bounded repair chunk; stop here. R4A-02 has not been started.
- **Approved implementation base:** `a90d3fd37226b5a648153edf7d134ec605a78dc0`.
- **Starting candidate:** `8fe22442eea79f82ff016daa015b27254ced638a` on `feat/004a-resource-creation-editing`; working tree was clean.
- **Current state:** `HEAD` remains `8fe22442eea79f82ff016daa015b27254ced638a`; repair is uncommitted. No staging, commit, push, merge, deployment, reset, or discard was performed.
- **Authority/freshness:** Current user authorized this Level 0 correction. Task 4A and ADR 004 are tracked/committed and unchanged; the user expressly excluded reopening D4-001–D4-003 and Task 4B/4C.
- **Independent review:** Complete report was read from the prior LabReserve review session. R4A-01 was a reproduced no-op defect. R4A-02 is a separate missing definite-COMMIT-rejection verification finding and remains outstanding.

## Exact files changed

1. `internal/database/migrations/005_resource_mutations.sql`
   - The edit capability now compares submitted canonical values against `pg_catalog.btrim` of the locked persisted name and description, using the existing Go-compatible Unicode whitespace set.
   - On equality it returns the untouched locked row, preserving stored bytes/timestamps and avoiding UPDATE/Activity. No bulk normalization or privilege/signature change was made.
2. `internal/database/resource_edit_integration_test.go`
   - Added real-PostgreSQL coverage for retained U+2003 and U+00A0 edge whitespace in both descriptive fields, cross-submitted edge whitespace, Go/SQL canonical agreement, raw UTF-8 byte preservation, `updated_at`, identity/code/state, and Booking/history relationships.
   - Reuses the existing Resource UPDATE detector and Activity-failure trigger; checks Engineer rejection and Coordinator unchanged success. Also asserts a real changed edit still reports `Changed` and creates exactly one edit Activity.
3. `internal/database/resource_concurrency_integration_test.go`
   - Added a deterministic independent-connection lock-wait test for a canonical-equivalent edit on a Resource retaining U+2003/U+00A0. It checks the waiter returns unchanged after the lock, does not read its operation clock before lock release, preserves bytes/timestamp, and adds no Activity or UPDATE.
4. `docs/handoffs/task-004a-review-r4a01-to-r4a02.md`
   - This handoff.

No service, auth, grant, schema-version, README, or Task 4B/4C files changed.

## Migration policy and database evidence

- Task 4A/PRD/README require numbered checksummed migrations, prohibit editing/re-checksumming an applied migration, provide no automatic down migration, and forbid reset as rollback.
- Migration 005 remains part of the unmerged feature-branch candidate (its parent is the approved base). Prior review/handoff evidence records its application only to disposable verification/review databases; the verification script removes its own volume.
- At this session’s inspection, Docker had no LabReserve containers or volumes (only unrelated services/volumes). `.env` existed but was not read. No local non-disposable LabReserve database was started or modified.
- Based on available evidence, editing 005 in place is compatible with the migration contract; therefore no 006 was added. The focused and full checks below applied the revised checksum only to isolated disposable databases, which were cleaned up.
- **Safety caveat:** If evidence emerges that 005 was applied to any non-disposable database, do not apply this candidate/checksum there and do not reset or alter its ledger. Stop and use an authorized additive correction migration instead.

## Verification performed

- Baseline `rtk go test ./internal/resources` — passed (39 tests in package before edits).
- `rtk gofmt -d internal/database/resource_edit_integration_test.go internal/database/resource_concurrency_integration_test.go` — clean.
- `rtk go test ./internal/database -run '^$'` — compile check passed; no tests selected.
- Focused real-PostgreSQL run in disposable Compose project `labreserve-r4a01-focused` — passed:
  - `go test -count=1 -run '^(TestResourceEditing|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait|TestConcurrentCanonicalResourceEditsProduceOneMutationAndOneNoOp)$' ./internal/database`
  - Migration 005 applied under the migrator role; use cases ran through the restricted runtime role. Cleanup removed only this project’s disposable database volume.
- `rtk make verify` — passed. Go suites, real-PostgreSQL integration tests, all 8 Playwright tests, database/application restart lifecycle and Resource replay checks completed; output ended `Repository verification passed.` The script cleans its own disposable verification volume.
- `rtk git diff --check` — passed.
- Final `rtk gofmt -d` on changed Go files — clean.
- Post-verification Docker inventory confirmed no LabReserve containers or volumes remained.
- **Failed validation attempts for R4A-01: 0/2.** The standalone compile check selected no tests; actual behavioral tests ran against PostgreSQL as listed above.

### Prior failure history retained

Do not reset prior Task 4A history: the Chunk 4 import-cycle issue had three earlier failures (authorized cumulative cap 3/5); the Chunk 5 timestamp-expectation issue had one failed `make verify` attempt (1/2), then passed. These were resolved before this repair and are not R4A-01 failures. The independent reviewer’s R4A-01 reproduction and R4A-02 verification gap remain recorded in the prior review report.

## Diff and risk assessment

- The final implementation/test diff was reviewed in full and is limited to the three files above; no unrelated changes or future-task scaffolding were found.
- **Functional:** The no-op predicate trims only for equality. It does not rewrite retained text; actual changes still use locked raw before-values and preserve the existing update/event transaction.
- **Security:** No authorization, function ownership/search-path, role, grant, signature, or trust-boundary changes. Existing UPDATE/event failure fixtures prove the no-op does not enter either side-effect path.
- **Migration:** The embedded checksum changed. Safe only under the evidence above that no non-disposable database has applied 005; an unknown existing database must not receive this candidate without checking migration state.
- **Concurrency:** A real PostgreSQL lock wait proves the retained-value comparison occurs after lock acquisition; broader Task 4A concurrency coverage still belongs to the already-completed implementation evidence.
- **Rollback:** No database was reset. The source change is uncommitted and can be reviewed/reverted as a bounded diff; never reset the demo database or edit an applied ledger to undo it.

## Remaining work

- **R4A-02 only (not started):** Add definite COMMIT-rejection evidence separately from ambiguous-acknowledgement and outer-rollback cases. Reviewer locations: `internal/resources/service_creation_test.go:110–119`, `internal/database/resource_mutations_integration_test.go:377–468`, and `internal/database/resource_edit_integration_test.go:329–382`. Use an appropriate transaction seam or deterministic scoped PostgreSQL deferred failure; assert operational outcome and no durable Resource/Activity mutation. Do not conflate this with unknown commit acknowledgement.
- Obtain fresh independent review of the corrected candidate as part of the authorized review process. This handoff/self-check is not independent acceptance.
- Do not commit, implement Task 4B/4C, or begin R4A-02 in this session.

**R4A-01 status: CONTINUABLE.**