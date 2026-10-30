# Task 4B — Atomic Resource Activation and Safe Deactivation: Implementation Plan

## 1. Plan identity and controlling sources

- **Mode:** `SINGLE_PLAN`; documentation only. **Planner assessment:** `PLAN_READY`, subject to subsequent independent plan validation and implementation preflight.
- **Controlling task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md` at specification commit `91c491eb098aa57c2a0524ebcdb26849f8b72857`.
- **Specification fingerprint:** SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`; committed blob `3bec8a41e298087182b44505ccc686410f9db4e0`. Both the committed content and working copy were inspected; hashes agree.
- **Approval facts:** the user reports independent `DRAFT_SPEC_READY`, subsequent human approval and commitment, and an approval-status-only change from the reviewed draft. The committed document says **approved for repository incorporation, not approved for implementation**. The earlier review transcript/draft was not independently available; these review/approval facts are user-supplied, not a new review verdict.
- **Planning baseline / candidate before writing:** branch `main`, HEAD `91c491eb098aa57c2a0524ebcdb26849f8b72857`, tree `f9b3034d58def8c243a1426585245716da8b4502`; clean index and working tree, no untracked files. Local branch reports one commit ahead of its local `origin/main` tracking reference; no fetch performed.
- **Parent:** `3542003c9929dc61aa8e9d3ce4be9adad9aff8c0`. HEAD adds only the Task 4B document to that parent. This current HEAD, not Task 4A's old implementation base, is the planning base.
- **Candidate after planning:** same HEAD/index plus exactly this new untracked Markdown file. Record its SHA-256 externally in the planning report; do not embed a recursive self-checksum. Future execution must fingerprint the then-current plan, sources, base and dirty delta anew.
- **Project mode:** existing application; accepted Resource creation/editing and Booking creation/cancellation are present. Task 4B state capability is absent.
- **Execution mode:** `UNSELECTED`. No implementation authorization, preflight, chunk execution, tests, builds, migrations or service startup occurred during planning. This plan does not supply any of those gates.
- **Sole controlling ladder:** section 9 of this file. No candidate ADS or other ladder controls execution. All implementation checkboxes are intentionally unchecked.
- **Destination:** no existing plan directory/convention was found under `docs`; both `docs/implementation-plan` and `docs/implementation-plans` were absent. The requested fallback destination was checked absent before creation. No earlier plan was overwritten.

**Smallest complete outcome:** trusted Coordinator + existing Resource ID + explicit desired state → authorized, serialized transition / genuine no-op / occupancy rejection → retained Resource and either exactly one atomic transition event or no mutation/event.

### Source inventory and authority

All following repository documents are tracked/committed at the planning HEAD and were read completely, including endings. Source sections, not historical line numbers, control interpretation.

| Source | Authority and use |
| --- | --- |
| `AGENTS.md`; session instructions from `/<REDACTED>/AGENTS.md` | Repository workflow, preservation, verification, authority and separate-session review. The global file is an external session instruction, not a claimed repository-tracked source. |
| `docs/prd.md`; `docs/baseline.md` | Product truth; especially PRD Resource management, atomicity, time/durability, AC-009/010/029/044; baseline §§12–13. |
| `docs/architecture/001-initial-architecture.md` §§2, 4, 6–11, 13, 15 | READ COMMITTED, trusted actor, Resource locking, Account coordination compatibility, atomic history, lifecycle. Historical proposed clauses are read with later accepted contracts. |
| `docs/architecture/002-my-bookings-and-cancellation.md` §§4–5 | Accepted owner-only cancellation and Resource → Booking locking. |
| `docs/architecture/003-cancellation-privilege-boundary.md` | Specific accepted direct-SQL cancellation-column limitation; not permission for an unaudited Resource write. |
| `docs/architecture/004-resource-creation-and-text-contracts.md` §§2–7 | Approved D4-001–D4-003; preserve text/code/initial-state behavior and guarded mutation/event pattern. Historical Task 4A gate wording does not reopen accepted Task 4A. |
| `docs/tasks/002a-booking-persistence.md`, `002b-atomic-booking-creation.md`, `002c-booking-web-workflow.md` | Accepted persistence/exclusion, Account → replay → Resource, retained replay, Jakarta/browser contracts. |
| `docs/tasks/003a-my-bookings.md`, `003b-atomic-owner-cancellation.md`, `003c-cancellation-web-workflow.md` | Accepted read visibility/pagination, cancellation time/no-op/retention and browser boundaries. Code names override historical API suggestions for navigation, not behavior. |
| `docs/tasks/004a-atomic-resource-creation-editing.md` | Accepted predecessor obligations, no-op and privilege regressions. Task 4B explicitly records its acceptance notwithstanding historical draft wording. |
| `docs/tasks/004b-atomic-resource-activation-deactivation.md` | Selected bounded contract: all objective outcomes, precedence, security, V1–V9 and completion obligations. |
| `docs/handoffs/task-004a-review-r4a01-to-r4a02.md`, especially “Migration policy and database evidence” | Historical evidence only: conditional earlier-005 checksum concern, not current database certification or execution policy override. |
| `README.md`, `Makefile`, `scripts/verify.sh`, `compose.yaml`, `Dockerfile`, `go.mod`, accepted code/tests listed below | Observed engineering interfaces and commands; not independent proof that behavior passes today. No other repository engineering-instruction file was discovered. |

## 2. Scope and exclusions

**Implement only:** Coordinator-authorized explicit desired-state commands; changed/unchanged outcomes; unknown/invalid/auth/occupancy/operational outcomes; activation; safe deactivation; authoritative post-lock time; Resource serialization; exactly one atomic Activity per actual transition; narrow migration-managed state capability; deterministic PostgreSQL evidence; additive upgrades, non-destructive seed/restart compatibility; regression preservation and accurate README notes.

**Never implement or scaffold:** Task 4C routes/handlers/status mapping/forms/navigation/buttons/new management Playwright workflows; automatic cancellation/shortening; cross-owner Coordinator cancellation/reasons; Activity-history API/UI; Resource deletion/code changes; additional descriptive editing/normalization/backfill; new Booking states or altered replay/validation/exclusion; Resource request-key tables; unrelated privilege hardening, dependencies, services, infrastructure, deployment or credential changes. Existing HTTP/browser regression tests remain required, not authority to add management adapters.

**Protected:** requirements/architecture/accepted task files; migrations 001–005 byte-for-byte; existing Resource/Booking/history/session data; trusted session/actor boundary; unrelated staged/unstaged/untracked work. Normal state operations write only `active` and `updated_at` and insert their required event; they do not rewrite raw text, even legacy edge whitespace.

**Required documentation:** internal service API/outcomes, event encoding, lock/time/transaction behavior, effective permissions and trusted identity/time limitations, additive upgrade prerequisites and non-destructive recovery implications, verification evidence, and explicit 4C/intervention/history deferral. No new browser demo is required in this task.

## 3. Observed baseline and evidence limitations

### Accepted implementation inspected

| Area / observed paths and symbols | Consequence for this plan |
| --- | --- |
| `internal/resources/service.go`: `Actor{AccountID}`, `Resource`, `ResourceTransaction`, `CreationRepository`, `EditingRepository`, `Repository`, `Service.Create`, `Service.Edit`, `EditResult.Changed`, sentinel errors, UUID helpers, injected `now` | Reuse actor/result/error conventions. Do not enlarge existing create/edit repository interfaces merely to make old fakes implement state work. |
| `internal/database/resource_creation.go`: `resourceCreationTransaction`, `ResolveCoordinator`, `LockResource`, create/edit adapters, `Commit`, `Rollback` | READ COMMITTED; ordinary stored-role read; Resource lock helper followed by separate row read; bounded rollback context in service. Small reuse permitted without changing old contracts. |
| `internal/database/migrations/005_resource_mutations.sql`: `public.labreserve_create_resource`, `public.labreserve_edit_resource` | Guarded create/edit plus internally derived Activity, Coordinator checks, non-superuser migration owner, fixed `pg_catalog` path, qualified objects, revoked PUBLIC EXECUTE. No existing state mutator. |
| `internal/bookings/service.go`; `internal/database/bookings.go`: `Service.Create`, `BeginBookingCreation`, `LockAccount`, `FindBookingByRequest`, `LockResource`, `InsertBooking`, `AppendBookingCreatedEvent` | Account `FOR NO KEY UPDATE` → separate retained replay/reuse → Resource for new request. Replays never need Resource eligibility/lock. Immediate exclusion remains final backstop. |
| `internal/bookings/cancellation.go`; `internal/database/booking_cancellation.go`: `CancellationService.Cancel`, `FindBookingResource`, `LockResource`, `LockBooking`, update/event methods | Actual accepted service is `Cancel`, despite older contracts mentioning `CancelOwner`. Resource → Booking, ownership before Cancelled no-op, fresh time for Confirmed transition. |
| Migrations 001–004; `deploy/postgres/01-roles.sql` | Stable UUIDs, non-null text/state/times, `lower(code)` unique index, two Booking states, retained owner/request key, immediate Confirmed exclusion, exactly-one Activity target. Runtime has no Resource writes; existing cancellation-column grants and Activity SELECT/INSERT remain. |
| `internal/auth/service.go`: `Login`, `Resolve`, `Logout`; `internal/database/store.go`: `SessionByTokenHash`, Resource list/detail reads; `internal/web/middleware.go` | Opaque sessions join current stored account/role. State commands get only a trusted account ID, not a submitted role. No new management HTTP surface. |
| `internal/database/migrate.go`: `Migrate`, `CheckSchema`, `readEmbeddedMigrationChecksums`, `validateMigrationLedger`, `requiredSchemaVersion`; `seed.go`; `cmd/labreserve/main.go` | Embedded sorted SQL, per-migration transaction/ledger, SHA-256, advisory lock; startup/seed require exact schema and do not migrate implicitly. Seed only inserts missing accounts/Resources. |
| Resource tests: `service_creation_test.go`, `service_edit_test.go`, `service_test.go`; database `resource_creation`, `resource_edit`, `resource_mutations`, `resource_concurrency`, `resource_commit_rejection`, `resource_lifecycle` integration files | Existing fixtures, no-UPDATE detector, scoped Activity failures, role/ACL checks, raw-data snapshots, real deferred-COMMIT rejection and actual restart seams can be extended. |
| `internal/database/booking_creation_integration_test.go`, `booking_cancellation_integration_test.go`, `foundation_integration_test.go` | `testPool` requires disposable `labreserve_test` but skips if URL absent; use actual runtime pool. `holdBookingResourceLock`, `holdBookingAccountLock`, `waitForBlockedBookingCalls`, `atomicBookingClock` available. |
| `internal/web/foundation_integration_test.go` | Existing authenticated session → internal Resource service and escaped read-path evidence; `webTestPool`, login helpers. New state seam tests may live in a separate test file without routes. |
| `scripts/verify.sh`, `Makefile`, `compose.yaml`, `tests/e2e` | `make verify` uses isolated `labreserve-verify`, PostgreSQL 17.7, Go 1.26.0, actual migrator/runtime logins, existing Playwright, fixture/replay runs around actual database/application restarts. |

Task 4A ancestry was checked: merge `3542003...` has parents `a90d3fd37226b5a648153edf7d134ec605a78dc0` and reviewed candidate `e5b3c738f6d014880efe6aa6f1315c67b37bca80`; that candidate is an ancestor of HEAD and its full tree equals the merge tree. Acceptance is reported in Task 4B/user instructions; ancestry/tree inspection does not re-run earlier acceptance.

### Immutable migration fingerprints

