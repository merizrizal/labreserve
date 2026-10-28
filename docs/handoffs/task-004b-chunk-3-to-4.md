# Task 4B — Chunk 3 to Chunk 4 Handoff

**Status: CONTINUABLE.** Chunk 3 exit conditions passed. This handoff records the activation vertical slice only; Chunk 4 has not started, Task 4B is incomplete, and no full Task 4B acceptance is claimed.

## Task, authorization, and source identity

- **Approved task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`
- **Task SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a` (matches the approved committed blob and the Chunk 2 handoff).
- **Controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`
- **Current plan SHA-256:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f` (matches the corrected current fingerprint in the Chunk 2 handoff).
- **Original plan SHA-256 at implementation base:** `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b`; independently recomputed from `61e7d010205f24478654a1b37e47ae75aec14c76`. The complete plan delta from that base remains only the three documented absolute-path redactions. No objective, scope, dependency, verification, or exit-condition change was found.
- **Preflight:** `PREFLIGHT_PASS`, tied to the committed Task 4B contract and approved implementation base; no material source/specification drift found.
- **Original approved implementation base:** `61e7d010205f24478654a1b37e47ae75aec14c76`.
- **Current branch / HEAD:** `feat/004b-resource-activation-deactivation` / `bd231b22ed18e41942ef2001bec71dca6d15ffe4`.
- The task and plan are tracked and committed. Migrations 001–005 match their accepted source hashes; Migration 006 matches the accepted Chunk 2 file hash. Exact hashes are recorded below.

## Execution policy and candidate state

- **Mode:** single-chunk; Chunk 3 only. No continuous loop.
- **Authorization:** explicit human authorization covered Chunk 3 only. The Chunk 3-to-4 handoff was explicitly authorized. Neither this handoff nor the previous authorization starts Chunk 4 automatically.
- **Retry policy:** two failed validation attempts per issue. Chunk 2's earlier fixture-typing issue used one failed attempt and was fixed with explicit casts. In Chunk 3, one focused unit run failed because the fake-repository test expected zero `BeginResourceStateChange` invocations when begin returned an error; the expectation was corrected to one invocation and the rerun passed. These are distinct resolved issues; neither issue's history is reset. No other validation attempt failed. The initially over-anchored broad regression selector passed but selected only its exact Booking test; discovery exposed the narrower selection, and a corrected prefix selector then discovered and ran the intended set. This was a selector correction, not a failed validation.
- **Working tree:** dirty by design from accepted earlier chunks. No staged changes; no implementation commit, push, merge, deployment, or credential changes.
- **Protected prior work preserved:** Chunk 1 fail-closed service contract; Chunk 2 Migration 006, guarded capability, migration adaptations, direct capability tests, and both earlier handoffs. Migrations 001–005 and Migration 006 were not edited in Chunk 3.
- **Disposable database:** used only Compose project `labreserve-verify`, against `labreserve_test` with the actual `labreserve_app` runtime pool for application operations. The project was absent before setup. Its PostgreSQL 17.7 container, volume, and network were removed after testing; no persistent/demo target was accessed. `.env` was neither read nor modified.

## Completed Chunk 3 — Coordinator activation vertical slice

Exact controlling-plan objective: **“inactive→active and authorized active→active no-op via trusted service/runtime database, V1/V2/V6/V7; FR-002/009/023/026–027, AC-016/021/028/041/044 state facets.”**

Implemented only that activation slice:

- `internal/resources/state.go` now implements `StateService.SetState` for activation. It canonicalizes and validates the trusted actor, resolves stored Coordinator authority before Resource validation or no-op success, validates/canonicalizes the Resource ID, and uses the state transaction's shared Resource lock and separate authoritative row read.
- An authorized Active→Active request commits as an unchanged no-op without reading the clock, calling the guarded mutator, updating `updated_at`, or attempting Activity.
- An actual Inactive→Active transition samples one UTC-microsecond operation time after the Resource lock, calls the guarded state capability, requires a changed Active result, and commits once. Repository/database details are reduced to safe domain/operational outcomes; commit failure is not reported as success or definite rollback.
- Deactivation remains fail-closed: desired Inactive returns `ErrOperational` after stored Coordinator authorization; it does not lock, check occupancy, mutate, or commit. `HasBlockingBookings` and adapter-side inactive mutation are explicit error stubs, never “no blocker” success.
- New `internal/database/resource_state.go` opens an explicit READ COMMITTED transaction. Its narrow transaction wrapper reuses the existing stored-role lookup, Resource lock/read, commit, and rollback implementation without widening `resources.Repository`. Activation invokes `public.labreserve_set_resource_state` through the runtime connection; no direct Resource UPDATE privilege or new caller was added. The adapter rejects an inactive request and does not make the incomplete deactivation path available.
- `internal/resources/state_test.go` preserves the explicit desired-state contract test and adds authorization/validation precedence, active no-op, post-lock time, repository error/rollback/no-retry, and fail-closed deactivation coverage. It replaces the obsolete assertion that activation itself must remain unimplemented.
- New `internal/database/resource_state_integration_test.go` exercises the service and adapter against real PostgreSQL using `labreserve_app`: Coordinator activation, two Engineer no-op denials, invalid/unknown actors and target outcomes, retained Resource identity/raw legacy text/HTML-like description, Confirmed and Cancelled Booking snapshots, retained earlier Activity, a valid Engineer Booking after activation, Activity failure rollback and explicit retry, Active no-op under scoped Resource-UPDATE and Activity-failure detectors, and an observed Resource row-lock wait proving fresh post-lock operation/event time.

No HTTP handler, route, browser workflow, Booking implementation, Migration 006 rewrite, new privilege, or other Chunk 4+ behavior was added. `cmd/labreserve` and web composition do not yet call this service; integration evidence invokes the internal service with the actual runtime database pool, as required for this slice.

## Files and cumulative-diff accounting

### Chunk 3 changes only

- Modified the existing Chunk 1 file `internal/resources/state.go` (replaced its fail-closed activation stub; kept deactivation fail-closed).
- Modified `internal/resources/state_test.go` (kept the original contract test; replaced the obsolete activation-stub test and added activation tests).
- Added `internal/database/resource_state.go`.
- Added `internal/database/resource_state_integration_test.go`.
- Added this authorized handoff: `docs/handoffs/task-004b-chunk-3-to-4.md`.

### Earlier-chunk changes preserved in this candidate

- Chunk 1's `internal/resources/state.go` / `state_test.go` contract was already untracked before Chunk 3; the files now contain that contract plus the Chunk 3 implementation/tests.
- Chunk 2's tracked changes remain in `internal/database/booking_persistence_integration_test.go`, `internal/database/migrate.go`, `internal/database/resource_mutations_integration_test.go`, and `scripts/verify.sh`.
- Chunk 2's untracked files remain `internal/database/migrations/006_resource_state.sql`, `internal/database/resource_state_capability_integration_test.go`, and `internal/database/resource_state_migration_integration_test.go`.
- Earlier handoffs remain: `docs/handoffs/task-004b-chunk-0-to-1.md`, `task-004b-chunk-1-to-2.md`, and `task-004b-chunk-2-to-3.md`.
- The base-to-HEAD history also contains the already documented Task 4A handoff path redactions, Chunk 0 handoff, and three plan path redactions. Those are not Chunk 3 edits.

The complete tracked Chunk 2 diff and all relevant untracked earlier-chunk files were inspected and preserved. The new Chunk 3 files and the final state of the continued service/test files were read and reviewed. No unrelated code or accidental file was found. The only new working-tree paths introduced in Chunk 3 are the adapter, integration test, and this authorized handoff.

### Fingerprints

Migrations (same as implementation base / Chunk 2 handoff):

- 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`
- 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`
- 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`
- 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`
- 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`
- 006 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`

At handoff creation, Chunk 3 source hashes were:

- `internal/resources/state.go`: `a4b52bb771b49f0bac658557355db6ed3d21f1c921fa325a048dd4d973b63c35`
- `internal/resources/state_test.go`: `8b8a145095999ed970ba6d2cb697fa1f562456e7ffa0bf5f43598540b252eaa6`
- `internal/database/resource_state.go`: `ce2206785e3e7d34f098241a43f9cb26ac7df4693321362ffcf8621dff205ad4`
- `internal/database/resource_state_integration_test.go`: `7a7a1bed055ec00c748258707be7dbf71b34630588126ab621f3c35484d16d3d`

