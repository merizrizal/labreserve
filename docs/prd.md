# LabReserve v0.1 — Product Requirements Document

## Problem Statement

Engineers on a small team share testing and demonstration resources, including a network test bench, a Kubernetes integration lab, and a customer demo environment. Reservations currently happen through chat, making availability unclear and allowing multiple people to believe they hold the same interval. When disagreements occur, the coordinator cannot reliably establish who changed a reservation or resource.

The team needs an internal browser application that prevents resource conflicts, enforces booking ownership, and retains a history of successful changes. Reserving an environment means recording a reservation, not controlling or connecting to infrastructure. Early release is a known problem but is deliberately deferred from v0.1.

## Source Materials

- **LabReserve v0.1 client baseline:** Primary source for actors, booking rules, security and local-operation constraints, acceptance scenarios, recommended technology, deferred change requests, and the initial-delivery boundary.
- **Repository agent instructions:** Establish this PRD as the authoritative product requirements, require one approved implementation task at a time, and require verification, escalation of genuine conflicts, and independent review in a separate agent session.
- **Repository inventory:** Contains the client baseline, repository instructions, license, and agent tooling. No application implementation, schemas, migrations, tests, README, domain glossary, or architecture decisions were found. There is no existing implementation or test seam to reuse.
- **Baseline technical references:** Playwright guidance supports user-visible, isolated browser tests; Go template guidance supports escaped HTML output; PostgreSQL documentation suggests range exclusion as a possible concurrency mechanism; Docker Compose documentation supports a simple local environment. These references are recommendations from the baseline, not independently evaluated architectural decisions.

## Project Mode

**New Project.** LabReserve has a product brief but no application implementation. This PRD specifies intended behavior and contract-level needs. Architecture, persistence mechanisms, endpoint design, and testing infrastructure must be defined before implementation; recommendations below are not evidence that those components exist.

## Solution

Provide a small, locally runnable internal web application with authenticated engineer and coordinator accounts. Engineers browse resources and their schedules, create bookings for themselves, inspect My Bookings, and cancel their own future bookings. Coordinators additionally manage resources, cancel another user's future booking with a reason, and inspect activity history.

The central workflow is:

1. Engineer A books the network lab for a valid interval.
2. Engineer B sees that reservation and cannot reserve an overlapping interval.
3. Engineer A cancels before the reservation starts.
4. Engineer B can now reserve that interval.
5. The coordinator sees who performed each successful change and when.

Schedules use visibly labelled Asia/Jakarta times regardless of the browser timezone. Resource and booking changes remain correct under retries and concurrent requests. The interface is a simple set of forms, lists, and tables, not a full calendar platform.

## Goals

1. Demonstrate the central booking, conflict, cancellation, and history workflow entirely through a browser.
2. Prevent overlapping non-cancelled bookings for the same resource, including concurrent submissions.
3. Enforce authenticated ownership and coordinator permissions on the server.
4. Make booking retries and repeated authorized cancellation safe without duplicate records or events.
5. Keep successful business changes and their required activity events atomic and durable.
6. Provide reproducible local startup, migrations, safe seeding, automated verification, and an explicit demonstration reset.
7. Deliver reviewable increments, starting with the foundation-only delivery specified by the client.

Approximately **20 users and 20 resources** are scope assumptions from the baseline, not a performance target or scalability benchmark.

## Non-Goals / Out of Scope

- Account registration, password recovery, user administration, and production identity integration.
- Recurring bookings, approvals, rescheduling, maintenance windows, or early release of an in-use booking.
- Business-hour restrictions or prevention of a user's simultaneous reservations of different resources.
- Resource deletion, booking deletion, or a product interface for editing/deleting activity history.
- Drag-and-drop calendars, calendar synchronization, email/chat notifications, and attachments.
- Billing, multi-tenancy, native mobile applications, and enterprise service management.
- Actual infrastructure control, Kubernetes connections, OpenStack management access, or external service accounts.
- Microservices, Kubernetes deployment, or a custom agent-orchestration framework.
- Cryptographically tamper-proof auditing or protection against database-administrator tampering.
- Implementing deferred features or speculative abstractions for future flexibility.

Atomic rescheduling and maintenance windows are explicitly deferred change requests, not authorized v0.1 features.

## User Stories

