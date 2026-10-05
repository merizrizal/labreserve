# Task 2B — Atomic booking creation and idempotent replay

## Status

Approved for implementation.

## Parent delivery

Delivery 2 — Reserve correctly.

## Prerequisite

Task 2A — Booking persistence and database invariants — is accepted and integrated.

The Booking and Activity-event persistence introduced by Task 2A are authoritative persistence foundations for this task.

## Objective

Implement the application-level booking creation use case.

The use case must:

- validate a proposed booking using authoritative server time;
- create a Booking for the authenticated user;
- preserve idempotent replay using the retained Booking request identifier;
- serialize same-user creation attempts according to the approved architecture;
- serialize Resource eligibility/occupancy decisions;
- rely on the PostgreSQL overlap constraint as the final overlap guarantee;
- create the required activity event atomically with the Booking;
- return stable domain outcomes that future HTTP/UI code can map appropriately.

This task implements the booking creation application/service behavior.

It does not implement the browser booking workflow.

## Authoritative request identity

A successful request is identified by:

authenticated owner account + request_id

The same request_id used by different authenticated accounts represents independent request identities.

The request identifier itself grants no authority.

## Canonical booking request data

For one authenticated owner and request_id, the canonical booking data is:

- Resource stable identity;
- start instant;
- end instant;
- canonical purpose.

### Resource

Compare the persisted stable Resource identity.

Different Resource identities mean different request data even if their display names or codes happen to resemble each other.

### Time

Normalize start and end to UTC instants.

Canonical precision is PostgreSQL microsecond precision.

Equivalent timestamp spellings or offsets representing the same microsecond instant are equal.

Do not use browser or host-local timezone interpretation inside this application use case.

### Purpose

Trim surrounding Unicode whitespace before validation and persistence.

The canonical persisted purpose is the trimmed result.

Purpose length must be 1–200 Unicode code points inclusive.

Compare canonical purposes exactly.

Do not:

- case-fold the purpose;
- Unicode-normalize it;
- collapse internal whitespace;
- otherwise rewrite its content.

## Replay behavior

After authenticating the actor and acquiring the approved per-account serialization lock, query Booking by:

(owner_account_id, request_id)

### Existing Booking with equal canonical data

Return the existing Booking successfully.

Do not:

- create another Booking;
- create another Activity event;
- acquire the Resource lock;
- re-check the current booking horizon;
- re-check current Resource active state;
- re-check current overlap eligibility.

This remains true if the retained Booking is now:

- Past;
- Cancelled;
- associated with a Resource that has since become inactive.

The replay answers what successful request the identifier already represents.

### Existing Booking with different canonical data

Reject the command as request-identifier reuse with changed data.

Perform this decision before Resource locking or current eligibility validation.

### No existing Booking

Continue with normal new-booking creation.

A failed creation that never commits a Booking does not consume the request identifier.

## Transaction and lock protocol

Use one explicit PostgreSQL transaction for creation.

For a new or potentially replayed request:

1. Begin the transaction.
2. Lock the authenticated Account row using the approved `FOR NO KEY UPDATE` protocol.
3. In a separate statement after the Account lock, query Booking by `(owner_account_id, request_id)`.
4. Return replay or changed-data-reuse outcome immediately when a retained Booking exists.
5. If no Booking exists, lock the requested Resource row using `FOR UPDATE`.
6. Capture fresh authoritative server time after required lock waits.
7. Perform final application validation.
8. Insert the Booking.
9. Append the required booking-created Activity event using the same transaction.
10. Commit once.

Do not change the approved lock order.

Do not replace the Account serialization with process-local mutexes, caches, advisory locks, or uniqueness-error-only replay handling.

Any database privilege changes needed to execute the approved row-lock protocol must be minimal, migration-managed, documented, and independently testable.

## New-booking validation

After Account serialization, replay lookup, and Resource locking, capture fresh authoritative time.

Validate the new booking against that time.

### Resource

The Resource must exist and be active.

An inactive Resource returns an inactive-resource outcome.

### Start horizon

The start instant must be:

- at least 5 minutes after authoritative current time; and
- no more than 30 × 24 hours after authoritative current time.

Both boundaries are inclusive.

Therefore:

now + 5 minutes

is valid.

And:

now + 30 × 24 hours

is valid.

