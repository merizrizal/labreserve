# Task 4B — Atomic Resource activation and safe deactivation

## Status and parent delivery

**Approved for repository incorporation; not approved for implementation.** Parent: Delivery 4 — Coordinator Resource management. This is one bounded application-service/database contract, not an implementation plan, new ADR, approval, or preflight result.

Task 4A is implemented, independently reviewed, human-accepted, and merged according to the current authoring request. Its contract and architecture 004 retain historical draft/gate wording; do not mistake that historical wording for an unresolved Task 4A acceptance gate or rewrite those sources here. Task 4B implementation still requires its own explicit approval and tracked, committed authoritative contract. No subsequent review, planning, preflight, implementation, commit, integration, or deployment is authorized by this draft.

## Merged baseline, evidence, and prerequisites

### Observed integration baseline

- **Branch:** `main`; **merged revision:** `3542003c9929dc61aa8e9d3ce4be9adad9aff8c0` (also observed as `origin/main`). This is the Task 4B source baseline, not Task 4A's earlier implementation base.
- Merge commit: `Merge pull request #4 from merizrizal/feat/004a-resource-creation-editing`.
- Parents: `a90d3fd37226b5a648153edf7d134ec605a78dc0` and the final independently reviewed Task 4A candidate `e5b3c738f6d014880efe6aa6f1315c67b37bca80`. The candidate is an ancestor of HEAD; its complete tree has no diff against the merge revision.
- Initial staged, unstaged, and untracked working-tree state: clean. This draft's destination did not exist. Governing requirements, architectures 001–004, repository instructions, and accepted Tasks 2A–3C/4A are tracked and present in the merged tree. Only this new draft is authored in this session.
- Embedded migrations are `001_foundation.sql` through `005_resource_mutations.sql`; `internal/database/migrate.go` requires `005_resource_mutations.sql` and validates the entire exact SHA-256 ledger. There is no migration after 005 at this baseline.
- Observed merged Migration 005 SHA-256: `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`. This is a source-file checksum, not evidence of any persistent database's ledger.
- Read-only Docker inventory found no LabReserve containers or volumes. No live/persistent database ledger was queried, no database was started, and no local `.env` contents were read. Absence from this inventory does not certify every possible persistent database; the compatibility precondition below remains binding.

A future authorized executor must record its actual implementation base and all local changes, reread the complete approved contract and referenced sources, and inspect any intervening changes. Do not substitute this documentation baseline for that later check. Preserve unrelated work.

### Accepted interfaces and persistence to preserve

| Merged evidence | Existing behavior / implication for Task 4B |
| --- | --- |
| `internal/resources/service.go`: `Actor`, `Resource`, `Service.Create`, `Service.Edit`, `ResourceTransaction` | Actor contains an account ID, not a role claim. The service resolves stored Coordinator authority in its transaction, uses safe sentinel errors, returns retained Resource data, and distinguishes changed/no-op editing with `Changed`. Reuse these conventions; exact state-operation API remains Level 1. |
| `internal/database/resource_creation.go`: `ResolveCoordinator`, `LockResource`, create/edit adapters | READ COMMITTED; shared Resource locking function followed by a separate authoritative Resource read; same transaction for guarded mutation/event and final commit. |
| `internal/database/migrations/005_resource_mutations.sql` | `public.labreserve_create_resource` and `public.labreserve_edit_resource` are operation-specific, Coordinator-checked SECURITY DEFINER capabilities. They generate mandatory `resource.created` / `resource.descriptive_edited` events internally. Edit locks/re-reads Resource, suppresses canonical-equivalent writes, and cannot change existing active state. Neither helper grants Task 4B behavior today. |
| Migrations 001–002 | UUID Resource identity; non-null text and boolean state; `created_at`/`updated_at`; `resources_code_case_insensitive_uq` on `lower(code)`. Retained Booking request identity, foreign keys, Confirmed-only immediate GiST exclusion, and Activity's exactly-one-target/object-details checks already exist. |
| Migration 003; `internal/database/bookings.go` | Narrow Account `FOR NO KEY UPDATE` and Resource `FOR UPDATE` helpers. Booking creation orders Account → separate replay lookup → Resource. Replay/reuse decisions precede current eligibility and never take the Resource lock. |
| Migration 004; `internal/bookings/cancellation.go`; `internal/database/booking_cancellation.go` | Owner-only cancellation orders Resource → Booking, re-reads and authorizes, uses fresh time for Confirmed transitions, retains request data, and appends one event. Existing cancellation-column UPDATE grants and architecture 003's accepted limitation remain unchanged. |
| `internal/auth/service.go`; `internal/database/store.go`: `SessionByTokenHash`; `internal/web/middleware.go` | Opaque sessions resolve identity/current role from PostgreSQL. Resource commands receive the resulting trusted actor, not browser identity. Existing authentication, session revocation, CSRF/Origin, and escaping boundaries remain intact. |
| `deploy/postgres/01-roles.sql`; migrations 001–005 | Separate non-superuser `labreserve_migrator` and restricted `labreserve_app`. No direct runtime Resource INSERT/UPDATE/DELETE/TRUNCATE, including individual UPDATE columns; no runtime Account mutation or schema ownership. Existing Activity SELECT/INSERT is not Activity edit/delete authority. |
| `internal/database/migrate.go`, `seed.go`; `cmd/labreserve/main.go` | Embedded checksummed explicit migrations under advisory lock; startup and seed check compatibility, startup does not migrate; seed inserts only missing records and preserves existing active state/history. |
| `scripts/verify.sh`, `Makefile`, `compose.yaml`, Resource/Booking integration tests | Existing isolated Go/PostgreSQL/Playwright entry point, actual restricted runtime login, controlled clocks, observed lock waits, scoped failure triggers, definite deferred-COMMIT-rejection evidence, and actual database/application restart lifecycle. Extend these, not a parallel stack. |

## Objective and acceptance outcomes

Implement only the internal capability for a trusted, authenticated Coordinator to submit an **explicit desired Active or Inactive state** for an existing Resource. Do not implement a toggle whose effect depends on a stale caller state.

Required observable outcomes:

1. Inactive → Active restores eligibility for the accepted Booking creation workflow, subject to all normal Booking rules.
2. Active → Inactive succeeds only when no Confirmed Booking for that Resource has `end_at >` fresh authoritative operation time.
3. Authorized submissions of the current state succeed as genuine unchanged no-ops: no Resource UPDATE, no timestamp rewrite, no Activity attempt/event.
4. Every actual transition commits exactly one corresponding Resource Activity event with correct actor, target, action, old/new state, and operation time. Resource and Activity persistence are inseparable.
5. Authorization failure, unknown Resource, occupancy rejection, or a definitely failed transaction changes no business record and creates no successful-change event. An unknown commit outcome is not reported as proven rollback.
6. Resource ID/code/name/description/creation instant, all retained Bookings and original request data, and all prior Activity remain unchanged by these state operations.
7. Independent concurrent operations obey the established Resource locking protocol; no incompatible new Booking and deactivation both commit.

This task completes service/database state behavior only. It does not claim complete Delivery 4 browser acceptance.

## Authoritative sources and decision fidelity

The PRD governs product behavior; baseline decisions and approved architecture constrain interpretation/engineering; accepted task contracts establish prior obligations; merged code is implementation evidence, not permission to weaken requirements. Historical proposed sections are read with the later accepted contracts and the current Task 4A acceptance statement.

Read completely, including endings:

- [Repository instructions](../../AGENTS.md), [PRD](../prd.md), and [baseline](../baseline.md), especially sections 3–5 and 12–13.
- [Architecture 001](../architecture/001-initial-architecture.md), particularly §§2, 4, 6–11, 13, 15; [002](../architecture/002-my-bookings-and-cancellation.md), particularly §§4–5; [003](../architecture/003-cancellation-privilege-boundary.md), Decision/Known limitation/Risk acceptance; [004](../architecture/004-resource-creation-and-text-contracts.md), §§2–7.
- Accepted contracts [2A](002a-booking-persistence.md), [2B](002b-atomic-booking-creation.md), [2C](002c-booking-web-workflow.md), [3A](003a-my-bookings.md), [3B](003b-atomic-owner-cancellation.md), [3C](003c-cancellation-web-workflow.md), and [4A](004a-atomic-resource-creation-editing.md).
- Relevant merged code/verification paths identified above and below. The [Task 4A repair handoff, “Migration policy and database evidence”](../handoffs/task-004a-review-r4a01-to-r4a02.md#migration-policy-and-database-evidence) is historical evidence of the Migration 005 compatibility precondition, not replacement implementation authority or current proof of database state.

No new product decision IDs are introduced. The following fidelity inventory records approved rules and their negative boundaries:

| Approved source / decision | Affirmative rule | Prohibition / preserved invariant | Task 4B obligation and future evidence |
| --- | --- | --- | --- |
| PRD FR-002/FR-023; architecture 001 §§1, 4 | Trusted authenticated Coordinator controls existing Resource state. | Submitted identity/role cannot grant authority; state no-op cannot bypass authorization. | Authorization section; V1. |
| PRD FR-023/“Manage resource” operation contract; architecture 004 §6 | Activation restores ordinary Booking eligibility. | No new booking rule, auto-created Booking, or separate state model. | State rules; V2, V7. |
| PRD FR-024/FR-025; baseline §13; architecture 001 §10 | Confirmed occupancy is `end_at > fresh now`, under shared Resource lock and separate subsequent query. | No start-time filter, stale snapshot, automatic cancellation, or lock-order replacement. | State/concurrency sections; V3–V5. |
| PRD FR-026/FR-027; architecture 001 §§7, 9 | Actual transition plus one accurate Activity commits atomically; unchanged state succeeds without change/event. | No self-assignment UPDATE, timestamp rewrite, rejected success event, asynchronous history, or partial commit. | Transaction/Activity/privilege sections; V2, V6. |
| D4-001, architecture 004 §2 | Canonical text validation/edit no-op behavior already accepted in 4A. | State operation cannot normalize, clear, rewrite, or extend descriptive editing, including legacy edge whitespace. | Retention/privilege boundaries; V7–V9. |
| D4-002; architecture 001 §2 / 004 §3 | Stable Resource ID and immutable case-insensitive code identity. | No code edit, forced uppercase, new pattern/length restriction, index replacement, or deletion. | Retention/privilege boundaries; V7–V9. |
| D4-003, architecture 004 §4 | Initial Active/Inactive choice and one creation event are accepted 4A behavior. | Initial state is not an existing-Resource transition; no extra activation event during creation. Creation form default remains 4C. | Preserve 4A; V9. |
| Baseline §12; architecture 002 §4; Task 2B “Replay behavior” | Owner-only future cancellation, authorized Cancelled no-op, retained creation identity and replay. | No cross-owner Coordinator clearing of occupancy; no Resource → Account coordination lock. | Concurrency/retention boundaries; V5, V7, V9. |
| Architecture 004 §5; Task 4A “Required security properties” | Guarded, operation-specific Resource mutation/event capability; runtime cannot omit required history. | Architecture 003 cancellation exception is not approval for unguarded Resource state writes. | Privilege section; V6, V8. |

## In-scope operations, precedence, and occupancy

### Command and authorization boundary

Accept a stable Resource identifier and an explicit desired boolean state (or equivalent explicit Active/Inactive representation). Activation and deactivation may be separate narrow methods or one desired-state method. A typed boolean `false` is an explicit Inactive value, not a missing/default field. Exact Go names/signatures are Level 1, not fixed by this draft.

Trusted actor context is separate from editable command data. Reuse `resources.Actor` and stored-role resolution or an equivalently narrow existing seam. An absent/malformed/unknown actor is unauthenticated; an existing Engineer is forbidden. Resolve stored Coordinator authority before target-dependent successful outcomes, including no-op. Database capabilities recheck actor existence/stored Coordinator role as defense in depth. Neither a browser role nor possession of a Resource identifier grants authority.

No caller-supplied current-state assertion, occupancy answer, actor role, timestamp, Booking mutation, text edit, or code change is needed. Keep state commands restricted to their allowed purpose; do not add incidental descriptive/code mutation authority. Existing UUID validation conventions apply: a malformed Resource identifier is invalid input, while a well-formed absent identifier is unknown Resource. Reuse Resource service authorization-before-command-validation convention; operational inability to resolve authority is a failure, not fabricated unauthenticated/forbidden success.

### Authoritative order

For an otherwise well-formed command, the required order is:

1. Establish trusted actor and stored Coordinator authorization; reject unauthorized actors before no-op or target-state success.
2. In one READ COMMITTED transaction, acquire the target Resource `FOR UPDATE` lock and read authoritative Resource values after acquisition. A preliminary unlocked read is not authority. If absent, return unknown Resource.
3. Compare locked `active` with the explicit desired state. Equality returns authorized unchanged success **before deactivation occupancy eligibility is applied**. An already-Inactive submission is not a deactivation transition, even if an isolated legacy/test fixture has future occupancy. Do not use it to repair data, cancel Bookings, rewrite time, or manufacture history. This order follows architecture 001 §7's unchanged-state no-op and PRD FR-026; architecture 001 §10's occupancy guard applies to a deactivation change.
4. For an actual transition, capture fresh authoritative time after relevant lock waits, then apply the direction-specific rule below. An operational failure while obtaining required state/authority is never converted into a no-op.
5. Perform only the permitted state/timestamp mutation plus mandatory corresponding Activity in that same transaction; commit once before reporting changed success.

### State-transition table

| Locked current state | Desired state | Required decision | Resource / Activity effect |
| --- | --- | --- | --- |
| Inactive | Active | Activate; there is no deactivation occupancy gate for activation. | Set `active = true`, set `updated_at` to operation time, append one activation event. |
| Active | Active | Authorized no-op. | No UPDATE, no time change, no event attempt. |
| Active | Inactive | Query authoritative Confirmed occupancy after locking and fresh time capture; reject if any blocker exists. | On rejection, nothing changes. Otherwise set `active = false`, set `updated_at`, append one deactivation event. |
| Inactive | Inactive | Authorized no-op, not a new occupancy decision. | No UPDATE, no time change, no event attempt. |

Activation does not require zero historical/current Bookings, revalidate existing Booking intervals, or change them. It enables only subsequent otherwise-valid **new** creation; it does not bypass time, purpose, overlap, ownership, or retained-request rules. Successful deactivation makes subsequent new creation return the existing inactive-resource outcome, before overlap probing. Identical creation replay remains valid even while Resource is inactive.

### Exact deactivation occupancy rule

After the Resource lock has been acquired, obtain fresh authoritative operation time and execute a **separate subsequent occupancy query** equivalent to:

`resource_id = target AND state = 'confirmed' AND end_at > operation_time`

“Non-cancelled” means Confirmed under the existing two-state model; completed/Past is a derived label, not another stored state. Apply the rule across all owners and all dates, not just a selected schedule page/day or the actor's Bookings. Do not restrict it to `start_at > now` or to currently running Bookings. An existence query is sufficient; a count/list of blockers is not a new product requirement.

- An Upcoming Confirmed Booking blocks.
- An In use Confirmed Booking blocks, including at its exact start.
- A Confirmed Booking blocks one canonical microsecond before its end, but not exactly at or after end.
- A Cancelled Booking never blocks, including when its retained `end_at` is in the future.
- Any one blocking Booking rejects; expired/Cancelled rows cannot mask another blocker. Other Resources' Bookings do not block this Resource.
- With no blocker, deactivation is allowed. No arbitrary waiting period, business hours, maintenance state, mandatory reason, or requirement to delete/clear history is approved.

Return an explanatory blocked-occupancy domain outcome, not invalid input, generic failure, or Booking overlap conflict. It must support later safe wording that upcoming Bookings need authorized cancellation or must finish, and in-use Bookings must finish. Task 4B does not implement that HTTP/browser presentation or permit a Coordinator to cancel another owner's Booking.

## Transactions, authoritative time, and Activity

### Atomic operation and retained data

The service owns one explicit READ COMMITTED transaction. Every SQL collaborator uses that transaction/connection; no independent event commit or after-commit append. The Resource lock remains held until commit/rollback. Authorized no-op releases the transaction without mutation, using current repository conventions; if a chosen no-op finalization fails, return operational failure rather than a false confirmed success.

For a changed state, write **only** `active` and `updated_at`. Preserve Resource ID, code (including casing), name, description (including raw legacy whitespace), and `created_at`. Do not pass canonicalized descriptive snapshots back as updates. Preserve all Booking IDs, resource/owner relationships, request identifiers, intervals, purposes, creation instants, stored states and cancellation metadata. State operations never UPDATE/DELETE a Booking or prior Activity, and do not synthesize cancellation or descriptive-edit events.

Capture one operation timestamp from the existing injected authoritative Go clock after required lock waits, normalized with the accepted UTC microsecond precision (`UTC().Truncate(time.Microsecond)`). Use it for both eligibility and successful `updated_at`/Activity `occurred_at`. Do not use request-arrival time, transaction-start PostgreSQL `now()`, browser time, or an earlier failed attempt's time. No strict timestamp increase beyond that clock/precision is required; clock equality does not make a real state change a no-op.

The service must establish Resource ownership of the lock **before** sampling time. A guarded capability may reacquire the same lock in the same transaction without a competing Resource lock wait. If a compliant design adds another eligibility-relevant lock wait, refresh time after it before final validation/mutation. Do not precompute occupancy/time before a privileged call that can first wait for Resource. An arbitrary SQL caller's supplied account ID or timestamp is not independent session/clock authentication; the trusted application must supply both correctly. This preserves the existing trust model rather than promising arbitrary-runtime-SQL impersonation resistance.

READ COMMITTED must see a preceding lock holder's committed Booking/cancellation in a separate statement after lock acquisition. Do not combine lock acquisition and occupancy into one stale statement snapshot, materialize a pre-lock occupancy result, or use an enclosing stale snapshot to decide eligibility. No Booking row lock is necessary merely to read occupancy under the shared Resource protocol.

### Required Activity contract

Activation and deactivation have distinct stable action identities consistent with existing dotted actions; precise strings/detail keys are Level 1 choices to document and test, not new approved IDs. Each actual transition records exactly one event containing:

- trusted Coordinator actor account ID;
- the appropriate activation or deactivation action;
- affected Resource ID, with Booking target null;
- authoritative UTC microsecond operation time, equal to the new `updated_at`;
- relevant actual locked before/after active values (`false → true` or `true → false`). Additional identity snapshot values may be included only if accurate, non-sensitive, and useful; no new requirement for unrelated text snapshots.

Derive action/target/details from authoritative state and the permitted operation, not caller-authored Activity payloads or an option to skip history. Preserve earlier event IDs/times/actors/details. No credentials/session secrets in events. Seed bootstrap exemption remains unchanged; do not use seed/backfill as an unaudited state-mutation route.

An Activity insertion error must abort the state change. A mutation error or cancellation before commit, or a definite commit rejection, must leave neither partial change nor successful-change event. Cancellation/transport loss around commit may instead leave an unknown outcome as specified below. Return operational failure and reliably release/rollback transactions using finite contexts/timeouts and existing cleanup conventions. Do not catch an Activity error and still commit. There is no asynchronous queue, event-sourcing rewrite, or second transaction.

## Concurrency and global lock order

Preserve architecture 001 §§7, 10 and architecture 002 §4:

- **Booking creation only:** Account `FOR NO KEY UPDATE` → separate retained owner/request lookup → Resource `FOR UPDATE` only for a new request → fresh time/validation → Booking/event.
- **Owner cancellation:** unlocked immutable Resource-ID/owner lookup → Resource `FOR UPDATE` → Booking lock/re-read → ownership/state decision → fresh time for a Confirmed transition → cancellation/event.
- **Resource state operation:** stored-role resolution without Account coordination locking → Resource `FOR UPDATE` → authoritative state/no-op → fresh time and separate occupancy query when deactivating → mutation/event.
- Resource descriptive editing still uses the same Resource lock. No state operation locks multiple Resources or takes an Account coordination lock after Resource. Ordinary account role reads and FK `KEY SHARE` must remain compatible with creator Account `FOR NO KEY UPDATE`; never upgrade creation's Account lock to `FOR UPDATE`.

| Independently ordered operation | Required outcome for the waiter / final state |
| --- | --- |
| New Booking creation commits before deactivation acquires Resource | At post-lock check, if that Booking is still Confirmed and `end_at > fresh now`, deactivation returns blocked; Resource remains Active, Booking/event retained. If time has reached its end, it no longer blocks. |
| Deactivation commits before a new Booking creation acquires Resource | Creator reads Inactive and returns its existing inactive-resource rejection; no new Booking/event or consumed successful request identity. Deactivation/event retained. This is for a new request, not retained creation replay. |
| Owner cancellation commits before deactivation acquires Resource | Separate occupancy query observes Cancelled and permits deactivation if no other Confirmed future-ending row exists. Keep cancellation metadata/event unchanged. |
| Deactivation checks before a cancellation can commit, with an Upcoming/In use Confirmed blocker at its fresh time | Deactivation rejects without mutation/event. It releases its transaction/lock; owner cancellation then independently rechecks its strict start boundary. If still future, it may cancel; after rejection, a separately resubmitted deactivation may succeed. Do not wait for cancellation while holding Resource or promise automatic deactivation. |
| Deactivation waits while time crosses a Booking end | Use fresh post-wait time; at/exceeding end permits deactivation if no other blocker remains. At one microsecond before end still reject. No stale request-time rejection. |
| Lock wait crosses cancellation start or new-Booking lead boundary | Existing services retain fresh-time validation and their existing outcomes; state operations must not undermine those checks or create Account/Resource cycles. |
| Concurrent repeated desired Active against Inactive, or repeated desired Inactive against eligible Active | Serialize; one transition/event commits, later authorized operation reads matching state and returns genuine no-op. No duplicate UPDATE/timestamp/event. |
| Concurrent activation and deactivation | Each evaluates its desired state in actual Resource-lock order. If it already matches, no-op; otherwise it transitions only when its own rules hold. E.g. from Inactive with no blockers: activation first then deactivation yields two real opposite transitions/events and final Inactive; deactivation no-op first then activation yields one event and final Active. Do not impose arrival order, last-request-wins, or collapse actual opposite changes. |
| Concurrent descriptive edit and state operation | Serialize on Resource; each preserves the other's permitted fields. State events use actual state and edit events retain correct descriptive snapshots; no lost descriptive change or unguarded state write. |
| Leading mutation, Activity insert, or definite commit fails while another operation waits | Roll back leader entirely; waiter reads committed state anew, obtains fresh time, and may transition, no-op, or reject as that state/time dictates. Never treat uncommitted/failed values as durable. |

If cancellation rolls back, its Confirmed row still blocks while `end_at > fresh now`; if Booking creation rolls back, it contributes no occupancy and its request identity is not consumed. If deactivation rolls back, a waiting valid creator may book the still-Active Resource. Required tests must force the named orderings; merely releasing two unordered waiters is insufficient proof of both outcomes.

## Authorization and migration-managed privilege boundary

Inspect and preserve Task 4A's effective permissions, not just its Go service checks. Migration 005 presently exposes only guarded create/descriptive-edit operations, owned by non-superuser `labreserve_migrator`, `search_path = pg_catalog`, schema-qualified SQL, revoked PUBLIC EXECUTE, and intended runtime EXECUTE. It does not provide an existing-Resource state mutator.

Additional **narrow state-transition capability** is required. Choose an operation-specific mechanism or equivalently restricted design at Level 1, with all these properties:

- No direct runtime Resource UPDATE, including `active`/`updated_at` column grants; no direct INSERT/DELETE/TRUNCATE. Do not relax existing Resource-denial tests to accept an unaudited state write.
- Runtime cannot commit a Resource state mutation by calling a new exposed capability and omitting the required Activity. Couple the permitted mutation and mandatory accurate event at the guarded database boundary, within the service's outer transaction. No exposed raw state setter or generic privileged SQL/column updater.
- Recheck actor existence/stored Coordinator role, authoritative locked current state, no-op suppression, and deactivation occupancy at the final capability boundary. Direct invocation must not bypass those guards by supplying a stale current state, precomputed occupancy, forged old/new event details, or a skip-event option. The application remains responsible for trusted identity and fresh clock input; database checks are defense in depth, not a new identity provider.
- The capability acquires/reuses Resource `FOR UPDATE`; it must not rely solely on a prior caller assertion that locking or occupancy was done. Its occupancy query is subsequent to the Resource lock. The service's post-lock clock protocol must compose correctly with this boundary.
- Only `active`/`updated_at` of the identified existing Resource may change. No authority to mutate code, identity, descriptive fields, creation time, Bookings, existing events, Account roles, or unrelated records. Task 4A create/edit behavior and grants remain intact.
- If using SECURITY DEFINER: migration-controlled non-superuser ownership; explicit schema qualification; fixed safe search path excluding caller-writable schemas; revoked PUBLIC EXECUTE; only necessary runtime EXECUTE without grant option; no runtime function replacement/alteration/owner-role assumption; no unnecessary dynamic SQL. Ownership/ACL changes commit with migration. Minimize privileges within the existing role model and document the choice.
- Preserve existing Activity SELECT/INSERT needed for Booking behavior; do not expand Activity UPDATE/DELETE/TRUNCATE or claim standalone-event forgery/admin tampering protection. Preserve existing Booking and session/read privileges and lock helpers.

A single desired-state guarded operation or separate guarded activation/deactivation operations can comply; neither is mandated. API/package/helper shape, safe SQL decomposition, and event encoding are engineering choices. Guarded mutation/event coupling, final state/occupancy checks, and denied direct Resource writes are not optional choices.

Architecture 003 accepts a specific development-only direct-SQL bypass for Booking cancellation columns. It does not approve an eventless Resource state mutation or weaken Coordinator authorization, shared locking, or Activity atomicity. Hardening cancellation or redesigning all Activity grants is outside Task 4B. If a compliant design actually needs a new material risk exception or architecture change, stop for a precise Level 2 decision; do not label it Level 1 convenience.

## Domain outcomes and failure behavior

Follow existing Resource service conventions (`ErrUnauthenticated`, `ErrForbidden`, `ErrInvalidInput`, `ErrUnknownResource`, `ErrOperational`, returned Resource and explicit changed/no-op indication). Add only the necessary distinct occupancy-blocked outcome; its exact Go name/type is Level 1. Do not embed HTTP status codes, SQL details, or a public API envelope in this service contract.

| Situation | Required domain behavior | Durable effect |
| --- | --- | --- |
| Authorized Inactive → Active | Changed activation success; return retained Resource identity/current values with Active. | One state update plus one activation event. |
| Authorized eligible Active → Inactive | Changed deactivation success; return retained Resource identity/current values with Inactive. | One state update plus one deactivation event. |
| Authorized desired-state equality | Explicit unchanged success, distinguishable from mutation. | No UPDATE, timestamp rewrite, or event. |
| Confirmed future-ending occupancy on an actual deactivation | Distinct explanatory blocked outcome. | Resource, Bookings, timestamps, history unchanged. |
| Well-formed absent Resource ID after authorization | Unknown Resource, not no-op success. | None. |
| Missing/malformed/unknown actor, existing non-Coordinator, malformed Resource ID | Unauthenticated, forbidden, invalid-input respectively, using authorization precedence above. | None, including no unauthorized no-op success. |
| Begin/role-read/lock/occupancy-query/mutation failure, timeout/cancellation, unexpected database rejection | Safe operational failure; do not translate generic database failure into occupancy, unknown, or successful no-op. | Rollback/no successful partial business mutation when failure is definite. |
| Required Activity insertion failure | Operational failure; rollback mutation and event together. | Original state/time and all records/history retained. |
| Definite transaction/COMMIT rejection | Operational failure; a mutation statement returning success is not committed success. | Neither state change nor event persists; waiting operations re-evaluate. |
| Lost/ambiguous commit acknowledgement | Operational result with **unknown durable outcome**; no confirmed-success result and no assertion of definite rollback. No blind automatic retry. | Either both Resource/event committed or neither; inspect durable state/history through trusted internal reads. A later explicit desired-state submission evaluates current state, time, and authorization. |

Desired-state repetition is state-based harmlessness, not a new booking-style idempotency protocol. If an opposite transition intervenes, a later submission of the original desired state may correctly cause another real transition/event. Do not promise exact-once identity across intervening transitions, introduce a Resource request key/table, or claim a no-op proves which earlier caller committed. Preserve current safe operational-error wrapping; a new externally visible commit-status protocol is not required.

## Migration and lifecycle compatibility

Use the existing embedded SQL migration runner, per-migration transaction plus ledger entry, SHA-256 checksums, and single-runner advisory lock. At the observed baseline, **006 is the next unused numeric prefix**, not a reserved filename or preselected schema. An authorized implementer must recheck collisions at its base and add the next available additive migration for the minimal guarded state capability. Exact filename/function layout is Level 1; no schema rewrite, new state table, or speculative infrastructure is prescribed.

- Preserve applied migrations 001–005 byte-for-byte, including the merged 005 checksum above. Do not modify/rechecksum migration history or silently rewrite retained data to fit a helper.
- Update `requiredSchemaVersion` and affected verification expectations consistently. Existing `scripts/verify.sh` has five-migration assertions; `TestResourceCapabilityMigrationPreservesVersion004Data` currently partitions the ladder using the latest required version. Adapt such assumptions to preserve the 004 → 005 test and add 005 → new-capability coverage, not remove earlier upgrade evidence.
- Prove both fresh installation and additive upgrade from the actual merged 005 schema/ledger with existing Resource states/text/timestamps, Confirmed/Cancelled Bookings/request identities, Resource/Booking Activity, accounts, and sessions. Preserve identities, metadata, code index, foreign keys, and effective existing capabilities. No synthetic backfill Activity or normalization.
- Explicit migration precedes startup/seed with the new binary. Startup refuses missing, unknown, or mismatched ledger entries; it does not auto-migrate. Repeat explicit migration is safe; capabilities/grants and their ledger entry commit together.
- Older binaries' exact-ledger checks reject the new ledger. Do not use an older binary as an assumed schema rollback. Document non-destructive recovery/roll-forward implications; no reset or automatic down-migration is authorized.
- Repeated seeding must preserve Coordinator-changed Active/Inactive state, Bookings, and events, without duplicate seed records/events. Actual PostgreSQL/application restarts must retain state/event pairs and Booking replay/cancellation history.

### Migration 005 persistent-ledger compatibility precondition

The Task 4A repair handoff records that 005 was corrected before merge on evidence of disposable-only applications, with a warning about any non-disposable database that had applied an earlier checksum. The merged history confirms 005 changed between `8fe2244` and `e5b3c73`; it is now applied-history source, not editable task material.

Before touching any persistent target, verify its recorded 001–005 versions/checksums and migration provenance against the accepted merged files. A target with no 005 can follow normal supported additive migration from compatible earlier history; a target with 005 matching the merged checksum can receive the subsequent migration. If a persistent target contains an earlier/different 005 checksum, or evidence shows the earlier 005 was applied persistently, **stop that upgrade**. Do not rechecksum/alter its ledger, replay 005 over it, reset its database/volume, or pretend a new 006 alone bypasses the runner's checksum rejection. Request an authorized, non-destructive compatibility/correction procedure, including the runner/ledger compatibility decision as necessary. This draft does not choose that procedure or approve destructive reconciliation.

This is a conditional environment/data-integrity prerequisite, not a presently established source contradiction. No affected persistent target was found or certified in this documentation session. Independent in-scope disposable verification can continue without prejudging a persistent-target remediation decision.

## Requirement and acceptance traceability

Source locations below are exact PRD identifiers and line ranges at the merged baseline; split rows deliberately classify different facets of broad requirements. No invented requirement or acceptance IDs are used. Verification labels V1–V9 below are local test groups, not product IDs.

- **CURRENT_TASK:** new internal state behavior or integrity constraint implemented/proved here.
- **ALREADY_ACCEPTED:** prior accepted behavior preserved and regression-checked, not reimplemented.
- **EXPLICITLY_DEFERRED:** named later adapter/workflow responsibility; still a product obligation.
- **OUTSIDE_SELECTED_SCOPE:** unrelated new feature work, not authority to implement.
- **MISSING_OR_CONFLICTING:** a genuine gap/contradiction; none identified for the bounded source contract.

### Functional and non-functional requirements

| Source location in `docs/prd.md` | Disposition | Contract / required evidence |
| --- | --- | --- |
| FR-001–FR-002 (lines 105–106); FR-023 state authorization (142); NFR-002 server enforcement (171) | CURRENT_TASK | Trusted Coordinator authorization before transition/no-op, Engineer/unknown-actor rejection, final capability checks; V1, V8. |
| FR-003–FR-004 (107–108), FR-006 (113); NFR-004 (173) | ALREADY_ACCEPTED | Session logout, fictional seeds, credential safety unchanged; V9. Account administration remains OUTSIDE_SELECTED_SCOPE. |
| FR-005 (112), FR-023 state/retention facets (142) | CURRENT_TASK | Existing active-state mutation; immutable Resource identity/code/text/creation time and retained relationships; V2–V3, V7–V8. |
| FR-007–FR-008 (114–115), FR-017–FR-018 (130–131) | ALREADY_ACCEPTED | Browse/state distinction, schedules/details/My Bookings visibility, derived labels and retained stable 25-record pages remain correct; V7, V9. |
| FR-009 active eligibility (119) | CURRENT_TASK | Activation/deactivation integrates with accepted new-Booking service, including inactive precedence; V2, V4, V7. |
| FR-009 remaining creation fields (119), FR-010–FR-015 (120–125), FR-016 (129) | ALREADY_ACCEPTED | Normal validation, two states, exclusion/concurrent conflict, canonical per-owner replay/reuse and restart identity remain unchanged; V4–V5, V7, V9. |
| FR-019–FR-022 owner-only facets (135–138) | ALREADY_ACCEPTED | Strict future cancellation, ownership before Cancelled no-op, metadata/replay retention and interval release; V5, V7, V9. |
| FR-019–FR-020 cross-owner/reason facets (135–136), FR-022 cross-owner retry facet (138); FR-028 inspection (150) | EXPLICITLY_DEFERRED | Separate Coordinator-intervention/history deliveries outside Delivery 4; no implied bypass to clear deactivation occupancy. |
| FR-023 creation/editing/uniqueness facets (142); architecture 004 D4-001–D4-003 | ALREADY_ACCEPTED | Task 4A guarded create/edit, canonical no-op/immutable code, both initial states and single creation event preserved; V7–V9. |
| FR-024–FR-025 (143–144) | CURRENT_TASK | Exact all-owner Confirmed future-end guard, no auto-cancellation, fresh lock/time/query protocol and both race orders; V3–V5. |
| FR-026–FR-027 state facets (148–149) | CURRENT_TASK | Correct single state event, no reject/no-op event/UPDATE, atomic rollback/definite rejection; V2, V6. |
| FR-026–FR-027 existing creation/edit/cancellation/seed facets (148–149); FR-028 no edit/delete (150) | ALREADY_ACCEPTED | Prior events/atomicity and bootstrap exemption retained; no history mutation authority; V7–V9. |
| FR-023 browser management (142), FR-029 Resource workflow (151); NFR-002 Resource CSRF (171); NFR-009 (178) | EXPLICITLY_DEFERRED | Task 4C: navigation/forms/routes, direct HTTP authorization, CSRF/Origin, safe blocked messages/redirects and browser lifecycle. Service outcomes must support them, but no adapter work here. |
| FR-029 existing Booking errors (151); NFR-001 display (170), NFR-003 (172) | ALREADY_ACCEPTED | Preserve safe errors, fixed Jakarta interpretation/display, plain-text escaping; state operation retains text; V7, V9. |
| NFR-001 operation-time persistence (170), NFR-007–NFR-008 (176–177) | CURRENT_TASK | UTC microsecond post-lock time; retained state/event pairs and Booking data; additive upgrade, exact ledger, seed/restart safety; V3–V4, V6–V9. |
| NFR-005–NFR-006 (174–175) | ALREADY_ACCEPTED | Local isolated stack, no new external service/credentials; V9. |
| NFR-010–NFR-011 (179–180) | CURRENT_TASK | Controlled clocks, independent real PostgreSQL, single verification gate, evidence and separate-session review; V1–V9/completion. |
| “Non-Goals / Out of Scope” (lines 48–61); “Further Notes” CR-01/CR-02 (lines 419–420) | OUTSIDE_SELECTED_SCOPE | Deletion, early release/rescheduling/maintenance, infrastructure and other excluded features are not authorized. |

PRD “Data and API Contracts → Operation contracts → Manage resource” (line 234), “Interval and concurrency invariants” (lines 238–242), “Testing Decisions” (lines 258–273), and “Delivery 4 — Coordinator Resource management” (lines 362–386) corroborate these assignments: state service/database evidence is current, browser management is 4C, Activity browsing and cross-owner intervention remain outside Delivery 4. PRD User Stories 24–26 and 30 (lines 88–90, 94) describe the same current behavior; story 23 is accepted 4A plus deferred 4C, story 29's browsing is a separate delivery. Baseline §3 FR-06/FR-07, §5 AC-09/AC-10/AC-12, and §13 supply the original corresponding rules; baseline §12 constrains cancellation.

### Acceptance criteria

| Exact PRD source | Disposition | Task 4B evidence / remaining boundary |
| --- | --- | --- |
| AC-001–AC-008 (lines 280–287) | ALREADY_ACCEPTED | Booking creation/overlap/adjacency/different Resource/replay/owner cancellation/browser restrictions remain regression obligations; V4–V5, V7, V9. |
| AC-009–AC-010 (288–289) | CURRENT_TASK | Block Upcoming/In use; both independently controlled Booking/deactivation orderings; V3–V4. Explanation is a domain outcome here; new browser presentation is EXPLICITLY_DEFERRED to 4C. |
| AC-011 (290), AC-014–AC-015 (293–294), AC-017–AC-020 (296–299) | ALREADY_ACCEPTED | Jakarta/content/session/actor/validation behavior unchanged; V1, V7, V9. |
| AC-012–AC-013 state-specific portions (291–292) | CURRENT_TASK | State Activity failure rollback and actual restart durability; V6, V9. Existing mutation coverage is ALREADY_ACCEPTED and retained. |
| AC-016 state-service restriction (295) | CURRENT_TASK | Engineer cannot activate/deactivate, including a current-state no-op; V1, V8. |
| AC-016 create/edit restrictions (295) | ALREADY_ACCEPTED | Preserve Task 4A authorization/privileges; V9. |
| AC-016 management direct-HTTP / Activity inspection portions (295) | EXPLICITLY_DEFERRED | Resource direct HTTP acceptance belongs to 4C; history inspection to separate delivery, not claimed here. |
| AC-021 (300) | CURRENT_TASK | New bookings after actual deactivation reject inactive, after activation obey normal eligibility; V2, V4, V7. Existing inactive error presentation remains ALREADY_ACCEPTED. |
| AC-022–AC-023 (301–302), AC-025–AC-027 (304–306) | ALREADY_ACCEPTED | Changed-key/per-user identity, strict cancellation boundary, derived states/retained owner-scoped pagination; V5, V7, V9. |
| AC-024 (303) | EXPLICITLY_DEFERRED | Cross-owner Coordinator cancellation/reason workflow in a separately approved delivery, not a Task 4B dependency. |
| AC-028 state/identity/event facets (307) | CURRENT_TASK | Both transitions, actor/action/Resource/time/old-new state, identity/text/history retention; V2–V3, V6–V8. Create/edit is ALREADY_ACCEPTED 4A; new management browser/no-delete presentation is EXPLICITLY_DEFERRED 4C. No deletion implementation is authorized. |
| AC-029 (308) | CURRENT_TASK | Cancelled/completed acceptance; strict `end_at > now` boundary; no Booking cancellation/shortening/deletion; V3, V7. |
| AC-030 state preservation portion (309) | CURRENT_TASK | Repeat seed/start preserves changed state/events/Bookings; V9. Existing explicit reset tooling is ALREADY_ACCEPTED; never reset a persistent target as an upgrade. |
| AC-031 (310) | ALREADY_ACCEPTED | Preserve existing CSRF and text rendering in V9; new management forms/CSRF evidence is EXPLICITLY_DEFERRED 4C. |
| AC-032–AC-040 (311–319) | ALREADY_ACCEPTED | Local operation, durable creation replay/My Bookings/owner cancellation, cancellation failure/concurrency/fresh-time/restart guarantees; V5, V7, V9. |
| AC-041 existing-Resource eligibility facet (320) | CURRENT_TASK | Activate → valid new Booking eligible; deactivate → new Booking inactive rejection; V2, V4, V7. Internal non-seeded creation is ALREADY_ACCEPTED 4A; browser creation/combined lifecycle is EXPLICITLY_DEFERRED 4C. |
| AC-042–AC-043 (321–322) | ALREADY_ACCEPTED | Guarded immutable/case-insensitive codes and descriptive edits/history preserved, including regression after state changes; V7–V9. State operation adds no editing authority. |
| AC-044 activation/deactivation facet (323) | CURRENT_TASK | One matching event per actual state change, rollback for each direction, none for rejects/no-ops; V2, V6. Create/edit evidence is ALREADY_ACCEPTED 4A; combined new browser workflow remains EXPLICITLY_DEFERRED 4C. |

**MISSING_OR_CONFLICTING:** None identified in the bounded source contract. Historical PRD Resource-form questions are resolved by D4-001–D4-003 and do not reopen Task 4A decisions. Persistent-target checksum divergence, if discovered, triggers the conditional decision boundary above; it is not silently resolved by this traceability table.

## Required verification — future implementation only

No implementation test, build, migration execution, or restart was run for this documentation authoring. The following are mandatory acceptance obligations for a later authorized implementation, not claims of current coverage.

### Infrastructure and evidence discipline

Reuse Go service/unit tests and `internal/database/*_integration_test.go`, actual PostgreSQL 17.7/pgx infrastructure, and existing verification lifecycle. `LABRESERVE_TEST_DATABASE_URL` is the migration/fixture pool; `LABRESERVE_TEST_APP_DATABASE_URL` is the runtime pool. Respect `testPool`'s disposable `labreserve_test` guard and the separate verification Compose project. Normal commands/capabilities must execute through the actual `labreserve_app` login, never a migrator approximation. Migration credentials are for migrations, isolated fixtures/failure triggers, and inspection only.

Concurrency requires independent connections/transactions, controlled clocks, and deterministic barriers/observed PostgreSQL lock waits. Existing `holdBookingResourceLock`, `holdBookingAccountLock`, `waitForBlockedBookingCalls`, and Resource clock/Activity-table-lock fixtures illustrate available seams. A shared enclosing transaction, process-local mutex, fake repository, or arbitrary sleep is not evidence of these guarantees. Fault triggers must be narrowly scoped to the test actor/Resource/action and reliably removed. No production time/fault endpoint or demo reset.

For every rejection/no-op, inspect persisted Resource fields/timestamps, Booking snapshots, and Activity counts/details as relevant, not just errors. Required cases:

### V1 — Trusted authorization and domain boundary

- Real stored Coordinator activates/deactivates; both Engineers are forbidden for either direction and both matching-state submissions.
- Absent/malformed/unknown actor cannot transition or receive unauthorized no-op. No command role/actor override can confer authority. Exercise the existing trusted session-resolution-to-Resource-service seam without adding Resource HTTP endpoints; preserve logout/current-role behavior.
- Verify authorization-before-target/no-op, valid unknown Resource outcome, malformed Resource ID invalid input, and database/role-lookup failure operational outcome without mutation/event. Direct new capability calls with Engineer/unknown/null actors fail; do not misdescribe a supplied known Coordinator ID as independently authenticated by PostgreSQL.

### V2 — Activation, deactivation, and genuine state no-ops

- Inactive → Active and eligible Active → Inactive each return changed success and correct retained Resource state. Active → Active and Inactive → Inactive return unchanged success with every field/time/event count preserved.
- Prove **no Resource UPDATE** for each no-op with a scoped UPDATE detector/failure trigger or equivalent database observation; timestamp equality alone is insufficient. No-op succeeds with a state-event insertion-failure fixture installed because no event is attempted. Verify Inactive no-op is not rejected by future occupancy in an isolated retained-data fixture, and unauthorized no-op still fails.
- After activation, an otherwise-valid Engineer creation through Task 2B succeeds. After deactivation, a fresh request returns `bookings.ErrInactiveResource` before overlap probing, creates no Booking/event, and does not consume a successful request identity. Existing creation replay is distinct and preserved.
- Distinct opposite transitions are real changes even if a controlled clock returns the same microsecond; they each require their corresponding event, while matching desired state never does.

### V3 — All-owner occupancy and controlled end boundaries

- Future Confirmed and ongoing Confirmed independently block actual deactivation; include another owner's Booking and exact start. Safe blocked outcome, Active state/timestamps/Bookings/history unchanged.
- No Bookings, only Cancelled future-ending Bookings, only completed Confirmed Bookings, and mixed nonblocking records allow deactivation. Another Resource's future Booking does not block. A single future-ending Confirmed row among nonblocking rows still rejects.
- For a Confirmed row, control `now = end_at - 1 microsecond`, exactly `end_at`, and `end_at + 1 microsecond`: block, allow, allow. Retain all Booking data/state/metadata and events; do not label completed as a new stored state.
- Hold Resource, start deactivation, observe its lock wait, advance controlled time across end, then release. Its occupancy decision and successful Resource/event timestamps must use post-lock time; assert no clock read used for eligibility before release. Include a still-before-end wait case that correctly rejects.

### V4 — Booking creation/deactivation race orderings

- Force a valid new Booking service operation to hold Resource and commit Booking/event before waiting deactivation proceeds. Separate post-lock occupancy sees it and returns blocked while still future-ending; exactly the Booking event, no deactivation event.
- Force eligible deactivation to hold Resource and commit state/event before waiting new creation proceeds. Creator returns inactive, no Booking/event/request binding; exactly one deactivation event. Verify no inactive Resource with an incompatibly newly accepted future Booking.
- Keep new-Booking horizon valid in these tests unless intentionally testing fresh time. Separately exercise a creator waiting on Resource while time crosses a lead-time boundary; retain Task 2B fresh-time behavior.
- Include a creator on the same Coordinator Account while Resource state history commits: observe Account `FOR NO KEY UPDATE` → Resource waiting and prove Resource event FK/role handling completes without a Resource → Account cycle. Retained-request replay/reuse still occurs before Resource locking/eligibility, including after deactivation.

### V5 — Cancellation, state submission, edit, and rollback races

- Force owner cancellation commit before deactivation: Cancelled row ignored, deactivation succeeds if no other blocker; cancellation/creation history retained. Include another blocker so cancellation of one row cannot incorrectly permit deactivation.
- Force deactivation to check the still-Confirmed future-ending row before cancellation can commit: deactivation rejects, then an otherwise-eligible owner cancellation succeeds after lock release; deactivation is not automatically retried. Re-submit desired Inactive explicitly and verify permitted transition. Preserve owner-only cancellation and exact/fresh start boundary, including a wait crossing start.
- Race repeated desired-state submissions independently in **each** direction: one transition/event, waiter no-op with no UPDATE/time rewrite. Force both lock orders for opposing desired states and assert actual serial outcomes/event before-after sequence from both initial states. Do not infer an arrival-order guarantee.
- Serialize a descriptive edit with a state operation; retain correct descriptive values, immutable code/creation time, final active state, and one accurate event per actual permitted change.
- Force leading Resource mutation failure and Activity insertion failure with a waiting state command, separately, proving rollback and fresh waiter decisions. Also cover failed Booking creation → waiting deactivation sees no new occupancy; failed deactivation → waiting valid creation sees Active; failed cancellation → waiting deactivation remains blocked if the Booking still ends in the future. Inspect exact winning/losing event counts.

### V6 — Activity, operation times, transaction rejection, and uncertainty

- For each transition direction, verify exactly one correct action, actor, Resource target/no Booking target, locked before/after active values, and event time equal to `updated_at`; `created_at` and prior history unchanged. Hold Resource and advance clock to prove timestamp freshness for activation as well as deactivation. Include PostgreSQL microsecond precision.
- Inject required Activity insertion failure for activation and deactivation after the state mutation would otherwise occur: entire state/time update rolls back, no new success event, Bookings/prior events unchanged. Remove failure and explicitly retry; one actual transition/event commits.
- Invoke the guarded capability inside an outer runtime transaction then roll it back; neither state nor Activity persists. Direct successful capability use cannot omit the mandatory event.
- **Definite COMMIT rejection in both directions:** use a deterministic deferred database rejection or equivalent actual commit seam following `resource_commit_rejection_integration_test.go`. Observe mutation statement success, then actual rejected COMMIT, safe `ErrOperational`, and no durable state/time/event change. An outer rollback or simulated lost acknowledgement alone does not cover this case.
- **Ambiguous acknowledgement:** using a narrow transaction test seam, model successful durable commit followed by lost acknowledgement and a noncommitted failure path. Verify operational/unknown handling, no blind retry or false rollback claim, and both-or-neither persistence. A later explicit desired-state command returns no-op if already matching or performs a freshly eligible transition if not; it does not fabricate a request identity. This is not a requirement to introduce network fault infrastructure.
- Begin/lock/query/mutation failures and context cancellation produce safe operational outcomes, release transactions/locks, and do not fabricate domain conflicts/success. Error text must not leak SQL/trigger diagnostics, tokens, or credentials.

### V7 — Retained identity, text, Booking/replay, and read compatibility

- Compare all Resource non-state fields and complete retained Booking/request/cancellation/history snapshots around activation, allowed deactivation, rejected deactivation, no-op, and rollback. Use retained future/in-use fixtures for blocked transitions and past/Cancelled fixtures for eligible deactivation. Activation with retained occupancy must not rewrite or revalidate those Bookings.
- Include raw legacy edge-whitespace Resource text and HTML-like descriptive content; state changes do not canonicalize or rewrite it. Existing list/detail/schedule/My Bookings reads reflect active state while retaining identity, text safety, and booking history/visibility.
- Create then cancel via accepted services, deactivate eligible Resource, and identically replay original creation: same Cancelled Booking, no restored occupancy/new Booking/creation event. Retained past Confirmed replay while inactive also succeeds. Changed request data still returns reuse before inactive/overlap; same key for different owners remains independent.
- Reactivate and confirm ordinary conflict/adjacency/different-Resource, time/purpose validation, ownership, and cancellation no-op/rebooking behavior remain accepted behavior, not newly bypassed eligibility.

### V8 — Effective runtime permissions and additive migration

- Inspect actual runtime table/column/function/schema/role privileges, including inherited/PUBLIC grants and grant options. Guarded state capability works, but direct Resource INSERT, UPDATE of **every column**, DELETE/TRUNCATE fail. Keep Task 4A capability grants and lock helpers unchanged.
- Direct state capability calls enforce Coordinator, authoritative state/no-op/occupancy, correct event derivation and coupling. They cannot change ID/code/text/creation time, skip history, accept a supplied occupancy verdict, or overwrite existing Activity/Bookings. Apply no-op UPDATE/event detectors at this seam too.
- If using SECURITY DEFINER, verify non-superuser migration-controlled owner, fixed safe path, schema-qualified dependencies, revoked PUBLIC EXECUTE, intended runtime-only EXECUTE without grant options, denied ungranted invocation, denied replacement/alteration/owner-role assumption, and caller-controlled search-path/shadow-object resistance (including occupancy dependencies).
- No new Account writes, broad/immutable Booking writes, Activity UPDATE/DELETE/TRUNCATE, schema/role administration, or new generic elevated helper. Preserve exactly the existing cancellation-column privilege limitation; do not assert those accepted column writes are denied or describe their existence as new Resource permission.
- Fresh migration and upgrade from actual merged 005 with retained records/checksums/capabilities preserve all data/index/FK semantics. Repeat migration is safe; missing, unknown, and mismatched ledger checks still reject. Preserve and adapt earlier 004 → 005 evidence; do not mask migration assumptions by weakening tests. Check the persistent-ledger precondition non-destructively before any real target upgrade; no reset/history rewriting as recovery.

### V9 — Lifecycle, accepted regressions, and required repository gate

- Extend the existing isolated restart fixtures/lifecycle to persist successful activation and deactivation, their exact state/timestamps/events and related past/Cancelled Bookings; include a Coordinator-deactivated seeded Resource so repeat seeding cannot restore Active. Verify after **actual PostgreSQL and application restarts**, through fresh runtime connections/services, not pool reconnection alone.
- Repeated seed/start preserves changed state, identity/text/history, accounts/sessions and request identity; no duplicate bootstrap records/events. After restart, matching state is genuine no-op, valid opposite change still works, privileges remain narrow, and creation/cancellation replay retains original Bookings/events. Keep existing Task 4A creation/edit restart assertions meaningful; use separate fixtures or accurate extensions rather than falsifying earlier expected history.
- Run focused Resource/Booking service and real-PostgreSQL suites first, then the full existing **`make verify`** entry point. Include new state tests and lifecycle evidence in that gate, along with all accepted foundation, Tasks 2A–3C and Task 4A Go/PostgreSQL/HTTP/Playwright verification. Do not silently skip database/browser suites or count an environment-skipped integration invocation as success.
- Preserve authentication/logout/CSRF/Origin/content safety, Jakarta intersections/labels, stable owner-filtered pagination, final exclusion, Account/replay ordering, canonical equality, cancellation ownership/time/no-op/interval release, code uniqueness/immutability, guarded Resource create/edit/no-op, definite rejection, and non-destructive migration/seed/restart behavior. Existing tests passing alone do not prove the new state invariants.
- Report actual commands, selected/executed suites, concurrency synchronization, observed results, failures/repairs, skips, candidate state, and limitations. New Resource management HTTP/browser tests are **not required or authorized** by Task 4B; existing browser regressions still run.

### Optional implementation-time checks

Additional repeated stress runs, Go race-detector runs, SQL query-plan inspection, and wider fault-injection exploration are Level 1 diagnostic choices when useful. They do not replace V1–V9, impose a new performance target, authorize external dependencies, or require new production observability/infrastructure. Formatting/compilation checks for changed code and complete final diff inspection remain ordinary required implementation hygiene through repository workflow, not optional acceptance substitutes.

## Explicit exclusions

Do not implement or scaffold:

- **Task 4C:** Resource-management HTTP routes/handlers, browser navigation/forms/buttons, CSRF/Origin adapters, response/status mapping/redirects, or new management Playwright workflows/placeholders. It will expose the accepted 4A/4B services later, not duplicate their rules.
- Automatic cancellation, shortening, deletion, or rewriting of any Booking during deactivation; Coordinator cross-owner cancellation/reason forms; changes to accepted owner cancellation.
- Activity-history browsing/query API/UI, Activity editing/deletion, or an audit-hardening redesign.
- Resource deletion, ID/code mutation, text-validation changes, descriptive-edit extensions, bulk normalization/backfill, or a new resource-creation/retry protocol.
- New Booking states, changed Booking horizon/duration/purpose/replay/overlap/visibility semantics, early release, rescheduling, maintenance windows, or speculative future modules/schemas/APIs.
- Cancellation privilege hardening, new database trust exceptions, new external infrastructure/dependencies/services, production deployment/integration, or credential changes.

Permitted future changes are limited to the Resource state service and narrow transaction/persistence capability, minimal additive migration/schema-expectation updates, focused tests and existing verification lifecycle integration, accurate README/implementation notes, and small necessary helper refactoring that preserves accepted behavior. Existing Booking/HTTP/browser calls in tests are integration/regression evidence, not authority to expand those features.

## Level 0/1/2 authority and unresolved matters

Only after separate human approval/commit and later authorized execution gates:

- **Level 0:** Routine in-scope implementation, compilation/formatting, deterministic fixtures, debugging, focused/full verification and repair; no repeated permission requests between ordinary steps.
- **Level 1:** Existing-service extension versus separate narrow state service; separate activation/deactivation versus explicit desired-state method; result/error naming consistent with Resource conventions; guarded capability decomposition or equally restrictive mechanism; stable event action/detail encoding; safe additive migration filename/ownership within the existing model; deterministic lock/fault/transaction seams and minimal shared refactoring. Document meaningful choices and prove the required properties.
- **Level 2:** Product-rule changes or actual material ambiguity; authorization/trusted actor or database-trust-model change; lock/isolation/exclusion/replay redesign; direct unaudited Resource writes or new Activity-integrity/security risk; destructive reconciliation/persistent-ledger rewriting; unapproved infrastructure/dependencies; scope expansion into 4C/cross-owner/history or other excluded features. Stop dependent work and report category, problem, exact evidence, alternatives/tradeoffs, recommendation, precise authorization needed, and blocked versus independent remaining work.

**Current Level 2 questions:** None identified for authoring this bounded contract. Multiple safe capability/API designs are Level 1, not unresolved product decisions. The conditional older-005 persistent-target issue requires evidence-based escalation if encountered: recommend non-destructive authorized compatibility/correction, not reset or checksum substitution. No remediation choice is approved here.

## Completion and independent review criteria

Task 4B implementation is complete for independent review only when:

- All seven objective outcomes and the authorization/state/occupancy precedence are implemented and verified, including exact end boundary and genuine no-UPDATE state no-op.
- Shared Resource serialization, fresh post-lock time, separate occupancy statement, Account/replay ordering, owner cancellation, and the required race/rollback outcomes are demonstrated with independent PostgreSQL operations.
- Both transition directions have accurate single atomic Activity events, complete event-failure rollback, definite commit-rejection evidence, safe ambiguous-acknowledgement handling, and no eventless guarded Resource mutation path.
- Runtime least privilege, immutable Resource/text/Booking/history retention, additive migration/005 compatibility, non-destructive seed and actual restart durability meet this contract without a new risk exception.
- V1–V9 and focused plus complete repository verification actually pass; no required database/browser evidence is silently skipped. README accurately distinguishes accepted internal state services from still-deferred 4C/intervention/history workflows and records privilege/upgrade/recovery limitations.
- Complete final tracked/staged/untracked diff is inspected, unrelated changes preserved or accidental new changes removed, and no excluded functionality/scaffolding is introduced.

A future implementation completion report must identify implementation base and exact candidate revision/working-tree state; changed components; service API/outcomes; state/no-op/occupancy semantics; transaction/lock/time/event behavior; migration/checksum/privilege and upgrade/recovery decisions; traceability including partial/deferred acceptance; actual commands/suites/results for V1–V9; significant Level 1 choices; anything unverified, limitations/deviations and unresolved concerns; and final-diff scope confirmation.

That separately authorized implementation reports exactly one of `READY_FOR_REVIEW`, `BLOCKED`, or `INCOMPLETE`, based on actual evidence. Self-check is not independent review. Independent implementation review requires a separate fresh agent session; human final acceptance/integration remains required. This draft itself grants no approval, commit, push, merge, deployment, or automatic progression to another workflow.