1. As an engineer, I want to sign in with an actual account, so that access and ownership are tied to my authenticated identity.
2. As an authenticated user, I want to sign out and invalidate my session, so that it cannot continue granting access.
3. As an engineer, I want to browse resource codes, names, descriptions, and active states, so that I can select the right environment.
4. As an engineer, I want to view a selected resource's schedule for a date, so that I can plan a reservation.
5. As an engineer, I want to distinguish an inactive resource from a booked active resource, so that I understand why I cannot reserve it.
6. As an engineer, I want booking times labelled in Asia/Jakarta, so that my browser timezone does not change the team's schedule.
7. As an engineer, I want to reserve an active resource for myself with a short purpose, so that teammates know who will use it and why.
8. As an engineer, I want useful feedback about invalid times, durations, or purposes, so that I can correct my request.
9. As an engineer, I want overlapping reservations rejected, so that the same resource is not promised to two people.
10. As an engineer, I want a reservation to start exactly when another ends, so that adjacent intervals remain available.
11. As an engineer, I want to book different resources for the same interval, so that resource conflict rules do not become personal-calendar restrictions.
12. As an engineer, I want to book across midnight without business-hour limits, so that longer tests can use valid intervals.
13. As an engineer, I want simultaneous conflicting submissions resolved correctly, so that a stale schedule cannot cause double booking.
14. As an engineer, I want retrying the same submission to return my existing booking, so that a retry does not create duplicate reservations.
15. As an engineer, I want reuse of a request identifier with different booking data rejected, so that a retry cannot silently become a different reservation.
16. As an authenticated user, I want to see booking owners, times, and purposes, so that I can coordinate with teammates.
17. As an engineer, I want My Bookings, so that I can find my own reservations without searching every resource.
18. As an authenticated user, I want confirmed bookings labelled Upcoming, In use, or Past and cancelled bookings labelled Cancelled, so that I understand their current status.
19. As an authenticated user, I want past and cancelled bookings retained in stable pages, so that I can inspect earlier reservations.
20. As an engineer, I want to cancel my own booking before it starts, so that its interval becomes available to others.
21. As an engineer, I want repeated authorized cancellation to be harmless, so that retrying an action does not duplicate history.
22. As an engineer, I want another engineer prevented from cancelling my booking, so that ownership remains protected even through direct requests.
23. As a coordinator, I want to create resources and edit their names and descriptions, so that the resource catalog remains understandable.
24. As a coordinator, I want to activate and deactivate resources without deleting them, so that availability can change without losing history.
25. As a coordinator, I want deactivation blocked while future or in-use bookings exist, so that accepted reservations are not invalidated.
26. As a coordinator, I want concurrent booking and deactivation handled consistently, so that an inactive resource cannot acquire a newly accepted future booking.
27. As a coordinator, I want to cancel another user's future booking with a reason, so that intervention is accountable.
28. As an authenticated user, I want cancellation rejected once a booking starts, so that the agreed v0.1 cancellation boundary is enforced.
29. As a coordinator, I want to inspect successful resource and booking changes with actor, timestamp, and details, so that I can resolve scheduling disagreements.
30. As a coordinator, I want a business change to fail if its required activity event cannot be recorded, so that history is not silently incomplete.
31. As an authenticated user, I want descriptions and purposes displayed as plain text, so that shared content cannot execute markup or scripts.
32. As a local operator, I want documented startup, migrations, seed data, and tests, so that I can reproduce the demonstration without production access.
33. As a local operator, I want restarts and repeated seeding to preserve committed data, so that normal operation does not erase reservations.
34. As a local operator, I want an explicit demonstration-reset procedure, so that destructive setup is deliberate rather than a startup side effect.
35. As a reviewer, I want a candidate revision, executed checks, observed results, and limitations for each delivery, so that readiness is supported by evidence.

## Functional Requirements

### Authentication and permissions

- **FR-001:** Users must authenticate before accessing any resource or booking information.
- **FR-002:** The server must derive identity and role from the authenticated account, not trust a browser-supplied user ID or role. Engineer and coordinator permissions must be enforced on direct requests as well as UI actions.
- **FR-003:** Signing out must invalidate the session; reuse of that session must not grant authenticated access.
- **FR-004:** The demonstration must seed two engineer accounts and one coordinator account with fictional identities and development-only credentials. No account-registration, recovery, or administration workflow is required.