Values one canonical precision unit outside either boundary are invalid.

### Interval

End must be after start.

Duration must be between 30 minutes and 8 hours inclusive.

The Task 2A database constraints remain defense in depth; application validation must return an appropriate domain validation outcome rather than intentionally relying on a database constraint failure for ordinary invalid input.

### Purpose

Trim surrounding Unicode whitespace.

Reject canonical purpose length:

- 0 code points;
- more than 200 code points.

Persist the canonical trimmed value.

## Overlap behavior

The Task 2A PostgreSQL exclusion constraint remains authoritative.

Normal new-booking creation also follows the approved Resource-row locking protocol.

A conflicting Confirmed Booking for the same Resource causes a booking-conflict domain outcome.

Map only the known exclusion constraint to that outcome.

Do not broadly translate unrelated database failures into booking conflicts.

Adjacent intervals remain valid.

Different Resources do not conflict.

Cancelled Bookings do not occupy their previous intervals.

## Activity-event atomicity

A successful new Booking must have exactly one booking-created Activity event.

The Booking insert and Activity-event insert must occur in the same transaction and commit together.

The event must identify:

- authenticated actor;
- booking-created action;
- affected Booking;
- authoritative operation timestamp;
- relevant booking details according to the approved architecture.

Do not expose credentials or session secrets in Activity details.

If Activity insertion fails:

- the Booking must not commit;
- the request identifier must not become a successful retained identity.

A successful replay creates no additional event.

A changed-request reuse rejection creates no successful-change event.

Other validation or conflict rejection creates no successful-change event.

## Domain/API boundary

Implement a narrow application-level booking creation API that future HTTP code can call.

It must accept authenticated actor context from the caller rather than a browser-supplied owner identity.

Return explicit outcomes/errors sufficient to distinguish at least:

- successful new Booking;
- successful replay;
- invalid input;
- unknown Resource;
- inactive Resource;
- overlapping Booking conflict;
- changed-data request-identifier reuse;
- operational/database failure.

Do not introduce HTTP status codes into the domain/application service unless existing repository conventions clearly place that responsibility there.

## Authoritative clock

Use the existing controllable Clock abstraction.

Do not call browser-provided time.

Do not use a request-arrival timestamp captured before potentially blocking row locks for final eligibility.

Final clock-relative validation must use fresh time obtained after the required lock acquisition.

## Required verification

Use real PostgreSQL for transactional and locking behavior.

Tests that prove concurrency or database interactions must use independent connections/transactions where applicable.

Do not use arbitrary sleeps as the primary synchronization mechanism.

### Canonicalization and validation

Cover at minimum:

- surrounding Unicode whitespace trimmed;
- empty-after-trim purpose rejected;
- 1-code-point purpose accepted;
- 200-code-point purpose accepted;
- 201-code-point purpose rejected;
- purpose case differences remain distinct canonical values;
- no unintended Unicode normalization;
- equivalent UTC instants compare equal after canonical precision handling;
- start exactly now + 5 minutes accepted;
- start immediately below the minimum rejected;
- start exactly now + 30 × 24 hours accepted;
- start immediately above the horizon rejected;
- 30-minute duration accepted;
- below-30-minute duration rejected;
- 8-hour duration accepted;
- above-8-hour duration rejected;
- end not after start rejected;
- inactive Resource rejected.

### Successful creation

Verify:

- authenticated owner is persisted from the actor, not caller-supplied booking ownership;
- canonical purpose is persisted;
- exactly one Booking commits;
- exactly one booking-created Activity event commits;
- event actor and affected Booking are correct.

### Replay

Verify durable replay for:

- same owner;
- same request_id;
- equal canonical booking data.

Assert:

- the same Booking identifier is returned;
- no duplicate Booking is created;
- no duplicate Activity event is created.

Verify replay continues to work when the retained Booking has become Past.

Where persistence state already supports it without implementing the deferred cancellation use case, directly prepare a retained Cancelled Booking in test setup and verify replay returns it without new creation or event.

Likewise verify replay is not invalidated merely because its Resource is currently inactive.

Do not implement cancellation or resource-management application behavior merely to create those fixtures.

### Changed request data

For the same owner/request_id, independently vary:

- Resource;
- start;
- end;
- purpose.

Each changed canonical request must be rejected as request-id reuse with changed data.