| Existing file under `internal/database/migrations/` | SHA-256 at planning HEAD |
| --- | --- |
| `001_foundation.sql` | `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f` |
| `002_booking_persistence.sql` | `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4` |
| `003_booking_creation_locks.sql` | `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf` |
| `004_booking_cancellation.sql` | `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45` |
| `005_resource_mutations.sql` | `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7` |

`requiredSchemaVersion` is currently `005_resource_mutations.sql`; 006 is currently unused. `scripts/verify.sh` asserts five migrations twice. `TestResourceCapabilityMigrationPreservesVersion004Data` currently identifies 005 via the latest required version and expects four predecessors; adding 006 without adapting this would break/pollute its upgrade simulation. Repair these assumptions with the new migration, preserving old upgrade evidence.

### What planning actually established

Read-only Git status/diffs/log/tree/ancestry/tracked-file checks, path discovery, source reading and SHA-256 checks ran. Staged/unstaged diffs were empty; no source drift was observed. No project tests/builds/formatters/migrations/lifecycle commands ran. No live ledger, container inventory or persistent database was inspected in this session; earlier inventory in Task 4B is historical only. `.env` existence was seen but its contents were not read. Runtime tooling/cache/access and current pass/fail baseline remain unverified until authorized Chunk 0. No current test count or passing baseline is claimed.

## 4. Requirement-to-chunk coverage

V1–V9 are Task 4B's local verification groups, not invented PRD identifiers. Chunk numbers refer only to section 9. “Preserve” means keep accepted implementation and prove regression evidence, not reimplement it. Each row includes both a change/preservation path and a verification path. All commands in sections 8–9 are **PLANNED_NOT_RUN**.

### Complete Task 4B verification map

| ID / authoritative source | Observable required behavior | Accepted dependency | Implementation / preservation | Verification and concrete intended evidence |
| --- | --- | --- | --- | --- |
| V1, “Trusted authorization and domain boundary” | Stored Coordinator succeeds; both Engineers rejected for both directions and matching states; missing/malformed/unknown actor unauthenticated; authorization before target/validation/no-op; invalid vs unknown target; role failure operational; trusted session seam; direct null/Engineer/unknown capability calls denied | Resource actor/errors/role read; auth/session resolution | 1, 3–5; preserve auth in 5, 11 | 3–5 unit + actual runtime PostgreSQL + session-seam tests; errors and unchanged Resource/Booking/event snapshots; no new routes |
| V2, “Activation, deactivation, and genuine state no-ops” | Both transitions changed; matching desired state unchanged with no UPDATE/event attempt/time rewrite, including inactive legacy occupancy; unauthorized no-op denied; activated valid new booking works; deactivated new request fails inactive before conflict and consumes no successful key; opposite transitions at equal timestamp remain real changes | Resource lock/no-op fixtures; Task 2B eligibility | 2–4, 8; preserve creation in 3–4 | 2–4 detectors and event failures installed during no-op; persisted fields/counts; eligibility outcomes; 8 serial equal-clock transitions; 11 gate |
| V3, “All-owner occupancy and controlled end boundaries” | Exact Confirmed AND end_at > fresh now, all owners/dates; future, ongoing, exact start block; empty/Cancelled/completed/mixed/other Resource permit unless one real blocker; end−1µs blocks, end/end+1µs permit; lock wait across end uses fresh time and still-before-end rejects; no Booking mutations | Two-state Booking data; Resource lock; injected microsecond clock | 2, 4 | 2 direct capability and 4 service PostgreSQL boundary matrix; observed wait, zero pre-release clock reads, timestamps equal post-lock time, full retained snapshots |
| V4, “Booking creation/deactivation race orderings” | Force creation first → blocked deactivation, deactivation first → inactive new creation/no key binding; fresh creator lead-time rejection; same Coordinator Account creation cannot cause Resource→Account cycle; replay/reuse before lock even inactive | Account `FOR NO KEY UPDATE`, replay lookup, Resource helper, exclusion | Preserve 2B; 2, 4, 6 | 6 independent runtime operations with identified holder/waiter PIDs/barriers; exact result/event counts; 10 retained replay; 11 gate |
| V5, “Cancellation, state submission, edit, and rollback races” | Both cancellation/deactivation orders, remaining blocker, explicit resubmission, cancellation wait crosses start; repeated desired state each direction one UPDATE/event then no-op; opposite orders both initial states; edit/state preservation; failing Resource mutation/Activity with waiter; failing creation/cancellation/deactivation with waiter | Owner-only cancellation, Resource→Booking, 4A edit | Preserve prior services; 4, 6–8 | 6 creation/deactivation rollback, 7 cancellation-specific and 8 state/edit/rollback PostgreSQL schedules, actual lock ordering not unordered starts; fresh-time/outcome and complete snapshot/event evidence |
| V6, “Activity, operation times, transaction rejection, and uncertainty” | Correct actor/action/Resource/null Booking/old-new state/time in each direction; fresh activation and deactivation time; Activity failure+explicit retry, outer rollback, actual rejected COMMIT both directions; lost acknowledgement with committed/noncommitted paths; no blind retry/false rollback claim; begin/role/lock/query/mutation/context failure safe and releases locks, diagnostics redacted | Atomic 005 pattern; safe operational wrapper; deferred trigger observation seam | 2–4, 8–9 | 2–4 basic atomicity alongside behavior; 8 waiter rollback; 9 real deferred COMMIT rejection after successful statement, fresh reads, transaction-seam acknowledgement tests and call counts; 11 gate |
| V7, “Retained identity, text, Booking/replay, and read compatibility” | Non-state Resource and complete Booking/request/cancellation/prior Activity unchanged around success/reject/no-op/rollback; raw legacy whitespace/HTML text untouched; existing reads correct; Cancelled/past replay while inactive; changed key before eligibility; per-owner keys; ordinary conflict/adjacency/different Resource/time/purpose/cancellation intact | Existing snapshot/read/escaping, replay/cancellation contracts | Preserve 4A/2A–3C in 3–4, 6–10 | 3–4 snapshots, 5 session/read integration, 8 edit interaction, 10 comprehensive retention/replay suite; 11 accepted HTTP/browser/database regressions |
| V8, “Effective runtime permissions and additive migration” | Only narrow capability EXECUTE; deny direct Resource INSERT/every-column UPDATE/DELETE/TRUNCATE; final Coordinator/no-op/occupancy/event guards; safe ownership/path/ACLs/shadow resistance, denied replacement/alteration/owner assumption/ungranted caller; no unrelated grant expansion; fresh + compatible 005 upgrade, exact ledger/repeat safety and preserved 004→005 evidence | 001–005, roles, migrator, effective privilege fixtures | 2, 5; preserve old grants/ledger in 10–11 | 2 upgrade/data/index/FK/schema tests; 5 catalog/effective/actual abuse attempts, including shadow bookings; 10 fresh/repeat/lifecycle; 11 unchanged hashes and full gate |
| V9, “Lifecycle, accepted regressions, and required repository gate” | Actual PostgreSQL AND app restarts retain both transitions and exact events/timestamps, past/Cancelled/replay/history, deactivated seeded state; seed/start repeat safe; post-restart no-op/opposite change/privileges; preserve 4A fixtures and all earlier Go/HTTP/Playwright checks; report execution/skips/synchronization/candidate | Existing separate fixture/replay lifecycle and full entry point | 2 count/upgrade adaptation; 10 lifecycle/docs; preserve all in 11 | 10 new state fixture/replay alongside unchanged 4A/3B evidence, actual restart and fresh connections; 11 full make verify with all expected suites and no hidden skips |

### PRD and acceptance allocation

Broad product criteria below are split by facet, as the controlling task requires; no claim of full Delivery 4/browser acceptance is made.

| Exact PRD sources (baseline equivalents where applicable) | Required behavior / disposition | Accepted dependency | Implementation / preservation chunks | Verification chunks and intended evidence |
| --- | --- | --- | --- | --- |
| FR-001–002, FR-023 state authority; NFR-002 server enforcement; AC-015–017 applicable state/session facets | Trusted identity/Coordinator authorization, never role claim/no-op bypass | Auth sessions and 4A authorization | 1, 3–5; preserve auth 11 | V1/V8 in 3–5; existing unauthenticated/logout/actor tests 11 |
| FR-005, FR-023 state/retention; AC-028 state/identity, AC-041 existing-state eligibility | Explicit active/inactive changes restore/remove ordinary new-booking eligibility without identity/text/Booking changes | Resource schema/read paths and Task 2B | 2–4, 10 | V2/V7 snapshots and new-booking results 3–4, 10; full reads 11 |
| FR-024–025; AC-009–010, AC-029; baseline FR-06, AC-09/10, §13 | Reject future-ending Confirmed occupancy, exact end allowed, no automatic cancellation; consistent races/fresh time | Shared Resource protocol, owner cancellation | 2, 4, 6–8 | V3–V5 controlled boundaries and both forced orderings; blocked outcome is explanation-capable domain error, not new UI |
| FR-026–027 state facets; AC-012 state, AC-028 event, AC-044 state; baseline FR-07, AC-12 | Exactly one accurate event with each actual state change; no event or UPDATE for no-op/reject; complete atomicity | Activity table, guarded 005 pattern | 2–4, 8–9 | V2/V6 detector, persisted details, failure, outer rollback, real COMMIT rejection and unknown outcome evidence |
| NFR-001 operation-time persistence; NFR-007–008; AC-013 state, AC-030 changed-state preservation | UTC microsecond post-lock operation time; additive durable upgrade/seed/restart | Checksummed runner/seed/restart harness | 2–4, 10 | V3/V6 boundary times; V8 upgrade; V9 actual restart including seeded Inactive preservation |
| NFR-010–011; User Story 35 | Controlled isolated tests, independent PostgreSQL operations, one verification entry point and candidate/evidence, independent review | Existing Go/Compose/Playwright and AGENTS | 0 and all change/preservation chunks | Each chunk gates; 11 full discovery/results/report; separate fresh reviewer after execution, not planner self-approval |
| FR-003–004/006; NFR-004–006; AC-015 logout, AC-032 | Preserve sessions, fictional safe seed/credentials, local/offline ordinary operation | Foundations/auth/Compose | Preserve 5, 10–11 | Existing security/seed/browser checks; no secrets/new external dependency in diff |
| FR-007–008, FR-017–018; NFR-001 display/NFR-003; AC-011/014, AC-026–027/031/034–035 | Preserve browse/state distinction, Jakarta labels/intersections, plain text and retained stable owner-filtered 25-record pages | Read models, templates, My Bookings | Preserve 5, 10–11 | V7 read integration and existing HTTP/Playwright ownership/pagination/timezone/content suites |
| FR-009 ordinary creation, FR-010–016; AC-001–006/018–023/033 | Preserve validation, two states, exclusion/adjacency/different Resources and canonical retained per-owner request identity; no altered replay on Inactive | Tasks 2A–2C | Preserve 3–4, 6, 10–11 | V4/V7 existing creation suites plus inactive Cancelled/past replay and lead-boundary waiter |
| FR-019–022 owner-only, FR-021 retention; AC-007–008/025/036–040 | Preserve ownership even Cancelled, strict fresh start boundary, interval release, atomic cancellation and replay across restart | Tasks 3A–3C; baseline §12 | Preserve 7–8, 10–11 | V5/V7 cancellation/rollback races plus original owner/browser/restart regressions |
| FR-023 create/edit/code facets; FR-026–027 prior mutations/seed; AC-028 create/edit, AC-042–043, AC-044 create/edit; D4-001–003 | Preserve canonical creation/edit/no-op, immutable case-insensitive code, both initial states with one creation event, all earlier history | Accepted 4A, architecture 004 | Preserve 2–5, 8, 10–11 | Old capability/uniqueness/no-UPDATE/commit/004→005/restart tests; state/edit before/after snapshots |
| FR-029 existing Booking errors; NFR-002 existing browser CSRF; AC-031 existing protections | Preserve safe current responses, auth/CSRF/Origin/content handling | Existing web middleware/adapters | Preserve 5, 11 | Existing HTTP/Playwright suites and no management adapter diff |

