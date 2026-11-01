# Task 4B — Chunk 2 to Chunk 3 Handoff

**Status: CONTINUABLE.** Chunk 2 exit criteria passed. This handoff is for the completed Chunk 2 only; Chunk 3 has not started, and Task 4B is not complete.

## Contract, source freshness, and execution identity

- **Task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`
- **Task SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a` (unchanged at implementation base and candidate).
- **Controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`
- **Current plan SHA-256:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f` (same at base and candidate).
- **Original plan fingerprint:** `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b` matches the plan at `61e7d010205f24478654a1b37e47ae75aec14c76`. The intervening plan diff through the implementation base contains only three absolute-path redactions; it does not change Chunk 2 scope, requirements, or exit criteria. The Task 4B contract is unchanged across those revisions. **PREFLIGHT_PASS**; no material plan drift or unresolved specification issue.
- **Fingerprint correction:** the preceding `task-004b-chunk-1-to-2.md` records `f6a266...` as the plan hash at `61e7...` and says the supplied hash matched neither revision. A raw `rtk proxy` recheck computes `aa02f5d8...` at that commit, matching the supplied fingerprint. That earlier handoff was preserved unmodified; use the fingerprints recorded here.
- **Implementation base / candidate HEAD:** `bd231b22ed18e41942ef2001bec71dca6d15ffe4` / the same revision. Branch: `feat/004b-resource-activation-deactivation` tracking its origin branch.
- **Execution:** explicitly authorized single-chunk execution. No continuous loop. No files staged; no commit, push, deployment, or persistent database operation.
- **Retry budget:** two failed validation attempts per issue. One focused-test attempt failed with PostgreSQL `42P08` (ambiguous fixture parameter typing) in two fixture statements; explicit `timestamptz` casts repaired it, and the complete focused rerun passed. No other validation failures. The candidate remains within the attempt budget.

## Chunk 2 implementation

Added the additive `internal/database/migrations/006_resource_state.sql` capability:

- Non-superuser `labreserve_migrator` ownership guard; one fixed signature, `public.labreserve_set_resource_state(UUID, UUID, BOOLEAN, TIMESTAMPTZ)`.
- Stored Coordinator check, qualified Resource `FOR UPDATE`, unknown-target empty result, and authorized desired-state no-op before occupancy or mutation.
- Separate post-lock occupancy query for deactivation: all Confirmed Bookings for the target Resource with `end_at > p_occurred_at` block; exact end does not. Activation does not reject retained occupancy.
- A real transition updates only `active` and `updated_at`; derives `resource.activated`/`resource.deactivated` and before/after details internally; writes Activity in the same transaction. Null state inputs are rejected.
- `SECURITY DEFINER`, fixed `search_path = pg_catalog`, qualified application objects, revoked PUBLIC privileges, runtime-only EXECUTE grant. No raw/general setter overload or application caller was introduced.

Integrated the new required schema version and six-row verification expectations. Kept historical migration evidence separate: the Booking foundation test still ends at 004; the prior 004→005 test explicitly excludes 006 and validates through 005; new tests simulate compatible 005→006 and refuse an incompatible recorded 005 checksum without applying 006 or changing business data.

Direct-runtime PostgreSQL coverage includes both transitions, matching-state no-ops with Resource-UPDATE and Activity-attempt detectors, Coordinator/Engineer/unknown/null inputs, unknown target, retained fields and event details/timestamps, future/ongoing/Cancelled/completed and exact-end occupancy, shadowed caller search paths, Activity failure and outer rollback in both directions, and an observed Resource row-lock wait where a committed Confirmed Booking becomes visible to the subsequent occupancy query. Upgrade evidence snapshots Active and Inactive Resources/raw text, Confirmed and Cancelled Booking/request/cancellation fields, Activity, accounts, sessions, indexes, FKs, and existing capability definitions/ACLs; it verifies repeat migration.

## Files and protected work

**Chunk 2 changed (seven files):**

- Added `internal/database/migrations/006_resource_state.sql`
- Added `internal/database/resource_state_capability_integration_test.go`
- Added `internal/database/resource_state_migration_integration_test.go`
- Updated `internal/database/migrate.go`
- Updated `internal/database/resource_mutations_integration_test.go`
- Updated `internal/database/booking_persistence_integration_test.go`
- Updated `scripts/verify.sh`