### Resources and schedule access

- **FR-005:** Each resource must have an immutable unique code, name, description, and active/inactive state.
- **FR-006:** Provide synthetic seed resources for the demonstration. The baseline's example inventory is `NET-01` (Network Test Bench), `K8S-01` (Kubernetes Integration Lab), and `DEMO-01` (Customer Demo Environment); these are examples, not a mandatory fixed catalog.
- **FR-007:** Authenticated users must be able to browse resources and select a resource and date to view its bookings using a list or table. Inactive resources must be visibly distinguishable from active resources with occupied intervals.
- **FR-008:** All authenticated users may view booking owners, times, and purposes. My Bookings must show the authenticated user's own bookings.

### Booking creation

- **FR-009:** An authenticated user must be able to create a booking for themselves on an active resource. The booking must contain resource, owner, start instant, end instant, and purpose.
- **FR-010:** Purpose must contain 1–200 characters after trimming surrounding whitespace. Empty or whitespace-only purposes and over-length purposes must be rejected.
- **FR-011:** For a new booking, start must be at least five minutes and no more than 30 days in the future, using the authoritative server clock. Both limits are inclusive.
- **FR-012:** End must be after start, and duration must be between 30 minutes and eight hours inclusive. Bookings may cross midnight; no business-hour restriction applies. The 30-day horizon limits start, not end.
- **FR-013:** The server must reject overlapping non-cancelled bookings for the same resource. Intervals are start-inclusive and end-exclusive; an end equal to another start is not a conflict. Identical intervals on different resources are allowed.
- **FR-014:** Against an otherwise healthy system, two simultaneous valid conflicting booking submissions must result in exactly one successful booking and a clear conflict response for the other. Browser-side availability checks cannot be the final enforcement mechanism.
- **FR-015:** Booking creation must use a stable request identifier scoped to the authenticated user. Repeating an identifier with the same booking data must return the existing booking's identifier without another booking or creation event. Reusing that identifier with different booking data must be rejected. This guarantee must survive normal restarts and remain valid until explicit demonstration reset.

### Booking views and states

- **FR-016:** The only stored booking business states must be Confirmed and Cancelled, with the transition Confirmed → Cancelled. Past bookings must not require a third stored state.
- **FR-017:** Confirmed bookings must display Upcoming when current time is before start, In use when current time is at or after start but before end, and Past when current time is at or after end. Cancelled bookings must always display Cancelled.
- **FR-018:** Past and cancelled bookings must remain available in booking views. Long booking lists must use pages of 25 records with a documented stable ordering, including a tie-breaker for equal sort values.

### Cancellation

- **FR-019:** An engineer may cancel only their own confirmed booking and only before its start instant. A coordinator may cancel anyone's confirmed booking only before its start instant. Neither role may cancel a confirmed booking at or after start.
- **FR-020:** A coordinator cancelling another user's booking must provide a reason. A reason is not required by the baseline when cancelling their own booking.
- **FR-021:** Successful cancellation must retain the booking record, change its stored state to Cancelled, and immediately release its interval for new reservations.
- **FR-022:** Repeating an authorized cancellation of an already-cancelled booking must be harmless and must not create another cancellation event. Authorization must not be bypassed because the booking is already cancelled.

### Resource management

- **FR-023:** Only coordinators may create resources, edit names/descriptions, or activate/deactivate resources. Codes must remain immutable and resource deletion must not be offered.
- **FR-024:** Deactivation must be rejected with an explanatory error if a resource has any non-cancelled booking whose end is still in the future, including upcoming and in-use bookings. Coordinators must cancel upcoming bookings first and wait for in-use bookings to finish.
- **FR-025:** Concurrent booking creation and resource deactivation must preserve both rules: booking requires an active resource, and deactivation requires no non-cancelled booking ending in the future. The final state must not contain an inactive resource with a newly accepted future booking.

### Activity history and errors

- **FR-026:** Every successful resource or booking change must have an activity event identifying actor, action, affected record, and server timestamp, with relevant details such as cancellation reason or old/new resource name.
- **FR-027:** Each required activity event and its associated business change must succeed or fail together. Rejected requests must not change business records or create events claiming successful changes.
- **FR-028:** Only coordinators may inspect activity history. No product interface may edit or delete activity events.
- **FR-029:** Normal workflows must be available through labelled browser forms and views without SQL or API tools. Rejected bookings must distinguish invalid input, inactive resource, and reservation conflict, without exposing raw database errors.

