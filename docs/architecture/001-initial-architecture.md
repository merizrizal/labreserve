# 001 — Initial LabReserve architecture

- **Status:** Approved for Delivery 1 implementation (foundation only).
- **Decision authority:** Case-insensitive resource-code identity in section 2 and bootstrap history in section 9 are approved product decisions. Architecture approval is limited to Delivery 1; later deliveries still require separate approval. Implementation defaults are distinguished in section 15.
- **Scope:** LabReserve v0.1 proof of concept, as defined by [the PRD](../prd.md).
- **Initial design context:** [Client baseline](../baseline.md). This architecture was written before the application, schema, and tests existed; Delivery 1/2 foundations and booking creation/schedules are now present.
- **Approval history:** The initial architecture approval covered Delivery 1 only. The separately approved [Task 2A](../tasks/002a-booking-persistence.md), [Task 2B](../tasks/002b-atomic-booking-creation.md), and [Task 2C](../tasks/002c-booking-web-workflow.md) supplied Delivery 2's authority. The client now reports Delivery 2 complete. Cancellation, My Bookings, resource management, and completed activity views are not implemented merely because this document designs them.
- **Delivery 3 decisions:** [002 — My Bookings and self-service cancellation](002-my-bookings-and-cancellation.md) records the approved owner-cancellation semantics and Resource → Booking lock order alongside the broader proposed design. Decision approval does not itself authorize implementation.