**Explicit deferrals / outside allocation:** FR-023/FR-029 Resource browser workflow, NFR-009 Resource usability and new Resource CSRF/direct-HTTP portions of NFR-002/AC-016/028/031/041/044 belong to Task 4C. Internal explanatory occupancy outcome is current. FR-019–020 cross-owner/reason and FR-022 intervention retry facets / AC-024 belong to a later intervention task. FR-028 inspection belongs to a later history delivery; preserve no history edit/delete surface here. PRD non-goals and CR-01/CR-02 remain outside scope. User Stories 24–26/30 are implemented internally here; story 23 is accepted 4A plus deferred 4C, story 29 browsing remains deferred while recording is mandatory. There is no missing in-scope behavior hidden by these deferrals.

## 5. Authorized engineering decisions and proposed interfaces

Everything named **proposed** below is new, not an assertion of an existing API/file. Confirm spellings/collisions in Chunk 0. These are Level 1 choices permitted by Task 4B; an executor may select an equally narrow alternative in this same plan with reasons and preserved coverage, not a second ladder.

1. **Proposed separate `resources.StateService`** in `internal/resources/state.go`, with `NewStateService`, `SetState(ctx, Actor, StateRequest) (StateResult, error)`. `StateRequest` contains only `ResourceID` and explicit `Active bool`; `StateResult` contains `Resource` and `Changed`. Reuse existing `Actor`, `Resource`, errors/UUID conventions. Proposed additional sentinel `ErrResourceOccupied` has explanatory safe wording; no HTTP status codes. Separate service keeps accepted create/edit `Repository` and fakes unchanged.
2. **Proposed narrow contracts:** `StateRepository.BeginResourceStateChange(ctx) (StateTransaction, error)`; `StateTransaction` embeds existing `ResourceTransaction`, exposes `LockResource(ctx,id) (Resource,bool,error)`, `HasBlockingBookings(ctx,id,at) (bool,error)` and `SetResourceState(ctx,actorID,resourceID,desiredActive,at) (Resource,bool,error)`. Exact Go parameter types follow existing context/string/bool/time conventions. The service owns authorization/transaction/lock/time/occupancy; the capability repeats final guards defensively.
3. **Proposed adapter** `internal/database/resource_state.go`: READ COMMITTED transaction reusing existing role/lock/commit/rollback behavior through narrow composition (e.g. embed `resourceCreationTransaction`), not a generic repository framework. Query occupancy via qualified `EXISTS` after locking and time capture; no Booking row or Account coordination lock. Every collaborator uses the same connection/transaction.
4. **Proposed additive migration** `internal/database/migrations/006_resource_state.sql` (next prefix rechecked at execution), providing **proposed** `public.labreserve_set_resource_state(uuid,uuid,boolean,timestamptz)`, returning retained Resource columns and changed flag. It authorizes stored Coordinator, locks Resource, reads authoritative current state, returns no-op before occupancy, and for deactivation executes a separate subsequent Confirmed/future-end query. Keep the mutation function VOLATILE, not STABLE/IMMUTABLE, so nested statements preserve the READ COMMITTED fresh-snapshot contract rather than reusing an enclosing stale snapshot. It changes only `active,updated_at` and inserts its derived event. No caller old-state/occupancy/event payload/skip flag. Map only a deliberately identified occupancy rejection to the occupancy sentinel; no broad SQL-error→domain mapping. Unknown row stays distinguishable. Null/invalid capability inputs safely reject; generic failures stay operational.
5. **Proposed actions/details:** `resource.activated`, `resource.deactivated`; `{"before":{"active":false},"after":{"active":true}}` and reverse. Actor/target/action/details come from the guarded operation and locked row; Booking target null; event time = new `updated_at`. Equal clock microseconds do not suppress real opposite transitions. No unrelated text snapshot requirement.
6. Service authorizes before target validation, locks/re-reads, compares desired state first, and commits an authorized no-op without calling mutation/occupancy. If no-op commit fails, return safe operational failure. Actual transitions sample injected `now().UTC().Truncate(time.Microsecond)` after Resource wait; deactivation uses it in separate occupancy query and capability/event. Capability reacquires same held lock; no competing wait is introduced. If another eligibility-relevant wait is added, refresh time before final decision. DB cannot authenticate arbitrary SQL-supplied actor/time; trusted application remains responsible.
7. SECURITY DEFINER follows 005: non-superuser migration-controlled owner, fixed `search_path=pg_catalog`, schema-qualified dependencies including bookings, no unnecessary dynamic SQL, PUBLIC EXECUTE revoked, only necessary runtime EXECUTE without grant option. No direct Resource writes, function replacement/alteration/owner assumption, or broader Account/Booking/Activity/schema grants. Existing cancellation exception and standalone Activity INSERT limitation remain explicitly unchanged.
8. Test-only transaction wrappers/channels may pause after actual lock/mutation and before commit, expose backend PID, or model acknowledgement loss. No production time/fault endpoint. Fixed `atomicBookingClock` is preferable at exact boundaries; `resourceOperationClock.Now` increments a microsecond and must not accidentally shift expected equality. Event UUID ordering is not causal ordering, especially at equal timestamps; inspect per-step snapshots or before/after transitions rather than claiming UUID order proves causality.
9. Commit errors remain safe `ErrOperational` with no confirmed success, no blind retry and no definite rollback assertion when acknowledgement is ambiguous. Test durable commit then synthetic acknowledgement loss and rollback-before-error separately. No new externally visible commit-status protocol or Resource idempotency key is required.

**Level 0:** ordinary implementation/debugging/fixtures/formatting and smallest in-scope repair. **Level 2:** any changed product/security/trust/locking/isolation/replay rule, eventless Resource write, new risk exception, destructive reconciliation or excluded feature. No unconditional Level 2 planning blocker identified. Missing implementation authorization is an intentional future gate, not permission inferred from `PLAN_READY`.

## 6. Effort and dependency summary

One experienced engineer; **1 engineer-day ≈ 6 focused hours**. Ranges include implementation, associated tests, per-chunk review and handoff; final row includes delivery-wide verification/docs audit without double-counting earlier implementation. Excludes image downloads, human review/authorization, persistent-target remediation and other long external waits. Not a promise of agent runtime or calendar duration.

| Chunk | Bounded outcome | Depends on | Focused hours | State |
| --- | --- | --- | --- | --- |
| 0 | Discovery and baseline | Separate authorization/preflight | 3–5 | Not started |
| 1 | Minimal fail-closed service contracts | 0 | 2–3 | Not started |
| 2 | Guarded state capability and safe schema integration | 1 | 7–11 | Not started |
| 3 | Activation vertical slice | 2 | 5–8 | Not started |
| 4 | Safe deactivation and genuine no-ops | 3 | 6–10 | Not started |
| 5 | Runtime privilege and trusted-session evidence | 4 | 5–8 | Not started |
| 6 | Booking creation/deactivation races | 5 | 5–8 | Not started |
| 7 | Owner cancellation/deactivation races | 6 | 4–7 | Not started |
| 8 | State/edit serialization and rollback waiters | 7 | 6–10 | Not started |
| 9 | Definite COMMIT rejection and ambiguous acknowledgement | 8 | 5–8 | Not started |
| 10 | Retention/replay and migration/restart lifecycle integration | 9 | 6–10 | Not started |
| 11 | Delivery-Wide Verification, Documentation, and Candidate Review | 10 | 4–7 | Not started |

**Total: 58–95 focused hours = 9⅔–15⅚ engineer-days.** Main uncertainty is deterministic negative/fault/commit/lifecycle fixture work, not routine boolean mutation. Material source drift or an incompatible persistent ledger requires re-estimation; no remediation effort is implicitly included.

## 7. Execution contract

### Compatibility and policy provenance

Installed `chunked-implementation` entrypoint and `core-safety-contract.md`, `execution-policy.md`, `delivery-execution-contract.md` were read, together with the single-plan contract/template/quality references. No executor version tag is supplied; inspected SHA-256 identities are:

| Executor source under `/<REDACTED>/chunked-implementation/` | SHA-256 |
| --- | --- |
| `SKILL.md` | `b83fdc31d9710b356e66c5966fac003d2749aec8b27eeab2b22a0e1de102cc45` |
| `references/core-safety-contract.md` | `4d5ed44e7b5823d7a005933bbf198cd391224e98cff57fb2c932cae1e83dd614` |
| `references/execution-policy.md` | `1f367383c371aac18dd2fbdea1a930015b718a190ad711d070d2d8c2fb9a2f31` |
| `references/delivery-execution-contract.md` | `f1dbc726a84774a52c1420eb75a707a63ffc8190624088665e2adcf14c3f0c38` |

The embedded rules below make the plan usable without these private paths. Re-read the installed executor at execution; reconcile material policy drift before work.

### Non-overridable safety

- Explicit implementation authorization and a current approved/committed contract/base-specific specification preflight must precede implementation. Repository-incorporation approval does not satisfy that authorization. This planning session does not begin preflight or emit `PREFLIGHT_PASS`.
- Use this one ladder. Re-read current AGENTS, whole Task 4B and referenced requirements/architecture plus chunk context/handoff before every fresh worker. Record source/plan fingerprints, actual base/candidate and existing dirty work. Revalidate affected controls after material drift; do not reuse stale gate evidence.
- Exactly **one chunk per fresh worker/session**. No multi-chunk same-session implementation. Each intermediate state compiles, fails safely when incomplete, and has its required focused evidence; no known safety/compile defect may be deferred as “later cleanup.”
- Do not weaken tests or mandatory security/integrity evidence, edit authoritative sources to fit code, expand scope, overwrite unrelated changes, touch credentials, or stage/commit/push/merge/deploy without separate explicit authorization. No production/live access or destructive operations. Cleanup may remove only task-created accidental artifacts, not user work.
- Every chunk: formatting/syntax/build/symbol checks, focused behavioral tests, discovery/skip review, complete relevant diff including staged and untracked content, functional/integration risk and rollback assessment. Expected results are never reported as actual PASS.
- Normal functions/capabilities run using actual `labreserve_app`; migrator is only for migrations/disposable fixtures/inspection/fault triggers. Independent PostgreSQL operations and deterministic waits are mandatory, not mocks/process mutexes/sleeps/shared enclosing transactions.
- Do not advance with required failures or evidence gaps. Final `READY_FOR_REVIEW` requires all mandatory checks executed successfully and scope complete; it is neither independent acceptance nor integration permission.

### Approved project settings and conservative defaults

