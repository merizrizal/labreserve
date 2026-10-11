# Task 3C — Cancellation Browser Workflow

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
- Task 3A My Bookings
- Task 3B atomic owner cancellation

Task 3B's CancelOwner service is the authoritative cancellation
application boundary.

## Objective

Allow an authenticated user to cancel their own future
Confirmed Booking through the browser.

The workflow must:

- expose cancellation from the existing Booking detail page;
- submit cancellation using authenticated, CSRF-protected POST;
- delegate cancellation to the existing Task 3B service;
- display successful cancellation through PRG;
- handle repeated cancellation safely;
- preserve Booking history;
- present useful errors without leaking internal information;
- maintain all existing booking creation and viewing behavior.

This task completes the owner-facing cancellation workflow.

## Architectural boundary

The web layer owns:

- routing;
- authentication/session integration;
- CSRF and Origin protection;
- form rendering;
- HTTP-to-domain adaptation;
- safe outcome mapping;
- PRG navigation.

Task 3B owns:

- ownership authorization;
- cancellation eligibility;
- persisted state validation;
- Resource and Booking locking;
- authoritative-time checks;
- Booking state transition;
- cancellation metadata;
- Activity-event insertion;
- transaction atomicity;
- idempotent cancellation.

The web layer must not reproduce or bypass these responsibilities.

In particular, handlers must not execute cancellation UPDATE
statements or append cancellation Activity events directly.

## Routes

Add:

    POST /bookings/{id}/cancel

Reuse existing routes:

    GET /my-bookings
    GET /bookings/{id}

Do not introduce a GET endpoint that mutates Booking state.

Do not introduce a separate cancellation API or JavaScript
framework.

## Cancellation form

Add a cancellation form to the existing Booking detail page.

Display the form only when all conditions hold:

- current authenticated actor owns the Booking;
- Booking is Confirmed;
- Booking has not yet reached its start instant.

Use the existing authoritative Clock for presentation decisions.

The form must include:

- an explicit cancellation action;
- valid CSRF token;
- clear identification of the Booking being cancelled.

A cancellation reason is not required for owner cancellation.

Do not accept browser-supplied owner, role, cancellation state,
cancellation actor, or cancellation timestamp as authority.

The URL identifies the target Booking.

The authenticated session identifies the acting user.

The Task 3B service makes the final authorization and
eligibility decision.

## Shared Booking detail visibility

The existing Booking detail page remains visible according to
its accepted authentication contract.

A non-owner may continue to view a Booking when permitted by
that contract.

However:

- a non-owner must not see the cancellation action;
- hiding the action is only a presentation rule;
- a forged direct POST must still be rejected by Task 3B.

Do not make the existing Booking detail owner-only merely to
simplify cancellation.

## Presentation and race conditions

The cancellation button represents eligibility at the time
the page was rendered.

It does not guarantee that cancellation will still be allowed
when the POST reaches the service.

If the Booking starts between page rendering and POST:

- Task 3B must reject cancellation;
- the handler must present a safe, useful response;
- no cancellation mutation or Activity event may occur.

Do not implement a second business-time validation rule inside
the handler.

The web layer may use the shared presentation-status helper
to decide whether to display the action.

## HTTP outcomes

Map Task 3B outcomes as follows.

### Successful first cancellation

Return:

    303 See Other
    Location: /bookings/{id}

The resulting Booking detail shows Cancelled.

### Already-Cancelled no-op

Return the same successful PRG response.

Do not display the operation as a second cancellation event.

### Booking unknown or inaccessible

Return:

    404 Not Found

Use the same response for unknown and unauthorized targets.

Do not disclose another owner's cancellation eligibility
or state through the POST outcome.

### Confirmed Booking no longer cancellable

Return:

    409 Conflict

Show a useful explanation that the Booking can no longer be
cancelled, with navigation back to Booking detail.

Do not expose internal domain or database implementation
details.

### Operational failure

Return a safe generic failure response consistent with the
existing application conventions, using HTTP 503.

Do not expose:

- database errors;
- SQL statements;
- constraint names;
- internal paths;
- stack traces;
- credentials;
- session secrets.

If the commit outcome is uncertain, do not falsely claim that
cancellation definitely failed or rolled back.

The user must be able to inspect the retained Booking state
and retry safely.

## Authentication, CSRF, and Origin

The cancellation POST requires an authenticated session.

Use the existing session-derived Actor.

Require valid CSRF protection.

Preserve the accepted Origin validation behavior.

Reject missing or invalid CSRF credentials.

Reject untrusted Origins.

Security rejection must occur before cancellation service
execution.

