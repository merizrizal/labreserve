# Task 2C — Booking web workflow and end-to-end acceptance

## Status

Approved for implementation.

## Parent delivery

Delivery 2 — Reserve correctly.

## Prerequisites

The following tasks are accepted and integrated:

- Task 2A — Booking persistence and database invariants.
- Task 2B — Atomic booking creation and idempotent replay.

Task 2B is the authoritative application-level booking creation implementation.

Task 2C must expose that capability through the existing browser application without duplicating its business, transaction, replay, locking, overlap, or authoritative-time logic.

## Objective

Allow an authenticated LabReserve user to create a Booking for themselves through the browser.

A user must be able to:

1. browse a Resource;
2. open a booking form for that Resource;
3. enter start time, end time, and purpose;
4. submit the form;
5. receive a successful Booking result or a useful validation/conflict response;
6. retry an ambiguous or repeated browser submission without creating a duplicate Booking;
7. see the successful Booking represented on the Resource schedule.

This task completes the user-facing booking-creation portion of Delivery 2.

## Architectural boundary

The HTTP/web layer is an adapter around the existing Task 2B booking service.

The handler may:

- authenticate the current user through the existing session boundary;
- decode and validate transport syntax;
- parse Jakarta-local form timestamps into unambiguous instants;
- obtain or carry the browser request identifier;
- construct the Task 2B service request;
- call the existing booking service;
- map explicit service outcomes to HTTP responses and user-facing messages;
- render templates and redirects.

The handler must NOT independently implement:

- booking horizon rules;
- duration rules;
- canonical purpose rules beyond required transport parsing;
- replay comparison;
- Account locking;
- Resource locking;
- overlap checks;
- transaction management;
- Activity-event creation;
- current Resource eligibility rules.

Those responsibilities remain in Task 2B.

If Task 2C discovers that the Task 2B API cannot support the required web workflow without duplicating domain logic, escalate rather than silently reimplementing the logic in the handler.

## Booking form

Provide an authenticated booking form associated with a specific Resource.

The form must contain:

- Resource identity/context;
- start time;
- end time;
- purpose;
- stable request identifier;
- CSRF token.

The authenticated owner must NOT be supplied as trusted browser input.

Any submitted owner/account/role value must not be able to change booking ownership.

## Jakarta time contract

The browser form represents booking times in Asia/Jakarta.

Use `datetime-local` style input or the existing approved equivalent.

Offsetless browser values are interpreted solely as Asia/Jakarta local date/time.

They must never be interpreted using:

- browser-configured timezone;
- host operating-system timezone;
- arbitrary user timezone.

The server converts the submitted Jakarta local date/time to the instant passed to Task 2B.

The UI must visibly communicate that booking times are in Asia/Jakarta.

Transport parsing errors must remain distinct from validly parsed requests rejected by Task 2B business rules.

## Stable browser request identifier

A new intentional booking form receives a fresh unpredictable request identifier.

The same request identifier must survive:

- ordinary validation errors;
- conflict responses where the user is expected to correct/retry the same intended submission;
- browser resubmission;
- network retry of the same submitted form.

Do not generate a replacement request identifier merely because a POST is repeated.

A newly initiated intentional booking uses a new request identifier.

The request identifier grants no authority and does not determine the booking owner.

Do not use process memory as the durable replay mechanism; Task 2B and persisted Booking request identity remain authoritative.

## Successful submission

On successful new creation:

- redirect using the existing POST/redirect/GET convention;
- show a stable successful result;
- expose the created Booking identifier through the resulting application view;
- do not create another Booking or Activity event during rendering.

On successful replay of the same request:

- return the existing Booking;
- follow the same user-facing successful result;
- do not present replay as a second successful creation;
- do not create a duplicate Booking or Activity event.

## Error/outcome mapping

Map Task 2B outcomes into appropriate browser responses.

The UI must distinguish at least:

### Invalid transport input

Examples:

- missing required form field;
- timestamp cannot be parsed as the approved Jakarta representation;
- malformed request identifier.

Return a useful form-level validation response.

