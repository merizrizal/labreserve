# Task 4A — Atomic Resource creation and descriptive editing

## Status and parent delivery

**Draft for human review; not approved for implementation.** Parent: Delivery 4 — Coordinator Resource management. Product decisions D4-001–D4-003 are approved and recorded in [architecture 004](../architecture/004-resource-creation-and-text-contracts.md). Approval of those decisions is not approval to execute this task.

This contract becomes implementation authority only after human task approval, review and commit of the authoritative documents, and the repository's implementation preflight. This documentation-authoring task changes no application behavior.

## Documentation baseline and authoritative sources

Documentation was prepared against `77605c28782c0b6883d6670d4cc17ce7bf8ff28f`, with a clean working tree before authoring. `AGENTS.md`, the PRD, baseline, architectures 001–003, and Tasks 2A–3C were tracked and committed. Architecture 004 and this contract are new review candidates, not committed implementation prerequisites yet. A future executor must record its actual implementation base and working-tree state; it must not substitute this historical documentation base.

Read these sources completely, including their endings and applicable references:

- [Repository instructions](../../AGENTS.md).
- [PRD](../prd.md): Resource identity/management, trusted permissions, atomic Activity, durability, verification, Delivery 4 scope, and open-question sections.
- [Baseline](../baseline.md), especially section 13; architecture 004 records the newly approved text and initial-state decisions that resolve the affected historical questions.
- [Architecture 001](../architecture/001-initial-architecture.md), especially sections 2–4, 6–11, 13, and 15.
- [Architecture 002](../architecture/002-my-bookings-and-cancellation.md): preserve the accepted owner-cancellation semantics and lock ordering; later accepted task contracts govern implemented seams described there historically as proposed.
- [Architecture 003](../architecture/003-cancellation-privilege-boundary.md): the accepted cancellation limitation is not permission for new unguarded Resource writes.
- [Architecture 004](../architecture/004-resource-creation-and-text-contracts.md): D4-001–D4-003 and their schema/privilege consequences.
- Accepted contracts [2A](002a-booking-persistence.md), [2B](002b-atomic-booking-creation.md), [2C](002c-booking-web-workflow.md), [3A](003a-my-bookings.md), [3B](003b-atomic-owner-cancellation.md), and [3C](003c-cancellation-web-workflow.md).

Do not interpret historical exclusions in an earlier task as removal of later accepted functionality. Equally, the full Delivery 4 PRD does not authorize all of Delivery 4 within Task 4A.

## Objective

Implement only the internal application-service and persistence capability for a trusted, authenticated Coordinator to:

1. Create a non-seeded Resource with canonical code/name/description and an explicitly chosen initial Active or Inactive state.
2. Edit an existing Resource's name and description, without changing its code, identity, active state, creation instant, Bookings, or prior Activity.
3. Receive explicit domain outcomes for actual success, unchanged-edit success, and safe rejection/failure.

Each actual mutation must commit with exactly one corresponding Activity event. Unauthorized/invalid/duplicate/code-change requests and unchanged edits must leave no successful-change event. The unchanged-edit result performs no Resource update or timestamp rewrite.

This is not a Resource-management browser workflow. It must be independently reviewable through the service, real PostgreSQL, existing read paths, and the existing verification entry point.

## Prerequisites and implementation readiness

- Delivery 1 and accepted Tasks 2A–2C and 3A–3C are integrated, including authentication/session security, Resource browsing, Booking creation/replay/schedules, My Bookings, and owner cancellation. Inspect the current implementation rather than relying only on delivery labels. This contract does not re-certify their acceptance.
- This task and architecture 004 must be human-reviewed, approved as applicable, tracked, and committed before implementation; verify their content at the implementation base, not merely that files exist.
- Read the complete task and governing documents, identify the actual base revision and local changes, establish practical current verification evidence, and report `PREFLIGHT_PASS` before implementation edits. An incomplete/contradictory contract or missing required approval/commit is `PREFLIGHT_BLOCKED`.
- Reinspect Resource, Booking, Activity, authentication, migration, runtime-role, and verification boundaries if the base has advanced. Preserve unrelated local work.
- Required PostgreSQL and existing verification tooling must be available before claiming completion. Missing access is a reported blocker, not permission to replace database evidence with mocks.

## Authoritative product rules

### D4-001 — Name, description, and unchanged edits