Rejected requests must not modify Bookings or Activity events.

## Idempotency

Do not introduce a new cancellation request identifier.

Task 3B already implements idempotency through the retained
Cancelled state.

Repeating an authorized cancellation POST must:

- return a successful outcome;
- preserve existing cancellation metadata;
- create no additional cancellation Activity event.

This must remain true after the original start instant.

Do not redirect an already-Cancelled request into a new
cancellation operation.

## Booking history and schedule integration

After successful cancellation:

- Booking detail displays Cancelled;
- My Bookings retains the record;
- Resource schedule retains the record and displays Cancelled;
- original Booking interval remains visible;
- original purpose remains visible;
- original owner remains unchanged;
- historical Activity records remain retained.

Do not physically delete the Booking.

Do not implement new persistence state labels.

Reuse existing status presentation logic.

## Interval release

After a successful cancellation, another eligible engineer
must be able to reserve the released interval.

Task 3B remains responsible for transactional interval release.

Task 3C must not manually remove or rewrite schedule records.

Use existing Task 2C booking creation behavior to demonstrate
the user-visible result.

## Content safety

All Booking data must render through the existing safe HTML
template mechanism.

User-controlled Booking purposes must remain escaped.

Do not introduce unsafe HTML rendering for cancellation
messages or confirmation pages.

## Required HTTP/PostgreSQL verification

Use the real application and PostgreSQL.

### Authorized cancellation

Verify:

- Engineer A can cancel their own future Booking;
- Coordinator can cancel their own future Booking;
- Booking becomes Cancelled;
- exactly one cancellation Activity event is committed;
- the original creation Activity event remains unchanged;
- the HTTP response follows PRG;
- the resulting detail page displays Cancelled.

### Unauthorized cancellation

Verify:

- Engineer A cannot cancel Engineer B's Booking;
- Coordinator cannot cancel another owner's Booking through
  this owner service;
- unknown and unauthorized Booking identifiers receive the
  same safe 404 outcome;
- forged owner/account/role form fields do not grant authority.

Verify no Booking or Activity mutation occurs.

### CSRF and Origin

Verify:

- valid CSRF succeeds;
- missing CSRF returns 403;
- invalid CSRF returns 403;
- unexpected Origin returns 403.

For every rejected mutation attempt, verify the Booking and
Activity state remains unchanged.

### Time boundaries

Using controlled time, verify:

- a future Confirmed Booking can be cancelled;
- a Confirmed Booking at its exact start cannot be cancelled;
- a started Confirmed Booking cannot be cancelled;
- a past Confirmed Booking cannot be cancelled;
- a future Confirmed Booking on an inactive Resource remains
  cancellable.

The web handler must rely on Task 3B for authoritative
cancellation eligibility.

### Cancellation form visibility

Verify:

- owner of future Confirmed Booking sees Cancel action;
- non-owner does not see Cancel action;
- owner of Cancelled Booking does not see Cancel action;
- owner of started/past Confirmed Booking does not see
  Cancel action.

A forged POST must not bypass service authorization.

### Repeated cancellation

Perform an authorized cancellation.

Submit the same cancellation operation again using valid
authentication and CSRF credentials.

Verify:

- success;
- Booking remains Cancelled;
- cancellation metadata remains unchanged;
- no additional cancellation event appears.

Exercise already-Cancelled replay after the original
Booking start instant.

### Operational failure

Inject a controlled cancellation-service/database failure
using the existing testing conventions.

Verify:

- safe HTTP response;
- no internal diagnostic leakage;
- atomic rollback when the service reports a failed
  transaction;
- no false claim of definite rollback when commit outcome
  cannot be established.

Do not create a new production fault-injection endpoint.

### GET remains read-only

Verify:

- GET Booking detail does not cancel;
- GET My Bookings does not cancel;
- GET Resource schedule does not cancel;
- GET requests cannot trigger cancellation through
  alternative routing or query parameters.

## Browser end-to-end acceptance

Extend the existing Playwright suite.

Use the real application and disposable PostgreSQL.

### Scenario 1 — Owner cancellation

1. Sign in as Engineer A.
2. Create a valid future Booking.
3. Open My Bookings.
4. Open Booking detail.
5. Observe Cancel booking.
6. Submit cancellation.
7. Observe PRG.
8. Verify Booking detail shows Cancelled.
9. Return to My Bookings.
10. Verify the retained Booking is marked Cancelled.
11. Open Resource schedule.
12. Verify the retained Booking is displayed as Cancelled.

Assert database state where necessary to prove exactly one
cancellation Activity event was created.