### Source traceability

The expanded requirement identifiers preserve the baseline's seven functional groups:

| Baseline group | PRD requirements |
| --- | --- |
| FR-01: Sign in and sign out | FR-001–FR-004 |
| FR-02: Browse resources and their schedules | FR-005–FR-008 |
| FR-03: Create a booking | FR-009–FR-015 |
| FR-04: View bookings | FR-008, FR-016–FR-018 |
| FR-05: Cancel a booking | FR-019–FR-022 |
| FR-06: Manage resources | FR-023–FR-025 |
| FR-07: Inspect activity history | FR-026–FR-028 |
| Cross-cutting user experience | FR-029 and non-functional requirements |

## Non-Functional Requirements

- **NFR-001 — Time consistency:** Persist unambiguous instants. Any HTTP interface accepting timestamps must require an explicit offset or an equally unambiguous documented representation. Display all booking times in visibly labelled Asia/Jakarta independently of browser timezone; interpret booking entry consistently with the intended Jakarta interval.
- **NFR-002 — Server security:** Enforce authentication, authorization, booking eligibility, and validation on the server. Protect state-changing browser requests against cross-site request forgery.
- **NFR-003 — Content safety:** Treat resource descriptions and booking purposes as plain text. Rendering HTML-like user content must not execute it.
- **NFR-004 — Credential safety:** Do not store plaintext passwords or log passwords/session tokens. Use only fictional demonstration identities and development-only credentials.
- **NFR-005 — Local isolation:** Bind locally by default and use synthetic data. Require no production credentials, OpenStack management-network access, or external service accounts.
- **NFR-006 — Offline ordinary operation:** After initial dependency and container-image downloads, ordinary application operation must not require an external service.
- **NFR-007 — Durability:** Application and database restarts must preserve committed bookings, resources, required activity history, and retry-identification records needed until reset.
- **NFR-008 — Reproducibility:** Provide documented startup, migrations, seed data, tests, and an explicit demonstration-reset procedure. Normal startup and repeated seeding must not erase existing bookings or duplicate seed accounts/resources. Only explicit reset may destroy demonstration data.
- **NFR-009 — Usability:** Normal workflows must be usable through a browser with labelled forms, meaningful empty and error states, and understandable inactive/conflict messages. A list or table is sufficient; a drag-and-drop calendar is unnecessary.
- **NFR-010 — Verification:** Provide one automated verification entry point in the repository. Tests must control time, isolate test data, and verify observable behavior. Concurrency guarantees require real-database tests with independent concurrent operations.
- **NFR-011 — Delivery discipline:** Before implementation, provide a short architectural design covering boundaries, records, authentication/authorization, time, and concurrency. Each delivery must identify the candidate revision, executed checks, observed results, and limitations, with independent review performed in a separate agent session.

No quantitative latency, throughput, availability, or browser-support benchmark is supplied by the baseline. Do not interpret the approximate team size as such a requirement.

## Implementation Decisions

### Required or strongly implied by the baseline

1. **Application responsibilities:** Separate the responsibilities for authentication/session handling, resource management, bookings/availability, activity history, and browser presentation. Exact module boundaries remain an architectural-design decision, not a mandate for separate services.
2. **Authenticated authority:** Resolve actors and permissions from server-authenticated accounts. Booking ownership cannot be delegated by a submitted owner field.
3. **Persistent records:** Retain accounts, resources, bookings, activity events, and per-user booking request identifiers. Retry identification must remain usable until explicit reset.
4. **Write-time consistency:** Enforce booking overlap, active-resource eligibility, deactivation eligibility, and retry semantics at the final write boundary, not merely when rendering a schedule.
5. **Atomic history:** Commit each business change and required activity event together. The architecture must describe how failure to record history prevents the associated change from committing.
6. **Derived labels:** Calculate Upcoming, In use, and Past from booking instants and current time. Do not introduce a background worker merely to transition display labels.
7. **Local lifecycle:** Include migrations, repeatable non-destructive seeding, durable storage across restarts, and a deliberately invoked reset workflow.
8. **Testable time:** Provide a controllable clock seam for automated behavior tests rather than relying on hard-coded future dates.

