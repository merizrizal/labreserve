# Task 4B — Chunk 0 to Chunk 1 Handoff

## Task and candidate identity

- **Task:** Task 4B — Atomic Resource Activation and Safe Deactivation.
- **Approved contract:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`.
- **Contract SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md` (the sole ladder; Chunks 0–11).
- **Plan SHA-256:** `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b`.
- **Implementation base:** `61e7d010205f24478654a1b37e47ae75aec14c76`.
- **Current branch / HEAD:** `feat/004b-resource-activation-deactivation` / `61e7d010205f24478654a1b37e47ae75aec14c76`.
- The task commit (`91c491e…`) and plan commit (`61e7d01…`) are tracked in the base history. The expected base, task/plan identities and clean starting tree were confirmed. The branch was created from that clean base. No commit, push, merge or deployment occurred.
- **Preflight:** `PREFLIGHT_PASS`, re-confirmed against the current task, plan, hashes and base. No source/specification blocker was found.

## Execution mode and resolved policy

- **Mode:** single-chunk; Chunk 0 only. No continuous loop was started.
- **Scope authorization:** the user explicitly authorized local development implementation but this session was authorized for Chunk 0 only. The user separately and explicitly authorized this Chunk 0-to-1 handoff. This handoff does not authorize executing Chunk 1.
- **Validation failure budget:** two failed validation attempts per issue, from the approved plan/skill default. Failed attempts used: **0**.
- **Environment:** repository-local, disposable Compose verification project `labreserve-verify` only. No production or persistent database access. The project was confirmed empty before use; the verification script's cleanup was confirmed afterward.
- **Verification entry point:** `rtk make verify`, per the plan and repository Makefile. No credentials were read or changed; ignored `.env` existed and remained untouched.
- **Handoff policy:** explicit user authorization satisfied the single-chunk confirmation requirement. Stop after this chunk.

## Completed Chunk 0 work

- Read the complete Task 4B contract and controlling plan, including their endings; repository `AGENTS.md`; PRD and baseline; architectures 001–004; accepted Task 2A–2C, 3A–3C and 4A contracts; and relevant Resource/Booking service, transaction, migration and test interfaces.
- Confirmed Task 4B contract and plan are tracked/committed at the approved base. Current task and plan fingerprints match the supplied values. Current working tree was clean before this handoff.
- Confirmed existing branch convention (`feat/004a-resource-creation-editing`) and created the requested Task 4B branch.
- Inspected `Makefile`, `scripts/verify.sh`, Compose services, Dockerfiles, test-pool guards and lifecycle flags. Confirmed the test pool requires database `labreserve_test`; Compose supplies separate migrator/runtime URLs and the actual app runs under `labreserve_app`.
- Inspected the existing Resource/Booking integration points and migration ladder. Existing migrations are 001–005; `requiredSchemaVersion` is `005_resource_mutations.sql`; 006 is unused.
- No Task 4B application code, test, configuration, or migration was added or changed. No Task 4C work was introduced.

## Environment readiness and ownership

- Docker Engine **29.9.2**, Docker Compose **5.6.1**, and GNU Make **4.4.1** were available; Docker access succeeded.
- `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify config --quiet` — PASS.
- Before verification, no containers or volumes for `labreserve-verify` or `labreserve` were found. PostgreSQL `postgres:17.7-alpine` and all configured verification image tags were present. Three upstream base tags were not initially locally tagged; the pinned image metadata/build resolved successfully and the builds passed.
- `.env` existed, was ignored by Git, was not read, and was not modified. Compose configuration resolved; actual migration, seed, runtime-role tests and the app startup then exercised local credentials without revealing them.
- The first focused setup launched the migration and seed commands concurrently; both completed successfully. I then reran them serially in migration-then-seed order, and the full verification script used that documented order. All of this used the newly created disposable database; no persistent target was involved.
- After `make verify`, no `labreserve-verify` or `labreserve` containers/volumes remained. The script's cleanup removed only its isolated verification project and volume. No persistent migration ledger was inspected, reset or modified.

