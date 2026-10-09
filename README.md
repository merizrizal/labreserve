# LabReserve

LabReserve is a local, server-rendered Go proof of concept for a shared engineering resource catalog. **Current scope includes the Delivery 1 foundation and Tasks 2A–2C plus Task 3A:** sign in/out, role-aware authenticated access, resource browsing and schedules, browser booking creation through the Task 2B service, database-backed booking/activity-event persistence with booking invariants, and authenticated My Bookings. My Bookings is read-only; cancellation (including coordinator cancellation), rescheduling, Resource management, and activity-history UI are deferred and not implemented.

## Requirements

- Docker Engine with Docker Compose v2
- GNU Make
- A browser

The first run downloads pinned Go, PostgreSQL, and Playwright images/dependencies. After those downloads, normal local application use needs no external service. No Go or Node installation is needed to start the Compose application or run `make verify`.

## Start the demonstration

From the repository root:

```sh
make up
```

On first use, Make copies `.env.example` to the ignored `.env` file. Review the development-only values before startup. `make up` builds the app, starts PostgreSQL, explicitly applies migrations, repeatably seeds demo records, and starts the app. Normal app startup checks the schema and refuses to serve an unmigrated database; it does not migrate, reset, or seed implicitly.

Open the `APP_ORIGIN` in `.env` (default **http://127.0.0.1:8080**). The app and PostgreSQL host ports bind to loopback only. If changing `APP_PORT`, change `APP_ORIGIN` to the matching origin too. The PostgreSQL port is exposed on loopback for local development only; it is not required for normal browser use.

### Fictional accounts

The example `.env` seeds these accounts; change the `SEED_*_PASSWORD` values **before first startup** if desired:

| Role | Login | Example development password |
|---|---|---|
| Engineer | `alex@example.test` | `LabReserve-Alex-Local` |
| Engineer | `sam@example.test` | `LabReserve-Sam-Local` |
| Coordinator | `jordan@example.test` | `LabReserve-Jordan-Local` |

Passwords are hashed with Argon2id before storage and are never logged. Seeding uses insert-if-absent semantics: a later `make seed` or `make up` will not replace existing passwords, roles, resource codes, names, descriptions, or active states. Editing `.env` after the first seed therefore does not change an existing account password.

### Demo flow

1. Sign in as Alex and confirm the page identifies the account as an engineer.
2. Browse the three seeded resources: `NET-01`, `K8S-01`, and `DEMO-01`.
3. Open a resource and choose a date. The schedule shows retained bookings intersecting that **Asia/Jakarta** date, ordered by start time and Booking ID, 25 per page.
4. Open **Book this resource**, enter a future Asia/Jakarta start/end time and purpose, and submit. The resulting Booking page displays its stable identifier; returning to the schedule shows its owner, complete interval, purpose, and derived status.
5. Open **My Bookings** to see only the signed-in account's retained Upcoming, In use, Past, and Cancelled records across resources. Lists use stable 25-record pages; open a booking entry to reuse the existing detail view. An account with no bookings sees an empty state and a link back to Resources.
6. Sign out, then sign in as Jordan and confirm the coordinator role is shown. Coordinators see their own My Bookings records; cancellation, resource management, and activity-history screens remain deferred.
7. In another terminal, run `make restart`. Refresh the browser: the database-backed session and seeded data survive both container restarts.

Booking creation uses a stable hidden request identifier, server-side Asia/Jakarta parsing, CSRF and session identity, and the Task 2B service for validation, replay, locking, overlap enforcement, transactions, and activity creation. The browser adapter does not implement those booking rules itself. The approved bootstrap policy remains in place: initial seed accounts/resources do not generate product activity events.

## Local lifecycle and data safety

- `make up` — build/start, apply migrations, safely seed, start the app.
- `make migrate` — explicitly apply outstanding checksummed migrations.
- `make seed` — repeat the non-destructive seed operation.
- `make restart` — restart PostgreSQL, wait for health, then restart the app without deleting the volume.
- `make down` — stop/remove containers and network; **keep** the named PostgreSQL volume.
- `make reset-demo` — destructive reset. It requires typing `DELETE LABRESERVE DEMO DATA`, removes only this Compose project's demo volume, then performs a fresh startup/seed.

Do not use `docker compose down -v` for ordinary shutdown. The demo database volume is durable across stop/start and restart. PostgreSQL runs with UTC configuration; the server embeds timezone data and displays schedule dates in Asia/Jakarta. Sessions are opaque, database-backed records with token digests, a 12-hour absolute authenticated lifetime, a 30-minute anonymous login lifetime, and immediate revocation on sign-out. Local HTTP explicitly uses a non-`Secure` cookie; direct development mode must bind to loopback, while the container exception is explicit and Compose publishes the app only on loopback. Cookies remain `HttpOnly` and `SameSite=Lax`.

The PostgreSQL bootstrap creates distinct, non-superuser migration and runtime roles. Only the migration role owns schema objects; the runtime role can read accounts/resources and manage session rows but cannot run DDL or mutate resources/accounts. Migrations run under a PostgreSQL advisory lock, each unapplied migration and its SHA-256 ledger entry commit together, and an applied migration checksum cannot silently change.

The `resources` table has a database-enforced unique index on `lower(code)`, so code lookup and uniqueness are case-insensitive. Re-seeding recognizes case variants without rewriting an existing resource's original code or metadata.

## Automated verification

Run the repository's single verification entry point:

```sh
make verify
```

It creates a separate `labreserve-verify` Compose project and disposable PostgreSQL volume, applies migrations, seeds, restarts PostgreSQL and checks retained foundation data, runs Go unit/real-PostgreSQL integration tests using separate migration/runtime roles, starts a verification-only Go E2E bootstrap, and runs pinned Chromium/Playwright tests in `America/New_York`. The bootstrap injects `LABRESERVE_TEST_NOW=2040-01-02T03:04:05Z`; booking dates and schedule labels are independent of wall-clock time. The production executable is unchanged and continues to use the server clock. After browser tests the script restarts both PostgreSQL and the app and verifies seeded data and server-side sessions remain. The script removes only its own verification containers, network, and volume on exit; it does not reset the demo database.

Foundation coverage includes authentication, session rotation/revocation, CSRF and Origin checks, seed repeatability, resource protection/escaping, database-role privileges, and restart persistence. Task 2A/2B real-PostgreSQL tests cover booking invariants, replay, atomic activity events, locking/concurrency, and exclusion-based overlap enforcement. Task 2C Go/PostgreSQL HTTP tests assert no Booking/event mutation for rejected CSRF, malformed timestamps/IDs, Task 2B validation, unknown/inactive resources, overlap, changed request-ID reuse, and operational failure; successful Engineer and Coordinator creation, authenticated ownership, exact Jakarta instants, PRG, escaping, one creation event, replay counts, adjacency, and identical intervals on different Resources are also checked. Playwright runs in `America/New_York` with a separate Engineer B context and covers booking/replay, conflict with one visible contested record, adjacent schedule ordering, a different Resource, Coordinator self-booking, cross-midnight visibility on both Jakarta dates, escaped purpose text, and UTC `datetime` attributes matching Jakarta form values. The E2E app uses the verification-only controlled clock described above. Task 3A Go/PostgreSQL tests cover owner filtering before pagination, retained Cancelled bookings on inactive Resources, stable pages, controlled status boundaries, and unchanged database state for read-only views; Playwright covers owner isolation, the empty state, retained status display, Jakarta labels in America/New_York, pagination through more than 50 records, existing Booking detail, escaped purpose text, and unchanged Booking/Activity counts.

The first `make verify` downloads the pinned Playwright browser image, which is large; later runs use the local image cache. A port conflict on the demo PostgreSQL port (default 54329) prevents demo startup; the verification database is private to its Compose network.

## Implementation defaults

- Go 1.26.0; PostgreSQL 17.7; `pgx` 5.7.6; Playwright 1.63.0/Chromium.
- Argon2id with 64 MiB memory, three iterations, one lane, and a random 16-byte salt.
- 32-byte random session and CSRF tokens; only the opaque session-token digest is stored.
- Embedded, numbered SQL migrations with transactional checksums and a single-runner advisory lock.
- UUID record IDs and a functional case-insensitive resource-code index.
- Login path `/login`, logout via CSRF-protected POST `/logout`, resource list `/resources`, resource schedule `/resources/{id}`, My Bookings `/my-bookings`, booking form `/resources/{id}/bookings/new`, and booking detail `/bookings/{id}`.
- Schedule date defaults to the current date in Asia/Jakarta. Schedule records intersecting that Jakarta day are sorted by `start_at ASC, id ASC`; page navigation uses validated page numbers and a 25-record SQL limit/offset.
- Booking forms use minute-resolution `datetime-local` values interpreted only in Asia/Jakarta. Successful submissions use POST/redirect/GET to the Booking detail page; replay is identified as an existing result rather than a second creation.

These defaults describe the current implementation and do not expand product scope. The PRD and approved architecture are unchanged. Task 2A implements persistence/database invariants; Task 2B implements the internal booking creation service; Task 2C exposes that service through the browser and populates Resource schedules.

## Current scope and deferred work

**Implemented — Task 2A persistence/database invariants:** booking and activity-event persistence; database-enforced `(owner_account_id, request_id)` uniqueness; static Booking invariants; PostgreSQL exclusion-based overlap enforcement for confirmed bookings; and real-PostgreSQL verification of these persistence guarantees.

**Implemented — Task 2B internal booking creation service:** canonical request handling; authoritative-time validation; Account serialization; post-lock replay lookup; Resource locking; idempotent replay; changed-request reuse detection; overlap conflict mapping; and atomic Booking + Activity creation.

**Implemented — Task 2C browser workflow:** authenticated CSRF-protected booking form; fresh stable request IDs; Jakarta-local transport parsing; service-outcome mapping; PRG booking detail; retained schedule display with derived states and stable 25-record pages; and cross-midnight Jakarta-day filtering.

**Implemented — Task 3A My Bookings:** authenticated, database-owner-filtered read-only views of retained bookings across resources; Upcoming/In use/Past/Cancelled labels derived from the authoritative clock; stable `start_at ASC, id ASC` pages of 25; escaped purpose text; and links to the existing Booking detail view. Pagination accepts one positive page number; invalid or repeated page values receive a safe HTTP 400 response.

**Still deferred:** booking cancellation (including coordinator cancellation), booking rescheduling, Resource management workflows, and activity-history UI. External authentication, Redis, infrastructure access, and production deployment are also out of scope. Local demo credentials are intentionally fictional and **not suitable for any non-demo environment**.
