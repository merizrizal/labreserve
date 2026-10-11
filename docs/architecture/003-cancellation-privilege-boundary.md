# Cancellation Database Privilege Boundary

## Decision

Task 3B's cancellation service is accepted.

The shared PostgreSQL runtime role has column-specific UPDATE
privileges for Booking cancellation fields.

The trusted application service is responsible for:

- authenticated ownership;
- cancellation eligibility;
- transaction locking;
- idempotency;
- cancellation Activity atomicity.

## Known limitation

Code with direct access to the runtime database role can bypass
the cancellation service and directly modify permitted columns.

This can include:

- cancelling another owner's Booking;
- rewriting cancellation metadata;
- reversing cancellation;
- modifying cancellation state without an Activity event.

The database privilege boundary alone does not prevent these
operations.

## Risk acceptance

Accepted for the current LabReserve development exercise.

Not considered independently hardened against arbitrary SQL
execution using runtime database credentials.

## Future hardening

Evaluate:

- narrowly guarded database mutation functions;
- revoking direct cancellation-column UPDATE grants;
- enforcing allowed state transitions at the database boundary;
- stronger database audit guarantees;
- minimizing which processes possess runtime credentials.

Any hardening must preserve the approved Resource → Booking
lock ordering and application transaction guarantees.

## Revisit before

- production deployment;
- allowing additional services to use runtime credentials;
- introducing arbitrary SQL execution capabilities;
- changing the database trust model.