## Verification executed and results

| Command / procedure | Result and evidence |
| --- | --- |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify build verify-migrate verify-seed verify-go verify-app e2e` | **PASS.** Pinned verification images/builds completed, including Go application and E2E support builds. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify up --wait -d verify-db` | **PASS.** Disposable PostgreSQL became healthy. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm verify-migrate` followed by `... run --rm verify-seed` | **PASS.** Fresh disposable database migrated and fictional seed rows loaded. Repeat seed also passed. |
| `rtk docker compose ... run --rm --no-deps verify-go go test -list '^(TestResource|TestConcurrentResource|TestConcurrentCanonicalResource|TestBooking|TestConcurrent.*Booking|TestOwnerCancellation|TestConcurrentOwner)' ./internal/database` | **PASS.** Discovered 50 matching Resource/Booking/cancellation database test names. |
| `rtk docker compose ... run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResource|TestConcurrentResource|TestConcurrentCanonicalResource|TestBooking|TestConcurrent.*Booking|TestOwnerCancellation|TestConcurrentOwner)' ./internal/database` | **PASS.** 46 top-level tests passed against real PostgreSQL using the guarded disposable database and runtime-role paths where applicable. Four lifecycle-only tests intentionally skipped in this ordinary focused invocation: `TestResourceMutationRestartFixture`, `TestResourceMutationRestartReplay`, `TestOwnerCancellationRestartFixture`, and `TestOwnerCancellationRestartReplay`. The repository script explicitly invoked all four in their pre-/post-restart phases; each passed there. |
| `rtk docker compose ... run --rm --no-deps verify-go go test -count=1 ./internal/resources ./internal/bookings` | **PASS.** Existing Resource and Booking service unit suites. |
| `rtk docker compose ... run --rm --no-deps verify-go go vet ./...` | **PASS.** Static analysis completed without diagnostics. |
| `rtk make verify` | **PASS.** Script printed `Repository verification passed.` Formatting check passed; verification images and app/E2E builds passed; Go tests passed for `internal/auth`, `internal/bookings`, `internal/config`, `internal/database`, `internal/resources`, and `internal/web` (`cmd/labreserve` and E2E support report no test files). The app started with restricted `labreserve_app`; Playwright ran in America/New_York and passed **8/8**. Migration and seed ran; PostgreSQL restart/seed checks passed; Resource and cancellation fixture/replay commands passed around actual PostgreSQL and application restarts; retained sessions/data checks passed. |

The focused real-PostgreSQL tests included booking creation/replay, overlap exclusion and concurrent conflict, cancellation owner/time/lock/event-failure/restart support, Resource create/edit/no-op/privilege/commit-rejection regressions, and the existing migration preservation test. No missing database URL skip occurred: Compose supplied the test URLs and `testPool` accepted only `labreserve_test`. The four ordinary-run lifecycle skips listed above were intentionally covered by the full script's explicit lifecycle phases. No Playwright tests were skipped.

The full baseline was run on the unchanged approved source candidate. It does **not** verify any Task 4B behavior, because none has been implemented.

## Existing interfaces and constraints for the next session

- `internal/resources/service.go`: existing `Actor`, `Resource`, `ResourceTransaction`, `Service.Create`, `Service.Edit`, and changed/no-op edit result. Creation/edit use separate narrow repository interfaces; do not casually widen their accepted contracts.
- `internal/database/resource_creation.go`: stored Coordinator lookup; Resource lock helper followed by a separate authoritative row read; READ COMMITTED transaction; guarded Task 4A create/edit capabilities; commit/rollback.
- `internal/bookings/service.go` and `internal/database/bookings.go`: booking creation order is Account `FOR NO KEY UPDATE` → separate retained owner/request lookup → Resource `FOR UPDATE` for a new request → fresh clock/validation → Booking/event. Replay/reuse precedes current Resource eligibility.
- `internal/bookings/cancellation.go` and `internal/database/booking_cancellation.go`: owner-only cancellation uses an unlocked owner-scoped Resource-ID lookup, then Resource lock → Booking lock/re-read → fresh time for a Confirmed transition → atomic update/event. No Resource → Account coordination lock.
- `public.labreserve_lock_booking_resource` is the established shared Resource locking capability. Task 4B must preserve its compatibility with creation, cancellation and Task 4A edits.
- Runtime Resource writes remain denied directly. Migration 005 supplies guarded Task 4A create/edit capabilities; it does not implement existing-Resource state transitions.