### Invalid booking

Examples returned by Task 2B:

- start outside allowed horizon;
- invalid duration;
- empty/oversized canonical purpose.

Present a useful validation message without exposing internal implementation details.

### Unknown Resource

Return the repository's established not-found behavior.

### Inactive Resource

Explain that the Resource cannot currently be booked.

### Booking conflict

Explain that the selected interval is no longer available.

Do not expose PostgreSQL constraint names or raw database errors.

### Changed request-id reuse

Treat this as a conflict/error condition distinct from ordinary overlap.

Do not silently generate another request identifier and resubmit.

### Operational failure

Return a generic safe failure response consistent with existing application conventions.

Do not expose SQL, stack traces, credentials, tokens, or internal database details.

## Resource schedule

The Resource detail/schedule page must now display retained Bookings intersecting the selected Asia/Jakarta day.

Resolve the previously deferred schedule semantics as follows:

A Booking appears on a selected day when its interval intersects that Jakarta calendar day:

`start < day_end AND end > day_start`

where:

- `day_start` is midnight at the beginning of the selected date in Asia/Jakarta;
- `day_end` is midnight at the beginning of the following Jakarta date.

A Booking ending exactly at midnight does not appear on the following day.

A Booking that began on the previous day but continues into the selected day does appear.

Display the complete original Booking interval rather than clipping persisted times.

The page must distinguish:

- Upcoming;
- In use;
- Past;
- Cancelled.

These are presentation labels derived from authoritative time and persisted Booking state.

Do not persist Upcoming, In use, or Past.

Cancelled Bookings remain visible but must be visually distinguishable from intervals that occupy the Resource.

All displayed Booking times use Asia/Jakarta and must be visibly labelled.

## Schedule visibility

All authenticated users may see:

- Booking owner;
- start/end;
- purpose;
- presentation state;

for Resource schedules, consistent with the approved product requirements.

Do not introduce per-user schedule hiding.

## Stable pagination

Resource schedules use pages of 25 retained Booking records.

Use a documented stable ordering with a unique tie-breaker.

The approved architecture recommends:

`start_at ASC, id ASC`

and keyset navigation.

Task 2C may select the concrete supported pagination approach permitted by the architecture, but it must preserve:

- page size 25;
- deterministic stable ordering;
- retained Past and Cancelled Bookings;
- validated/server-controlled Resource/date filters.

Do not introduce a calendar framework or drag-and-drop scheduling UI.

## Authentication and CSRF

All booking form and schedule operations require authenticated access.

Booking POST:

- must use the authenticated actor from the current server-side session;
- must require valid CSRF protection;
- must follow existing Origin/CSRF conventions;
- must not trust browser role or owner fields.

Unauthenticated access must follow existing application behavior without exposing protected Booking data.

## Escaping and content safety

Purpose and any other user-controlled Booking text must render as plain escaped text.

Do not bypass `html/template` contextual escaping.

HTML-like Booking purpose text must remain visible text and must not execute.

## Required HTTP/integration verification

Cover at minimum:

- unauthenticated booking form access rejected/redirected appropriately;
- authenticated Engineer may open booking form;
- authenticated Coordinator may create a Booking for themselves;
- owner is derived from authenticated session;
- valid Booking submission succeeds;
- successful POST follows PRG;
- Booking persists;
- exactly one creation Activity event exists;
- missing/invalid CSRF rejected with no Booking/event;
- malformed Jakarta timestamp rejected;
- Task 2B validation failures render useful errors;
- inactive Resource response;
- overlapping Booking conflict response;
- changed request-id reuse response;
- operational errors do not leak database details;
- HTML-like purpose renders as text.

Do not merely assert HTTP status where absence of mutation matters.

## Browser end-to-end acceptance

Extend the Playwright suite to cover the central Delivery 2 workflow
using the real Go application and PostgreSQL.

At minimum:

### Successful booking

1. sign in as Engineer A;
2. browse a Resource;
3. create a valid Booking;
4. observe successful result;
5. return to the Resource schedule;
6. see the Booking owner, purpose, Jakarta times, and correct
   presentation label.

