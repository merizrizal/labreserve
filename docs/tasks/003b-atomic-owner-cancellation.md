# Task 3B — Atomic Owner Cancellation

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
- Task 3A read-only My Bookings

## Objective

Implement the application-level owner cancellation use case.

An authenticated booking owner must be able to cancel a future
Confirmed Booking.

Cancellation must:

- enforce ownership;
- preserve the retained Booking;
- serialize correctly with booking creation;
- observe authoritative time after lock acquisition;
- transition Confirmed to Cancelled;
- release the reserved interval;
- atomically append one cancellation Activity event;
- treat repeated authorized cancellation as a successful no-op.

This task implements the application service and persistence behavior.

It does not implement cancellation HTTP routes or browser UI.

## Authoritative cancellation rules

### Ownership

Only the booking owner may cancel a booking through this service.

Both Engineers and Coordinators may cancel their own bookings.

Coordinator cancellation of another user's Booking is out of scope.

The authenticated actor must be supplied through the trusted
application-service boundary.

Do not accept an arbitrary owner ID in the cancellation command
as authorization.

### Confirmed future booking

A Confirmed Booking may be cancelled only when:

    authoritative_now < booking.start_at

The exact start boundary is not cancellable.

A booking that has started or ended cannot transition from
Confirmed to Cancelled through owner cancellation.

### Already-cancelled booking

An already-Cancelled Booking produces a successful no-op after
ownership authorization.

Do not re-check its original start-time eligibility.

The behavior remains successful even if the original start time
has passed.

Do not:

- update cancellation metadata;
- create another Activity event;
- generate a second cancellation transition.

Return the retained Booking or an equivalent explicit successful
domain outcome.

### Unknown or unauthorized Booking

Unknown Bookings and Bookings not owned by the authenticated actor
must not be cancellable.

The service must not disclose sensitive information about another
owner's booking through its cancellation outcomes.

Use a consistent not-found/not-accessible domain outcome unless
the accepted architecture explicitly establishes another policy.

### Resource status

An inactive Resource does not prevent cancellation of its owner's
existing future Confirmed Booking.

Resource activity determines creation eligibility, not whether
an existing booking may be cancelled.

### Owner cancellation reason

Owner cancellation does not require a reason.

Preserve the accepted cancellation metadata schema.

Do not introduce coordinator-specific reason handling in Task 3B.

## Transaction and locking protocol

Use one explicit PostgreSQL READ COMMITTED transaction.

The required lock order is:

    Resource → Booking

Do not acquire the Booking row lock before the Resource row lock.

### Required operation

1. Begin transaction.

2. Resolve the target Booking's Resource identity without
   acquiring a Booking row lock.

3. Establish that the authenticated actor is permitted to access
   the Booking for cancellation.

4. If the Booking is unknown or not owned, return the approved
   inaccessible/not-found domain outcome.

5. Lock the associated Resource row.

6. Lock the Booking row.

7. Re-read and validate the authoritative Booking state after
   acquiring the locks.

8. Re-check ownership.

9. If already Cancelled, return successful no-op without mutation.

10. Capture fresh authoritative current time after lock acquisition.

11. If Confirmed but no longer future, reject cancellation.

12. Transition the Booking to Cancelled.

13. Set cancellation metadata according to the existing schema.

14. Append one booking-cancelled Activity event in the same
    transaction.

15. Commit once.

The initial Booking lookup must not hold a Booking row lock while
waiting to acquire the Resource lock.

The implementation must remain compatible with the Task 2B
booking-creation locking protocol.

Do not introduce process-local mutexes, caches, distributed locks,
or a different global lock ordering.

## Concurrent state changes

The initial lookup identifies the Resource associated with the
Booking.

After acquiring the required locks, authoritative state must be
re-read rather than assuming the initial lookup remains current.

The implementation must not create a deadlock with the accepted
booking-creation protocol.

Preserve the approved immutability of Booking resource identity.

## Authoritative clock

Use the existing injectable Clock abstraction.

Clock-relative cancellation eligibility must use fresh time
captured after required lock waits.

Do not use a timestamp captured when the request first arrived.

Use precision consistent with the established PostgreSQL
microsecond timestamp contract.

Tests must be able to control time without adding a production
time-changing endpoint.

## State transition

The only state change implemented here is:

    Confirmed → Cancelled

Preserve:

- Booking ID;
- Resource identity;
- owner account;
- original request identifier;
- start and end instants;
- canonical purpose.

Set the existing cancellation actor/time metadata appropriately.

Do not physically delete a Booking.

Do not introduce new states such as:

- Expired;
- Completed;
- In use;
- Past.

These are not persisted business states.

## Activity-event atomicity

A successful first cancellation must create exactly one
booking-cancelled Activity event.

The event must identify:

- authenticated cancellation actor;
- the affected Booking;
- the cancellation action;
- operation timestamp;
- relevant non-sensitive details consistent with the existing
  Activity model.