**Preserved from Chunk 1, unchanged:** `internal/resources/state.go`, `internal/resources/state_test.go`, and `docs/handoffs/task-004b-chunk-1-to-2.md`. The state service remains unreachable and fail-closed. No Task 4B service adapter/caller, HTTP route/template, Booking behavior, or Task 4C behavior was added.

New-file SHA-256 identities at handoff creation:

- Migration 006: `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`
- Capability integration tests: `4923397dd1d305915da39f65dafd6b447ab74bcdc4006e66e2623a7b56171c14`
- Migration integration tests: `b9acefac467ad644cc73c5d244d6125a86d2b67090dd7c056ac88ad199de01ed`

Migrations 001–005 match their implementation-base bytes and checksums:

- 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`
- 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`
- 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`
- 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`
- 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`

Migration 006 checksum is `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`; it matched the fresh disposable ledger. No migration history was changed outside disposable verification fixtures.

## Verification evidence

All commands ran in the repository or the repository's isolated `labreserve-verify` Compose project; no persistent/demo database was accessed. The focused project and its volume/network were absent before setup and absent after final verification.

- `rtk go test ./internal/resources`: **PASS**, 43 tests; fail-closed Chunk 1 behavior preserved.
- `rtk proxy go test -run '^$' ./...`: **PASS**, all Go packages/test packages compile.
- `rtk gofmt -l` on changed Go files, including in the rebuilt verification image: **PASS**, no output.
- `rtk bash -n scripts/verify.sh`: **PASS**.
- Focused L discovery for `^(TestResourceStateCapability|TestResourceStateMigration|TestResourceCapabilityMigrationPreservesVersion004Data|TestCheckSchemaRequiresExactMigrationLedger|TestResourceCapabilities)` discovered all 13 expected top-level tests.
- Focused D run in real PostgreSQL with `-count=1 -v -p 1` for that selector: **PASS**. Capability, privilege, row-lock wait, 004→005, compatible 005→006, incompatible-005 refusal, and exact missing/unknown/checksum ledger cases all executed; no skips.
- Disposable fresh migration and seed: **PASS**. The ledger contained exactly versions 001–006 with the checksums listed above.
- `go build -o /tmp/labreserve-task4b-check ./cmd/labreserve` in the verification image: **PASS**.
- `rtk git diff --check`: **PASS**; staged diff empty.
- `rtk make verify`: **PASS**, ended with `Repository verification passed.` All Go packages passed (including database and web integration packages); Playwright passed 8/8; the repository's PostgreSQL/app restart, seed-repeat, and retained-state checks passed. No required suite was skipped.

## Decisions, risks, and rollback

- Kept the implementation at the database-capability boundary specified for Chunk 2. The test-only PostgreSQL lock-wait case substantiates Resource locking and post-lock occupancy visibility without introducing a Go state caller.
- The capability validates the stored Coordinator role but is not an authentication/session boundary by itself. Future service wiring must supply the trusted session actor; that work belongs to later chunks.
- Persistent upgrade compatibility remains conditional: a target must have compatible 001–005 provenance/checksums. The test proves an incompatible 005 checksum is refused in an isolated schema. No persistent target was inspected or upgraded. Do not rewrite/rechecksum an applied migration, alter a persistent ledger, or reset a persistent volume.
- Migration 006 is additive and has no down migration. A persistent applied 006 must be rolled forward, not reverted by editing migration history or running an old binary against the new exact ledger.
- No independent fresh-session review has occurred. This implementation handoff does not replace that review or authorize integration.

## Exact next-session instruction

Review this complete Chunk 2 delta independently. If accepted and separately authorized to continue, execute **Chunk 3 only — Coordinator activation vertical slice**, re-reading the complete task, controlling plan, repository instructions, and both handoffs first. Preserve the Chunk 1 fail-closed contract until the Chunk 3 activation path is complete; do not implement deactivation, HTTP composition/routes, Booking changes, or Task 4C. Do not stage or commit without explicit instruction.