| Setting | Resolved policy / provenance |
| --- | --- |
| Execution mode | **UNSELECTED**. Request explicit selection at later execution; this plan authorizes no loop. |
| Runtime environment | Existing local-only Compose isolation and pinned Go/PostgreSQL/Playwright from repository. No Python needed; if later necessary, default requires a user-provided or verified approved venv, activate/reuse it and record path before executing Python. |
| Verification | Repository focused Go/PostgreSQL tests plus full `make verify`; commands in section 8. Do not substitute runtime credentials with migrator credentials. |
| Failed validation budget | **Two failed validation attempts per issue**, conservative default; report counts and stop unresolved at the second failure. No authorized Task 4B override exists. Old 4A issue-specific extra budget is not inherited. No session restart resets a count; another finite budget needs explicit compatible authorization before further attempts. |
| Repair authority | Diagnose and smallest current-chunk in-scope repair autonomously within budget. Distinguish pre-existing/environment failures from new failures; do not hide either. Unavailable mandatory tooling/access is a blocker, not a skipped-success gate. |
| Single-chunk handoff | Stop after selected chunk. By default ask confirmation before creating an unrequested handoff; current planning request does not authorize execution handoffs. Completion report still contains resume evidence. |
| Continuous-loop handoff | Only separately authorized real coordinator may launch fresh sessions. Every non-final worker writes a complete handoff and stops; coordinator accepts evidence/exit condition before dispatching next. If orchestration unavailable, report gap and provide manual next-session prompt; never pretend fresh sessions occurred. |
| Future handoff location | Existing `docs/handoffs/` convention: proposed `task-004b-chunk-N-to-M.md`, M = next numbered chunk. Created only under later selected mode/authorization, not during planning. |
| Evidence | Per-chunk completion report and mode-authorized handoff; no new evidence document now. Redact secrets. Final report gives base, candidate HEAD/dirty delta/content identity, command/results, decisions, limitations and independent-review boundary. |

**Handoff payload H (used by every non-final chunk below):** task/spec/plan paths and fingerprints; chosen mode/policy/budget and failed attempts; original/current base/candidate and protected dirty work; completed/next chunk; inspected/changed files and symbols; actual commands, discovery, PASS/FAIL/SKIP/NOT_RUN; complete diff summary; functional/integration/security risks, gaps, rollback note; decisions/blockers; tool/runtime prerequisites; exact next-session instruction. Single-chunk: report H, ask before writing an unrequested artifact, stop. Authorized loop: write H, stop worker, coordinator validates. Non-final status `CONTINUABLE`, `BLOCKED` or `INCOMPLETE`; final status `READY_FOR_REVIEW`, `BLOCKED` or `INCOMPLETE`.

## 8. Verification command and evidence contract

### Required commands, working directory and prerequisites

All shell commands below are future execution, **not run in planning**. Working directory for every command is repository root `/<REDACTED>/labreserve`; container Go cwd is observed `/src`. Use RTK per installed shell discipline (proxy passthrough if filtering hides evidence). No new dependency installation or formatter is prescribed. Existing `.env` must be safely supplied/reviewed by operator without logging its contents. The Make entry point can create ignored `.env` if absent; record that side effect, never stage it.

For focused work, use the existing verification services in the separate `labreserve-verify` project only after checking it is not another worker's active project. A uniquely named disposable project is a Level 1 alternative if concurrent ownership conflicts; record it and use it consistently. Never point `testPool`/fault fixtures at demo/persistent data. Images copy source, not a bind mount: rebuild affected test/app images after changes so evidence tests the candidate rather than stale code.

**Setup (documented service structure; proposed focused invocation):**

```sh
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify build verify-migrate verify-seed verify-go verify-app e2e
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify up --wait -d verify-db
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm verify-migrate
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm verify-seed
```

Use **only the already-owned disposable verification project** for focused setup/cleanup. Finish/release it before full `make verify` owns that name. Do not run `make up`, `reset-demo`, down-volume commands on persistent/demo targets, or copy fixture ledger-alteration procedures to real targets. Full `make verify` performs its own isolated cleanup; any additional focused cleanup requires confirming exact test-project ownership/volume first.

The following command IDs are expanded procedures, not existing repository functions. In `D(P)` / `W(P)` / `U(P)`, substitute the chunk's literal proposed/observed regex P for `PATTERN`; verify names before trusting selection.

| ID | Required future command/procedure | Intended evidence |
| --- | --- | --- |
| U(P) | `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run 'PATTERN' ./internal/resources` | Unit contracts/order/error/time/call counts. No PostgreSQL claim from unit fakes. |
| D(P) | `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run 'PATTERN' ./internal/database` | Actual PostgreSQL fixture/runtime operations, persisted outcomes and deterministic waits. |
| W(P) | Same Compose prefix/run as D(P), ending `go test -count=1 -v -p 1 -run 'PATTERN' ./internal/web` | Existing authentication/session→internal state-service/read seams plus accepted HTTP regressions; no new management routes. |
| L(P,package) | Same prefix/run, ending `go test -list 'PATTERN' ./internal/<package>` | Inspect expected test names; fail evidence gate if none or unintended subset selected. Proposed names are not currently discovered tests. |
| C | Same prefix/run, ending `go test -run '^$' ./...`, then `go build -o /tmp/labreserve-task4b-check ./cmd/labreserve` in a fresh test container | Whole-repository type/symbol/import compile and executable build. Compile-only selects no tests and is never behavior evidence. |
| G | Use the repository's gofmt convention: inspect with `gofmt -l` inside rebuilt verify-go. If changes are needed, format the actual working-copy files using host `rtk gofmt -w <changed-go-files>` when the matching toolchain is available, or the explicitly write-mounted formatter invocation below; rebuild and confirm no output afterward. `scripts/verify.sh` supplies the whole-repo formatting gate. | No unformatted/new invalid Go. Formatting only the image copy would not repair repository files. Do not format unrelated files. |
| S | `rtk bash -n scripts/verify.sh` | Changed shell syntax only; actual lifecycle still mandatory. |
| F | `rtk make verify` | Observed repository entry point: formatting, Go/actual PostgreSQL/HTTP, Playwright, seed and real restart fixtures/replays; require final success message AND discovery/results. |
| R | `rtk git status --short --untracked-files=all`; `rtk git diff --check`; `rtk git diff --cached`; `rtk git diff`; `rtk git diff <execution-base> -- <task-files>`; read every relevant untracked file fully | Whole relevant chunk/task delta, including earlier chunk changes, staged work and new files; compare protected baseline before removing anything. |

For C's second step the full invocation is `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go build -o /tmp/labreserve-task4b-check ./cmd/labreserve`. The temporary output remains in the removed test container, not repository artifacts. Proposed container formatting fallback: `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps --volume "$PWD:/src" verify-go gofmt -w <changed-go-files>` with explicitly enumerated relative paths. This one formatter invocation intentionally writes the working copy; ordinary build/tests still use rebuilt image copies without a source mount. Review ownership and diff afterward.

Every changed-code chunk runs G, C, its focused test selectors plus L, and R, followed by explicit risk review; SQL changes are parsed/applied by real migrations plus D, not a fictional SQL linter. Shell changes also run S. All chunk-specific tests below are **proposed new names** unless identified as existing. Prefixes may be refined, but keep discovery aligned with the final selector and full gate. `-count=1` prevents cached PASS; `-p 1` preserves fixture isolation across packages. Do not parallelize shared ledger/fault/lifecycle fixtures without isolation proof.

Expected discovery rules: normal integration suites must not skip for missing database URLs. Fixture/replay tests intentionally skip in the general run **only** if separately selected with their environment flags around actual restarts; record those later executions. Inspect both `current_user` runtime assertions and all skips/results; exit zero is insufficient if required tests never ran. Include lock holder/waiter identity and SQL wait evidence, state/Booking/event snapshots and fault cleanup in reports, not merely response errors.

**Recommended, not acceptance substitutes:** additional repeated stress schedules, Go race detector, SQL query-plan inspection, broader fault experiments. No new throughput target, external infrastructure or fault network is required. Required behavior below may not be made optional because tooling is unavailable.

## 9. Implementation Ladder: Task 4B

Common per-chunk rules in sections 7–8 are part of **each** numbered chunk's contract. H is mode-dependent, never unconditional single-chunk file creation. A fresh session must read those sections as well as its own chunk. Proposed paths/symbols below do not yet exist. Multiple-file slices are limited to files necessary for one safe behavior with its tests; no independent feature is hidden in a large horizontal chunk.

### Chunk 0 — Discovery and Baseline

- **Goal / requirements:** confirm actual execution base, authorization, sources, dependencies, ladder and verification baseline; map all V1–V9 and section 4 PRD facets, implement none.
- **Dependencies / entry:** separate human implementation authorization; committed controlling task; current formal preflight evidence tied to actual contract/base (or separately authorized preflight before implementation). Do not treat this plan as that evidence. Selected execution mode and safe local tooling/target ownership must be explicit.
- **Existing files to inspect:** all section 1 sources; accepted service/adapter/migration/role/test/lifecycle paths in section 3; this complete plan and current handoff if any.
- **Proposed changes / symbols:** no application/schema/test/runtime changes. Only later mode-authorized evidence/handoff; no competing plan.
- **Ordered tasks:**
  - [ ] Re-read whole contract/endings and gate identities; inspect branch/HEAD/tree/index/unstaged/untracked, Task 4A ancestry and intervening diffs; hash task/plan/migrations and protect pre-existing work.
  - [ ] Confirm proposed API/file/migration-prefix/test selectors remain available; resolve Level 1 choices and test image rebuild commands; verify installed executor/policy compatibility and finite failure budget.
  - [ ] Confirm local Docker/Compose/Make, disposable project ownership, role URLs and guarded `labreserve_test`; do not inspect/log secrets. No persistent-target changes. If such a target is separately authorized later, perform the section 11 ledger prerequisite non-destructively first.
  - [ ] Establish focused existing Resource/Booking and full repository baseline; distinguish any environment/pre-existing failures before code.
- **Compile-safe state:** no production changes; actual baseline, not historical claims.
- **Focused verification / discovery:** section 8 setup, L/D with `^(TestResource|TestConcurrentResource|TestConcurrentCanonicalResource|TestBooking|TestConcurrent.*Booking|TestOwnerCancellation|TestConcurrentOwner)`; unit Resource/Booking packages in existing verify-go; F. Existing restart fixture skips must be satisfied by F's explicit phases. Record executed suite names/results, current skips and errors.
- **Formatting/build/test and diff:** no formatting edits; F builds/formats/tests; R and side-effect audit after commands. No tests were run merely by planning this chunk.
- **Risk / repair:** baseline target mix-up or stale source. Repair only invocation/isolation/ordinary evidence problems within two failed attempts/issue; no unrelated accepted-code repair disguised as baseline. Required unavailable tooling or source/approval mismatch blocks dependent work.
- **Rollback:** no source rollback needed; clean only owned disposable artifacts, not ignored user `.env` or persistent volumes.
- **Handoff:** H with actual implementation base, source fingerprints, baseline and selected policy; apply selected-mode rule.
- **Exact stop:** baseline and all source/authorization/environment/ladder checks evidenced with no unresolved mandatory gap, or report BLOCKED/INCOMPLETE. Stop session; do not implement Chunk 1.

### Chunk 1 — Minimal state contracts with fail-closed entry point

- **Goal / requirements:** narrow explicit desired-state/types/outcomes before callers, V1/V2/V6 contract path; FR-002/023/026–027.
- **Dependencies / entry:** accepted Chunk 0; proposed separate StateService decision recorded; no state callers or HTTP adapters.
- **Inspect existing files:** `internal/resources/service.go`, `service_creation_test.go`, `service_edit_test.go`; `internal/bookings/cancellation.go`; Task 4B domain/failure sections.
- **Proposed files to change:** new `internal/resources/state.go`, `internal/resources/state_test.go` only. Existing `resources.Repository` and callers remain unchanged.
- **Symbols / contracts:** proposed StateService/NewStateService/SetState, StateRequest/StateResult, StateRepository/StateTransaction, ErrResourceOccupied from section 5; reuse Actor/Resource/ResourceTransaction.
- **Ordered tasks:**
  - [ ] Define only state-needed types and transaction seams; document explicit `false` and changed/no-op semantics, trusted Actor separate from command.
  - [ ] Add unreachable-from-web fail-closed SetState stub returning safe ErrOperational, never changed/no-op success or mutation. Do not invent temporary success or a toggle.
  - [ ] Add `TestResourceStateContractExplicitDesiredValue` and `TestResourceStateUnimplementedFailsClosed` covering true/false representation, no mutation/commit/retry, safe errors and compatibility with existing services.