### Suggested defaults, not binding requirements

| Area | Baseline recommendation | Decision status |
| --- | --- | --- |
| Application | Go, one application with clear internal boundaries | To confirm in architectural design |
| Browser UI | Server-rendered HTML with small amounts of JavaScript | To confirm in architectural design |
| HTML rendering | Go `html/template`, without bypassing escaping for user text | Recommended if Go is selected |
| Database | PostgreSQL | To confirm in architectural design |
| Overlap enforcement | Evaluate PostgreSQL time ranges and exclusion constraints | Candidate mechanism, not selected here |
| Local environment | Docker Compose | To confirm in architectural design |
| Verification | Go tests, real-database integration tests, Playwright browser tests | Suggested tools; behavior and concurrency evidence are required |

No specific isolation level, locking protocol, authentication library, session storage scheme, endpoint set, or frontend component structure is selected by this PRD. The architectural design must justify those choices without expanding product scope.

## Data and API Contracts

### Persistent product records

| Record | Required information and invariants |
| --- | --- |
| Account | Stable identity, authenticated credentials stored safely, and Engineer or Coordinator role; three seeded fictional accounts |
| Resource | Immutable unique code, name, description, active/inactive state; retained rather than deleted |
| Booking | Stable identifier, resource reference, authenticated owner reference, unambiguous start/end instants, purpose validated after trimming, Confirmed or Cancelled state; retained after cancellation and after end |
| Booking request identity | Authenticated user plus stable request identifier, associated booking identifier, and enough booking data to distinguish identical replay from changed-data reuse; retained until explicit reset |
| Activity event | Actor, action, affected record, server timestamp, and relevant change details; no product edit/delete operation |

Exact field encodings, schema layout, and identifiers are architectural decisions. No new stored business state is implied by presentation labels or request-handling metadata.

### Operation contracts

- **Sign-in/sign-out:** Establish an authenticated session using seeded account credentials; invalidate it on sign-out. The browser cannot grant itself a role or identity.
- **Browse:** Return resources and selected-resource/date schedules only to authenticated users. Return My Bookings scoped to the authenticated account. Booking list pagination must use 25-record pages and a documented stable order.
- **Create booking:** Accept a resource identifier, start/end timestamps, purpose, and stable request identifier. Resolve owner server-side. Return the accepted booking's identifier, or return the same identifier for an identical retry. Distinguish invalid input, inactive-resource rejection, reservation conflict, and changed-data request-identifier reuse.
- **Cancel booking:** Identify the booking, check the actor's permission, and require a reason for coordinator cancellation of someone else's future booking. A successful transition retains the record and releases availability; an authorized repeat creates no new event.
- **Manage resource:** Restrict creation, name/description changes, and activation changes to coordinators. Reject code changes and deactivation blocked by a non-cancelled booking ending in the future.
- **Inspect history:** Restrict event access to coordinators and expose relevant actor/action/record/time/details without edit or delete actions.
- **Errors:** Do not expose database internals. Authentication, authorization, validation, inactive-resource, conflict, and request-identity errors must be distinguishable at the appropriate UI or HTTP boundary. Exact HTTP status codes and response envelopes are deferred to architectural design.

### Interval and concurrency invariants

For the same resource, two non-cancelled intervals conflict when each starts before the other ends. Equality between one end and the other start is allowed. Cancellation removes a booking from conflict consideration without deleting its record. Different resources are evaluated independently.

A correct booking/deactivation race may accept the booking and reject deactivation, or accept deactivation and reject the booking. It must not accept both incompatible changes. Every accepted mutation must have its required event; losing or rejected operations must not leave partial successful state.

## UX / Workflow Notes