### Scenario 2 — Non-owner restriction

1. Engineer A creates a Booking.
2. Engineer B signs in through an isolated browser context.
3. Engineer B opens that Booking's detail.
4. Engineer B must not see Cancel booking.
5. Forge a direct cancellation POST with Engineer B's valid
   session and CSRF credentials.
6. Verify the POST is rejected.
7. Verify the Booking remains Confirmed.
8. Verify no cancellation Activity event was created.

### Scenario 3 — Released interval

1. Engineer A creates a future Booking.
2. Engineer A cancels it.
3. Engineer B creates a Booking on the same Resource for the
   same interval.
4. Verify Engineer B's Booking succeeds.
5. Verify Engineer A's cancelled record remains visible.
6. Verify only Engineer B's Booking currently occupies the
   contested interval.

Use the existing booking creation workflow.

Do not introduce special rebooking logic.

### Scenario 4 — Repeated cancellation

1. Engineer A successfully cancels a Booking.
2. Repeat the authorized cancellation POST.
3. Verify successful no-op.
4. Verify exactly one cancellation Activity event exists.

Direct HTTP within the browser test is acceptable when it
provides more deterministic repeated-request coverage.

## Controlled time and timezone

Use the existing verification-only controlled-clock bootstrap.

Do not add a production time-changing endpoint.

Browser tests must not depend on today's real date.

Run browser acceptance with a non-Jakarta browser timezone,
consistent with existing verification conventions.

Displayed Booking intervals remain labelled Asia/Jakarta.

## Existing verification

All accepted tests must continue to pass:

- Delivery 1;
- Tasks 2A, 2B, 2C;
- Tasks 3A and 3B.

The normal repository verification entry point must include
Task 3C tests.

Do not silently skip PostgreSQL or browser verification.

Do not replace existing service-level concurrency tests
with browser tests.

## Documentation

Update README to describe:

Implemented:

- owner cancellation through Booking detail;
- CSRF-protected cancellation POST;
- PRG;
- retained Cancelled history;
- idempotent cancellation;
- interval release.

Deferred:

- Coordinator cancellation of another owner's Booking;
- mandatory Coordinator cancellation reason;
- booking rescheduling;
- Resource management;
- activity-history UI;
- recurring bookings;
- external notifications and integrations.

Keep the previously documented database privilege limitation
accurate.

## Explicitly out of scope

Do NOT implement:

- Coordinator cancellation of another owner's Booking;
- Coordinator cancellation reasons;
- inline cancellation actions on My Bookings;
- rescheduling;
- Resource-management workflows;
- Activity-history UI;
- additional Booking state transitions;
- new cancellation idempotency tables;
- new infrastructure services;
- database privilege redesign;
- speculative future workflow scaffolding.

## Implementation authority

The implementation agent may independently:

- inspect existing booking web conventions;
- reuse Task 3B CancelOwner;
- add the cancellation route;
- add the owner-only cancellation form;
- map domain outcomes to HTTP responses;
- update existing templates and navigation;
- add HTTP/PostgreSQL integration tests;
- add Playwright acceptance tests;
- update README;
- run focused and complete verification;
- repair in-scope failures.

Follow AGENTS.md.

Do not request additional permission for routine implementation,
verification, or in-scope repair.

Escalate genuine requirements or architecture conflicts.

## Completion criteria

Task 3C is ready for independent review only when:

- authenticated owners can cancel future Confirmed Bookings
  through the browser;
- non-owners cannot cancel another owner's Booking;
- cancellation uses POST with CSRF/Origin protection;
- HTTP handlers delegate to Task 3B;
- first cancellation follows successful PRG;
- authorized already-Cancelled replay follows successful PRG;
- stale/started Booking cancellation is safely rejected;
- Booking history remains retained;
- cancelled intervals become available for new bookings;
- no duplicate cancellation Activity event is created;
- operational failures do not leak internals;
- GET operations remain read-only;
- required HTTP/PostgreSQL tests pass;
- required Playwright acceptance passes;
- previous accepted verification continues to pass;
- README accurately describes the resulting implementation;
- deferred workflows remain unimplemented;
- final diff has been inspected for unrelated changes.

Finish with exactly one of:

READY_FOR_REVIEW
BLOCKED
INCOMPLETE

For READY_FOR_REVIEW, report:

- implementation base revision;
- exact candidate state;
- routes/templates modified;
- CancelOwner integration approach;
- HTTP outcome mapping;
- security boundaries preserved;
- verification commands actually executed;
- observed results;
- limitations;
- anything not verified;
- deviations, if any.