Earlier handoff hashes match Chunk 2 evidence: state capability integration `4923397dd1d305915da39f65dafd6b447ab74bcdc4006e66e2623a7b56171c14`; migration integration `b9acefac467ad644cc73c5d244d6125a86d2b67090dd7c056ac88ad199de01ed`; prior handoffs `37e3e2fb246b86c9b0d4a07c1b458763649324a5f5fc5f54e4a7c2b9b42cf50d` and `13c91f2c2f39fc46d87e075b2dae871862b4d77439e75ba3f02c37219b580f8c`.

## Verification evidence

All shell commands below ran through `rtk` from the repository root. Focused PostgreSQL commands ran in the freshly built repository verification image and isolated Compose project.

| Command / procedure | Result |
| --- | --- |
| Baseline: `rtk go version && rtk go test ./internal/resources` | **PASS**, host Go 1.27.3; 43 pre-existing Resource tests. |
| First post-edit `rtk go test ./internal/resources` | **FAIL, one attempt for one test-expectation issue.** Output: `TestResourceStateActivationRepositoryFailuresAreOperational/begin: state_test.go:157: begin calls = 1, want 0` (the parent test also reported failure). The fake records the one begin method invocation even when begin returns an error; corrected the test expectation to one. |
| `rtk gofmt -w internal/resources/state_test.go && rtk go test ./internal/resources` | **PASS**, 58 Resource package tests. The current candidate was also fully formatted after the final integration-test assertion adjustment. |
| `rtk proxy go test -run '^$' ./... && rtk proxy go build -o /tmp/labreserve-task4b-chunk3 ./cmd/labreserve` | **PASS**, all Go packages/test packages compiled; no behavior tests were selected by the first command. Application build succeeded. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify build verify-go verify-migrate` | **PASS**, images built from current candidate source (Go test image uses repository-pinned Go 1.26). |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify up --wait -d verify-db`; `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm verify-migrate`; `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm verify-seed` | **PASS**, fresh disposable PostgreSQL 17.7, explicit migrations and seed. No persistent target was touched. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState' ./internal/resources` | **PASS**, discovered all 7 Resource state unit tests. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResourceState(Activation|Capability)|TestResourceCreation)' ./internal/database` | **PASS**, discovered the state activation/capability and Resource creation suites. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources` | **PASS**, all 7 state unit tests and subtests. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v ./internal/resources` | **PASS**, full Resource service unit package, including accepted create/edit regressions. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResource(Creation|Edit|Capabilities)|TestResourceState(Activation|Capability)|TestBookingCreationValidationAndEligibilityOutcomes)' ./internal/database` | **PASS**, discovered 29 top-level tests. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResource(Creation|Edit|Capabilities)|TestResourceState(Activation|Capability)|TestBookingCreationValidationAndEligibilityOutcomes)' ./internal/database` | **PASS**, all 29 discovered top-level real-PostgreSQL tests and their subtests. Covered new activation integration, direct Migration 006 capability/privilege behavior, prior Resource create/edit regressions, and normal Booking eligibility. Actual runtime operations used `labreserve_app`; no test skips were reported. |
| `rtk gofmt -l internal/resources/state.go internal/resources/state_test.go internal/database/resource_state.go internal/database/resource_state_integration_test.go` | **PASS**, no output. |
| `rtk git diff --check` | **PASS** for tracked changes; complete new/continued source files were also reviewed. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify down --volumes --remove-orphans` | **PASS**, removed only the created disposable container, volume, and network; filtered follow-up discovery found no project container/volume. |

The initial broad regression selector was accidentally end-anchored and selected only the exact `TestBookingCreationValidationAndEligibilityOutcomes`; it passed. The corrected prefix selector above then discovered and passed all 29 intended tests. This was not counted as a failed validation attempt. No required selected test was skipped.

**Not run:** full `make verify` (not required by Chunk 3's controlling-plan validation; no HTTP/web composition changed); Playwright/browser tests (Task 4C excluded); Migration 005→006 upgrade/mismatch tests (unchanged Chunk 2 tests and source, with prior PASS evidence retained in the Chunk 2 handoff); definite COMMIT-rejection and ambiguous-acknowledgement suites (later Chunk 9); deactivation/booking and cancellation race suites (later Chunks 4, 6, and 7); and full lifecycle/restart verification (later Chunk 10/11). The fresh explicit migration and runtime invocation of Migration 006's guarded capability did run in this chunk.

## Diff, invariants, and risk review

- **Scope:** Chunk 3 changed the activation service slice, its narrow database adapter, and focused tests/handoff only. No Chunk 4 or HTTP/browser work.
- **Chunk 2 preservation:** Migration 006 source/hash and capability tests remain byte-identical to the Chunk 2 handoff. Existing ACL/guard tests passed; no direct Resource privileges or runtime grants changed. Migrations 001–005 remain byte-identical.
- **Authorization:** service resolves persisted Coordinator authority before target validation and Active no-op success; actual runtime tests rejected both Engineers and unknown/malformed actors. The database capability retains its own Coordinator guard. The application still supplies the trusted actor ID; this is not a new SQL authentication boundary.
- **Resource/data integrity:** activation writes only via the guarded capability. Integration snapshots verified code, name, description including legacy edge whitespace/HTML-like text, creation time, retained Confirmed/Cancelled Booking rows, and prior Activity; only state/update time and one activation event changed.
- **Lock/time/Activity:** shared Resource `FOR UPDATE` plus separate locked read is reused; an actual PostgreSQL waiter was observed and clock calls remained zero until lock release. The one post-lock UTC-microsecond time was used for `updated_at` and event. Activity-trigger failure left Resource/history unchanged; explicit retry produced one event. Active no-op was exercised with both UPDATE and event-failure detectors installed.
- **Functional risk:** deactivation is intentionally unavailable until Chunk 4. The service has no web/composition-root caller yet; browser behavior is out of scope. Activation after a future Confirmed Booking was retained is covered at the guarded capability boundary from Chunk 2, and ordinary Engineer Booking after service activation passed.
- **Integration/validation gap:** no browser/session-to-service call path, deactivation behavior, actual COMMIT rejection, ambiguous acknowledgement, or full restart/replay lifecycle is claimed here. These remain in their assigned later chunks. Persistent Migration 005 ledger compatibility remains unresolved for uninspected persistent targets; no such target was inspected or changed.
- **Rollback:** to revert only Chunk 3, restore the Chunk 1 fail-closed `SetState` and its previous test contract, remove `internal/database/resource_state.go` and `internal/database/resource_state_integration_test.go`, and remove this handoff only if authorized. Preserve Chunk 1/2 work and Migration 006; do not down-migrate or alter any ledger.

## Exact next-session instruction — Chunk 4 only

A fresh session must revalidate authorization, branch/HEAD/working-tree state, the complete Task 4B contract and plan, current plan/task hashes, and this complete handoff before editing. Preserve all accepted Chunk 1–3 code and Chunk 2 migration evidence. The next authorized implementation request should execute **Chunk 4 — Safe deactivation, occupancy boundaries and complete no-ops** only, per the controlling plan.

Chunk 4 must implement authorization-before-equality, both genuine no-ops, and only the actual Active→Inactive path's fresh post-lock time plus separate occupancy query `resource_id = target AND state = 'confirmed' AND end_at > operation_time`. Keep the guarded Migration 006 checks; do not rewrite Migration 006 for convenience. Never infer “unoccupied” from an error, mutate/cancel Bookings, introduce HTTP handlers, or proceed into Chunk 5. Add the plan's real-PostgreSQL boundaries/failures and service tests, run that chunk's focused verification, inspect the cumulative diff, write its authorized handoff only if the fresh session is authorized, and stop.

Persistent Migration 005 compatibility remains a prerequisite only before any persistent-target operation. Disposable migration success is not evidence that an uninspected persistent ledger is compatible.

**Chunk 3 status: CONTINUABLE.**
