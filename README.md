# LabReserve

LabReserve is a local, server-rendered Go proof of concept for a shared engineering resource catalog. **Current scope includes the Delivery 1 foundation, Task 2A persistence, and the Task 2B internal booking creation service:** sign in/out, role-aware authenticated access, resource browsing, an empty schedule, and database-backed booking/activity-event persistence with booking invariants. The booking service is not exposed through browser workflows; resource management, cancellation workflows, and activity-history screens are not implemented.

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
3. Open a resource, choose a date, and confirm the labelled **Asia/Jakarta** schedule is empty.
4. Sign out, then sign in as Jordan and confirm the coordinator role is shown. Resource reading is available to both roles; coordinator-only management/history features are intentionally not part of this delivery.
5. In another terminal, run `make restart`. Refresh the browser: the database-backed session and seeded data survive both container restarts.

There are no booking HTTP routes or forms, so the demo flow creates no bookings or activity-event records and every schedule remains empty. Task 2A provides booking and activity-event persistence; Task 2B adds an internal booking creation service, but it is not available through the browser. The approved bootstrap policy is implemented: initial seed accounts/resources do not generate product activity events.

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

It creates a separate `labreserve-verify` Compose project and disposable PostgreSQL volume, applies migrations, seeds, restarts PostgreSQL and checks retained foundation data, runs Go unit/real-PostgreSQL integration tests using separate migration/runtime roles, starts the application with the runtime role, and runs pinned Chromium/Playwright tests in `America/New_York`. After browser tests it restarts both PostgreSQL and the app and verifies seeded data and server-side sessions remain. The script removes only its own verification containers, network, and volume on exit; it does not reset the demo database.

Foundation coverage includes successful/failed login, unauthenticated route protection, server-derived role/identity despite request parameters, session rotation/revocation and persistence across an application restart, CSRF and origin rejection, seed repeatability/non-overwrite, PostgreSQL case-insensitive resource uniqueness, resource list/detail and empty schedule rendering, escaped HTML-like resource content, database role privileges, and persistence through database restarts. Task 2A real-PostgreSQL integration tests cover booking persistence, foreign keys, owner/request uniqueness, static Booking invariants, activity-event target constraints, and PostgreSQL exclusion-based overlap enforcement—including direct concurrent conflicting inserts on independent connections and preservation of existing foundation rows during migration. The browser suite exercises the visible login, resource, empty-schedule, and logout flow; it does not exercise booking workflows.

The first `make verify` downloads the pinned Playwright browser image, which is large; later runs use the local image cache. A port conflict on the demo PostgreSQL port (default 54329) prevents demo startup; the verification database is private to its Compose network.

## Implementation defaults

- Go 1.26.0; PostgreSQL 17.7; `pgx` 5.7.6; Playwright 1.63.0/Chromium.
- Argon2id with 64 MiB memory, three iterations, one lane, and a random 16-byte salt.
- 32-byte random session and CSRF tokens; only the opaque session-token digest is stored.
- Embedded, numbered SQL migrations with transactional checksums and a single-runner advisory lock.
- UUID record IDs and a functional case-insensitive resource-code index.
- Login path `/login`, logout via CSRF-protected POST `/logout`, resource list `/resources`, and resource schedule `/resources/{id}`.
- Schedule date defaults to the current date in Asia/Jakarta. The browser schedule remains explicitly empty: Task 2A provides persistence and Task 2B provides internal booking creation, but populated schedules are deferred.

These defaults describe the current implementation and do not expand product scope. The PRD and approved architecture are unchanged. Task 2A implements persistence/database invariants; Task 2B implements the internal booking creation service. Booking HTTP routes/forms and the browser booking workflow remain deferred.

## Current scope and deferred work

**Implemented — Task 2A persistence/database invariants:** booking and activity-event persistence; database-enforced `(owner_account_id, request_id)` uniqueness; static Booking invariants; PostgreSQL exclusion-based overlap enforcement for confirmed bookings; and real-PostgreSQL verification of these persistence guarantees.

**Implemented — Task 2B internal booking creation service:** canonical request handling; authoritative-time validation; Account serialization; post-lock replay lookup; Resource locking; idempotent replay; changed-request reuse detection; overlap conflict mapping; and atomic Booking + Activity creation. This service is internal and is not exposed through browser routes or forms.

**Still deferred:** booking HTTP routes/forms; browser request-id handling; populated schedules; My Bookings; cancellation workflows; resource management workflows; and activity-history UI. External authentication, Redis, infrastructure access, and production deployment are also out of scope. Local demo credentials are intentionally fictional and **not suitable for any non-demo environment**.