- Name is required. Trim surrounding Unicode whitespace; reject an empty-after-trim value.
- Description is optional. Trim surrounding Unicode whitespace; persist omitted/empty/whitespace-only description as the valid empty string, consistent with `TEXT NOT NULL`.
- For both fields, preserve internal characters/whitespace and casing. Do not apply Unicode normalization, collapse whitespace, or add arbitrary product-level maximum lengths.
- Use the established Go Unicode-whitespace trimming semantics, not an ASCII-only trim. Use exact canonical equality; case, internal spacing, and composed versus decomposed Unicode remain distinct where their characters differ.
- Compare submitted canonical name/description with the locked Resource's canonical current values. Equality is successful no-op, including surrounding-whitespace-only submissions. Do not issue even a self-assignment UPDATE, rewrite `updated_at`, or append Activity. Comparing legacy values does not authorize rewriting them on a no-op.
- Authorize before no-op success. Real descriptive changes apply to Active and Inactive Resources, even when they retain future, in-use, past, or Cancelled Bookings; editing does not alter reservation eligibility.

### D4-002 — Code and identity

- Code is required and nonempty after surrounding Unicode-whitespace trimming. Persist the trimmed value, preserving submitted casing and internal characters.
- Do not add uppercase-only, pattern, Unicode-normalization, or arbitrary product-level length restrictions.
- Preserve PostgreSQL's existing `resources_code_case_insensitive_uq` index on `lower(code)` and its uniqueness semantics, including concurrent case variants. Do not replace it with only a pre-insert lookup or a different application case-folding definition.
- Duplicate creation returns an explicit duplicate-code rejection with no new Resource/event and no rewrite of the existing Resource. Do not upsert, merge, or treat a duplicate as a successful Resource-creation replay.
- Code is immutable after creation, including case-only changes. A requested code change must be rejected without any partial descriptive update or successful-change event; do not silently ignore it.
- A narrow edit command may accept only descriptive changes, with explicit rejection of code-mutation input, or carry a code assertion compared to the retained code after trimming. The exact interface is Level 1; an observable/testable code-change rejection and absence of mutation authority are mandatory.
- Resource ID, code, and `created_at` remain unchanged by descriptive editing. Existing Booking identities, resource links, original request data, states/metadata, and prior Activity details remain intact.

### D4-003 — Initial state

- Creation permits Active or Inactive. The application command and persistence call carry and explicitly persist the chosen state, including Inactive; never omit `false` and accidentally obtain SQL's `DEFAULT TRUE`.
- The browser creation form must visibly default to Active in the separately approved browser task. Record this requirement now; do not implement or test a new Resource-management form in Task 4A. The internal service must not depend on an absent form or SQL default to choose state.
- One Resource-created Activity event records the selected initial state and relevant creation values. Do not emit a separate activation event during creation.
- Initial-state selection does not authorize activation/deactivation of any existing Resource.

### Cross-cutting rules

- Only trusted authenticated Coordinators may invoke these use cases successfully. Engineer, absent/unknown-account, and forged-role attempts must not mutate Resource or Activity state, including through the unchanged-edit path.
- New Resources participate in existing catalog/schedule/Booking reads. An active new Resource can be booked under normal Task 2B rules; an inactive new Resource is visible but new booking is rejected as inactive.
- Resource text is plain text. Persist canonical content without HTML sanitization/rewriting and preserve existing `html/template` contextual escaping on read paths.
- Follow established valid-text/encoding handling; never silently repair invalid input. Technical encoding or transport safety controls must not become undisclosed product length/pattern restrictions.
- Seed accounts/Resources remain bootstrap state, producing no product events. Ordinary application creation is not a seed/backfill exemption.
- No new Resource retry key/table or booking-style Resource creation replay contract is requested. Preserve the existing duplicate-code outcome and safe handling of uncertain commit results.

## PRD traceability and bounded acceptance

The approved client decisions in architecture 004 resolve Resource text/initial-state details; they do not silently rewrite the PRD or claim completion of its broader browser/state criteria.