Booking transition and Activity insertion must occur within the
same PostgreSQL transaction.

If Activity insertion fails:

- the cancellation transition must roll back;
- the Booking must remain Confirmed;
- its interval must remain occupied;
- no successful cancellation Activity event may remain.

An already-Cancelled no-op must create no additional event.

An unauthorized or rejected cancellation must create no
successful cancellation event.

## Idempotency

Cancellation is idempotent by the retained Booking's terminal
Cancelled state.

Do not introduce another cancellation request-id table.

Do not require a new cancellation idempotency key.

After a successful cancellation:

    repeated authorized cancellation
        → successful no-op

The persisted original creation request ID must remain unchanged.

The Task 2B booking-creation replay contract must continue to work
for retained Cancelled bookings.

## Interval release

The existing Task 2A exclusion constraint remains authoritative.

Cancelling a Confirmed Booking must release its interval from
the confirmed-booking overlap constraint.

After cancellation commits, another valid booking creation may
reserve the same interval when it otherwise satisfies Task 2B's
creation requirements.

Do not delete historical booking records to release intervals.

Do not duplicate the exclusion constraint in application code.

## Domain/API boundary

Implement a narrow internal owner-cancellation application API.

It must accept the authenticated Actor separately from the
target Booking identity.

Return explicit outcomes sufficient to distinguish:

- successful first cancellation;
- successful already-cancelled no-op;
- Booking not found or not accessible;
- confirmed Booking no longer cancellable because its start
  time has arrived;
- operational/database failure.

Do not embed browser HTTP status codes in the domain service.

Follow existing application conventions where appropriate.

Do not introduce cancellation handlers or routes in this task.

## Database privilege boundary

Inspect the existing runtime database privileges and Task 2B
row-lock helpers.

If cancellation requires additional privileges, use the smallest
migration-managed capability consistent with the accepted
least-privilege design.

Do not grant broad UPDATE permissions on business tables merely
to simplify cancellation.

The runtime must not gain arbitrary authority to modify:

- Booking ownership;
- Resource identity;
- start/end timestamps;
- canonical purpose;
- original creation request identifier;
- Account roles;
- Resource configuration.

If a SECURITY DEFINER helper is introduced, inspect and enforce
at minimum:

- narrow mutation capability;
- explicit object qualification;
- fixed safe search_path;
- least-privileged function ownership;
- revoked PUBLIC EXECUTE;
- tightly scoped runtime EXECUTE permission;
- no unnecessary dynamic SQL;
- authorization and transition guards appropriate to its role.

Any privileged helper must not silently bypass the application
service's authorized cancellation protocol.

Document meaningful privilege decisions.

## Required verification

Use real PostgreSQL for transaction, privilege, and concurrency
behavior.

### Ownership

Verify:

- Engineer A can cancel their own future Booking;
- Engineer A cannot cancel Engineer B's Booking;
- Coordinator can cancel their own future Booking;
- Coordinator cannot use this owner service to cancel someone
  else's Booking;
- arbitrary caller-supplied Booking identifiers do not grant
  cancellation authority.

Verify unauthorized attempts produce no Booking mutation or
Activity event.

### Cancellation boundaries

Using controlled time, verify:

- Confirmed Booking with now < start can be cancelled;
- exactly one microsecond before start can be cancelled;
- exactly at start cannot be cancelled;
- after start cannot be cancelled;
- after end cannot be cancelled.

Use the approved canonical timestamp precision.

Verify a future Booking on an inactive Resource remains
cancellable by its owner.

### State retention

After cancellation, verify:

- state becomes Cancelled;
- cancellation actor is correct;
- cancellation timestamp is correct;
- original owner remains unchanged;
- Resource remains unchanged;
- interval remains unchanged;
- purpose remains unchanged;
- original request ID remains unchanged;
- Booking remains queryable;
- existing My Bookings and schedule read models continue to
  include it as Cancelled.

Do not add new browser functionality merely for this assertion.

### Successful cancellation event

Verify:

- exactly one cancellation Activity event commits;
- actor and affected Booking are correct;
- existing creation Activity event remains unchanged;
- repeated cancellation does not append another event.

### Already-cancelled replay

Verify:

- repeated owner cancellation is successful;
- it preserves the original cancellation metadata;
- no second event is created;
- it remains successful after the original start instant;
- non-owners cannot use the no-op behavior.

### Atomic rollback

Force Activity-event insertion to fail after the Booking
transition would otherwise succeed.

Verify:

- transaction rolls back;
- Booking remains Confirmed;
- its original interval remains occupied;
- no cancellation event commits.

Remove the failure condition and retry.

Verify the subsequent authorized cancellation succeeds with
exactly one cancellation event.

### Concurrent identical cancellation

Run two independent cancellation operations against the same
future Booking.

Verify:

- exactly one state transition commits;
- exactly one cancellation event commits;
- both authorized callers receive successful outcomes;
- the second operation observes the retained Cancelled state.