### Conflict

1. Engineer A has a Booking;
2. Engineer B attempts an overlapping interval for the same Resource;
3. submission receives a clear conflict;
4. exactly one contested Booking remains.

Do not infer database concurrency guarantees from browser timing;
Task 2A/2B tests remain authoritative for concurrency.

### Adjacent booking

A Booking beginning exactly when another ends succeeds and appears
correctly.

### Different Resource

An identical interval on a different Resource succeeds.

### Replay / repeated submission

Exercise the same browser booking submission request identifier more
than once.

Verify:

- the same Booking is returned;
- no duplicate Booking exists;
- no duplicate creation Activity event exists.

The test may exercise HTTP directly after the browser obtains the
form/request identifier if doing so is more deterministic than
attempting to manufacture browser network retransmission.

### Timezone independence

Run at least one browser context configured to a non-Jakarta timezone.

Enter the same Jakarta form values and verify:

- the persisted interval represents the intended Jakarta instant;
- displayed schedule values remain Jakarta-labelled;
- browser timezone does not reinterpret them.

### Cross-midnight schedule

Create a Booking crossing Jakarta midnight.

Verify it appears on every selected Jakarta date whose interval it
intersects, according to the approved intersection rule.

### Escaping

Submit HTML-like purpose text.

Verify it appears as text and does not execute.

## Controlled time

Browser/E2E verification must use the existing test-only
controlled-clock mechanism.

Do not add a production endpoint that changes server time.

Tests must not rely on today's real date or become invalid as wall-clock
time advances.

## Existing verification

All accepted Delivery 1, Task 2A, and Task 2B verification must continue
to pass.

The repository verification entry point must include Task 2C's browser
and relevant HTTP verification and must not silently skip them.

## Documentation

Update README/demo documentation to reflect:

Implemented through Task 2C:

- browser booking creation;
- Resource schedule population;
- idempotent/retry-safe Booking submission.

Clearly distinguish still-deferred functionality.

Do not claim cancellation, My Bookings, Resource management, or
activity-history UI exist.

## Explicitly out of scope

Do NOT implement:

- My Bookings;
- Booking cancellation;
- coordinator cancellation;
- Resource create/edit/activate/deactivate workflows;
- activity-history UI;
- rescheduling;
- recurring bookings;
- email/chat notifications;
- calendar integrations;
- infrastructure control.

Do not implement convenience features outside the approved Task 2C
scope.

## Implementation authority

The implementation agent may independently:

- inspect the accepted Task 2B service and current web conventions;
- select route/template/form organization consistent with existing
  repository design;
- add the minimum query/read model required for Resource schedules;
- implement HTTP adapters around Task 2B;
- implement stable schedule pagination;
- add HTTP and Playwright verification;
- update current implementation documentation;
- run focused/full verification;
- diagnose and repair Task-2C-related failures.

Routine implementation, testing, and in-scope repair do not require
further approval.

Escalate according to `AGENTS.md`.

## Completion criteria

Task 2C is ready for independent review only when:

- authenticated users can create Bookings through the browser;
- HTTP handlers delegate booking correctness to Task 2B rather than
  duplicate it;
- stable browser request identifiers support retry/replay;
- schedule display follows the approved Jakarta date-intersection
  semantics;
- 25-record stable pagination is implemented;
- browser-visible states are derived, not persisted;
- CSRF/authentication boundaries remain enforced;
- content rendering remains escaped;
- required HTTP and Playwright scenarios pass;
- all prior accepted verification continues to pass;
- README accurately describes the resulting implementation;
- no deferred workflows have been introduced;
- final diff has been inspected for unrelated changes.

Finish with exactly one of:

READY_FOR_REVIEW
BLOCKED
INCOMPLETE

For READY_FOR_REVIEW, report:

- base revision;
- exact candidate state;
- routes/templates/query behavior added;
- request-id lifecycle implemented;
- schedule/pagination design selected;
- verification commands actually executed;
- observed results;
- anything not independently verified;
- limitations;
- deviations, if any.