| PRD requirement / acceptance | Task 4A evidence and boundary |
| --- | --- |
| FR-001–FR-002, FR-023; NFR-002; AC-015–AC-017 | Trusted Coordinator service authorization, Engineer/unknown-actor rejection, role-spoof resistance, and unchanged existing session security. Resource-management direct HTTP/browser acceptance is deferred, not claimed here. |
| FR-005, FR-023; AC-028, AC-042–AC-043 | Stable Resource identity, immutable code, retained Bookings/request data/history, database-enforced case-insensitive uniqueness, canonical creation and descriptive edits. AC-028's existing-state changes and all management UI remain deferred. |
| FR-007, FR-009, FR-023; AC-021, AC-041 | Service-created Active/Inactive Resources work with existing browse/schedule/Booking paths. AC-041's browser creation and activation/deactivation portions are not completed by 4A. |
| FR-026–FR-027; AC-012, AC-028, AC-043–AC-044 | Exactly one creation/edit event, correct actor/target/time/details, complete event-failure rollback, and no successful-change event for rejects/no-ops. AC-044 activation/deactivation evidence belongs to a later approved task. |
| NFR-001, NFR-003; AC-011, AC-031 | Preserve timestamp conventions, Jakarta Booking display, and safe Resource text rendering in existing reads. No new forms or CSRF surface; existing CSRF checks remain regression obligations. |
| NFR-007–NFR-008; FR-015, FR-021; AC-013, AC-030, AC-033, AC-040 | Non-destructive migration/seed/restart lifecycle, durable Resource and Activity changes, unchanged Booking retention and creation/cancellation replay. |
| NFR-005–NFR-006, NFR-010–NFR-011; AC-032 | Existing local isolated stack, single `make verify` entry point, controlled time, independent real-database concurrency, evidence report and separate-session independent review. |

**Explicitly deferred, not dropped:** FR-024–FR-025 and AC-009–AC-010/AC-029 (existing-Resource deactivation eligibility and booking/deactivation races); existing-Resource activation; FR-029/NFR-009 and the new Resource-management browser portions of AC-016/AC-028/AC-041; coordinator cross-owner cancellation under FR-019–FR-020/AC-024; Activity inspection under FR-028. No Task 4B or Task 4C implementation is authorized by this contract.

## Architectural constraints

### Existing boundaries to reuse

- Go modular monolith, existing `database.Store`/`pgx` transaction conventions, injectable authoritative clock, PostgreSQL, and local Docker Compose. No new runtime service, framework, ORM, queue, worker, or event-sourced state model.
- `internal/database/migrations/001_foundation.sql` supplies Resource storage and the code index; migration 002 supplies Booking/Activity foreign keys and constraints. Preserve migrations 001–004 unchanged.
- `internal/auth/service.go` and `internal/database/store.go` resolve current authenticated accounts/roles through server-side sessions. The Resource service receives trusted actor context separately from editable Resource data; browser-supplied actor/account/role is not authority.
- `internal/bookings/service.go`, `cancellation.go`, and their database adapters remain authoritative for Booking behavior. Reuse migration 003's `public.labreserve_lock_booking_resource` where appropriate; do not grant UPDATE just to obtain Resource row locks.
- Activity writes use the existing `activity_events` table and exclusive Booking-or-Resource target contract. Event persistence never independently commits, opens a second connection for the mutation, or runs asynchronously.

### Service, transactions, locking, and time

Provide narrow internal creation and descriptive-edit operations. Services own authorization, canonical validation, explicit domain outcomes, and the outer transaction. Persistence capabilities enforce their final write guards and mandatory Activity coupling; this does not move business logic into a future HTTP handler.

**Creation:**

1. Resolve/authorize the trusted actor as an existing Coordinator using stored account authority, not a submitted role. Canonicalize/validate input and require an explicit initial state representation.
2. Begin one explicit READ COMMITTED transaction (the authorization lookup may occur within it). Use one authoritative operation timestamp at established UTC microsecond precision, captured after any required lock waits.
3. Through the guarded capability, insert one Resource with stable server-generated identity, canonical text, explicit active state, and explicit `created_at`/`updated_at` equal to the operation timestamp. There is no pre-existing Resource row to lock; the case-insensitive unique index arbitrates competing creations.
4. Couple the insert to one Resource-created Activity event in that same transaction; commit once before reporting success. A uniqueness rejection rolls back, not an update/replay of a competing Resource.

**Descriptive edit:**

1. Authorize the trusted Coordinator. Canonicalize/validate supplied text and identify the retained Resource by stable ID.
2. Begin one explicit READ COMMITTED transaction; lock that Resource `FOR UPDATE` using the shared protocol. Re-read authoritative values after acquiring the lock, in a fresh statement where needed. Do not decide no-op or before-values from a stale pre-lock read.
3. Reject unknown Resource or attempted code mutation, without applying any requested name/description changes. Authorize before returning any no-op.
4. Compare canonical submitted/current name and description. If equal, return unchanged success without any Resource UPDATE, timestamp change, or event.
5. Otherwise capture fresh authoritative time after the required lock waits. Update only name/description and `updated_at`; retain ID/code/active/`created_at` and every Booking/history record. Append one descriptive-edit event using the actual locked before-values and canonical after-values, at that same operation time.
6. Commit once, then report changed success.