- **Compile-safe state:** new internal stub compiles with no consumers; state commands cannot falsely succeed. Temporary unavailability is explicit and must be removed by Chunk 4, not accepted as delivered behavior.
- **Focused verification / discovery:** U/L `^TestResourceState(Contract|Unimplemented)` plus existing Resource package tests. Expected two named tests and their subcases; no database behavior claimed.
- **Formatting/build/test and diff:** G, C, R; verify no imports/callers to nonexistent database state API and no old interface widening.
- **Risk / repair:** accidental breaking of old fake repositories or unsafe zero-success. Small type/test repairs only within budget; widen neither task nor old contracts for convenience.
- **Rollback:** remove only new unconsumed files/delta if needed; preserve user work.
- **Handoff:** H, including exact temporary stub and next capability work.
- **Exact stop:** contracts compile, fail-closed tests and accepted Resource tests pass, scoped diff/risk recorded; stop before migration implementation.

### Chunk 2 — Guarded state capability and additive schema integration

- **Goal / requirements:** safe database write boundary with inseparable Activity and occupancy/no-op guards, V1–V3/V6/V8; FR-023–027, NFR-007–008/010; AC-009/012/028–030/044 internal facets.
- **Dependencies / entry:** 1 accepted; latest prefix confirmed; immutable hashes match; disposable database only, or separately approved compatible target checks already met. No generic state setter ever exposed.
- **Inspect existing files:** migrations 001–005; `deploy/postgres/01-roles.sql`; `migrate.go`; `resource_mutations_integration_test.go`; `foundation_integration_test.go`; `scripts/verify.sh`.
- **Proposed changes:** new `internal/database/migrations/006_resource_state.sql`, new `internal/database/resource_state_capability_integration_test.go`, new `internal/database/resource_state_migration_integration_test.go`; existing `internal/database/migrate.go`, `resource_mutations_integration_test.go`, `scripts/verify.sh`. This cohesive six-file boundary is necessary to ship a safe capability with tests and keep schema/version/lifecycle assertions valid immediately; it is not permission for unrelated SQL changes.
- **Symbols / contracts:** proposed guarded function in section 5, next requiredSchemaVersion, existing checksummed runner; old upgrade test must explicitly select 005 rather than assume latest=005.
- **Ordered tasks:**
  - [ ] Add migration ownership guard consistent with 005; guarded Coordinator/Resource lock/read/no-op; separate post-lock occupancy query for deactivation; only active/updated_at update; internally derived distinct state event; safe ACL/path/qualification and null-input rejection.
  - [ ] Add direct-runtime capability tests alongside SQL: both transitions and matching states, unauthorized/null/unknown actor/target, future/ongoing vs Cancelled/completed occupancy and exact end, event details/times/retained fields, no-UPDATE and no-event-attempt detectors, Activity failure and outer rollback both directions. Extend the existing scoped Activity-failure helper's action allowlist for the two state actions here if reused, rather than depending on a later chunk.
  - [ ] Add fresh-install and **actual compatible merged-005 schema/ledger → new migration** tests with Active/Inactive/raw text, Confirmed/Cancelled/request/cancellation/Activity/accounts/sessions snapshots, index/FK/old-capability checks and repeat migration. Use isolated transaction/schema simulation consistent with existing fixtures, removing only new capability inside a rolled-back disposable fixture; do not call that a persistent recovery procedure.
  - [ ] Anchor retained `TestResourceCapabilityMigrationPreservesVersion004Data` explicitly to 005 and its four predecessors; exclude new capability in its disposable 004 simulation as necessary, then prove 004→005 separately without loosening earlier assertions. Add exact missing/unknown/checksum rejection for new schema and a targeted older-005 mismatch refusal with unchanged retained-data evidence.
  - [ ] Update required version and both five-migration script assertions consistently with actual count. Do not rewrite any 001–005 bytes/grants. No startup auto-migration or seed changes.
- **Compile-safe state:** new guarded capability is fully safe in either direction before any Go caller; state service remains fail-closed. Fresh full verification stays runnable with new ledger expectations. No partial/eventless mutation path exists between chunks.
- **Focused verification / discovery:** D/L `^(TestResourceStateCapability|TestResourceStateMigration|TestResourceCapabilityMigrationPreservesVersion004Data|TestCheckSchemaRequiresExactMigrationLedger)`; include old capability tests `^TestResourceCapabilities`; run S, G, C, R and F after schema/script changes. Evidence includes all expected named cases actually executed under runtime role, exact source hashes and retained-data comparisons.
- **Risk / repair:** SQL snapshot/qualification/guard or old-upgrade coupling. Repair this migration only while demonstrably unaccepted/disposable-only; once applied to persistent/accepted history, do not edit it. No resets/ledger substitution. Security/trust exception requires Level 2.
- **Rollback:** outer transaction proves pair rollback; schema recovery is roll-forward, not down/reset/old binary. Isolated simulated history is rolled back, never copied to a persistent ledger.
- **Handoff:** H plus capability signature/SQLSTATE mapping, migration/hash/permission findings and compatibility condition.
- **Exact stop:** new capability safe, direct behavior/upgrade/old-upgrade/exact-ledger/full current gate pass, old hashes unchanged; stop before Go state adapter/callers.

### Chunk 3 — Coordinator activation vertical slice

- **Goal / requirements:** inactive→active and authorized active→active no-op via trusted service/runtime database, V1/V2/V6/V7; FR-002/009/023/026–027, AC-016/021/028/041/044 state facets.
- **Dependencies / entry:** 2 accepted, capability available; no HTTP composition changes.
- **Inspect existing files:** `resources/service.go`, proposed `state.go/state_test.go`; `database/resource_creation.go`, `bookings.go`; capability SQL/tests and Activity failure helpers.
- **Proposed changes:** `internal/resources/state.go`, `state_test.go`; new `internal/database/resource_state.go`, new `internal/database/resource_state_integration_test.go`; narrowly extend existing `installResourceActivityFailureTrigger` action allowlist in `resource_mutations_integration_test.go` if reused. Source+unit+adapter+integration is one vertical slice; helper edit only adds scoped state actions.
- **Symbols / contracts:** BeginResourceStateChange/StateTransaction adapter; SetState activation path; existing ResolveCoordinator/LockResource/Commit/Rollback reused without old Repository widening.
- **Ordered tasks:**
  - [ ] Replace activation stub with authorization-before-validation, canonical UUID handling, Resource lock+separate read, unknown outcome, matching-state no-op with commit, post-lock normalized clock for actual activation, guarded call and one final commit.
  - [ ] Implement narrow adapter and exact error mapping; deactivation entry and HasBlockingBookings remain explicit safe operational stubs until 4, never return guessed occupancy false/success. No new runtime app caller.
  - [ ] Add activation unit trace tests for begin/role/lock/mutation/commit errors, missing/malformed/unknown actors and target, both Engineers, auth precedence/no-op failure; safe rollback cleanup and no automatic retry. Remove activation-specific obsolete stub assertion, not its safety requirement.
  - [ ] Add `TestResourceStateActivation...` PostgreSQL cases: changed event/fields, Active no-op with UPDATE/event failure detectors, Engineer matching-state rejection, activation retaining Bookings/raw legacy whitespace/HTML text; valid Engineer booking after activation, normal invalid/conflict checks preserved. Hold Resource and advance clock, proving no pre-release time read and matching event/updated_at.
  - [ ] Inject activation event failure and outer rollback, inspect complete state/time/history, remove scoped failure and explicitly retry one successful event.
- **Compile-safe state:** activation fully works; desired Inactive still unavailable safely with ErrOperational. Existing creation/editing and web unchanged; no success placeholder. H must identify remaining Inactive stub.
- **Focused verification / discovery:** U/L `^TestResourceState`; D/L `^TestResourceState(Activation|Capability)` plus existing `^TestResourceCreation`; G, C, R. Expect activation suite, both role/no-op/time/fault cases, actual runtime PostgreSQL and one matching event per change.
- **Risk / repair:** unsafe stub/no-op commit, role/clock placement, accidental text rewrite. Repair slice/helper scope only; no state toggling or Booking changes.
- **Rollback:** bounded source/test delta; accepted additive migration remains, inert to old app. Never run an old binary against new exact ledger as schema rollback.
- **Handoff:** H plus activation API/error/evidence and explicit remaining Inactive stub.
- **Exact stop:** complete activation/no-op/failure/eligibility evidence passes, all builds/old Resource tests pass, Inactive path fail-closed; stop before safe deactivation implementation.

### Chunk 4 — Safe deactivation, occupancy boundaries and complete no-ops

- **Goal / requirements:** complete state service, V1–V3/V6/V7; FR-024–027/009/023, NFR-001, AC-009/012/021/028–029/041/044.
- **Dependencies / entry:** 3 accepted; final capability already guards both directions; controlled non-incrementing microsecond clock available for boundary tests.
- **Inspect existing files:** state service/adapter/tests from 3; Task 4B authoritative order/table; `booking_cancellation_integration_test.go`; `resource_edit_integration_test.go` snapshot helpers; `resource_concurrency_integration_test.go` clocks.
- **Proposed changes:** `internal/resources/state.go`, `state_test.go`; `internal/database/resource_state.go`, `resource_state_integration_test.go`. Keep capability SQL unchanged unless a substantiated current migration defect can be repaired within applied-history constraints.
- **Symbols / contracts:** HasBlockingBookings, ErrResourceOccupied, desired-state comparison and complete SetState.
- **Ordered tasks:**
  - [ ] Remove Inactive/occupancy stubs. After authorization and locked state, return equality no-op **before** occupancy; only actual Active→Inactive captures fresh time and separately queries `resource_id = target AND state='confirmed' AND end_at > operation_time`.
  - [ ] Repeat final guards in capability; distinguish occupancy from unknown/invalid/operational. Never use selected-day/owner/start-time filtering or stale pre-lock data; no Booking locks/mutations, Account coordination or automatic retry/cancellation.
  - [ ] Extend unit trace tests: auth before equality/validation; no occupancy/mutation for either no-op; no-op commit failure operational; query/lock/context failures safe; one timestamp after lock shared with mutation; exact error precedence and no SQL diagnostic leakage.
  - [ ] Add `TestResourceStateDeactivation...`, `TestResourceStateNoOp...`, `TestResourceStateOccupancy...`: empty, each Engineer's future/ongoing, exact start, all-owner/multiple-date, mixed nonblocking plus one blocker, other Resource, only Cancelled/past; end−1µs/exact/end+1µs; retain all metadata/history. An isolated inactive+future-occupancy no-op must succeed without query/update/event; Engineer must not.
  - [ ] Force lock waits with still-before-end and across-end clock changes; assert zero eligibility clock reads before release and post-lock event/update time. Test inactive new creation outcome before overlap with no key/event, and deactivation Activity failure/outer rollback + explicit retry.
  - [ ] Test repeated both desired states with UPDATE detectors and event failure fixtures, and opposite real transitions at equal operation microsecond; no timestamp-based false no-op. Remove all temporary state stubs.
