# Task 2A — Booking persistence and database invariants

## Status

Approved for implementation.

## Parent delivery

Delivery 2 — Reserve correctly.

## Objective

Introduce the minimum PostgreSQL persistence model required for
LabReserve booking creation and prove the database-enforced booking
invariants independently of the future HTTP and application-service
workflow.

This task establishes persistence guarantees only.

It does not implement booking creation as a user-facing capability.

## Requirements

### Booking persistence

Add the Booking persistence model required by the approved architecture.

A Booking must persist:

- stable identifier;
- resource identifier;
- authenticated owner account identifier;
- immutable request identifier;
- start instant;
- end instant;
- canonical trimmed purpose;
- state: Confirmed or Cancelled;
- creation instant;
- cancellation metadata required by the approved architecture.

Do not store Upcoming, In use, or Past as business state.

The database must preserve referential relationships to the existing
Account and Resource records.

### Request identity

The database must enforce uniqueness of:

(owner_account_id, request_id)

across all Booking states.

Cancelled and past Bookings retain their request identifiers.

This task establishes persistence and uniqueness only.

Replay interpretation and application idempotency behavior belong to
Task 2B.

### Static booking invariants

The database must reject persisted Booking data that violates static
invariants that can be enforced without consulting the current clock.

At minimum:

- end must be after start;
- duration must be at least 30 minutes;
- duration must be at most 8 hours;
- persisted purpose length must be between 1 and 200 Unicode code
  points;
- Booking state must be one of the approved states;
- cancellation metadata must remain structurally consistent with state.

Clock-relative requirements such as:

- start at least five minutes in the future;
- start no more than 30 days in the future;

belong to Task 2B application validation and must not be implemented as
time-dependent database checks.

### Overlap invariant

PostgreSQL must prevent overlapping Confirmed Bookings for the same
Resource.

Intervals use start-inclusive/end-exclusive semantics:

[start, end)

Therefore:

10:00–11:00
11:00–12:00

may coexist.

But overlapping, identical, enclosing, and contained intervals for the
same Resource must conflict.

Bookings for different Resources do not conflict.

Cancelled Bookings remain retained but do not occupy their former
interval.

Use the approved PostgreSQL exclusion-constraint architecture.

The database constraint is authoritative even when writes bypass future
application-level locking.

### Activity persistence

Introduce only the activity-event persistence required for future
Booking creation to atomically append its required event.

The persistence model must support:

- actor account;
- action;
- server occurrence instant;
- affected Booking or Resource;
- relevant action details.

An event references exactly one supported affected-record type.

Do not implement:

- activity-history UI;
- event sourcing;
- background event processing;
- asynchronous delivery;
- speculative event types unrelated to approved v0.1 behavior.

Initial seed accounts/resources remain bootstrap state and must not
generate product activity events.

## Concurrency verification

Use real PostgreSQL and independent connections/transactions.

Add deterministic integration coverage proving the exclusion constraint
itself remains correct when concurrent writes bypass future application
resource locking.

For two concurrent conflicting Confirmed Booking inserts against the
same Resource:

- exactly one transaction may commit;
- the other must receive the expected overlap/exclusion failure;
- exactly one matching Confirmed Booking must remain.

Do not use arbitrary sleeps as the primary concurrency synchronization
mechanism.

## Required integration coverage

Cover at least:

- valid Booking persistence;
- end equal to start rejected;
- end before start rejected;
- duration below 30 minutes rejected;
- duration above 8 hours rejected;
- 30-minute duration accepted;
- 8-hour duration accepted;
- purpose lower and upper length boundaries;
- invalid Booking state rejected;
- owner foreign key;
- Resource foreign key;
- duplicate owner/request identifier rejected;
- the same request identifier allowed for different owners;
- partial overlap rejected;
- contained overlap rejected;
- enclosing overlap rejected;
- identical overlap rejected;
- adjacent intervals accepted;
- identical intervals on different Resources accepted;
- Cancelled Booking does not block another Booking for the interval;
- concurrent conflicting inserts produce one success and one conflict.

Use the real PostgreSQL database for tests of PostgreSQL constraints.

## Migration/lifecycle requirements

Add this schema through the existing explicit migration mechanism.

Normal application startup must continue to reject incompatible
migration state.

Existing Delivery 1 data must survive migration.

Repeated application startup and ordinary seeding must remain
non-destructive.

Do not modify existing seed data merely to satisfy tests.

## Explicitly out of scope

Do NOT implement:

- booking HTTP routes or forms;
- booking creation service/use case;
- request replay/idempotency behavior;
- Account → Resource application lock protocol;
- current-time booking validation;
- My Bookings;
- schedule population;
- cancellation behavior;
- coordinator cancellation;
- resource-management changes;
- activity-history UI;
- Delivery 2C browser flows.

Do not scaffold these merely because later tasks will need them.

## Implementation authority

The implementation agent may independently:

- inspect the existing repository;
- select migration/schema implementation details consistent with the
  approved architecture;
- implement Task 2A;
- add focused unit/integration tests;
- run verification;
- diagnose and repair Task-2A-related failures;
- make small implementation choices within the approved architecture.

Routine implementation, testing, and in-scope repair do not require
additional approval.

Escalate only according to AGENTS.md.

## Completion criteria

Task 2A is ready for independent review only when:

- the migration is implemented;
- the required PostgreSQL constraints exist;
- required focused integration coverage passes;
- existing Delivery 1 verification still passes;
- the implementation agent has inspected the final diff;
- no Task 2B/2C behavior has been introduced.

The implementation agent must finish with:

READY_FOR_REVIEW
BLOCKED
or
INCOMPLETE

and report:

- base revision;
- exact candidate state;
- implementation decisions;
- migrations/schema changed;
- verification actually executed;
- observed results;
- anything not verified;
- limitations or deviations.