Privileged helpers must also acquire/reuse the Resource lock and enforce guards when directly invoked, not assume that an earlier service call locked/validated it. A safe default is service acquisition of the existing Resource lock, then a guarded mutation/event helper reusing that lock. Do not compute final before-values from caller-authored snapshots. Do not add a Resource → Account coordination lock: ordinary account role reads/FK KEY SHARE are compatible with booking creation's Account `FOR NO KEY UPDATE`. No edit acquires a Booking row lock or mutates Bookings.

Preserve Account `FOR NO KEY UPDATE` → separate retained-request lookup → Resource for Booking creation and Resource → Booking for owner cancellation. Keep immediate Confirmed-only exclusion and all replay semantics. Later deactivation still requires Resource locking, fresh post-lock time, and a separate post-lock occupancy query; do not implement that query/use case here or preclude it by changing the protocol.

Every error/cancellation path releases/rolls back the transaction. Use existing finite operation/database timeouts; do not blindly retry database errors or replay ambiguous Resource creation as a fresh operation. Definite insert/event/commit rejection means no partial successful mutation. Transport loss around commit is an operational result with unknown durable outcome, not proof of rollback; callers can inspect retained Resource state. No new automatic retry/idempotency product behavior is authorized.

### Activity contract

Creation and descriptive editing must have distinct, stable action identities consistent with existing dotted action naming. Exact names/detail keys are Level 1 choices to document and test. Each event contains:

- authenticated Coordinator actor account ID, never a submitted actor/role;
- the operation action and the affected Resource ID, with no Booking target;
- authoritative UTC microsecond operation time, equal to creation/update time as applicable;
- creation: canonical code, name, description, selected initial active state;
- edit: relevant before/after name and description, sufficient to identify which values changed. Record both descriptive fields' snapshots or an equally complete changed-field representation; obtain before-values from the locked row, not the caller. Resource identity remains stable.

No extra activation event for creation, no success event for invalid/forbidden/duplicate/immutable-code rejection, and no event for unchanged edits. No credentials/session secrets in details. Prior Resource/Booking events remain unchanged; an earlier name is not retroactively replaced by the new name. Activity insertion is mandatory, and any failure must prevent the Resource mutation from committing.

## Narrow migration-managed database capabilities

The runtime currently has Resource SELECT, no Resource writes, existing row-lock EXECUTE, Booking SELECT/INSERT plus cancellation-column UPDATE, and Activity SELECT/INSERT. Do not infer new Resource authority from architecture 003's accepted cancellation bypass.

### Required security properties

- Add only operation-specific Resource creation/descriptive-edit capabilities through a new checksummed migration. Prefer guarded SECURITY DEFINER functions, following existing capability conventions. Equivalent mechanisms are Level 1 only if they demonstrably preserve every guard and effective-permission property here.
- Do not grant direct runtime Resource INSERT/UPDATE/DELETE/TRUNCATE, including column-level UPDATE. No exposed raw write-only helper may let runtime commit a Resource mutation without its corresponding event. Couple mutation and mandatory event at the guarded database boundary, not solely by convention in Go. An outer rollback must undo both.
- Guard actor existence and stored Coordinator role inside the capability as well as at the application boundary. A database check of a supplied account ID is defense in depth, not independent authentication; trusted application code must supply the authenticated account. Do not add caller-selected roles or use a transaction setting supplied by arbitrary input as proof of identity.
- Enforce only the operation's allowed fields: creation's explicit text/state; edit's descriptive fields/timestamp only. Code/ID/active/creation timestamp are not mutable through the edit capability. Reject code-change attempts at a tested command/persistence boundary, including case-only changes.
- Enforce canonical/nonempty text rules at the final capability boundary, or verify canonical inputs with exactly matching Unicode semantics. A direct helper call must not bypass required name/code validation, no-op suppression, locked before-values, or Activity details. Do not substitute SQL's default space trim for the approved Unicode contract.
- Generate corresponding action/target/details from the operation and authoritative persisted values, rather than accepting arbitrary success-event payloads or an option to skip history. The timestamp is the trusted service clock value, not browser data.
- SECURITY DEFINER functions require explicit schema qualification, a fixed safe `search_path` excluding caller-writable schemas, no unnecessary dynamic SQL, non-superuser migration-controlled ownership, revoked PUBLIC EXECUTE, narrowly scoped runtime EXECUTE, and no runtime ability to replace/alter functions, assume the owner role, or alter their dependencies. Keep ownership/ACL setup transactional with migration application. Minimize definer authority and document the choice using the existing role setup.
- Preserve existing Booking/session/read privileges and lock helpers. Do not expand Account writes/role changes, Booking writes, Activity UPDATE/DELETE/TRUNCATE, schema/role administration, or grant options. No new generic privileged mutation framework or bulk import/backfill capability.