## Migration findings — preserve for Chunk 2

- Current migration set is 001–005; current required schema version is `005_resource_mutations.sql`. The SHA-256 of the accepted Migration 005 source is `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7` (unchanged). All 001–005 hashes were rechecked after verification and match the controlling plan.
- **Required historical-test adaptation:** `internal/database/booking_persistence_integration_test.go:413–440` uses `requiredSchemaVersion` to delimit the historical 001–004 migration range and asserts four migrations. When Chunk 2 advances the required version to 006, keep this test anchored at its original 004 boundary; do not let its fixture expand to include 005. Do not edit this test in Chunk 0.
- Also inspect `TestResourceCapabilityMigrationPreservesVersion004Data` in `internal/database/resource_mutations_integration_test.go`: it currently treats `requiredSchemaVersion` as the Task 4A capability migration and expects four pre-capability versions. Chunk 2 must preserve explicit 004→005 evidence and separately add compatible 005→006 coverage rather than weakening the existing test.
- Preserve the **conditional Migration 005 persistent-ledger compatibility restriction**. Before any separately authorized persistent-target upgrade, verify its recorded versions/checksums/provenance non-destructively. A different/earlier 005 checksum or evidence of persistent application of the earlier checksum requires stopping that target upgrade and obtaining an authorized non-destructive compatibility procedure. Do not rewrite/rechecksum the ledger, replay 005, reset a persistent volume/database, or assume 006 bypasses exact-ledger validation.
- Chunk 0 used only a newly created disposable verification database. It did not inspect or certify any persistent ledger.

## Risks, gaps, and rollback

- **Functional:** No Task 4B behavior is implemented or claimed. The feature's primary correctness risks remain explicit desired-state no-ops, the all-owner `Confirmed AND end_at > fresh operation time` guard, lock ordering, atomic Activity and least privilege.
- **Integration:** Future state capability must compose with the observed Resource/Booking lock order and actual runtime role; browser/HTTP Task 4C work remains excluded.
- **Validation gap:** This baseline is pre-implementation. It provides no activation/deactivation coverage. Full Task 4B V1–V9 evidence remains for later chunks.
- **Migration/data integrity:** The conditional Migration 005 checksum precondition remains unresolved for any hypothetical persistent target; none was found or inspected. Disposable verification is not evidence of persistent compatibility.
- **Rollback:** No source or migration rollback is needed. The handoff is the only authorized file added. Do not delete ignored `.env` or any later session's work. The local feature branch is intentional and must not be overwritten or rewritten.
- **Failed validation attempts:** zero. No baseline failure or outstanding environment blocker.

## Exact next chunk and fresh-session instructions

**Next planned chunk: Chunk 1 — Minimal state contracts with fail-closed entry point.** Its planned source scope is `internal/resources/state.go` and `internal/resources/state_test.go` only. Follow the actual Chunk 1 section in the controlling plan; do not start Chunk 2 or add migrations, database adapters, callers or HTTP routes.

A fresh session must revalidate branch/HEAD and working-tree state, read this handoff plus the complete Task 4B contract and controlling plan (including applicable sources), confirm plan/task hashes and the applicable authorization/preflight remain current, and use the same single-chunk policy and two-failure budget. The current handoff is not permission to proceed automatically: obtain an explicit Chunk 1 instruction, execute only Chunk 1, and stop.

**Chunk 0 status: CONTINUABLE.**