- **Login:** Require actual account credentials, not a role-selection control pretending to authenticate. Present understandable authentication failures and a sign-out action.
- **Resource/schedule:** Show code, name, description, active state, selected date, and booking details. Use a clear empty schedule when no bookings exist and a distinct inactive indication.
- **Booking form/detail:** Show the resource, Jakarta time context, start/end inputs, and purpose. Explain validation, inactive-resource rejection, and conflict. Show the resulting booking after acceptance or identical replay.
- **My Bookings:** Show the user's own retained bookings with derived labels and cancellation actions only where permitted. Paginate long lists consistently with the schedule view.
- **Cancellation:** Explain the future-only rule and the need for a reason when a coordinator cancels another user's booking. After success, show Cancelled and make the interval available immediately.
- **Resource management:** Provide coordinator-only create/edit/activate/deactivate actions. When deactivation is blocked, explain that upcoming bookings must be cancelled and in-use bookings must finish.
- **Activity:** Provide coordinators a view of successful changes with actor, affected record, timestamp, and relevant details, including reasons and resource-name changes.
- **Role-aware presentation:** Do not offer unauthorized actions to engineers, while retaining server checks for direct requests.
- **Timezone independence:** A non-Jakarta browser must still enter and see the intended Jakarta interval. Date-boundary behavior for bookings crossing midnight must be explicitly decided before implementing date filtering.

These views may share pages. No component layout, separate SPA, or calendar interaction model is prescribed.

## Testing Decisions

No existing tests or test infrastructure are available. Establish these seams before implementing the corresponding behavior:

1. **Browser workflow tests:** Use the highest practical seam for login/logout, resource/date browsing, creating and viewing bookings, conflict feedback, cancellation/rebooking, My Bookings, coordinator resource management, and activity history. Verify user-visible behavior, not component structure. Isolate test data and exercise a non-Jakarta browser timezone.
2. **HTTP/handler permission tests:** Send direct unauthenticated and engineer/coordinator requests to protected operations. Verify server-derived ownership/roles, cross-user cancellation rejection, coordinator-only resources/history, session invalidation, and CSRF protection. Check both responses and absence of unauthorized mutations/success events.
3. **Real-database integration tests:** Verify persistence, overlap enforcement, retry behavior, resource eligibility, atomic events, cancellation releasing availability, and restart durability. Use independent concurrent operations for competing bookings and booking/deactivation races; fake repositories alone are insufficient for those guarantees.
4. **Failure-injection tests:** Make required event recording fail and verify the business mutation does not commit. Cover booking and resource changes, not only UI error presentation.
5. **Controlled-clock boundary tests:** Verify five-minute and 30-day start boundaries, 30-minute and eight-hour durations, end-after-start, cancellation just before and exactly at start, and display labels exactly at start/end. Do not depend on dates that eventually become past.
6. **Contract and focused rule tests:** Cover trimmed purpose lengths, all overlap shapes, adjacent intervals, cross-midnight bookings, different-resource simultaneous reservations, request-identifier scope, identical replay, changed-data reuse, and cancelled bookings excluded from conflicts. Use isolated unit tests only where they meaningfully complement higher-level evidence.
7. **Security/content tests:** Verify HTML-like resource descriptions and purposes render as text, reject state-changing requests without required CSRF protection, and ensure authentication failures/direct requests cannot select a role. Review credential persistence and logs for plaintext passwords or session-token disclosure.
8. **Lifecycle tests:** Verify clean startup, migrations, repeated non-destructive seeding, explicit reset, and application/database restarts. Verify retained booking/history data and retry identity across normal restarts.
9. **List behavior tests:** Verify 25-record booking pages, documented ordering/tie-breakers, retained past/cancelled bookings, and My Bookings ownership filtering.

The baseline's 14 client acceptance scenarios below are mandatory, not an exhaustive suite. The automated entry point must report observed results; test-tool choices remain subject to the architectural design.

## Acceptance Criteria

The first 14 criteria preserve the baseline AC-01 through AC-14 in the same order, renumbered with three digits. Remaining criteria make additional explicit baseline rules independently checkable.