Use deterministic lock coordination where practical.

Do not use arbitrary sleeps as the primary synchronization
mechanism.

### Cancellation versus creation

Use real concurrent PostgreSQL transactions.

Exercise cancellation and creation for overlapping intervals on
the same Resource.

Verify both important orderings.

If cancellation commits first:

- a subsequently eligible new creation may succeed;
- no overlapping Confirmed Bookings remain.

If creation obtains the Resource lock while the original Booking
is still Confirmed:

- the overlapping creation receives the existing conflict outcome;
- cancellation may subsequently succeed;
- the failed creation does not consume its request identifier.

Do not weaken Task 2B's overlap or replay behavior.

### Fresh time after lock wait

Use deterministic Resource-lock coordination and controlled time.

Create a future Confirmed Booking.

Start cancellation while the relevant Resource lock is held
elsewhere.

Advance authoritative time across the booking start boundary
while cancellation is waiting.

Release the Resource lock.

Verify cancellation now rejects the transition because the
Booking is no longer future.

The test must fail if time was captured before the lock wait.

Verify the Booking remains Confirmed and no cancellation event
was created.

### Privilege regression

Verify that any new runtime database permissions:

- permit the approved cancellation operation;
- do not permit arbitrary business UPDATE/DELETE;
- do not permit role/ownership changes;
- do not permit unrestricted schema modification.

If privileged functions are introduced, verify their catalog
properties and effective permissions.

### Migration lifecycle

If a new migration is required:

- include it in the checksummed migration ledger;
- refuse ordinary startup when the required migration is absent;
- apply it only through the explicit migration path;
- preserve existing Booking and Activity records;
- make repeated migration execution safe;
- preserve accepted runtime privilege restrictions.

### Creation replay compatibility

Create a Booking using Task 2B.

Cancel it using Task 3B.

Replay its original creation request using the same actor,
request identifier, and canonical data.

Verify:

- the original Booking is returned;
- it remains Cancelled;
- no second creation event is added;
- no new Booking is created.

### Restart durability

Cancel a Booking successfully.

Restart the application/database using the existing
verification lifecycle.

Verify cancellation state and event remain persisted.

Repeat authorized cancellation and verify successful no-op
without another event.

## Existing verification

All accepted Delivery 1, Delivery 2, and Task 3A tests must
continue to pass.

The normal repository verification entry point must include
Task 3B tests.

Do not silently skip real-PostgreSQL integration tests.

## Documentation

Update README to distinguish:

Implemented:
- internal atomic owner cancellation service;
- retained cancellation state;
- interval release;
- idempotent cancellation;
- cancellation Activity atomicity.

Deferred:
- browser cancellation form/button;
- cancellation HTTP handlers;
- coordinator cancellation of another owner;
- cancellation reason workflow;
- Resource management;
- activity-history UI;
- rescheduling.

## Explicitly out of scope

Do NOT implement:

- cancellation HTTP routes;
- cancellation buttons or browser forms;
- Task 3C browser workflow;
- coordinator cancellation of another user's Booking;
- mandatory coordinator cancellation reasons;
- booking rescheduling;
- Resource management;
- activity-history UI;
- recurring bookings;
- notifications or external integrations.

Do not introduce future workflow scaffolding.

## Implementation authority

The implementation agent may independently:

- inspect existing Booking/Activity persistence;
- design the internal owner cancellation service;
- implement the approved Resource → Booking locking protocol;
- make minimal migration-managed privilege changes;
- add focused domain and real-PostgreSQL tests;
- implement deterministic concurrency verification;
- run focused and complete verification;
- repair in-scope defects;
- update implementation documentation.

Follow AGENTS.md.

Escalate genuinely conflicting requirements or architecture
decisions rather than silently redesigning the protocol.

## Completion criteria

Task 3B is ready for independent review only when:

- owner authorization is enforced;
- future Confirmed Bookings can be cancelled;
- started/past Confirmed Bookings cannot be cancelled;
- authorized already-Cancelled replay succeeds without mutation;
- Resource → Booking lock ordering is implemented;
- authoritative time is captured after lock acquisition;
- Booking transition and Activity event commit atomically;
- interval release is correct;
- concurrency behavior is verified with real PostgreSQL;
- Task 2B creation/replay remains correct;
- least-privilege boundaries remain intact;
- all prior accepted verification passes;
- README is accurate;
- no Task 3C or deferred workflow was introduced;
- final diff has been inspected.

Finish with exactly one of:

READY_FOR_REVIEW
BLOCKED
INCOMPLETE

For READY_FOR_REVIEW report:

- implementation base revision;
- exact candidate state;
- service API and outcomes;
- transaction and lock protocol;
- privilege/migration decisions;
- concurrency verification;
- atomicity verification;
- commands actually executed;
- observed test results;
- anything not verified;
- known limitations;
- deviations, if any.
