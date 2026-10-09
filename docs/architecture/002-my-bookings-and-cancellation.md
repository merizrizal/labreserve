# 002 — My Bookings and self-service cancellation

- **Status:** Proposed for review; not approved for implementation.
- **Product authority:** [Baseline section 11](../baseline.md) and [PRD](../prd.md), especially FR-008, FR-016–FR-022, FR-026–FR-027 and the proposed Delivery 3 boundary.
- **Architectural authority:** Extends [001 — Initial architecture](001-initial-architecture.md), particularly sections 4–10, without replacing its correctness or security constraints.
- **Context:** The client reports Delivery 2 complete. My Bookings and cancellation are not yet implemented. This is a documentation-only proposal, not an implementation task contract or verification result.

## 1. Scope and architectural impact

The request advances existing v0.1 behavior rather than introducing a new domain model. Add an authenticated own-bookings view and self-service cancellation through the existing Go application, PostgreSQL persistence, and server-rendered browser boundary.

Include the same self-service capabilities for a coordinator's own bookings. Do not implement coordinator cancellation of others' bookings, reason forms, resource management, or activity-history UI in this delivery. These remain later v0.1 obligations. Early release, rescheduling, deletion, filters/search, calendar frameworks, and infrastructure control remain outside this request.

No new service, state-transition worker, request-identity table, or stored Upcoming/In use/Past state is needed. Required cancellation events must be written even though their coordinator inspection screen is deferred.

## 2. Existing seams and records to reuse

| Existing component | Evidence and proposed reuse |
| --- | --- |
| Booking application service | Task 2B provides controlled time, typed outcomes, explicit transactions, creation/replay, and the Account → replay lookup → Resource protocol. Add a narrow cancellation use case without duplicating its rules in HTTP handlers or changing creation locking. |
| PostgreSQL Booking | Migration 002 already stores Confirmed/Cancelled, cancellation actor/time, immutable successful-request data, and owner/request uniqueness across all states. Use those records, not a replacement reservation table. |
| Overlap constraint | The immediate partial GiST exclusion applies only to Confirmed rows. Cancellation releases occupancy by changing state at commit; no separate availability cache or deletion is needed. |
| Activity persistence | Booking creation already appends an event in the caller's transaction. Cancellation likewise appends one event in its own mutation transaction, using the existing event table and target/actor relationships. |
| Resource locking | Migration 003 exposes a narrow resource-row locking function to the runtime role. Reuse the existing Resource `FOR UPDATE` boundary; do not grant Resource UPDATE merely to obtain locks. |
| Web read models | Resource schedules and booking detail already expose resource/owner/interval/purpose/state. Add an owner-scoped list and reuse the derived-label, Jakarta rendering, escaped-text, and pagination conventions. |
| Authentication and browser security | Existing database-backed sessions supply actor identity; middleware and forms provide CSRF/Origin protection. Preserve these boundaries on both the new view and cancellation POST. |
| Verification | Existing Go service/HTTP/PostgreSQL tests and isolated Playwright fixtures use controlled time. Extend them and the existing `make verify` entry point rather than building another stack. |

These are codebase observations, not claims that cancellation or its verification already exists. Delivery 2 task contracts settle canonical request equivalence and Jakarta-day intersections that were still proposed when architecture 001 was written.

## 3. My Bookings query and browser behavior

- Require an authenticated account. Scope the query to that account's stable ID on the server, across resources and all retained booking states/times. Do not accept an owner selector from the browser or apply ownership filtering only after pagination.
- Return booking ID, resource code/name, full original start/end, purpose, and stored state. The browser derives Upcoming, In use, Past, or Cancelled using the existing label semantics and one authoritative time captured per render. Render all times visibly in Asia/Jakarta and all user text through contextual escaping.
- Provide a clearly labelled My Bookings navigation entry, an empty state, and 25-record pages. Proposed implementation default: reuse `start_at ASC, id ASC` with validated page numbers and SQL limit/offset, already used by resource schedules. An alternative stable order may be documented in the approved task; it must preserve the unique tie-breaker and owner scope. No frozen cross-request snapshot is required.
- Show a cancel action only for the authenticated account's own Upcoming Confirmed records. Reuse existing booking detail if a separate view is convenient; exact route/template organization is an implementation choice. State-changing actions use POST with valid CSRF protection, never GET.
- A stale browser page is only a presentation issue: the cancellation service rechecks ownership, stored state, and fresh server time. After success, redirect using the existing POST/redirect/GET convention and show the retained Cancelled record. Never imply that a retry created another cancellation.
- Do not change shared-resource schedule visibility. An account's My Bookings is private to its owner, but authenticated users still see team bookings on schedules/details under the existing policy.

## 4. Cancellation transaction and lock protocol

The service accepts the booking identifier and an actor obtained from the authenticated session. A submitted owner/account/role cannot confer authority. This delivery supports only self-service, including for coordinators; it does not exercise their later cross-owner privilege.

1. Resolve the retained booking's immutable resource ID. An ordinary preliminary read may identify the resource, but cannot substitute for the locked authoritative booking read or authorize a mutation.
2. Begin one explicit READ COMMITTED transaction and acquire the existing Resource `FOR UPDATE` lock.
3. Lock and re-read the Booking row in a subsequent statement. Preserve Resource → Booking order; cancellation must not acquire the booking-creation Account coordination lock after Resource or reverse the resource/booking order.
4. Check that the booking belongs to the authenticated actor, including when its state is already Cancelled. Unknown bookings and forbidden requests produce safe, distinguishable outcomes without mutations or successful-change events.
5. For a Confirmed booking, capture fresh authoritative server time after all required locks. Require `now < start`; at or after start, reject without changing state or availability. Do not use request-arrival time or transaction-start database `now()` after a wait. No active-resource/horizon validation is needed to release an existing booking.
6. Update only state, cancellation actor, and cancellation instant. Retain booking ID, resource, owner, request identifier, original interval/purpose, and creation instant.
7. Append exactly one booking-cancelled event in the same transaction, identifying actor, affected booking, operation time, and relevant resource/original-interval details. No reason is required for self-service. Retain the original creation event.
8. Commit once, then report success. A definite event/commit rejection rolls back all changes. A transport loss around commit is an unknown outcome, not proof of rollback; retry the same cancellation target and rely on its authorized state-based no-op behavior.