**Current reading guide:** The Delivery 1 approval statements below are historical. Task 2B subsequently settled canonical request equivalence/purpose counting, and Task 2C settled Jakarta-day intersection filtering; those are no longer unresolved gates in sections 5 and 15. Owner cancellation and authorized already-Cancelled replay, including after original start, are now approved in [architecture 002, section 4](002-my-bookings-and-cancellation.md#4-cancellation-transaction-and-lock-protocol). Coordinator cross-owner cancellation/reason details remain deferred. The addendum describes the current implementation seams and proposed Delivery 3 boundary; it does not approve coordinator intervention or other deferred workflows.

## 1. Application boundaries and components

Use a **modular monolith**: one long-running Go web application, one PostgreSQL database, and server-rendered HTML. Docker Compose supplies local containers; Playwright supplies browser verification. All recommended technologies fit the requirements; none needs replacement.

```text
Browser
  │ HTTP forms and HTML; authenticated session and CSRF protection
  ▼
Go web layer: routing, request decoding, templates, safe error mapping
  │ authenticated actor; typed commands and queries
  ├── Authentication: credentials, sessions, current account
  ├── Resources: catalog, metadata, active-state eligibility
  ├── Bookings: creation/replay, schedules, My Bookings, cancellation
  └── Activity: transactional event append; coordinator-only history queries
             │ explicit transaction-scoped SQL
             ▼
         PostgreSQL
```

- The web layer renders forms, lists, and tables using `html/template`. Small JavaScript enhancements are optional; ordinary workflows do not depend on a SPA, CDN, or browser-side timezone conversion.
- Application services own authorization, business validation, transaction boundaries, and the lock protocol. They accept an actor resolved by authentication, never a browser-supplied actor or role.
- Resources owns resource rules; Bookings coordinates resource eligibility with reservation writes. Activity exposes an append operation taking the caller's transaction and a restricted history reader. It does not independently commit events.
- Small SQL repositories use parameterized queries with explicit transaction ownership; `pgx` is the recommended driver, selectable during Delivery 1. No ORM, generic repository framework, service-to-service calls, event bus, or speculative plugin boundaries are needed.
- A shared clock, configuration loader, and database pool are technical dependencies, not separate services. Transaction-scoped collaborators cannot silently open a second connection for part of a mutation.
- There is no infrastructure integration: a resource named Kubernetes lab is a catalog entry, not a cluster connection. No microservices, Kubernetes deployment, Redis, brokers, event sourcing, external identity providers, or infrastructure automation are introduced.

## 2. Domain records and relationships

Use stable server-generated record identifiers; UUIDs are a recommended encoding, not a required product contract. Resource codes are human-readable identifiers, not primary keys. Account login identifiers are distinct from display names. Resource code identity is resolved by the approved decision below; other text validation questions are gated in section 15.

**Approved product decision — resource-code identity:** resource codes are case-insensitive identifiers. `NET-01`, `net-01`, and `Net-01` represent the same resource code and must not coexist as separate resources. Initial seed resources use the canonical uppercase spellings `NET-01`, `K8S-01`, and `DEMO-01`. This does not require uppercase input or impose arbitrary maximum lengths or allowed-character patterns. Select an appropriate database-enforced case-insensitive uniqueness approach during Delivery 1; resource lookup and seed conflict handling must use the same identity semantics. Application-only duplicate checks are insufficient.

| Record | Fields and invariants |
| --- | --- |
| Account | ID, unique login, fictional display name, password hash with algorithm parameters/salt, role `Engineer` or `Coordinator`. No registration, account administration, or recovery endpoints. |
| Resource | ID, immutable code with database-enforced case-insensitive uniqueness, name, description, active flag, creation/update instants. Retained permanently until explicit reset. |
| Booking | ID, resource ID, authenticated owner account ID, immutable request identifier, start/end instants, trimmed purpose, state `Confirmed` or `Cancelled`, creation instant; cancellation actor/instant populated only after cancellation. `UNIQUE(owner_account_id, request_id)` across all states. Resource, owner, request identifier, times, and purpose are immutable in v0.1; together they retain successful request identity/data until reset. |
| Activity event | ID, actor account ID, action, server occurrence instant, affected resource or booking ID, relevant action-specific details. Insert-only through the application; JSONB is a suggested details encoding. |
| Session | Digest of an opaque token, optional account ID for anonymous login sessions, CSRF secret, creation/expiry instants. Technical authentication state, not booking business state. |

Relationships:

- An Account owns many Bookings and performs many Activity events; it may hold multiple authenticated Sessions.
- A Resource has many Bookings. Each Booking belongs to exactly one Resource and one owner.
- Every accepted booking creation has one request identifier stored on its Booking and one creation event; cancellation adds at most one cancellation event. Request identity is a logical product contract, not a separate table.
- The Booking's authenticated owner scopes its request identifier. Its immutable resource, times, and purpose supply the canonical replay data; no duplicated request snapshot or pre-booking foreign key is needed.
- An Activity event references exactly one affected Resource or Booking, enforced by foreign keys plus an exclusive-target check. Details hold relevant snapshots, such as old/new names, booking interval/purpose, or cancellation reason; a renamed resource does not erase the meaning of an earlier change.

Constrain roles and booking states in the database; text columns with checks are a suggested encoding. No stored Upcoming, In use, or Past state is allowed. Database checks enforce end after start, duration bounds, purpose length, and valid cancellation metadata. The clock-relative booking horizon is application validation, not a time-dependent database check. Exact DDL and indexes belong to the delivery introducing each record; this full-v0.1 model does not require Delivery 1 to create deferred booking/history structures.

## 3. Authentication

Use seeded local accounts and **opaque server-side sessions stored in PostgreSQL**, not JWTs or an external authentication provider (FR-001–FR-004).

1. Seed two fictional engineers and one fictional coordinator. Development-only passwords are supplied through the documented local setup, hashed before storage, and never printed in application logs. Repeated seeding does not overwrite passwords or existing roles.
2. Use Argon2id with a random salt per account and stored algorithm parameters; compare derived hashes in constant time. The maintained `golang.org/x/crypto` package is recommended. Select and verify reviewed memory-hard parameters during Delivery 1 rather than fixing tuning values here.
3. Generate high-entropy session tokens using a cryptographically secure source. Store only a token digest; the bearer secret is carried in a cookie, never a query string. A 32-byte token and SHA-256 digest are recommended implementation defaults.
4. The cookie is `HttpOnly`, `SameSite=Lax`, host-only, and `Path=/`. Use `Secure` with HTTPS. Only the explicitly loopback HTTP development mode omits `Secure`; it must not silently become a remotely exposed deployment mode.
5. Use an absolute 12-hour authenticated session lifetime as an engineering default. Resolve the account and current role from the database on every authenticated request; the browser cannot set either. Expired/revoked tokens grant no access.
6. Establish a short-lived anonymous session for the login form's CSRF token. On successful login, rotate both session token and CSRF secret, invalidate the anonymous session, and associate the new session with the authenticated account. Avoid session fixation and generic role-selection login.
7. Sign-out is a CSRF-protected POST that revokes the session in PostgreSQL and clears its cookie. Subsequent reuse of that token fails, including after application restart.

Use synchronizer CSRF tokens on login and every state-changing form, including sign-out; validate them server-side and additionally reject unexpected origins. GET requests never mutate business state. Hide credentials, cookies, CSRF secrets, and request bodies containing passwords from logs. Use generic credential-failure messages, escaped templates, and no-store caching for authenticated pages. Session expiry cleanup may remove expired technical records; it must never remove booking retry identities or business history.

## 4. Authorization and browser boundary

| Capability | Engineer | Coordinator |
| --- | --- | --- |
| Browse resources, schedules, and any booking's owner/time/purpose | Yes | Yes |
| View My Bookings | Own records | Own records |
| Create booking | For self only | For self only |
| Cancel own confirmed booking before start | Yes | Yes |
| Repeat own already-Cancelled cancellation, regardless of original start | Authorized successful no-op | Authorized successful no-op |
| Cancel someone else's confirmed booking before start | No | Yes, with reason; later capability |
| Cancel confirmed booking at/after start | No | No |
| Create/edit/activate/deactivate resources | No | Yes |
| Inspect activity history | No | Yes |

Authentication middleware protects all resource/booking/history routes. Application services repeat operation-specific permission checks, including ownership on already-cancelled bookings. Templates hide unauthorized actions but are not a security boundary. My Bookings always filters by the authenticated account, not a submitted owner or cursor field.

Route names are delivery-time defaults, not architectural contracts. A suggested route set uses ordinary GET views and POST forms: `/login`, `/logout`, `/resources`, `/resources/{id}?date=...`, `/resources/{id}/bookings/new`, `/bookings`, `/bookings/{id}`, `/bookings/{id}/cancel`, `/my-bookings`, coordinator resource create/edit/activation forms, and `/activity`. No separate JSON API is needed.

Recommended HTTP mapping, selectable with the relevant delivery: successful POSTs redirect with HTTP 303 to a detail or list view. An identical booking replay redirects to the same booking ID. Direct unauthenticated requests redirect to login without returning protected data; forbidden operations return 403. Invalid input returns 422, inactive-resource/conflicting-booking/changed-request reuse and blocked deactivation return 409, and unknown records return 404. CSRF failure returns 403 before mutation. Unexpected failures return a generic 500, or 503 for transient database unavailability, with a safe internal correlation identifier. Do not expose SQL, constraint names, or database errors in HTML.

## 5. Booking lifecycle, retained views, and cancellation

```text
                         authorized cancellation, now < start
Confirmed  ──────────────────────────────────────────────────► Cancelled

Confirmed presentation: now < start       → Upcoming
                        start ≤ now < end → In use
                        end ≤ now         → Past
Cancelled presentation: always Cancelled
```

- A new booking validates trimmed purpose length 1–200, start in `[now + 5 minutes, now + 30 days]`, positive duration in `[30 minutes, 8 hours]`, active resource, and no overlap. The horizon constrains start only. Crossing midnight and simultaneous bookings of different resources are allowed.
- Issue a stable request identifier when showing the booking form, retain it across validation errors and network retries, and submit it as a hidden field. A random UUID is a suggested encoding. This identifier grants no permission. A fresh intentional booking uses a new identifier; PRG reduces accidental resubmission but is not the retry guarantee.
- Cancellation first resolves the booking's immutable resource ID, locks Resource → Booking in that required order, re-reads the Booking, and checks ownership before any transition or no-op. For a Confirmed booking, capture fresh time after all required locks and require `now < booking.start_at`; reject at the exact start instant and thereafter. Owner cancellation needs no reason for either role. Update state/metadata and append exactly one cancellation event in the same transaction. Coordinator cancellation of someone else's booking and its required reason are a separate deferred capability.
- Cancellation releases the interval at commit, when the row no longer participates in the Confirmed-only partial exclusion constraint. Retain the PostgreSQL Booking's original ID, resource, owner, interval, purpose, creation instant, creation request identifier, and creation event. No booking row is deleted, and no early-release or rescheduling operation exists.
- **Approved owner-cancellation replay:** after ownership authorization, an already-Cancelled booking returns successful no-op even at or after its original start, changing neither cancellation metadata nor history. The time guard applies only to the Confirmed → Cancelled transition. Non-owners cannot use the no-op. See [architecture 002, section 4](002-my-bookings-and-cancellation.md#4-cancellation-transaction-and-lock-protocol); coordinator cross-owner retry/reason details are not settled by this approval.

### Date filtering and pagination

**Proposed selected-date semantics, awaiting confirmation:** include every booking intersecting the selected Asia/Jakarta day, including retained cancelled and past records. Compute local midnight `day_start` and next local midnight `day_end`, then filter `start < day_end AND end > day_start`. An interval ending exactly at midnight does not appear on the following day. Show the original full interval, mark continued/cross-midnight occupancy, and clearly mark cancelled rows as not occupying the resource. This avoids hiding a booking that began the previous day; it is not yet a settled product requirement.

Schedules and My Bookings require 25-record pages, retained cancelled/past rows, and a documented stable order with a unique tie-breaker (FR-018). Recommended defaults are **`start_at ASC, id ASC`** and keyset next/previous cursors; select and document the concrete ordering/navigation in the delivery introducing non-empty lists. Scope/date/ownership filters remain server-controlled or validated independently of navigation input. Activity ordering/page size are not prescribed by the PRD; **`occurred_at DESC, id DESC`** and 25-record pages are suggested defaults for the activity delivery. Identifiers supply deterministic tie-breakers, not causal ordering. Pagination need not provide a frozen multi-request snapshot; newly committed records may appear on refresh.

## 6. Time and timezone contract

Persist booking, session, and history instants as PostgreSQL `timestamptz`; initialize database connections in UTC. PostgreSQL stores instants, not the original display timezone. The application formats all booking times using the IANA location **Asia/Jakarta**, visibly labelled on forms, lists, and detail views.

- Inject a Go `Clock` dependency. Production reads the application server's wall clock in UTC; tests supply controlled time. Never accept `now` from the browser.
- Capture fresh authoritative time **after acquiring the required locks**, immediately before final time-dependent validation and mutation. Do not use PostgreSQL transaction-start `now()` for eligibility after a lock wait. Use this operation timestamp for the associated mutation/event, and one captured time per rendered page for consistent labels. Successful replay does not repeat new-booking horizon checks.
- Define 30 days as 30 × 24 hours. Duration and horizon calculations use instants, not browser-local calendar arithmetic.
- The selected browser timestamp contract is HTML `datetime-local` entry with HTTP representation `YYYY-MM-DDTHH:mm` interpreted solely in Asia/Jakarta. Suggested field names are `start_jakarta` and `end_jakarta`; names can be selected during booking delivery. Parse on the server with that location. This is the equally unambiguous fixed-zone representation allowed by NFR-001; it must never be interpreted in the browser's timezone or the host's default timezone. It works without JavaScript.
- Do not offer a generic offsetless timestamp field. If an offset-bearing timestamp representation is added to a form/HTTP contract, require RFC 3339 with an explicit offset, normalize to the same UTC instants, and reject unsupported precision rather than silently round. No additional API is authorized by this design.
- Canonical timestamp precision must be explicit and consistent across decoding, persistence, replay comparison, and boundary tests. PostgreSQL microseconds are the recommended precision; minute-resolution browser entry is a subset. Finalize this with the request-data equivalence decision before booking delivery, not as an implicit rounding policy.
- Parse the selected date in Jakarta, not UTC. Load/ship timezone data with the Go application (for example embedded `time/tzdata`), so behavior does not depend on the container host or an external timezone service.

## 7. Database and transaction strategy

Use a supported pinned PostgreSQL major, a bounded connection pool, SQL migrations, and explicit **READ COMMITTED** transactions. Select and pin the supported database release, driver/pool (recommended `pgx`/`pgxpool`), and dependency/container versions during Delivery 1; no particular major is fixed here. A named Compose volume holds demonstration data across restarts.

- Foreign keys retain ownership/resource/history relationships. Database-enforced uniqueness protects case-insensitive resource codes, login identifiers, and `(owner_account_id, request_id)` on Booking. Select the resource-code uniqueness mechanism during Delivery 1; it must reject case-only duplicates, including concurrent inserts. Booking request uniqueness is not partial, state-dependent, or expiring.
- SQL checks protect static booking invariants. Column-specific updates or equivalent narrowly scoped repository statements prevent changing resource codes and booking owner/request identifier/resource/interval/purpose. No product delete operations exist.
- Separate schema/migration ownership from the application's runtime database role. Runtime permissions exclude DDL and activity update/delete. Direct administrator tampering remains outside scope.
- Each mutation's application service begins one transaction, acquires locks, performs final validations, applies the change, appends its required event, and commits once. Every collaborator receives that same transaction. An append failure or definite database rejection of commit rolls the whole operation back; a connection loss around commit leaves the outcome unknown and must not be treated as proof of rollback.
- Resource create/edit/activation changes follow the same atomic-event discipline as bookings. Identical resource edit/state submissions that change nothing return a no-op without a new successful-change event.
- Use finite request/database timeouts and rollback on errors or cancellation. Do not blindly retry all database errors. Deadlock/serialization failures may be retried only as a bounded whole transaction, with fresh clock validation. Booking retries retain their request identifier. A connection failure around commit may mean the result is unknown; the client retries the same identifier rather than generating a replacement.

### Persistent request identity on Booking and replay

The canonical request data is `(resource ID, UTC start instant, UTC end instant, trimmed purpose)`; owner is the authenticated account in the unique key. State, session, CSRF token, and request transport spelling are not booking data. Proposed normalization uses valid UTF-8, Unicode whitespace trimming, Unicode code-point counting (matching PostgreSQL `char_length`), and no case folding or Unicode normalization of purpose. Thus surrounding whitespace does not change intent; different actual purpose characters do. These conventions require confirmation before booking implementation. Store the canonical values directly in Booking, not in a second snapshot.

**Selected simpler design:** a non-null immutable request identifier on Booking with `UNIQUE(owner_account_id, request_id)`, plus transaction-scoped serialization using the existing authenticated Account row. A unique constraint alone prevents duplicates but does not order replay discovery before resource validation; the lock protocol is part of the correctness design, not an optional optimization.

1. Authenticate, authorize booking creation, and decode the identifier/canonical data. Begin one READ COMMITTED transaction and lock the authenticated Account row with **`FOR NO KEY UPDATE`**. All booking-creation paths for that account follow this protocol and hold the lock until commit/rollback.
2. In a **separate statement after acquiring the lock**, query Booking by `(owner_account_id, request_id)`. READ COMMITTED then sees a preceding creator's committed row, including after waiting. Equal canonical data returns that Booking ID; different data returns changed-request reuse. Neither outcome takes a resource lock, changes records/events, or rechecks the new-booking horizon/resource active state. Authentication still applies even after cancellation, expiry of the interval, or resource deactivation.
3. If no row exists, lock the requested Resource, capture fresh time, and run final new-booking validations. Insert Booking with its request identifier and canonical values, append the creation event in that same transaction, then commit. Allocate the Booking ID at insertion; no pre-existing claim, deferred booking reference, or durable pending state is needed.
4. Validation rejection, event failure, or definite database rejection of commit rolls everything back and releases locks. No successful identity is consumed by a rolled-back creation. A waiter then sees no Booking and can attempt creation with fresh eligibility validation. If a connection is lost around commit, do not assume rollback: retrying the same identifier resolves whether a Booking/event committed and returns it or safely attempts creation.

Use Account `FOR NO KEY UPDATE`, **not `FOR UPDATE`**: it serializes creators on the same account while remaining compatible with foreign-key `KEY SHARE` locks taken by booking/event/session inserts. Otherwise a cancellation holding Resource and appending an event for that account could wait on the Account lock while its creator waits on Resource, forming a lock cycle. No ordinary resource/cancellation path acquires this account coordination lock after Resource. The database unique constraint remains a backstop; any unexpected request-key violation requires rollback and a fresh canonical lookup, not blanket conversion of uniqueness errors into success.

### Comparison with the former separate request-identity table

| Case | Separate claim + allocated Booking ID + deferred FK | Request identifier on retained Booking + Account serialization |
| --- | --- | --- |
| Identical request replay | Read committed claim, compare duplicated canonical snapshot, return referenced Booking ID. | Read retained Booking by owner/key, compare its immutable data, return its ID. No extra record or duplicated snapshot. |
| Same identifier, changed data | Reject after comparing the claim snapshot, before resource eligibility checks. | Reject after comparing Booking data, before resource eligibility checks. |
| Concurrent identical requests | Unique claim insert waits for the creator; after commit it replays, after rollback it can claim. | Account lock waits for the creator; a subsequent statement replays after commit or can create after rollback. Exactly one Booking/event. |
| Concurrent same-key/different-resource requests | Claim uniqueness serializes before either contender locks Resource; the later request rejects changed data if the first commits. | Account serialization precedes any Resource lock with the same result, even if the later resource is inactive or occupied. If the first rolls back, no successful key was bound. |
| Failed booking creation | Roll back claim, Booking, and event together; deferred FK prevents an orphan committed claim. | Roll back Booking/key and event together. No separate claim exists to orphan; failed requests are not retained as successful identities. |
| Cancellation and historical retention | Keep claim and Booking until reset; never expire the claim. | Keep Booking/key and immutable canonical fields until reset. Cancellation changes state/metadata only; uniqueness covers Cancelled and past records too. |
| Transaction and lock ordering | Claim unique-index coordination → Resource → existing Booking, with a deferred referential check and snapshot consistency to maintain. | Account `FOR NO KEY UPDATE` → replay lookup → Resource → existing Booking where applicable; immediate owner/key uniqueness and ordinary foreign keys. Cancellation uses Resource → Booking without account coordination. |

**Why replace the former design:** FR-015/NFR-007 require durable successful-request identity, not independently retained rejected attempts or a pending workflow. Both designs meet those requirements and coordinate same-key/different-resource attempts before resource validation. The former design allows unrelated keys for one user to proceed concurrently; the simpler design serializes that user's booking creations even across resources. Different users can still book different resources independently, and same-user simultaneous reservations remain allowed. This is a concurrency/throughput tradeoff, not an important correctness advantage for the approximately 20-user POC; there is no requirement for independent throughput per key. The simpler design removes a table, duplicated data, early ID allocation, and deferred FK lifecycle without weakening replay or atomicity. A per-key advisory-lock scheme could preserve finer concurrency, but adds key encoding/collision/locking conventions without a current product need; it is not introduced here.

## 8. Preventing overlapping bookings, including concurrency

Use a PostgreSQL **partial GiST exclusion constraint** over resource equality and `tstzrange(start_at, end_at, '[)')` overlap, applying only to `Confirmed` rows. Enable `btree_gist` in the booking migration for the chosen resource-ID type's equality operator in the GiST index. Keep the constraint immediate, so an overlap is detected during the insert and can be mapped to a conflict response (FR-013–FR-014).

This guarantees:

- Partial, enclosing, contained, and identical overlaps on one resource are rejected.
- Adjacent intervals do not overlap because the upper bound is exclusive.
- Different resources do not conflict.
- Cancelled rows stay retained but leave the constraint's participating set.
- Past confirmed rows still participate; do not add a moving-clock predicate to the index.

Normal booking writes additionally lock the resource row as described below. Two distinct valid request identities for conflicting reservations therefore serialize on that resource. The first valid transaction inserts booking/history and commits; the second then encounters the exclusion constraint and returns a clear conflict. Against an otherwise healthy system, exactly one succeeds, with one booking retaining its request identifier and one creation event. A browser availability preview or optional pre-query can aid UX, but is never the final enforcement.

Map only the known overlap constraint's exclusion violation to reservation conflict; distinguish request-key and resource-code uniqueness errors. All other database errors remain operational errors, not fabricated conflicts. The constraint remains a safety backstop even if a future application path forgets the advisory availability query.

## 9. Atomic business changes and activity history

For booking creation, the Booking insert (including its request identifier) and creation-event insert share **one transaction and one commit**. Cancellation similarly updates the booking and appends its cancellation event atomically; resource mutations likewise commit with their corresponding events (FR-026–FR-027).

An event contains actor, action, typed affected-record reference, server timestamp, and explicitly shaped details. Use ordinary append-only history rows, not event-sourced aggregates. Reads derive current state from Resources and Bookings, never by replaying history. Exclude passwords/session secrets from details. Escape all details at rendering, including reasons and old/new text.

Event insertion is mandatory and its errors cannot be ignored. No after-commit callback, asynchronous history queue, second transaction, or successful response before commit is allowed. A definite constraint/commit rejection fails the entire operation; transport loss near commit leaves an unknown outcome, resolved for booking creation by same-identifier replay and never a reason to accept partial history. Rejected commands emit no successful-change event; sanitized diagnostic logs are separate from product history. An identical creation replay or authorized cancellation no-op emits nothing new. Resource/booking events are durable in the same volume as business records.

**Approved product decision — bootstrap history:** initial seeded accounts and resources are bootstrap state, not authenticated product actions, and create **no product activity events**. This applies to initial population, including reinitialization after explicit demonstration reset. Do not invent a System role or attribute seed work to a fictional signed-in actor. Repeat seeding preserves existing state and produces no duplicate records or events; the bootstrap exemption is not a resource-management/backfill route. After initialization, every ordinary successful resource or booking mutation still requires its authenticated actor and atomic activity event under FR-026–FR-027. Rejected requests and authorized no-ops create no successful-change event. This resolves the PRD's open seed-history question without changing ordinary activity requirements.

## 10. Resource deactivation and booking races

Every operation affecting resource eligibility or booking occupancy acquires the same **resource row `FOR UPDATE` lock** and retains it until commit/rollback. This includes creation, cancellation, activation/deactivation, and resource edits. The global application lock order is:

1. For booking creation only, lock the authenticated Account row `FOR NO KEY UPDATE`, then look up the owner/request key in a subsequent statement. Return replay/reuse outcomes before resource validation.
2. Lock the Resource row `FOR UPDATE` if the operation still requires a business mutation.
3. Lock an existing Booking row, when cancelling.
4. Capture fresh time for final time-dependent checks, mutate, and append required history.

Cancellation may read the immutable resource ID without locking the booking, then lock Resource followed by Booking; it must not reverse the order or acquire the Account coordination lock. Account serialization orders same-key creations, including changed-data attempts naming a different resource. It also serializes unrelated creation keys for that account; it does not forbid overlapping bookings on different resources. No request operation acquires multiple resource locks. Foreign-key account `KEY SHARE` locks remain compatible with creator `FOR NO KEY UPDATE` locks, as explained in section 7.

**New booking:** after Account serialization and finding no retained owner/key match, lock Resource, capture fresh time, require active state, validate times/purpose, and insert under the unique/exclusion constraints. An inactive resource receives the inactive explanation before overlap probing; an already-bound changed key is rejected before either check.

**Deactivation:** after locking Resource, capture fresh time and run a separate occupancy query for `state = Confirmed AND end_at > now`. Reject with an explanation if any row exists; otherwise set inactive and append its event. READ COMMITTED's subsequent statement sees bookings committed by a preceding lock holder. Do not combine a pre-lock occupancy snapshot and resource update in a way that reuses stale eligibility.

| Lock winner | Outcome for the competing operation |
| --- | --- |
| Booking creation commits first | Deactivation sees the new future-ending booking and is rejected. |
| Deactivation commits first | Booking creation reads inactive and is rejected. |
| Cancellation commits before deactivation | Deactivation can proceed if no other future-ending confirmed booking exists. |
| Winning mutation/event fails | Its changes roll back; the waiting operation re-evaluates the committed state. |

Thus both incompatible mutations cannot succeed. An inactive resource may retain past confirmed bookings and any cancelled bookings; neither blocks deactivation. A long lock wait requires fresh time validation, not the request-arrival time. This protocol protects the normal application boundary; it does not claim to prevent an administrator bypassing application rules with direct SQL.

## 11. Testing and verification strategy

Provide one repository entry point, proposed **`make verify`**, running format/static checks, Go tests, real-database integration tests, lifecycle checks, and Playwright. Dependency/image installation is documented separately; verification must report failures rather than silently skip database/browser suites. Record candidate revision, executed commands, observed results, and limitations for each delivery. Independent review happens in another agent session.

### Unit and HTTP tests

- Table-driven pure-rule tests cover purpose trimming/counts, all inclusive start/duration bounds, end-after-start, cross-midnight, display labels at start/end, canonical request equality, and overlap mathematics. Database tests remain authoritative for overlap enforcement.
- Test the injected clock at exact boundaries, including fresh time after a simulated lock wait. No hard-coded dates relative to the real clock.
- `httptest` checks login/logout, cookie/session behavior, CSRF, authenticated route protection, server-derived actor/role, direct unauthorized resource/history calls, cross-owner cancellation including already-cancelled records, and useful error responses. Assert absence of mutations/events, not just HTTP status.
- Template/content tests confirm purposes, descriptions, reasons, and activity details remain text. Never cast user input to `template.HTML` or otherwise bypass contextual escaping.

### Real PostgreSQL integration tests

Apply real migrations to isolated disposable databases. Use independent pool connections/transactions and synchronized barriers/observed lock waits, not arbitrary sleeps or a shared enclosing transaction, for concurrency tests.

- Delivery 1 verifies canonical uppercase initial seeds and case-insensitive lookup/reseeding without duplicates or rewriting retained codes. Direct database inserts of `NET-01`, `net-01`, and `Net-01`, including concurrent case variants, must demonstrate database-enforced uniqueness without relying on application duplicate checks. This requires no resource-management product endpoint in foundation.
- Cover all overlap shapes, adjacency, different resources, cancelled exclusion, inactive-resource precedence, cancellation release, and deactivation with upcoming/in-use/past/cancelled bookings.
- Race two conflicting creations and assert exactly one success plus one conflict, one booking retaining its key, and one creation event. Separately exercise the exclusion constraint through direct test inserts that bypass the normal resource lock and request uniqueness through inserts that bypass Account coordination.
- Race identical same-user/same-key requests, changed payloads under one key (including different resources and a later target that is inactive/occupied), and same key for different users. Assert fresh post-lock replay lookup, canonical changed-key rejection before eligibility, and no duplicate events. Replay retained cancelled/past bookings and bookings on subsequently deactivated resources.
- Race same-user distinct keys for different resources: both valid non-conflicting bookings succeed after serialization. Exercise a creator waiting on Resource while cancellation/resource history for that account commits, proving Account `FOR NO KEY UPDATE` does not block foreign-key `KEY SHARE` and create the Account/Resource cycle.
- Force both resource-lock orders in booking/deactivation races, then assert final states, response categories, and exact event counts. Test cancellation races and a lock wait crossing the start/deactivation boundary with controlled time.
- Fail event writing through a transaction-scoped test seam or test-only database failure mechanism. Verify rollback for Booking/request identifier, cancellation, resource creation/edit/activation/deactivation. Include definite commit-time rejection and a same-key waiter following rollback: no retained key/event from failure, fresh eligibility checks, and one successful creation if still valid. Simulate lost acknowledgement around commit and verify same-key retry resolves the durable outcome without duplicate events.
- Verify 25-record booking pages, the documented stable order/tie-breakers, retained records, canonical timestamp/text equality, ownership filtering, and schema checks/foreign keys.
- Restart application and database against the same test volume, then verify bookings, history, and Booking-based retry identity survive. Re-seed and verify no duplicates or overwritten data. Assert initial seed accounts/resources create zero product events, repeated seeds add none, and ordinary post-bootstrap resource/booking changes require atomic events. Verify explicit reset only on a disposable test volume.

### Playwright browser/end-to-end tests

Use an actual Go HTTP server and PostgreSQL, seeded synthetic accounts, and isolated browser contexts for different actors. Provision each worker with its own disposable database/app and fresh fixture data; do not share or reset the developer's demonstration volume. A test-only Go bootstrap injects a controlled clock. No production endpoint for changing time is added.

- Exercise the central booking/conflict/cancel/rebook/history story, My Bookings, coordinator intervention with reason, resource lifecycle, errors, empty states, and retained pagination via accessible labels and visible text.
- Verify login/logout including reuse of the invalidated session, role-aware navigation, and server behavior under direct form requests/tampered fields. Concurrency correctness is primarily proven in database tests, not inferred from browser timing.
- Run a non-Jakarta browser timezone (for example America/New_York); enter Jakarta times and inspect the intended persisted interval and visible Asia/Jakarta labels. Include midnight intersections once their semantics are approved.
- Enter HTML-like purposes/descriptions and assert text is visible without script execution. Exercise missing-CSRF rejection.
- Prefer role/label locators, independent fixture data, automatic waiting, and failure traces/screenshots. Pin Playwright/browser versions together; require no remote CDN during ordinary operation.

Trace these tests to PRD AC-001–AC-033. Foundation checks cover only its approved subset and explicitly do not claim complete v0.1 acceptance.

## 12. Proposed repository/package structure

The following is a navigational sketch, not a fixed directory/API contract; finalize package names and command layout in the delivery that needs them. This task creates none of these application files. Delivery 1 must not scaffold empty booking/activity modules, later routes, or full-v0.1 schemas merely to match this sketch.

```text
cmd/
  labreserve/              Go HTTP executable and composition root
  db/                      explicitly invoked migrate, seed, and reset commands
internal/
  auth/                    account verification, sessions, authenticated actor
  resources/               resource rules, queries, mutations, SQL storage
  bookings/                booking rules, replay, cancellation, SQL storage
  activity/                event shapes, transactional append, restricted reader
  web/                     routes, middleware, form parsing, template rendering
    templates/             embedded html/template files
    static/                embedded local CSS and optional small JavaScript
  platform/
    clock/                 production clock and test seam
    database/              pgx pool, transaction helper, migration support
    config/                local configuration and validation
migrations/                numbered SQL migrations
tests/                     isolated cross-module and browser verification
  integration/             multi-module/database and lifecycle tests
  e2e/                     Playwright specs, fixtures, package.json and lockfile
  support/                 test-only bootstrap, clock and database helpers
docs/
  prd.md                   unchanged product requirements
  architecture/            proposed and reviewed decisions
README.md                  startup, demonstration, tests, limitations, reset
compose.yaml               application, PostgreSQL, optional verification profile
Dockerfile                 reproducible application build
Makefile                   documented local commands; one verification entry
.env.example               configuration names; development-only setup guidance
go.mod, go.sum             one Go module; pinned dependencies
```

Keep focused unit tests adjacent to packages. SQL storage lives beside the module it serves; a service imports only the narrow collaborator contracts it actually needs. Bookings may depend on Resources' eligibility/locking operations and Activity append; Resources may query occupancy through its transaction without importing the Booking service, preventing a package cycle. The web layer composes modules; neither domain service imports HTTP/templates. Node/TypeScript dependencies belong only to Playwright tests, not the application runtime. Do not create empty modules for later deferred features.

## 13. Local development and data lifecycle

- Compose runs the Go application and PostgreSQL with a durable named demonstration volume. Publish the app on loopback only (for example `127.0.0.1:8080`); PostgreSQL stays on the private Compose network by default. An explicit loopback database port is permitted for host-run Go development/tests, not a public binding.
- Build a self-contained Go application with local templates, CSS, and timezone data; embedding is the recommended packaging default. Pin the database, build, and test images/dependencies. After initial downloads, login, bookings, resources, and history need only local services; no external account, network integration, or production secret is required.
- Use `.env` for local setup, already ignored by the repository; provide configuration guidance without real credentials. Session verification and revocation need only the local database, not an external session store. The future README documents fictional demo logins and how to supply their development-only seed passwords.
- Use an explicitly invoked migration step, with a migration ledger and safe single-runner coordination. Select the migration runner and coordination mechanism during Delivery 1. Normal application startup verifies the expected schema and refuses incompatible versions; it never resets, silently downgrades, or destroys data.
- Seed synthetic accounts/resources transactionally by stable unique login and case-insensitive resource-code identity. Initial resource codes are `NET-01`, `K8S-01`, and `DEMO-01`; repeat seeding recognizes existing case variants without duplicating resources or rewriting their immutable codes. On conflict, preserve existing data: no forced password/role/metadata/active-state changes, duplicate events, or deletion of bookings. Initial seeding emits no product activity events under the approved bootstrap decision in section 9; ordinary post-initialization mutations remain subject to atomic history.
- Offer a separately invoked, prominently destructive demonstration reset with explicit confirmation, restricted to the known local demo database/volume. It removes that demonstration dataset, reapplies migrations, and seeds anew. Compose restart/stop and ordinary startup must not remove the volume; destructive volume deletion belongs only to reset instructions.
- Verification uses independently named disposable databases/volumes. Migration/seed/reset tooling must distinguish demo and test targets and never assume an arbitrary configured database is safe to erase. Schema changes preserve retained data; reset is not a migration or rollback strategy.

## 14. Important tradeoffs and alternatives

| Decision | Tradeoff and alternative considered |
| --- | --- |
| Go modular monolith + HTML templates | Few runtime dependencies and clear transaction ownership. Less frontend interaction than a SPA; the required forms/tables do not justify separate frontend deployment. No reason to adopt microservices. |
| PostgreSQL instead of SQLite/in-memory storage | Adds a local database container, but provides durable multi-connection transactions, ranges, and concurrency evidence. SQLite/in-memory fakes cannot stand in for the chosen PostgreSQL guarantees. |
| GiST exclusion constraint instead of check-then-insert | Database-enforced overlap including concurrent writes; requires `btree_gist` and PostgreSQL-specific migrations. An unprotected SELECT followed by INSERT is incorrect. A locked resource alone could work only if every writer obeyed it; the constraint adds defense in depth. |
| Account → Resource locking + READ COMMITTED | Account `FOR NO KEY UPDATE` serializes a user's creations before replay lookup; Resource `FOR UPDATE` serializes eligibility/occupancy changes. Different-user/different-resource creations remain independent; same-user creations serialize even across resources. Fresh post-lock statements/time avoid stale decisions. SERIALIZABLE adds retry complexity; a different coordination design would need renewed concurrency analysis, not a silent implementation swap. |
| Request identity on retained Booking | Owner/key uniqueness plus immutable canonical Booking fields supplies replay until reset without a separate table, duplicated snapshot, preallocated ID, or deferred booking FK. The former claim table offered finer per-key concurrency, not stronger required correctness; section 7 compares each case. A unique key without the ordering protocol, memory cache, expiring key, or broad uniqueness-error handling is insufficient. |
| PostgreSQL sessions instead of JWTs | Extra authentication lookup, but immediate server-side logout and no separate revocation store. Browser tokens do not encode authoritative roles. Redis and external identity providers are unnecessary and excluded. |
| Same-transaction activity instead of asynchronous history | A history outage rejects the business change, intentionally meeting FR-027. No broker/outbox is needed because there is no external consumer; event sourcing would add an unnecessary second state model. |
| Derived labels + fixed Jakarta entry | No worker or stale stored Past state. Browser-native local timezone assumptions cannot be used; server parsing, embedded timezone data, and explicit labels keep interpretation consistent. |
| Stable pages; keyset is a delivery-time default | Booking pages of 25 and a documented unique tie-breaker are mandatory. Keyset avoids offset shifting but makes arbitrary page jumps less convenient; OFFSET can be selected if it meets the documented ordering/navigation contract. Activity page size/order are delivery choices. No frozen snapshot or calendar UI is required. |
| Compose and local-only defaults | Reproducible durable demonstration with no production access. HTTP is deliberately loopback-development-only; this proposal does not claim production readiness, HA, administrator-proof audit, or a quantitative performance target. |

## 15. Unresolved questions and review gates

No actual contradiction was found between the PRD's explicit requirements. Full-v0.1 scope and foundation-only delivery are different milestones, not conflicting obligations. Early release remains excluded. The architecture is approved for Delivery 1. Remaining PRD-open product details below must be settled before their affected later deliveries, not become silent implementation assumptions:

| Question | Proposed resolution or information needed | Resolve before |
| --- | --- | --- |
| Selected-date schedule semantics | Recommend every interval intersecting the Jakarta day, retaining cancelled/past rows with distinct labels; confirm this rather than start-date-only filtering. | Non-empty schedule/date filtering. |
| Coordinator cross-owner already-Cancelled retry / reason resubmission | Resolve the intervention-specific authorization/reason contract in its later task. Owner-only no-op success regardless of former start is already approved in architecture 002, section 4. | Coordinator intervention implementation, not owner cancellation. |
| Request-data equivalence | Recommend resource identity + exact UTC microsecond instants + Unicode-trimmed purpose; equivalent offsets/transport spelling are not differences. Confirm trim/equality conventions. | Booking creation/replay. |
| Other text contracts | Recommend Unicode code-point purpose counting, matching Go/PostgreSQL. Name/description limits/empty handling and cancellation-reason maximum are not specified. Confirm contracts when needed; do not invent extra purpose normalization or arbitrary business length limits. Reason must at least be nonempty after trimming when required. Do not introduce resource-code length/pattern restrictions or uppercase-only input. Transport/body safety limits are engineering controls, not undocumented domain limits. | Affected booking/resource-management/cancellation forms and schemas; foundation must not impose additional unapproved product restrictions. |

Resource-code identity and bootstrap history are **resolved and approved**, not remaining review gates: resource codes are case-insensitive with canonical uppercase initial seeds (section 2); seed accounts/resources create no product activity events, while all ordinary post-initialization resource/booking mutations retain required atomic history (section 9). These explicit approved resolutions supply the previously open policies. Owner-cancellation semantics and Resource → Booking lock order are also resolved and approved in architecture 002, section 4 and baseline section 12, without approving implementation or coordinator intervention.

### Architectural constraints versus delivery-time defaults

| Area | Fix in the architecture now | Select during the appropriate delivery |
| --- | --- | --- |
| Boundaries and runtime | Go modular monolith, escaped server-rendered HTML, PostgreSQL, local Compose lifecycle, no external ordinary-operation dependency. Services own authorization/transactions; activity append uses the caller's transaction. | Delivery 1: driver/pool (`pgx` recommended), package/command names, template/static packaging, migration runner, supported pinned versions, local ports, pool sizing/timeouts. No future-module scaffolding. |
| Identity and persistence | Stable identities, immutable case-insensitive resource codes, constrained roles/states, foreign-key relationships, retained business history, and Booking owner/request uniqueness across all states until reset. | Delivery 1: appropriate database-enforced case-insensitive resource-code uniqueness mechanism. Record ID encoding (UUID recommended), SQL names, checks versus enum encoding, query/index layout, and details encoding (JSONB recommended), in the delivery introducing the record. Do not replace required invariants with application-only best effort. |
| Authentication and security | Local seeded accounts, Argon2id password hashing, opaque database-backed revocable sessions, server-derived actor/role, rotation against fixation, CSRF protection, safe cookies/escaping/logging, loopback-only HTTP exception. | Delivery 1: maintained libraries, reviewed hash parameters, token/digest encoding and session/anonymous-session lifetimes (32-byte token, SHA-256, 12-hour authenticated lifetime recommended), cleanup cadence. Defaults must preserve the security constraints. |
| Booking correctness | READ COMMITTED; Account `FOR NO KEY UPDATE` before a separate replay lookup, then Resource `FOR UPDATE`; cancellation Resource → Booking; fresh server time after waits; immediate partial GiST exclusion; business change/event in one commit. These choices have an interdependent correctness argument. | Booking delivery: exact SQL/helper APIs, bounded retry/timeouts, identifier generation and failure-injection seams. A lock/isolation/constraint replacement requires architecture review of its guarantees, not merely a default selection. |
| Time and request equality | Unambiguous stored instants, Jakarta display and fixed-zone form interpretation, controllable authoritative clock, immutable canonical request data, successful replay before current eligibility checks. | Booking delivery: field names, explicit canonical precision (microseconds recommended), and approved trim/count/equality conventions. Product equivalence questions in the table above are not freely selectable engineering defaults. |
| HTTP and lists | Protected browser workflows; distinguish validation, inactive-resource, conflict, changed-key, and permission errors without raw database details. Booking pages contain 25 records with documented stable order/unique tie-breaker and retained history. | Relevant view delivery: route paths, exact HTTP status mapping, sort direction/tie-breaker, keyset versus OFFSET, activity ordering/page size, event detail schemas. Required relevant details and cancellation reasons are not optional. |
| Verification and lifecycle | One verification entry point, Go/real-PostgreSQL/Playwright evidence, controlled time, independent concurrent operations, isolated fixtures, failure/rollback/restart coverage, non-destructive migrations/seeds and explicit guarded reset. | Delivery 1: entry-point spelling (`make verify` recommended), test layout, supported pinned browser, fixture/bootstrap implementation; later deliveries add their own acceptance coverage. Foundation does not claim full-v0.1 acceptance. |

Delivery-time selections must be documented and tested but need not be fixed in advance where they leave these constraints intact. No quantitative performance or broad browser-support commitment is inferred; pinned Chromium verification does not imply a general compatibility guarantee.

**Delivery 1 readiness:** approved for foundation implementation. Resource-code identity and bootstrap history are settled; no Delivery 1 product question remains in this architecture. Choose the database-enforced case-insensitive uniqueness approach during Delivery 1 without adding code length/pattern restrictions or requiring uppercase input. Foundation must not introduce other unapproved domain text restrictions. Cross-midnight date filtering, request equivalence/purpose counting, and coordinator intervention/reason details remain later-delivery gates, not reasons to add their implementation to foundation. Engineering defaults above are choices for the approved delivery, not unresolved architectural blockers.

**Approval boundary:** architecture approval covers Delivery 1 only; booking creation and subsequent deliveries still require separate approval and resolution of their applicable product questions. The owner-cancellation decision is approved in architecture 002, section 4; implementation still needs a separately approved, tracked and committed task. This documentation update changes no application code, schema, configuration, or credentials.