- **Compile-safe state:** entire explicit desired-state service works safely; no TODO/unimplemented path remains. No UI caller or Booking behavior changed.
- **Focused verification / discovery:** U/L `^TestResourceState`; D/L `^TestResourceState(Deactivation|NoOp|Occupancy|Activation|Capability)`; existing inactive/replay and owner boundary tests; G, C, R. Evidence must include precise predicate/end decisions and persisted unchanged rejection/no-op snapshots, not just errors.
- **Risk / repair:** stale snapshot, > vs >=, inactive no-op occupancy order, arbitrary error mapping. Small in-scope SQL/Go/test repair only; no lock/isolation redesign or scope expansion.
- **Rollback:** reverse only current source delta if necessary; prior migration/history unchanged. Mutation failures roll back pair; no destructive data repair of legacy fixture/state.
- **Handoff:** H with completed state precedence, time, occupancy/fault evidence and no remaining stubs.
- **Exact stop:** all activation/deactivation/no-op/boundary/error/fault tests and build pass with no known safety gap; stop before wider privilege evidence.

### Chunk 5 — Effective runtime privileges and trusted-session boundary

- **Goal / requirements:** final-write defense and actual authority evidence, V1/V8 plus V7 read preservation; FR-001–002/023/026–027, NFR-002, AC-015–017/028/031 applicable facets.
- **Dependencies / entry:** 4 accepted; actual runtime and migration test logins; earlier direct capability functional tests already pass.
- **Inspect existing files:** `resource_mutations_integration_test.go` permission/shadow tests; roles/migrations; `auth/service.go`, `database/store.go`, `web/middleware.go`, `web/foundation_integration_test.go` session/resource seam.
- **Proposed changes:** new `internal/database/resource_state_privilege_integration_test.go`; new `internal/web/resource_state_session_integration_test.go`. Guarded capability defects must be substantiated and repaired within task/applied-history constraints, not by loosening tests.
- **Symbols / contracts:** catalog ACL/ownership/search path; proposed state capability/service, existing auth Resolve/Login/Logout and read routes; no new production symbols.
- **Ordered tasks:**
  - [ ] Assert current_user=labreserve_app; inspect effective table/every-column/function/schema/role/PUBLIC/inherited/grant-option permissions. Attempt Resource INSERT/every-column UPDATE/DELETE/TRUNCATE, function replace/alter/owner change/SET ROLE, schema DDL and unrelated writes in isolated transactions; require denial. Preserve approved cancellation columns and Activity SELECT/INSERT exactly.
  - [ ] Prove non-superuser migration-controlled definer owner/fixed path/revoked PUBLIC/runtime-only EXECUTE. Add denied-ungranted-invocation evidence by temporarily revoking runtime EXECUTE in a serialized, owned disposable fixture, asserting denial through a fresh runtime operation, and reliably restoring the exact ACL; alternatively use an already available ungranted test role. The migrator cannot CREATE ROLE under the accepted bootstrap, so do not assume it can provision another role or add production role infrastructure. Shadow accounts/resources/**bookings**/activity and caller path cannot bypass Coordinator/occupancy or redirect writes.
  - [ ] Call capability directly with Engineer/unknown/null actor, both no-ops, blocker, and valid transitions; verify internally derived event/locked state, no update/event attempts on no-op, no writable code/text/Booking/history or skip-event/occupancy input. Inspect both guards and absence of generic raw setter overload.
  - [ ] Through existing login/Resolve seam invoke internal state service with session-derived ID; verify Coordinator vs both Engineers, tampered role ignored, logout/unknown actor rejected, current stored-role changes reflected safely. Test-only role fixtures restored; never claim a cached Actor authenticates a logged-out session by itself.
  - [ ] Existing list/schedule/detail/My Bookings reads continue to retain safe HTML-like/legacy text and correct state; add no management HTTP route, status mapping or form.
- **Compile-safe state:** production behavior unchanged; stronger actual-runtime evidence, no new trust assertion or endpoint.
- **Focused verification / discovery:** D/L `^TestResourceStatePrivilege` and existing `^TestResourceCapabilitiesExposeOnlyGuardedRuntimeWrites`; W/L `^(TestResourceStateSession|TestFoundationHTTPAuthenticationAndResourceBrowsing|TestResourceTemplateEscapesHTMLLikeContent)`; G, C, R. Expect no DB/password-env skips and actual denied-operation SQLSTATE/catalog evidence.
- **Risk / repair:** migrator masquerading as runtime, effective inherited grants, shadow occupancy dependency or overclaim of SQL identity. Repair minimal capability/test seam, never permission broadening/003 hardening. Material privilege exception is Level 2.
- **Rollback:** dispose scoped test objects/restore fixture roles; no production grants/credentials changed. Preserve capability migration history.
- **Handoff:** H with effective ACLs/abuse attempts, trusted actor/time limitation and test cleanup evidence.
- **Exact stop:** all actual privilege/session/read seam tests pass with no bypass or broadened grants; stop before concurrency schedules.

### Chunk 6 — Booking creation versus deactivation in both lock orders

- **Goal / requirements:** V4 and creation portions of V5/V7; FR-009/015/024–027, AC-005–006/010/021–023/033 preservation.
- **Dependencies / entry:** 5 accepted; runtime service works; deterministic bounded transaction wrappers/barriers allowed only in tests.
- **Inspect existing files:** `booking_creation_integration_test.go` lock/PID/replay helpers; `resource_concurrency_integration_test.go`; state service/adapter; Account/Resource lock migration.
- **Proposed changes:** new `internal/database/resource_state_booking_concurrency_integration_test.go`; optional new `internal/database/resource_state_test_helpers_test.go` for narrow shared holder/waiter and snapshot wrappers used here/next chunks. No production locking change.
- **Symbols / contracts:** existing booking Service.Create/lock helpers/atomicBookingClock; proposed test wrapper pauses actual leading operation before Commit and identifies backend PID.
- **Ordered tasks:**
  - [ ] Force a valid creator to hold Resource after Booking/event insert before commit; start deactivation, observe its PostgreSQL wait on that holder, release creator commit. Require fresh separate occupancy to block; exactly one Booking event and no deactivation event.
  - [ ] Force deactivation holder through its actual mutation before commit; start fresh creator, observe Resource wait, release commit. Require ErrInactiveResource, no Booking/event/successful request binding and exactly one deactivation event.
  - [ ] Repeat same Coordinator Account creator while state holder writes history: observe Account NO KEY UPDATE → Resource wait, allow state event FK/role reads to finish without Account inversion. Explicitly inspect lock modes/order, not only timeout absence.
  - [ ] Hold Resource and advance time across new-booking minimum-lead boundary; preserve Task 2B fresh-time rejection. While Resource is held on inactive state, prove retained identical replay and changed-data reuse complete without waiting on it or reading new eligibility.
  - [ ] Fail leading Booking creation then let waiting deactivation see no occupancy/key/event; fail leading deactivation then waiting valid creation sees committed Active and may succeed. Force named leader, do not release unordered waiters and infer both orders.
- **Compile-safe state:** tests only, accepted services unchanged; bounded independent runtime transactions, no shared enclosing transaction for contested operations.
- **Focused verification / discovery:** D/L `^TestResourceStateBookingRace`; existing `^(TestBookingCreationValidatesTimeAfterResourceLockWait|TestBookingCreationChangedDataWinsBeforeResourceEligibility|TestBookingLockFunctionsGrantOnlyScopedRuntimeCapability)`; G, C, R. Expected subcases identify both holder orders, same-account lock modes, lead/replay and rollback variants, exact final counts.
- **Risk / repair:** nondeterministic order, invalid horizon fixtures, cycle hidden by unrelated account or migrator execution. Adjust fixtures/wrappers/diagnostics within budget; no production lock replacements.
- **Rollback:** scoped test failures/barriers cleaned with finite contexts; no production changes to revert; leave no locked pool connections.
- **Handoff:** H with each forced schedule and actual wait evidence, not aggregate “concurrency passed.”
- **Exact stop:** named race/fresh-time/replay/rollback schedules pass independently on PostgreSQL with correct final state/event/request evidence; stop before cancellation races.

### Chunk 7 — Owner cancellation versus deactivation

- **Goal / requirements:** V5 cancellation schedules/V7 preservation; FR-019–025/027, AC-008–010/025/036–039.
- **Dependencies / entry:** 6 accepted; helpers available; owner cancellation remains unchanged and no cross-owner authority is added.
- **Inspect existing files:** `bookings/cancellation.go`, `database/booking_cancellation.go`, `booking_cancellation_integration_test.go`; new state/helpers/race tests; Task 4B concurrency table.
- **Proposed changes:** new `internal/database/resource_state_cancellation_concurrency_integration_test.go`; helper file from 6 only if a specific bounded barrier extension is needed.
- **Symbols / contracts:** actual `CancellationService.Cancel`, Resource → Booking; proposed test pause at post-occupancy decision/rejection and before commit.
- **Ordered tasks:**
  - [ ] Hold successful owner cancellation before commit, observe waiting deactivation, commit then assert separate query sees Cancelled and allows if no other blocker. Add second future-ending Confirmed row that still blocks despite the first cancellation.
  - [ ] Force deactivation's fresh occupancy check while future Confirmed row still exists; hold its transaction/test decision long enough to observe cancellation waiting, then return blocked/release. Cancellation independently succeeds if still future. Require explicit new deactivation submission for eventual transition, not automatic retry.
  - [ ] Advance cancellation clock across exact start while waiting for Resource; require existing started rejection and Confirmed occupancy retained. Check Coordinator cannot cancel another owner to clear the guard, including Cancelled repeat.
  - [ ] Fail leading cancellation Activity/transaction while deactivation waits; after rollback the still-future-ending Confirmed Booking blocks and no cancellation/state event persists. Snapshot request/creation/cancellation metadata and prior history.
- **Compile-safe state:** test-only, same normal runtime services; no new cancellation semantic or Booking state.
- **Focused verification / discovery:** D/L `^TestResourceStateCancellationRace`; existing `^(TestOwnerCancellationUsesFreshTimeAfterResourceLockWait|TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp|TestOwnerCancellationEventFailureRollsBackAndKeepsIntervalOccupied)`; verify all listed existing tests are discovered. G, C, R. Expected both forced orderings, remaining blocker, exact-start waiter, rollback and explicit resubmission evidence.
- **Risk / repair:** rejected deactivation releasing before order observed, cancellation fixture already started, accidental coordinator bypass. Fix test synchronization/time only; no change to owner-only/time rule.
- **Rollback:** remove scoped triggers/barriers, release independent transactions; no schema/data recovery needed outside disposable fixtures.
- **Handoff:** H with cancellation lock order/fresh time, both outcomes and retained snapshots.
- **Exact stop:** all ordered cancellation/deactivation/failure cases and accepted cancellation focused tests pass; stop before state/edit stress cases.

### Chunk 8 — Concurrent desired states, descriptive edits and rollback waiters