The existing shared role and Activity INSERT grants are not newly hardened against arbitrary runtime SQL impersonation or fabricated standalone events, nor is administrator-proof audit promised. Do not claim the database authenticates a supplied actor ID. Task 4A must nonetheless add **no Resource-mutation-without-Activity path** or new integrity-risk exception. Existing cancellation-column hardening or broad Activity-grant redesign is outside this task. If satisfying these properties is found to require a material risk/trust-model change, escalate before adopting it.

### Migration and lifecycle constraints

- Use the existing embedded SQL migration runner, SHA-256 ledger, and single-runner advisory lock. Add the next migration after 004, update required-schema expectations, and test exact-ledger rejection. Do not edit/rechecksum applied migrations.
- Retain existing tables, Resource `TEXT`/non-null boolean storage and code index, foreign keys, Bookings, original request data, and Activity. No replacement Resource identity/table, destructive reset, nullable-description redesign, speculative schema, or arbitrary length/pattern checks.
- Do not normalize/backfill retained text/codes or fabricate historical events as a side effect of adding capabilities. Investigate any legacy-data incompatibility; do not silently destroy, rename, or rewrite data to make a migration pass.
- Startup refuses missing/incompatible migration state and does not implicitly migrate. Explicit migration is repeat-safe; permissions and migration ledger commit together. Repeated seeding preserves existing names/codes/descriptions/states and creates no product events.
- Document the local upgrade ordering (explicit migration before the new binary), compatibility checks, and non-destructive recovery implications. The old binary's exact-ledger check rejects a newer schema; reset or ledger tampering is not rollback. No production deployment or automatic down-migration is authorized.

## Domain outcomes and implementation boundaries

At minimum distinguish, without HTTP status codes or raw PostgreSQL diagnostics in the service contract:

- successful creation, returning stable Resource identity and chosen persisted values;
- successful changed descriptive edit;
- successful unchanged descriptive edit, explicitly distinguishable from a mutation;
- authentication/Coordinator authorization rejection;
- invalid input/canonical validation rejection;
- unknown Resource for editing;
- immutable-code edit rejection;
- duplicate-code creation rejection;
- operational/database failure, without claiming definite rollback when commit outcome is unknown.

Exact Go types, error/result names, and package placement are Level 1 choices following existing service conventions. Known code-index uniqueness is the duplicate outcome; do not map all uniqueness failures, event failures, or database errors to duplicate/validation success. No separate public JSON API, HTTP mapping, form, navigation entry, or management placeholder is required.

Permitted implementation changes are limited to the Resource service, narrow transaction/persistence adapters, necessary migration/schema-version expectations, focused tests, existing verification lifecycle integration, and accurate README/implementation notes. Minimal shared-helper refactoring is permitted only to support these requirements without changing accepted behavior. Updating verification assertions for a new migration/capability is necessary; weakening security checks or removing earlier coverage is not.

## Explicit exclusions

Do not implement or scaffold:

- existing-Resource activation/deactivation, deactivation occupancy checks, or the booking/deactivation race use case;
- Resource-management HTTP routes, handlers, browser forms, navigation, management UI tests, or Task 4B/4C placeholders;
- Coordinator cross-owner cancellation, its reason workflow, or changed owner-cancellation semantics;
- Activity-history inspection UI/query API or Activity editing/deletion;
- Resource/Booking deletion, Booking mutation/rescheduling, early release, new stored Booking states, or new Booking request-identity behavior;
- cancellation privilege hardening/redesign, unrelated security work, new infrastructure services/dependencies, notifications, maintenance windows, or external integration.

Existing Booking service invocations and HTTP/browser reads in tests are regression/integration evidence, not authorization to change those use cases. Creating a Resource initially Inactive is in scope; changing an already-created Resource's active flag is not.

## Required verification

### Infrastructure and evidence discipline

Reuse Go unit/service tests, `internal/database/*_integration_test.go` runtime/migrator pools and fixtures, existing web/HTTP read tests, test-only controlled time, and `scripts/verify.sh`/`make verify`. Existing database test environment names are `LABRESERVE_TEST_DATABASE_URL` and `LABRESERVE_TEST_APP_DATABASE_URL`; respect their disposable `labreserve_test` target guard. Use the migration role only for migration, isolated fixtures/failure injection, and inspection; run normal Resource use cases through the actual restricted runtime role.

Concurrency uses independent PostgreSQL connections/transactions and deterministic barriers or observed lock waits, reusing existing lock-wait helpers where suitable. No process-local mutex, arbitrary sleep, shared enclosing transaction, or mock repository substitutes for the required database evidence. Scope failure triggers to the fixture/action and remove them reliably. Keep fixtures isolated and never reset the developer's demo volume.