This decision must occur before current Resource eligibility/overlap validation.

Include a case where the changed target Resource is inactive or otherwise unsuitable and prove changed-request reuse remains the reported domain outcome.

### Failed request does not consume identifier

Cause a new booking attempt to fail before commit.

Retry the same owner/request_id with a valid request afterward.

Verify the later valid request can create the Booking and produces exactly one successful creation event.

Include Activity-event failure rollback coverage using an appropriate transaction-scoped test seam or controlled database failure mechanism.

### Concurrent identical request replay

Run two independent concurrent creation operations using:

- the same authenticated owner;
- the same request_id;
- equal canonical booking data.

Verify:

- exactly one Booking commits;
- exactly one booking-created Activity event commits;
- both operations resolve to the same Booking;
- the second operation observes the committed request only after Account serialization/replay lookup.

### Concurrent same key with different data

Run concurrent operations for the same owner/request_id but different canonical data.

Verify:

- at most one Booking binds the request identifier;
- the committed winner is returned for its own canonical request;
- the other operation receives changed-request reuse after serialization;
- no duplicate successful Activity event is produced.

Include a different-Resource case so correctness is not accidentally dependent on both requests contending on one Resource lock.

### Same user, different request IDs

Use two valid requests for the same account with different request identifiers and different Resources.

Verify both may succeed, although the approved Account serialization may order them.

The locking protocol must serialize execution; it must not treat the second request as duplicate/reuse.

### Conflicting booking creation

Run two independent valid creation operations with distinct owners/request IDs for overlapping intervals on the same Resource.

Verify:

- exactly one succeeds;
- exactly one returns booking conflict;
- exactly one Booking for the contested interval commits;
- exactly one corresponding creation event commits.

The Task 2A exclusion constraint remains the database backstop.

### Fresh-time validation after lock wait

Use controlled time and deterministic lock coordination.

Exercise a creation operation that must wait for the required Resource lock.

Advance authoritative test time while it waits so that an otherwise valid request crosses a time-validation boundary.

After acquiring the lock, the creation must validate using fresh time and return the result implied by the new authoritative time.

Do not satisfy this test merely by capturing time before blocking.

## Existing verification

All accepted Delivery 1 and Task 2A verification must continue to pass.

The repository verification entry point must remain authoritative and must not silently skip the new Task 2B tests.

## Explicitly out of scope

Do NOT implement:

- booking HTTP routes;
- booking HTML form;
- browser request-id generation;
- populated Resource schedule;
- My Bookings;
- cancellation application behavior;
- coordinator cancellation;
- Resource create/edit/activation/deactivation application workflow;
- activity-history UI;
- booking rescheduling;
- recurring bookings;
- Task 2C browser tests.

Do not scaffold those features merely because they are described in the full architecture.

## Implementation authority

The implementation agent may independently:

- inspect existing code and accepted persistence;
- choose package/API details consistent with repository conventions;
- implement the Task 2B application service;
- make minimal migration/permission changes required by the approved lock protocol;
- add focused unit and PostgreSQL integration tests;
- introduce narrow transaction/test seams needed for atomicity verification;
- run focused and full verification;
- diagnose and repair Task-2B-related failures;
- document meaningful implementation choices.

Routine implementation, verification, and in-scope repair do not require additional human permission.

Escalate only according to `AGENTS.md`.

## Completion criteria

Task 2B is ready for independent review only when:

- canonical request handling is implemented;
- authoritative-time validation is implemented;
- the approved Account → replay lookup → Resource transaction/lock protocol is implemented;
- successful creation atomically persists Booking + Activity event;
- replay and changed-request behavior are implemented;
- concurrency behavior is covered by deterministic real-PostgreSQL tests;
- rollback behavior is covered;
- accepted Delivery 1 and Task 2A verification still passes;
- no Task 2C HTTP/UI behavior has been introduced;
- the final diff has been inspected for unrelated changes.

Finish with exactly one of:

READY_FOR_REVIEW  
BLOCKED  
INCOMPLETE

For `READY_FOR_REVIEW`, report:

- base revision;
- exact candidate state;
- implementation decisions;
- transaction and lock behavior implemented;
- verification commands actually executed;
- observed results;
- limitations;
- anything not verified;
- deviations, if any.