- **Goal / requirements:** remaining V5, V2/V6/V7; FR-023–027, AC-028/043–044 internal facets; accepted 4A text/no-op preservation.
- **Dependencies / entry:** 7 accepted; deterministic independent state service scheduling/cleanup established.
- **Inspect existing files:** state tests/helpers; `resource_concurrency_integration_test.go`, `resource_edit_integration_test.go`, mutation failure/no-op triggers and Resource/Booking snapshot helpers.
- **Proposed changes:** new `internal/database/resource_state_concurrency_integration_test.go`; bounded extensions to `resource_state_test_helpers_test.go`. Only if needed extend existing test-only scoped update-failure helper; no production mutation feature.
- **Symbols / contracts:** StateService.SetState; existing Service.Edit; no-op changed flag; test transaction gates and actual persisted event sequences.
- **Ordered tasks:**
  - [ ] From Inactive, two desired Active operations; from eligible Active, two desired Inactive operations: force holder/waiter, one changed UPDATE/event, waiter unchanged with original updated_at and no event attempt. Use detector that permits leader but would reject any waiter's UPDATE.
  - [ ] For both initial states and both forced orders of opposite desired states, assert real serial changed/no-op results, final state and before/after events. Repeat with equal clock values to disprove timestamp-based no-op. Use per-operation snapshots, not time/UUID sorting as causal proof.
  - [ ] Force descriptive edit before/after state operation, including raw legacy whitespace and future/Cancelled retained Bookings appropriate to state rule. Preserve each operation's fields and accurate old/new details; immutable code/created_at unaffected.
  - [ ] Force leading Resource UPDATE failure and leading state Activity failure separately with waiting state command; after rollback waiter rereads committed state and fresh time and legitimately transitions/no-ops/rejects. Pair state/cancellation/creation rollback variants from 6–7 with explicit snapshot/count cross-checks, not duplicated production logic.
  - [ ] Verify time advance while waiter blocked matters; no uncommitted leader value/history treated as durable; no new request identity/last-request-wins promise.
- **Compile-safe state:** tests only; no unsafe partially implemented behavior.
- **Focused verification / discovery:** D/L `^TestResourceStateConcurrent`; existing `^(TestConcurrentResourceEdits|TestConcurrentCanonicalResourceEdits|TestConcurrentCanonicalResourceEditRetainedWhitespaceNoOpAfterLockWait)`; G, C, R. Expected each direction/initial/order/mutation-vs-event-failure subcase executed with real holder/waiter and exact update/event counts.
- **Risk / repair:** permissive detector accidentally allows waiter UPDATE, raw text canonicalization via shared snapshots, wrong causal ordering. Tighten tests/helpers and smallest substantiated state defect only; never change 4A equality or product ordering.
- **Rollback:** test-only delta and scoped fixtures; rolling back leader must preserve both-or-neither. Do not delete accepted history to normalize tests.
- **Handoff:** H with schedule matrix, no-UPDATE detector scope, all rollback waiter results and remaining commit-failure gap.
- **Exact stop:** repeated/opposite/edit/mutation-failure/Activity-failure schedules pass with correct persisted snapshots and cleanup; stop before COMMIT uncertainty evidence.

### Chunk 9 — Actual COMMIT rejection and ambiguous acknowledgement

- **Goal / requirements:** V6 in full, failure re-evaluation V5/V7; FR-026–027, AC-012/044 state atomicity.
- **Dependencies / entry:** 8 accepted; per-direction Activity insertion failure already evidenced; do not confuse outer rollback or synthetic error with definite COMMIT rejection.
- **Inspect existing files:** `resource_commit_rejection_integration_test.go`, resources safe operational handling, state adapter/tests/helpers; Task 4B failure/uncertainty sections.
- **Proposed changes:** new `internal/database/resource_state_commit_integration_test.go`; `internal/resources/state_test.go`; minimally extend existing `installResourceCommitRejectionTrigger` switch to supported state actions and Resource-ID scope if reused. Keep old create/edit COMMIT tests intact.
- **Symbols / contracts:** existing observation/trigger pattern; proposed state transaction wrapper records successful statement and actual Commit call/error; no public commit-status protocol.
- **Ordered tasks:**
  - [ ] In each direction install actor/action/Resource-scoped DEFERRABLE INITIALLY DEFERRED rejection, prove guarded statement returned changed success, then actual PostgreSQL COMMIT rejects with expected fixture code/message. Service returns ErrOperational without diagnostic leak; fresh connections see original state/time/history/Bookings and no transition event.
  - [ ] Add waiting state/booking operation around a rejected commit, proving lock release and committed-state/fresh-time re-evaluation; cannot use ambiguous acknowledgement to claim rollback.
  - [ ] Test-only wrapper delegates real successful commit then returns simulated lost acknowledgement; second variant rolls back before returning error. Both produce operational unknown result, exactly one service attempt and one call to its Commit seam with no blind automatic retry; the first seam commits durably before returning the acknowledgement error, while the noncommitted seam rolls back instead of executing a durable commit. Fresh reads show both Resource/event or neither. Later explicit desired-state request no-ops if committed, otherwise performs freshly eligible transition.
  - [ ] Complete begin/role/lock/query/mutation/context cancellation safe-error and finite rollback/lock-release evidence; no SQL/trigger/token/credential leakage, no domain conflict fabricated from generic database error. A no-op finalization failure is operational too.
- **Compile-safe state:** test seams only unless minimal in-scope error/cleanup defect is substantiated; no new network fault service or retry protocol.
- **Focused verification / discovery:** D/L `^TestResourceStateCommit`; U/L `^TestResourceState`; existing `^TestResource(Creation|Edit)DefiniteCommitRejection`; G, C, R. Expected both directions, real rejected COMMIT observations and separately labelled committed/noncommitted acknowledgement cases; capture persisted pairs and retry counts.
- **Risk / repair:** swallowing Activity/commit errors, false rollback assertion, trigger scope affecting other cases. Repair transaction safety/tests only within budget; no loosening atomicity or adding infrastructure.
- **Rollback:** remove scoped triggers reliably; tests must leave no deferred rejection/locks. No schema or ledger rollback; unknown outcomes inspected, not blindly replayed.
- **Handoff:** H separating definite rollback evidence from unknown acknowledgement semantics and any trusted-read limitations.
- **Exact stop:** actual COMMIT rejection, ambiguous paths, release/re-evaluation and safe operational tests pass independently; stop before lifecycle integration.

### Chunk 10 — Retention, replay, migration and actual restart integration

- **Goal / requirements:** complete V7–V9 and lifecycle portions V2/V6; FR-005/015/021/023/026–027; NFR-007–010; AC-013/028/030/033/040–044 applicable facets.
- **Dependencies / entry:** 9 accepted; guarded additive upgrade evidence from 2 preserved; same immutable 001–005 hashes; safe disposable project ownership.
- **Inspect existing files:** `resource_lifecycle_integration_test.go`, owner cancellation fixture/replay in `booking_cancellation_integration_test.go`, `scripts/verify.sh`, `seed.go`, schema tests, accepted read paths, README.
- **Proposed changes:** new `internal/database/resource_state_retention_integration_test.go`, new `internal/database/resource_state_lifecycle_integration_test.go`, `scripts/verify.sh`, `README.md`; optional bounded extension to `internal/web/resource_state_session_integration_test.go` for existing read views. Cohesive integration boundary: lifecycle phase tests must be wired with script in the same chunk, not left permanently skipped.
- **Symbols / contracts:** proposed `TestResourceStateRestartFixture`, `TestResourceStateRestartReplay`, flags `LABRESERVE_TEST_RESOURCE_STATE_RESTART_FIXTURE=1` / `LABRESERVE_TEST_RESOURCE_STATE_RESTART_REPLAY=1`; existing seed and 4A/3B lifecycle unchanged in meaning.
- **Ordered tasks:**
  - [ ] Add complete Resource non-state, raw UTF-8 legacy whitespace/HTML text, Bookings/request/cancellation and prior-history snapshots around activation/allowed and blocked deactivation/no-op/rollback. Existing read views show correct state/context without rewriting text/history.
  - [ ] Create/cancel using accepted services, deactivate, then replay original request to same Cancelled Booking on Inactive Resource with no occupancy/event recreation; include past Confirmed replay, changed-data reuse before inactive/conflict, per-owner same key independence; reactivate and preserve normal conflict/adjacency/different Resource/time/purpose/cancellation/no-op/rebooking behavior.
  - [ ] Prepare separate state restart fixtures with one committed activation and one committed deactivation, related past/Cancelled Booking histories, known timestamps/details and deactivated **seeded** Resource. Prefer K8S-01/DEMO-01 after verifying other fixtures do not depend on it; preserve 4A's NET-01 descriptive-edit history assertions rather than faking their expected state/counts.
  - [ ] Wire state fixture before and replay after existing actual PostgreSQL/application restarts in scripts/verify.sh; ordering preserves original 4A and owner-cancellation fixture/replay evidence and seed/session counts. Repeat seed before/after restart; no restoration to Active, duplicate seeds/events or erased requests. New fixture flags must each execute in explicit phases, not only skip in general tests.
  - [ ] Fresh runtime connections after restart assert exact state/update/event pairs, retained creation/edit/cancellation/Booking history and effective capability; matching states pass no-UPDATE/event detectors; valid opposite change still works. Snapshot retained history before any post-restart real opposite transition.
  - [ ] Re-run fresh/additive 005→new and explicit repeat migration/exact-ledger/004→005 tests; no persistent target action. Document upgrade ordering and conditional older-005 concern, no automatic down migration, old binary rejection, roll-forward recovery and remaining 4C/intervention/history deferrals.
- **Compile-safe state:** new lifecycle tests are integrated into existing entry point immediately; no runtime config/production endpoint changed; accepted old fixtures remain meaningful.
- **Focused verification / discovery:** D/L `^TestResourceStateRetention`; W `^TestResourceStateSession` if changed; D/L `^(TestResourceStateMigration|TestResourceCapabilityMigrationPreservesVersion004Data|TestCheckSchemaRequiresExactMigrationLedger)`; G, C, S, R, then F. Require F logs for **both** new state fixture phases and existing 4A/3B phases across actual db/app restart; flag-only tests skipped in general run are not the evidence.
- **Risk / repair:** shared seeded fixture/count interference, apparent restart from pool reconnect, lifecycle flag omission, old migration simulation contamination. Repair task tests/script/docs only; no weakening old history assertions or destructive upgrade assumptions.
- **Rollback:** revert only scoped source/script/docs delta; persistent schema remains forward-only. Disposable fixture cleanup belongs to owned verification project; never reset demo volume/ledger as upgrade recovery.
- **Handoff:** H with actual lifecycle sequence/results, retained snapshots, checksum/upgrade evidence and complete draft README notes.
- **Exact stop:** all retention/upgrade/real restart/seed/replay and existing lifecycle gates pass with no required skip; stop before final delivery audit.

### Chunk 11 — Delivery-Wide Verification, Documentation, and Candidate Review

