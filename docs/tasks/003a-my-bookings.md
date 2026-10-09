# Task 3A — My Bookings

## Status

Approved for implementation.

## Parent delivery

Delivery 3 — Manage My Bookings.

## Prerequisites

The following are accepted and integrated:

- Delivery 1 foundation
- Task 2A booking persistence
- Task 2B atomic booking creation
- Task 2C booking web workflow

## Objective

Implement an authenticated My Bookings view where users can browse
their own retained bookings.

This is a read-only capability.

Do not implement cancellation in this task.

## User-facing requirements

An authenticated user must be able to:

1. Navigate to My Bookings.
2. See their own booking records.
3. See the associated Resource name and code.
4. See the original Booking interval.
5. See the Booking purpose.
6. See the derived Booking presentation status.
7. Open the existing Booking detail view.
8. Navigate through multiple pages of results.
9. See a useful empty state when no records exist.

All displayed booking times use Asia/Jakarta.

## Authentication and ownership

The view requires authentication.

The current owner identity must be derived from the authenticated
server-side session.

Do not accept an arbitrary user/account/owner identifier as the
authority for selecting My Bookings records.

An Engineer sees only their own records.

A Coordinator also sees their own records in My Bookings.

A Coordinator's elevated role does not automatically turn My Bookings
into an all-users booking-management view.

Ownership filtering must occur in the database query, before
pagination.

Do not fetch all bookings and then filter them in application memory.

Do not alter the existing visibility contract for shared Resource
schedules or authenticated Booking detail views.

## Retained booking records

My Bookings must include:

- Upcoming confirmed bookings;
- In-use confirmed bookings;
- Past confirmed bookings;
- Cancelled bookings.

Do not exclude a booking merely because:

- its interval has ended;
- it has been cancelled;
- its Resource is currently inactive.

Booking history remains retained.

## Presentation statuses

Use the approved Booking state model.

Persisted state:

- Confirmed
- Cancelled

Derived presentation labels:

- Upcoming
- In use
- Past
- Cancelled

For a Confirmed booking:

now < start:
    Upcoming

start <= now < end:
    In use

end <= now:
    Past

For a Cancelled booking:

    Cancelled

Cancelled takes precedence over time-derived labels.

Use the existing controllable authoritative Clock.

Use one captured authoritative current time per rendered page.

Do not persist derived status labels.

Where suitable, reuse the existing Task 2C status presentation logic.

Do not introduce a second conflicting implementation of the same
status rules.

## Pagination

The My Bookings list must use pages of 25 records.

Use deterministic ordering with a unique tie-breaker.

Follow the accepted architectural ordering unless there is a
concrete reason to change it:

    start_at ASC, id ASC

Use an existing repository pagination approach when it satisfies
the requirements.

Offset or keyset pagination is acceptable when consistent with
the approved architecture.

Pagination must preserve:

- authenticated owner filtering;
- retained Past and Cancelled records;
- deterministic ordering;
- a maximum of 25 records per page;
- correct navigation between pages.

Pagination must not permit the browser to switch the owner identity.

Invalid pagination input must produce safe, documented behavior.

A frozen multi-request snapshot is not required.

## Existing Booking detail

Reuse the existing authenticated Booking detail view introduced
during Task 2C.

My Bookings entries should link to that view.

Do not create a separate duplicate Booking detail implementation
without an architectural reason.

Do not weaken existing authentication rules.

## Empty state

When the authenticated user has no Bookings:

- show a useful empty-state message;
- provide navigation back to Resource browsing;
- do not show another user's records.

## HTML and content safety

Render all user-controlled Booking text using the existing safe
template mechanism.

Booking purposes containing HTML-like content must appear as text.

Do not bypass contextual escaping.

## Architecture and implementation boundaries

Prefer a narrow read/query capability integrated with the existing
Booking persistence and web conventions.

The web layer owns:

- routing;
- authentication/session integration;
- pagination input decoding;
- template rendering;
- HTTP-safe error handling.

The database/query layer owns:

- authenticated-owner filtering;
- retained-record selection;
- stable ordering;
- pagination.

Reuse existing presentation helpers where appropriate.

Avoid introducing unnecessary abstractions.

## Required verification

### Ownership

Verify:

- unauthenticated access is rejected;
- Engineer A sees Engineer A's records;
- Engineer A cannot retrieve Engineer B's records through My Bookings;
- Engineer B sees Engineer B's records;
- Coordinator sees their own records;
- browser-supplied owner/account parameters cannot override the
  authenticated identity.

### Booking states

Using controlled time, verify:

- Upcoming;
- In use;
- Past;
- Cancelled.

Verify exact boundaries:

- now immediately before start;
- now exactly at start;
- now immediately before end;
- now exactly at end.

Verify Cancelled remains Cancelled regardless of time.

Do not use wall-clock-dependent fixtures.

### Retention

Verify that My Bookings includes retained:

- Past bookings;
- Cancelled bookings;
- bookings associated with inactive Resources.

Cancellation application behavior is not part of this task.

Test fixtures may prepare Cancelled records directly in an isolated
database without implementing the cancellation use case.

### Pagination

Use real PostgreSQL to verify:

- zero records;
- one record;
- exactly 25 records;
- 26 records;
- more than 50 records;
- deterministic ordering when start times are equal;
- no duplicates or omissions between pages for unchanged data;
- ownership filtering before pagination.

Verify that a different user's records cannot influence the current
user's displayed page membership or navigation.

Do not rely on arbitrary sleeps.

### Browser behavior

Extend Playwright verification to cover:

1. Engineer A signs in.
2. Engineer A opens My Bookings.
3. Only their bookings are displayed.
4. Engineer A opens an existing booking detail.
5. Engineer B signs in through an isolated browser context.
6. Engineer B sees only their own bookings.
7. Empty-state behavior is correct.
8. Pagination navigation works.

Use the existing verification-only controlled-time bootstrap.

Run at least one browser context in a non-Jakarta timezone.

Displayed times must remain visibly labelled Asia/Jakarta.

### Content escaping

Verify an HTML-like Booking purpose is displayed as text rather than
executed markup.

### Read-only behavior

Opening My Bookings, navigating pages, and opening booking detail
must not create or modify Bookings or Activity events.

Where practical, assert unchanged database business-record counts
or state around these operations.

## Existing verification

All accepted Delivery 1 and Delivery 2 tests must continue to pass.

The normal repository verification entry point must include the new
Task 3A tests.

Do not silently skip relevant Go, PostgreSQL, or browser suites.

## Documentation

Update README.md to describe:

- the My Bookings capability;
- retained Upcoming/In use/Past/Cancelled presentation;
- navigation to Booking detail;
- pagination.

Clearly distinguish implemented features from deferred features.

## Explicitly out of scope

Do NOT implement:

- Booking cancellation;
- Coordinator cancellation;
- Booking rescheduling;
- Resource management;
- activity-history UI;
- recurring bookings;
- notifications;
- external integrations;
- new infrastructure services.

Do not introduce cancellation buttons or nonfunctional action
placeholders merely for future convenience.

Do not introduce new database business tables or migrations unless
an independently justified requirement makes them necessary.

## Implementation authority

The implementation agent may independently:

- inspect existing Booking queries and web routes;
- implement authenticated My Bookings querying;
- implement pagination;
- reuse or minimally refactor presentation helpers;
- add the My Bookings browser view;
- add Go/PostgreSQL/Playwright tests;
- update README;
- run focused and full verification;
- repair in-scope failures.

Follow AGENTS.md.

Do not ask for additional permission between routine implementation,
testing, and in-scope repair steps.

Escalate genuine architecture or requirements conflicts.

## Completion criteria

Task 3A is ready for independent review only when:

- My Bookings is accessible to authenticated users;
- ownership is determined by authenticated session;
- database queries enforce owner filtering;
- all retained Booking states appear correctly;
- statuses are derived using authoritative time;
- pagination satisfies the 25-record stable-order contract;
- existing Booking detail is reused;
- HTML content is escaped;
- required automated verification passes;
- all prior accepted verification passes;
- README is accurate;
- no cancellation or deferred workflow was introduced;
- final diff has been inspected.

Finish with exactly one of:

READY_FOR_REVIEW
BLOCKED
INCOMPLETE

For READY_FOR_REVIEW report:

- base revision;
- exact candidate state;
- implementation decisions;
- query and pagination approach;
- routes/templates changed;
- verification actually executed;
- observed results;
- known limitations;
- anything not independently verified;
- deviations, if any.