Focused canonicalization/service unit tests complement, but do not replace, all of the following real-PostgreSQL coverage. For every rejection/no-op, inspect persisted Resource fields/timestamps and relevant Activity counts/details, not only the returned error.

### V1 — Trusted authorization

- A real Coordinator creates and edits successfully; each Engineer is rejected for creation and editing.
- Missing/unknown actor and attempts to supply a Coordinator role for an Engineer cannot confer permission. Resolve stored authority rather than trusting command fields. Include an unauthorized unchanged-edit attempt.
- Exercise the trusted account/session-to-service seam using existing authentication support, without adding Resource HTTP endpoints. Preserve current session invalidation/role resolution behavior.
- Direct guarded-capability calls with Engineer/unknown actor fail without mutation/event. Document that supplying a known Coordinator ID through arbitrary runtime SQL is outside independent authentication guarantees; do not claim a database role check proves session identity.

### V2 — Canonical creation and edit validation

- Reject missing/empty/Unicode-whitespace-only name and code. Include non-ASCII edge whitespace, not only ASCII spaces.
- Accept omitted/empty/whitespace-only description and persist empty string; accept nonempty Unicode description.
- Assert exact trimmed persistence and preservation of casing/internal spaces, tabs/newlines, punctuation, non-ASCII and composed/decomposed characters for the relevant fields. Prove there is no forced uppercase, normalization, internal whitespace collapse, or invented code pattern.
- Include representative long valid text values exceeding plausible accidental UI/domain caps without inventing a new maximum. Verify the service/storage impose no arbitrary product limit; do not test beyond PostgreSQL's technical representation limits.
- Test name-only, description-only, and both-field changes; allow clearing description, reject clearing name. Validate edits on both Active and Inactive Resources, including those with retained bookings.
- Exercise invalid/noncanonical direct capability inputs so its final guards cannot be bypassed.

### V3 — Duplicate code and independent concurrency

- Reject an exact duplicate and case variants of an existing code; surrounding Unicode whitespace is trimmed before the same uniqueness check. The existing Resource, metadata/timestamps, bookings, and events are unchanged.
- Run two independent valid Coordinator creations with the same canonical code but different casing; against a healthy database exactly one creates, the other returns duplicate-code, and exactly one Resource plus its single creation event commits. Prove database-enforced uniqueness, not just a precheck.
- Preserve existing direct-insert uniqueness regression tests as the index backstop. Add service/capability concurrency evidence; existing migrator-only insert tests are insufficient for Task 4A.
- Force a first contender's event insertion to fail while a case-variant contender waits. After rollback the waiter may create; no orphan Resource/event remains from the failed contender and there is exactly one successful creation/event.

### V4 — Immutable code and no-op semantics

- Reject attempted code edits to another code, an existing code, and a case-only variant; no partial descriptive change, timestamp rewrite, or event. Exercise the selected command rejection/assertion boundary and verify direct runtime code UPDATE is denied.
- Exact same canonical name/description and surrounding-whitespace-only edits succeed as unchanged; preserve every Resource field, `created_at`, `updated_at`, and Activity count/details.
- Verify absence of any Resource UPDATE, not merely equality of timestamps. Use a scoped test-only UPDATE detector/failure trigger or equivalent observable database assertion that would catch self-assignment UPDATE. A no-op also succeeds when an Activity insertion-failure fixture is installed, because it must not attempt event append.
- Case-only descriptive changes, internal-spacing differences, and distinct Unicode composition are real changes, not false no-ops.
- Serialize two identical descriptive edits after a held Resource lock: the first actual change emits one edit event; the waiter sees the committed canonical values and returns unchanged with no second update/event. For two different edits, before/after event snapshots must reflect their actual serial order, not stale caller data.

### V5 — Activity details, time, locking, and atomicity

- For creation in each initial state, inspect one creation event's actor/action/Resource target, no Booking target, exact canonical values and selected state, and equality of event time with Resource creation/update time. Assert zero separate activation events.
- For descriptive edits, inspect exactly one event, actual locked before-values, canonical after-values, correct actor/target, event time equal to `updated_at`, and retained `created_at`/code/active. Prior Resource and Booking events remain byte-for-byte or structurally unchanged as appropriate to their storage.
- Hold the Resource lock, start an edit, advance controlled time while it waits, release the lock, and assert fresh post-lock operation/event time. A preceding committed edit must be reflected in the waiter's before-values/no-op decision.
- Fail required Activity insertion after creation/edit would otherwise mutate. Both creation states and actual descriptive editing must roll back entirely: no created Resource, or original existing values/timestamps retained, and no success event. Existing Bookings/history/replay stay intact. Remove the failure and retry successfully with exactly one event for the committed mutation.
- Exercise direct capability calls and an outer transaction rollback, proving that capability invocation cannot commit Resource changes while omitting their event. Include definite commit rejection and safe operational handling of an unconfirmed commit result using existing transaction test seams where available; do not mislabel an unconfirmed result as rollback evidence.
- Exercise edit serialization with accepted Booking creation and owner cancellation on the same Resource using independent operations. Preserve successful Booking/replay/cancellation outcomes and avoid Account/Resource lock cycles; an edit cannot change eligibility or occupancy. Do not implement activation/deactivation to construct these tests.