- **Goal / requirements:** all V1–V9 and every current/preservation row in section 4; no new feature work.
- **Dependencies / entry:** chunks 0–10 accepted with evidence/required handoffs; no fail-closed temporary stub, unresolved mandatory check or current-chunk failure remains.
- **Inspect existing/proposed files:** entire task delta from actual implementation base including new SQL/service/adapter/tests, script/README/plan and authorized evidence; original governing docs, migrations 001–005 and pre-existing protected work.
- **Proposed changes / symbols:** only accurate README/evidence/plan corrections or removal of task-created accidental artifacts. No new runtime/service/API/SQL capability. If a real defect appears, route to explicit bounded in-scope repair under this controlling plan and re-run affected/final gates; do not smuggle implementation into cleanup or reset failure budget.
- **Ordered tasks:**
  - [ ] Revalidate source/approval/base/plan fingerprints and full requirement matrix against actual candidate; verify migration 001–005 hashes and next migration/source checksum, permissions, no stubs or management UI scaffolding.
  - [ ] Run focused U/D/W selectors covering all Task 4B cases, old Resource creation/edit/no-op/commit tests, Booking creation/replay/cancellation cases and upgrade/privilege suites; inspect discovery/skips/real runtime use.
  - [ ] Run full F from current rebuilt source, with existing Go/PostgreSQL/HTTP/Playwright plus new state and old Resource/Booking lifecycle phases; require actual db/app restarts. Verify documented local ordering/setup remains accurate without using a persistent demo reset or claiming 4C browser acceptance.
  - [ ] Inspect complete base-to-candidate, staged, unstaged and full relevant untracked contents; preserve unrelated work, remove only accidental task artifacts. Check all exclusions, lock/isolation/trust boundaries, immutable data/history and unchanged accepted functionality.
  - [ ] Finalize README and evidence: service API/outcomes/precedence/event encoding, migration/permission/005 compatibility/recovery limitations, actual commands/suites/concurrency synchronization/fault/restart results, choices/risks and explicit partial/deferred acceptance. Re-run affected checks after any correction.
  - [ ] Produce completion report with implementation base, candidate HEAD/tree and dirty-delta fingerprints, exact changed files, V1–V9/PRD coverage, pass/fail/skip/not-run, significant Level 1 decisions, limitations/unresolved concerns, complete diff/scope audit and independent-review boundary.
- **Compile-safe state:** complete task only; no later-delivery feature. Documentation cannot reclassify missing required checks as optional.
- **Focused verification / discovery:** L/U `^TestResourceState`; L/D `^TestResourceState`; L/W `^TestResourceStateSession`; all old regression selectors above; G/C/S/R; F. Compare actual required case inventory including separately flagged restart tests. Recommended stress/race checks remain optional and labelled.
- **Risk / repair:** green last test hiding skipped/new uncovered behavior, source drift or unrelated cleanup. Apply two-failure budget per issue; explicit bounded repair/retest only, stop for unavailable required access/Level 2.
- **Rollback:** source reversal only with unrelated work preserved and never mass reset/clean; no accepted migration rewrite, ledger change, older-binary schema rollback or destructive recovery.
- **Handoff:** final evidence report to user/coordinator; no automatic next-task handoff. Separate fresh implementation reviewer and human acceptance/integration remain necessary.
- **Exact stop:** `READY_FOR_REVIEW` only if full approved implementation and mandatory verification pass with honest evidence and scoped diff; otherwise `BLOCKED` or `INCOMPLETE`. Stop; no commit/merge/deploy/Task 4C execution.

## 10. Final acceptance and evidence obligations

All boxes below remain unchecked until separately authorized execution supplies evidence.

- [ ] Seven Task 4B objective outcomes and authorization → locked state → equality no-op → fresh actual-transition time/eligibility precedence hold, with explicit desired state and distinguishable safe outcomes (V1–V3).
- [ ] Exact all-owner `confirmed AND end_at > fresh operation_time` post-lock separate query; no start/date/owner filter, stale snapshot, automatic Booking change or Resource→Account coordination lock (V3–V5).
- [ ] No-op means no UPDATE, timestamp rewrite or event attempt at service **and** guarded capability; unauthorized no-op denied; raw retained data unchanged (V1–V2/V7–V8).
- [ ] Each actual direction has exactly one correct atomic Resource event, actor/target/null Booking/old-new active/time equal updated_at; no events on rejects/no-ops (V2/V6).
- [ ] Both creation/deactivation and cancellation/deactivation orders, same-account compatibility, fresh-time boundaries, repeated/opposite states, edit serialization and rollback waiter re-evaluation proved with independent real runtime PostgreSQL operations and deterministic barriers (V3–V5).
- [ ] Both-direction Activity failure, outer rollback and **actual rejected COMMIT after statement success** retain neither partial mutation nor event; ambiguous committed/noncommitted acknowledgement paths remain operational unknown without blind retry/false rollback (V6).
- [ ] Effective role/table/every-column/function/schema/ACL/shadow abuse checks prove no new bypass, direct Resource grant or generic mutator; existing trusted actor/time and cancellation limitations accurately retained (V8).
- [ ] Fresh install, compatible merged-005→new upgrade, preserved 004→005 evidence, repeat migration, exact missing/unknown/mismatch refusal and immutable 001–005 source hashes verified with retained data/index/FK/capabilities (V8–V9).
- [ ] Repeat seed/start and **actual PostgreSQL/application restarts** preserve exact changed state/events, seeded Inactive choice, accounts/sessions/Booking/request/cancellation/history; post-restart no-op/opposite operation works; old 4A/3B lifecycle still passes (V7–V9).
- [ ] Focused checks then complete `make verify` pass with all required database/browser/lifecycle evidence actually executed; skipped/unrun checks cannot stand in for acceptance (V9).
- [ ] README, complete diff and final evidence report cover actual candidate/source fingerprints, decisions, results, limitations and deferrals; no future-task scaffolding, unrelated overwrite, source-of-truth modification or credential exposure. Independent review is a separate fresh session, not implementation self-check (NFR-011).

## 11. Migration prerequisites, risks and escalation boundaries

### Safe upgrade and failure behavior

Before any **separately authorized persistent target** is touched, identify target/provenance and read its ledger non-destructively against all accepted 001–005 files. Do not infer compatibility from a version name or absence of local containers.

- Compatible existing history without 005 may follow the normal explicit additive sequence; a 005 entry exactly matching `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7` may receive the next migration after prerequisite checks.
- An earlier/different 005 checksum, or evidence of persistent application of earlier 005, **stops that upgrade**. A new 006 does not bypass exact-ledger validation. Do not reset/recreate volume/database, rewrite/rechecksum ledger, replay 005, silently normalize retained data or prescribe an unauthorized recovery.
- Request an authorized non-destructive compatibility/correction procedure, including runner/ledger decision if needed. Recommend evidence-preserving roll-forward compatibility design, not destructive reset. Independent safe disposable verification may continue without prejudging target remediation.
- New binary must be built with new migration; apply explicit migration before startup/seed. `make migrate` alone does not build an updated image in the current Makefile: rebuild the tool image first or use the verified updated artifact; avoid stale-image instructions. Normal startup refuses mismatch/missing/unknown entries and never auto-migrates.
- Each migration/capability/ACL/ledger entry commits together; repeat migration is safe. Old binary rejects new exact ledger, so it is not an assumed rollback. There is no authorized down migration, deployment or destructive recovery. No schema/table/index/backfill beyond minimal guarded capability is planned.

### Material risk register

| Risk | Mitigation / verification evidence | Authority / blocking condition |
| --- | --- | --- |
| Resource→Account lock inversion or Account FOR UPDATE substitution | Stored-role ordinary read; creation NO KEY UPDATE unchanged; same-account holder/waiter FK case in 6, cancellation in 7; inspect SQL and actual lock modes | Lock/isolation redesign is Level 2, not performance discretion |
| Stale pre-lock/one-statement occupancy snapshot | Service Resource lock before clock; separate row read/occupancy; guarded PL/pgSQL lock then subsequent query; committed creator/cancellation waiter cases 4/6/7 | No stale snapshot allowance; design defect repaired in scope |
| Time precision or wrong exact-end comparison | Fixed UTC microsecond clock, end−1/exact/+1 and held-lock crossing cases; one operation time for eligibility/update/event, 3–4 | Changing predicate/time authority is Level 2 |
| False no-op from timestamp/text equality or stale desired toggle | Explicit bool, locked active equality only, auth before equality, no-UPDATE/event detectors, equal-time opposite transitions, legacy inactive occupancy, 2–4/8 | No toggle, arrival-order or exact-once request identity promise |
| Missing/duplicate/wrong Activity, catch-and-commit, false rollback claim | Guarded internally derived event; fault/outer rollback early; actual deferred rejection and acknowledgement split in 9; snapshot pair/retry counts | Eventless path/new audit risk exception requires Level 2 |
| Runtime privilege bypass via column grant, PUBLIC/inherited privilege, path/shadow bookings | Final capability guards, qualified SQL, safe owner/path, effective/attempted privileges in 2/5; no direct Resource grants | New risk/trust exception is Level 2; 003 is not precedent |
| Migration 005 persistent checksum divergence | Immutable hash table and per-target prerequisite; exact mismatch refusal tests 2/10 and documented stop behavior | Conditional target-specific Level 2; no affected target established in planning |
| New required version breaks old 004→005 or five-count lifecycle assumptions | Adapt explicit version slices and both count assertions in 2, keep old upgrade tests, new merged-005 upgrade evidence | Routine Level 0/1 repair, never remove earlier acceptance |
| Regression in creation/edit/replay/cancellation, including raw text and old event details | Do not widen old contracts; complete snapshots/read tests, edit/state schedules, inactive Cancelled/past replay, full existing suites in 10–11 | New behavior outside task needs authorization |
| Stale Docker image, test skip/no discovery, unordered waiters mistaken for proof | Rebuild images, L and verbose uncached tests, actual roles and flag phases, identify holder/waiter/PID before release, complete gate | Missing mandatory infrastructure/evidence blocks completion |
| Unsupported deployment/rollback assumption or shared-volume damage | Local disposable-only verification, confirm project ownership; document rebuilt migration-first artifact and old-binary incompatibility; no production target/access assumed | Required external/destructive action needs separate authorization |
| Environment, source or authorization drift | Chunk 0 and per-session fingerprints/preflight; no historical review/test result reused as current PASS | Missing authorization/access or material source contradiction stops dependent execution |

Escalate with **category, specific problem, source/code/error evidence, alternatives/tradeoffs, recommendation, precise decision required and blocked versus independent remaining work**. Ordinary API layout/test synchronization/compilation problems are Level 0/1 and repaired within budget. Material requirements ambiguity, security/data-integrity exception, lock/trust/isolation change, extra dependency/infrastructure, persistent-ledger reconciliation or 4C/intervention/history work is Level 2. Stop affected work without speculative dependent changes. No genuine unconditional Level 2 planning decision remains unresolved.

## 12. Future execution handoff instructions and planning limits

This is the only controlling ladder and contains **12 chunks, numbered 0–11**. All effort and verification obligations remain prospective. Subsequent independent plan validation should check source freshness, complete matrix/case coverage, safe intermediate states, fixture ownership and executor compatibility before any implementation preflight. Human implementation authorization remains separately necessary.

Suggested **future** invocation after those gates (not an instruction executed by this document):

> Use `chunked-implementation` with `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md` as the sole ladder. Execute **Chunk 0 only**, in a fresh session, after confirming explicit implementation authorization and current base-specific preflight authority/evidence. Re-read the complete Task 4B, referenced requirements/architecture and repository instructions. Resolve/report execution policy and fingerprints, establish baseline using the existing isolated verification infrastructure, preserve unrelated work, report evidence and stop. Ask before writing an unrequested single-chunk handoff. Do not implement Chunk 1 or enter continuous-loop mode.

Later chunk requests must name exactly one next accepted chunk and supply prior report/handoff and current policy/candidate. A separately authorized loop must dispatch one fresh worker per chunk with non-final H handoffs; lack of orchestration is disclosed, never simulated. Nothing here permits a branch, commit, push, merge, deployment, credential change or automatic transition into Task 4C.

**Planning self-check boundary:** complete named authoritative documents and relevant accepted code/test seams were inspected; source/candidate/ancestry/migration identities were recorded. The author must re-read this entire plan including this ending, check numbering/dependencies/coverage/compile safety/command feasibility/effort totals and inspect the full untracked documentation diff before reporting its final planning status. Only this plan may differ from the initial clean tree. No test/build/migration/lifecycle PASS is claimed from documentation checks, and no implementation preflight is performed by writing or validating this plan.

**End of controlling Task 4B plan.**