| ID | Scenario and required result |
| --- | --- |
| AC-001 | An engineer creates a valid booking; it persists, appears in the UI, and has exactly one creation event. |
| AC-002 | Another user requests a partially overlapping, contained, enclosing, or identical interval on the same resource; every case is rejected. |
| AC-003 | A booking starts exactly when another ends on the same resource; it is accepted. |
| AC-004 | Identical intervals target different resources; both are accepted. |
| AC-005 | Two users concurrently submit valid conflicting bookings against a healthy system using independent real-database operations; exactly one succeeds, the other receives a conflict, and only one booking is stored. |
| AC-006 | The same user retries the same request identifier and data; the existing booking identifier is returned with no additional booking or creation event. |
| AC-007 | An engineer directly requests cancellation of someone else's booking through HTTP; the request is rejected without changing the booking or creating a successful cancellation event. |
| AC-008 | An authorized user cancels a future booking and repeats the cancellation; its interval is available for rebooking and exactly one cancellation event exists. |
| AC-009 | A coordinator attempts deactivation with an upcoming or in-use non-cancelled booking; deactivation is rejected with an explanation. |
| AC-010 | Booking creation races with deactivation using independent real-database operations; final state satisfies active-resource and deactivation rules, with no inactive resource holding a newly accepted future booking. |
| AC-011 | The browser uses a non-Jakarta timezone; entered and displayed times represent the intended Jakarta interval and displayed booking times are visibly labelled Asia/Jakarta. |
| AC-012 | Recording a required activity event fails; the associated business change does not commit. |
| AC-013 | Application and database restart; previously committed bookings and activity history remain. |
| AC-014 | A booking purpose contains HTML-like content; it appears as text and does not execute. |
| AC-015 | An unauthenticated user requests resource or booking information; access is rejected. A signed-out session cannot regain authenticated access. |
| AC-016 | An engineer directly requests resource creation or modification, activation/deactivation, or activity inspection; access is rejected with no unauthorized change or successful-change event. |
| AC-017 | A browser supplies another owner or role; booking ownership and permissions still come from the authenticated account. |
| AC-018 | New-booking starts exactly five minutes and exactly 30 days ahead are accepted when otherwise valid; starts outside those boundaries are rejected using controlled server time. |
| AC-019 | Durations exactly 30 minutes and eight hours are accepted when otherwise valid; shorter, longer, or non-positive intervals are rejected. A valid cross-midnight booking is accepted. |
| AC-020 | Purposes of 1 and 200 characters after trimming are accepted; empty, whitespace-only, and 201-character purposes are rejected. |
| AC-021 | A booking targets an inactive resource; it is rejected with an inactive-resource explanation rather than being accepted or reported merely as a conflict. |
| AC-022 | A user reuses their request identifier with different booking data; the request is rejected without another booking or creation event. |
| AC-023 | Two users use the same request identifier for valid non-conflicting bookings; they are treated as separate per-user request identities. |
| AC-024 | A coordinator cancels another user's future booking without a reason; the request is rejected. With a reason, cancellation succeeds and the event records that reason. |
| AC-025 | Either role attempts to cancel a confirmed booking exactly at or after start; cancellation is rejected. |
| AC-026 | Controlled time is before start, exactly at start, and exactly at end; a confirmed booking displays Upcoming, In use, and Past respectively. A cancelled booking remains Cancelled. |
| AC-027 | More than 25 bookings are available; views use 25-record pages and the documented stable order. Past and cancelled bookings remain accessible, and My Bookings contains only the authenticated user's bookings. |
| AC-028 | A coordinator creates a resource, changes its name/description, and changes eligibility where allowed; each successful change has an actor, action, affected record, server timestamp, and relevant details. Resource code remains immutable and unique, and no deletion action is offered. |
| AC-029 | A resource has only cancelled bookings or non-cancelled bookings whose end is no longer in the future; those bookings do not block deactivation. |
| AC-030 | Startup and seeding are repeated after data exists; bookings are preserved and seed accounts/resources are not duplicated. Explicit reset returns the demonstration to its documented initial state. |
| AC-031 | A state-changing browser request lacks required CSRF protection; it is rejected without a business mutation. Resource descriptions containing HTML-like content render as text. |
| AC-032 | The documented demonstration runs locally with synthetic data and no production credentials; after initial downloads, ordinary workflows require no external service. |
| AC-033 | A booking request is identically replayed after normal application/database restarts; its existing identifier is returned without duplicate booking or event. |

## Rollout / Migration Plan

### Before implementation

Submit the short architectural design requested by the client. Cover application boundaries, main records, authentication/authorization, time handling, and write-time concurrency correctness. Resolve blocking product ambiguities rather than encoding undocumented assumptions.

### First implementation delivery: foundation only

The first delivery is strictly limited to:

- Reproducible local startup and durable storage setup.
- Migrations and safe seed data.
- Sign-in/sign-out and server-enforced, role-aware access.
- Resource browsing with an empty schedule.
- Automated verification and a demonstration procedure for that foundation.