### V6 — Resource identity, retained Bookings, and existing read paths

- Create through the service, retrieve via existing Resource list/detail/schedule reads, and successfully book an Active result through Task 2B. An Inactive result appears with inactive state and Task 2B returns its existing inactive-resource rejection with no Booking creation event.
- Prepare confirmed future/in-use/past and Cancelled retained Bookings and their existing history using existing services or isolated fixtures. Descriptively edit their Resource and assert unchanged Resource ID/code/active/creation time and all Booking IDs, resource/owner/request IDs, intervals, purposes, states, creation/cancellation metadata, and prior events.
- Original Booking creation replay still returns the same Booking after a Resource edit, including a Cancelled Booking, without adding Booking/event or restoring occupancy. Existing schedules, detail, and My Bookings still show retained records with the updated Resource description/name context where applicable.
- HTML-like Resource name/description content from the service remains exact plain text in storage and safely escaped in existing authenticated list/detail views. Reuse real HTTP/PostgreSQL tests without introducing management routes; existing browser content/CSRF/timezone tests continue to run.

### V7 — Effective runtime database permissions

Use the actual runtime login, not a superuser or a migration-role approximation. Inspect effective table/column/function/schema/role privileges (including PUBLIC and inherited grants) and attempt disallowed operations in isolated transactions:

- approved create/edit capability EXECUTE works; direct Resource INSERT, every column UPDATE (especially ID/code/active/timestamps), DELETE, and TRUNCATE fail;
- no new Account mutation/role administration, immutable Booking-field writes, broad Booking UPDATE/DELETE, Activity UPDATE/DELETE/TRUNCATE, schema DDL, function replacement/alteration, owner-role assumption, or grant-option authority;
- architecture 003's existing cancellation-column grants are precisely preserved and remain accurately documented; do not assert those existing column writes are denied;
- catalog checks prove definer ownership is non-superuser/migration-controlled, fixed safe search path, schema qualification, PUBLIC EXECUTE revoked, and only intended runtime EXECUTE granted. Verify an ungranted caller cannot execute and caller-controlled search paths/shadow objects cannot redirect the privileged operation;
- direct capability calls cannot mutate code/existing active state, accept invalid required text, forge before-values/details, skip Activity, or turn a no-op into an update/event. Confirm the existing Activity grants are unchanged, not newly described as a fully hardened audit boundary.

Permission tests must fail if a broad/table or descriptive-column grant is added. Do not weaken earlier Resource-denial tests to accommodate such a grant.

### V8 — Migration, seed, and restart lifecycle

- Verify fresh migration and upgrade from actual migration-004 data containing Resources, Bookings/request identities, Activity, and sessions. Preserve those records, IDs, index behavior, and runtime permissions; no bootstrap/backfill events.
- Verify exact ledger/startup rejection for missing new migration, unknown migration, and checksum mismatch. Explicit repeat migration is safe. Retain previous migration checksums and non-destructive seed/reset safety checks.
- Test repeat seed after an application-created Resource and after edits to a seeded Resource: preserve codes/names/descriptions/active states, Bookings, and Activity, without duplicate seeds/events.
- Extend the existing isolated restart lifecycle: create Active and Inactive Resources through the service, descriptively edit, retain related Booking/history, restart PostgreSQL and application on the same verification volume, then reload through fresh runtime connections/service instances. Assert Resource values/identities/timestamps, creation/edit events and counts/details, and effective capabilities survive.
- After restart, equal descriptive submission is still no-op, duplicate case-variant creation still rejects, and existing Booking creation/cancellation replay remains unchanged. Reconnecting a pool without an actual PostgreSQL/application restart is not sufficient restart evidence.
- Update migration-count/required-version assertions in existing verification to reflect the additive migration, without removing earlier foundation/cancellation persistence assertions or using reset as upgrade/recovery.

### V9 — Required repository regression gate