The existing foreign-key and lock design must remain compatible with concurrent creation. Cancellation/event insertion must not introduce a Resource → Account coordination-lock cycle; creation retains Account `FOR NO KEY UPDATE`, which remains compatible with account foreign-key `KEY SHARE` locks.

### Repeated cancellation policy gate

Before start, an authorized repeat against a Cancelled booking is a harmless no-op with no additional event. Concurrent authorized requests serialize on Resource/Booking; after the first transition commits, the second observes Cancelled and does not repeat it. If the first rolls back, the waiter re-evaluates the unchanged booking and fresh time.

**Proposed, not yet approved:** apply that same no-op success after the booking's former start. Ownership is checked first; the start-time guard applies only to a real Confirmed → Cancelled transition. Confirm this policy before cancellation implementation. The later question about resubmitting a coordinator intervention reason is outside this self-service delivery.

## 5. Availability, retention, and permissions

Cancellation releases the interval **at commit**, when the row no longer participates in the Confirmed-only overlap constraint. Another engineer can then reserve the interval through the existing creation workflow; the historical Cancelled row can coexist with the new Confirmed booking.

Resource locking serializes cancellation with creation. A creation that obtains the lock before cancellation may correctly receive conflict. A creation after cancellation commit sees the released interval. If cancellation's event fails, the booking remains Confirmed and overlapping creation still conflicts. Do not promise availability before commit or bypass the existing creation checks.

The existing state and cancellation-metadata checks already support this change. No new business-state columns are needed. The runtime role currently has Booking SELECT/INSERT but no UPDATE capability, so a **new migration-managed, narrowly scoped cancellation capability is required**. Proposed default: column-level UPDATE only for `state`, `cancelled_by_account_id`, and `cancelled_at`, sufficient for the Booking lock/update. Do not grant blanket Booking UPDATE, DELETE, edits to immutable request data, new Account/Resource mutation privileges, or activity UPDATE/DELETE. Server-side ownership checks remain mandatory; these database privileges do not themselves authorize a user.

Use the existing explicit checksummed migration mechanism. Do not edit applied migrations 002/003, reseed destructively, or reset the demonstration database to introduce cancellation permissions. Test permitted and forbidden column updates using the real runtime role.

Cancelled and past Bookings retain their request identity until explicit reset. An identical creation replay after cancellation returns the original Cancelled booking, without creating a replacement, restoring occupancy, or adding a creation event. Normal application/database restarts must preserve the booking and both events.

## 6. Required verification evidence

| Seam | Proposed Delivery 3 evidence |
| --- | --- |
| Controlled-clock behavior | Labels before/exactly at start/end; Cancelled always Cancelled; cancellation just before start accepted and exactly at/after start rejected; lock wait crossing start uses fresh time. |
| HTTP authorization/security | Anonymous access blocked; My Bookings ignores tampered owner filters on every page; cross-owner cancellation rejected for Confirmed and Cancelled rows; missing/invalid CSRF and unexpected Origin rejected with no mutation/event; GET never cancels. |
| Real PostgreSQL transactions | Successful cancellation retains original fields and atomically writes metadata plus one event; event failure rolls back and keeps the interval occupied; two independent concurrent cancellations produce one transition/event; cancellation/creation lock ordering and runtime column privileges are exercised. Use deterministic lock coordination, not arbitrary sleeps. |
| Retention and replay | Past/Cancelled own bookings remain accessible across resources; lists longer than 25 preserve documented ordering/tie-breakers and owner scope; cancellation history survives restarts; original creation request still replays the Cancelled booking without another creation/event. |
| Browser workflow | Engineer A navigates to My Bookings, sees statuses/Jakarta times, cancels an upcoming booking, and sees retained Cancelled state. Engineer B's My Bookings excludes A's records, while B sees the released interval in the shared workflow and successfully reserves it. Include empty state, pagination, non-Jakarta browser timezone, and escaped purpose text. |
| Regression entry point | Run existing Delivery 1/2 checks and the new focused suites through `make verify`; report actual candidate revision, checks, outcomes, limitations, and independent review separately. |

Trace new evidence to PRD AC-034–AC-040 and the existing cancellation, label, pagination, atomicity, authentication, CSRF, and timezone criteria. Full-v0.1 coordinator intervention/resource/activity-view acceptance is not claimed by this delivery.

## 7. Review and implementation gates

1. Review and approve the proposed Delivery 3 scope, including deferral of coordinator cross-owner cancellation and activity-history UI without removing them from v0.1.
2. Confirm the already-cancelled retry policy after former start. This is the remaining in-scope product decision; no new time-based stored states or early-release interpretation is implied.
3. Prepare separately approved implementation task contracts consistent with these existing architectural constraints, verification expectations, and completion criteria. This addendum does not create them or authorize code/schema/configuration changes.

No approved architectural constraint needs replacement. The change extends existing queries, application-service behavior, browser adapters, and narrowly scoped runtime permissions; it does not justify redesigning booking creation or broadening the release.