**Booking creation is the next delivery, not part of the first one.** This foundation is a partial delivery and does not claim to satisfy the complete v0.1 acceptance suite. Resource management, cancellation, and the completed activity workflow must not be silently added to the initial delivery.

### Subsequent reviewable deliveries

Deliver booking creation next, including its validation, conflict/concurrency, replay, and required-event guarantees. Remaining v0.1 behaviors must be divided into separately approved, reviewable tasks. This PRD does not pre-approve a detailed implementation plan or authorize the later change requests.

Use migrations and a non-destructive startup/seed process from the beginning. There is no existing production dataset to migrate. For later schema changes, document deployment ordering, compatibility with retained demonstration data, and rollback or recovery implications in the corresponding design/delivery. Do not treat explicit reset as routine migration or rollback.

Each delivery must provide its candidate revision, checks actually executed, observed results, and remaining limitations. Implementation and independent review occur in separate agent sessions. Do not push, merge, deploy, or change credentials without explicit instruction.

No feature-flag system, production rollout, or external monitoring service is required. Local verification and review evidence are the rollout gates for this proof of concept.

## Risks and Open Questions

### Confirmed facts and boundaries

- No implementation, existing contracts, schemas, or tests were found; technology choices and concurrency mechanisms are not yet finalized.
- The application history is not tamper-proof against database administrators.
- The first implementation delivery excludes booking creation even though the full product requires it.
- The baseline's introductory concern about forgetting to release resources does not authorize early release; cancellation remains future-only.
- No conflicting explicit product requirements were identified. The recommended stack is explicitly non-binding.

### Product details to resolve before affected implementation

1. **Selected-date schedule semantics:** Bookings may cross midnight, but the baseline does not explicitly define whether a date view includes every booking intersecting that Jakarta day or only bookings starting that day. **Proposed assumption, not a settled requirement:** show intervals intersecting the selected Jakarta day so continuing occupancy is visible. Confirm before implementing date filtering, including how retained cancelled/past records appear.
2. **Already-cancelled replay after original start:** The baseline requires harmless authorized repeated cancellation and prohibits cancelling a started confirmed booking. It does not expressly define the response when an already-cancelled booking is replayed after its former start, or whether a coordinator must resubmit the original reason. Decide without introducing another mutation/event or weakening ownership checks.
3. **Request-data equivalence:** Define the canonical booking data used for identical replay, particularly trimmed purpose and equivalent timestamps with different offsets. Preserve the invariant that genuinely different data under the same per-user identifier is rejected. Retaining replay behavior until reset must not become a short-lived cache.
4. **Text validation details:** The baseline specifies purpose length after trimming but not the character-count convention, resource-field length limits, code case sensitivity/format, or cancellation-reason length/whitespace handling. Clarify necessary validation contracts rather than inventing additional business restrictions.
5. **Presentation ordering and history details:** Choose and document the stable booking sort and tie-breaker. Activity ordering/pagination, exact detail shapes, and whether initial seeding emits product events are not specified; these must not silently change the required change history.

### Engineering decisions and delivery risks

- The architectural design must define authentication/session protections, timestamp encoding, HTTP errors, and the database mechanism for concurrent booking/deactivation and atomic history.
- Real concurrent operations and failure injection are required evidence; tests based only on mocks would leave the central correctness guarantees unproven.
- Preserve development credential safety while making setup reproducible. No actual passwords or tokens are specified in this PRD.
- Browser compatibility and quantitative performance objectives are unspecified. Any proposed targets are assumptions requiring agreement, not established acceptance requirements.

These open questions do not prevent documenting the known v0.1 scope. Decisions that affect an implementation task must be resolved before that task proceeds.

## Further Notes

- This PRD synthesizes the client baseline and is the repository's authoritative product specification. Proposed changes must be explicit, not silently introduced through implementation.
- **Deferred CR-01 — Atomic rescheduling:** After v0.1 passes and separate approval is granted, allow moving a future booking; failure must preserve the original interval, and success must record an activity event.
- **Deferred CR-02 — Maintenance windows:** After v0.1 passes and separate approval is granted, let coordinators block intervals; reject overlap with existing reservations without silently cancelling them.
- Acceptance coverage is broader than the initial foundation delivery. Passing foundation checks is not equivalent to accepting v0.1.
- The requested publication target for this task is the local PRD document. Issue-tracker publication is not part of this delivery.