Run focused tests first, then the complete existing `make verify` entry point. It must include the new Task 4A Go/real-PostgreSQL/lifecycle tests and all accepted Delivery 1–3 Go, HTTP, PostgreSQL, and Playwright suites. It must not silently skip database/browser checks. No new Resource-management Playwright workflow is required in this internal-service task.

Preserve authentication/logout/CSRF, content escaping, Jakarta schedule intersections/labels, stable 25-record owner-filtered pagination, overlap/exclusion, retained request identity, creation replay and cancellation no-ops, time boundaries/freshness, interval release, non-destructive seeding, and restart durability. Passing old tests alone is not evidence for the new capability/atomicity invariants.

Record actual commands, executed suites, results, skips/failures, candidate state, and limitations. A unit-only invocation that skipped database tests is not a completed verification gate.

## Implementation authority

After this contract is approved and committed and implementation preflight passes:

- **Level 0:** Implement only Task 4A, add required tests/docs, run focused/full verification, diagnose and repair in-scope compile/test/formatting/fixture failures, and inspect/remove accidental unrelated changes without routine permission requests.
- **Level 1:** Choose internal Resource package/API/result/error shapes; explicit-state representation; code-mutation rejection/assertion interface; guarded function names/decomposition or an equivalently restrictive capability; canonical-validation implementation matching Go's Unicode contract; stable event action/detail keys; non-destructive migration details/ownership within the existing role setup; deterministic test/failure-injection seams and small in-scope helper refactoring. Document meaningful choices and verify all required properties.
- **Level 2:** Stop for product-rule changes/ambiguities, lock/isolation/invariant redesign, unapproved infrastructure/dependencies, destructive data remediation, broader Resource writes, a new Activity-integrity/security risk, changes to the trusted actor/database trust model, or functionality outside Task 4A. Architecture 003 is not blanket risk acceptance. Report category, precise problem/evidence, alternatives/tradeoffs, recommendation, exact decision requested, and blocked versus safe independent work.

No material Level 2 question is presently identified for this bounded design. Difficulty implementing/testing guards is a Level 0/1 engineering problem unless investigation establishes that an approved boundary cannot be preserved. Do not escalate routine helper design, nor silently accept a material risk as an implementation detail. No commit, push, merge, deployment, credential change, or final acceptance is authorized.

## Completion criteria

Task 4A is `READY_FOR_REVIEW` only when:

- trusted Coordinator creation and descriptive editing implement D4-001–D4-003 and explicit domain outcomes;
- canonical validation, immutable/case-insensitive code identity, both initial states, and real unchanged-edit no-op are verified;
- every actual Resource change commits exactly one accurate Activity event, with complete rollback on required event failure;
- guarded migration-managed capabilities meet the final-write, effective-privilege, lock/time, and no-eventless-Resource-mutation requirements without new risk acceptance;
- Resource identity, existing Booking/request data/history, accepted authentication/security behavior, migration/seed lifecycle, and restart durability are retained;
- V1–V9 coverage is implemented and required focused plus complete repository verification actually pass without concealed skips;
- README accurately describes the internal capability, migration/privilege and recovery choices, unchanged cancellation limitation, and still-deferred management browser/state/history workflows;
- complete final diff is inspected, unrelated changes removed, and no Task 4B/4C or other excluded functionality/scaffolding is introduced.

Implementation self-check does not replace independent review in a separate fresh agent session. Human final acceptance/integration remains required.

## READY_FOR_REVIEW reporting requirements

Finish implementation with exactly one status: `READY_FOR_REVIEW`, `BLOCKED`, or `INCOMPLETE`.

For `READY_FOR_REVIEW`, report:

- implementation base revision and exact candidate revision/working-tree state, including all uncommitted files;
- service API, explicit outcomes, canonical/state/no-op and immutable-code decisions;
- transaction/lock/time protocol, Activity action/detail shapes, event coupling and rollback evidence;
- new migration/schema-version changes, effective runtime permissions and helper ownership/security properties, upgrade/recovery implications;
- PRD requirement/acceptance coverage and explicitly partial/deferred criteria;
- verification commands actually executed, observed results, independent concurrency, event-failure, no-UPDATE no-op, migration/privilege, restart, and Delivery 1–3 regression evidence;
- meaningful Level 1 choices, anything not verified, limitations/deviations, and unresolved concerns;
- confirmation that the full diff was reviewed and excluded Task 4B/4C behavior was not implemented.

If a material Level 2 decision or required verification remains unresolved, report `BLOCKED` or `INCOMPLETE` as appropriate, not `READY_FOR_REVIEW`. Do not claim application acceptance, independent review, commit, or deployment from implementation readiness alone.
