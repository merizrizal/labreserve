# LabReserve

LabReserve is a local, server-rendered Go proof of concept for a shared engineering resource catalog. **Current scope includes the Delivery 1 foundation, Tasks 2A–2C, Tasks 3A–3C, and the internal Task 4A/4B Resource services:** sign in/out, role-aware authenticated access, resource browsing and schedules, browser booking creation through the Task 2B service, authenticated My Bookings, owner cancellation through the Task 3B service, and Coordinator-authorized Resource creation/descriptive editing and explicit existing-Resource activation/deactivation through internal services. Cancellation is a CSRF-protected POST followed by PRG; the Booking remains in history and its interval is released on commit. My Bookings remains read-only. Resource-management HTTP/browser workflows remain deferred to Task 4C, as do coordinator cancellation of another user's Booking, rescheduling, and activity-history UI.

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
6. Open one of your Upcoming Bookings, choose **Cancel booking**, and verify the retained detail and schedule show Cancelled after the redirect. The released interval can be booked by another engineer; an authorized retry is safe.
7. Sign out, then sign in as Jordan and confirm the coordinator role is shown. Coordinators can cancel their own Upcoming Bookings, but cannot cancel another user's Booking through this workflow.
8. In another terminal, run `make restart`. Refresh the browser: the database-backed session and seeded data survive both container restarts.

Booking creation uses a stable hidden request identifier, server-side Asia/Jakarta parsing, CSRF and session identity, and the Task 2B service for validation, replay, locking, overlap enforcement, transactions, and activity creation. The browser adapter does not implement those booking rules itself. The approved bootstrap policy remains in place: initial seed accounts/resources do not generate product activity events.

## Local lifecycle and data safety

- `make up` — build the images, start PostgreSQL, explicitly apply outstanding migrations, safely seed, then start the app.
- `make migrate` — explicitly apply outstanding checksummed migrations.
- `make seed` — repeat the non-destructive seed operation.
- `make restart` — restart PostgreSQL, wait for health, then restart the app without deleting the volume.
- `make down` — stop/remove containers and network; **keep** the named PostgreSQL volume.
- `make reset-demo` — destructive reset. It requires typing `DELETE LABRESERVE DEMO DATA`, removes only this Compose project's demo volume, then performs a fresh startup/seed.

Do not use `docker compose down -v` for ordinary shutdown. The demo database volume is durable across stop/start and restart. `make up` rebuilds the app and migration/seed tools, applies outstanding migrations explicitly, then seeds and starts the app. If applying a migration separately with `make migrate`, first rebuild its tool image (for example, `docker compose build migrate`) so it contains the current embedded migrations. Startup checks the exact migration ledger and never migrates implicitly; an older binary refuses to start against a newer ledger. Migrations 005 and 006 are additive and have no automatic down-migration. Before applying 006 to a database that already records 005, confirm its checksum matches the accepted merged `005_resource_mutations.sql` (SHA-256 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`). If 005 has an earlier or different checksum, stop and obtain an authorized non-destructive compatibility procedure; do not rewrite the ledger, replay 005, or reset a retained volume. Resetting a ledger or demo volume is not a schema rollback and can destroy retained data. PostgreSQL runs with UTC configuration; the server embeds timezone data and displays schedule dates in Asia/Jakarta. Sessions are opaque, database-backed records with token digests, a 12-hour absolute authenticated lifetime, a 30-minute anonymous login lifetime, and immediate revocation on sign-out. Local HTTP explicitly uses a non-`Secure` cookie; direct development mode must bind to loopback, while the container exception is explicit and Compose publishes the app only on loopback. Cookies remain `HttpOnly` and `SameSite=Lax`.

The PostgreSQL bootstrap creates distinct, non-superuser migration and runtime roles. Only the migration role owns schema objects. The runtime role can read accounts/resources and manage session rows, but cannot run DDL, directly insert/update/delete/truncate Resources, or mutate Accounts. Migration 005 adds only two Coordinator-checked `SECURITY DEFINER` Resource capabilities, owned by the non-superuser migration role with a fixed `pg_catalog` search path: create (including explicit initial Active/Inactive state) and descriptive edit. Migration 006 adds the narrow `labreserve_set_resource_state` capability for existing-Resource state transitions. Each capability couples its permitted Resource change to its required Activity event in the same transaction; state changes cannot edit code, identity, descriptive text, or creation time. The application service still supplies trusted session identity; the database role check does not independently authenticate an actor ID supplied through arbitrary SQL. Migration 004 grants the cancellation service column-level Booking updates only for `state`, `cancelled_by_account_id`, and `cancelled_at`; it grants no general Booking update/delete authority. These are column privileges, not row-level authorization: code with direct runtime-role SQL access can bypass the service's ownership and Activity checks by updating those fields. This accepted cancellation limitation is unchanged and is not hardened against arbitrary SQL execution. Migrations run under a PostgreSQL advisory lock, each unapplied migration and its SHA-256 ledger entry commit together, and an applied migration checksum cannot silently change.

### Internal existing-Resource state service

`resources.StateService.SetState` accepts a Resource ID and an explicit desired Active value from a trusted actor; stored Coordinator authority is required. Deactivation is rejected while any Confirmed Booking for that Resource, across all owners, has `end_at` later than the fresh operation time captured after the Resource lock. Past and Cancelled records do not block; the service never cancels or rewrites a Booking. A real transition updates only `active` and `updated_at` and commits one matching Activity event atomically. An authorized matching-state request is a true no-op with no Resource update, timestamp change, or Activity attempt. Existing Booking request replays remain valid while a Resource is Inactive, while new Booking creation uses the existing inactive-resource rejection. The database capability rechecks stored Coordinator role and transition guards, but the application remains responsible for supplying trusted session identity and operation time to the function; direct SQL execution is not a separate authentication or clock source. This capability is internal only: Task 4C's Resource-management routes, forms, CSRF workflow, and browser acceptance are not included.

The `resources` table has a database-enforced unique index on `lower(code)`, so code lookup and uniqueness are case-insensitive. Re-seeding recognizes case variants without rewriting an existing resource's original code or metadata.

## Automated verification

Run the repository's single verification entry point:

```sh
make verify
```

It creates an isolated Compose project selected by `LABRESERVE_VERIFY_PROJECT` (default `labreserve-verify`) and its disposable PostgreSQL volume, applies migrations, seeds, restarts PostgreSQL and checks retained foundation data, runs Go unit/real-PostgreSQL integration tests using separate migration/runtime roles, starts a verification-only Go E2E bootstrap, and runs pinned Chromium/Playwright tests in `America/New_York`. The bootstrap injects `LABRESERVE_TEST_NOW=2040-01-02T03:04:05Z`; booking dates and schedule labels are independent of wall-clock time. The production executable is unchanged and continues to use the server clock. After browser tests the script creates Active and Inactive Resources through the internal service, edits those and a seeded Resource, repeats the seed, creates/cancels a related Booking, and prepares restart fixtures for owner cancellation and Resource state. It then restarts PostgreSQL and the app, re-seeds, reloads Resource and Booking state through fresh runtime connections, verifies Resource capabilities/no-op/duplicate behavior and Booking creation/cancellation replays, confirms a retained Coordinator session reads the deactivated seeded Resource through the restarted app, and checks Resource/Booking history and state transitions. The script removes only the selected verification project's containers, network, and volume on exit; it does not reset the demo database. To preserve another existing verification project, choose a previously unused name with the required prefix, for example `LABRESERVE_VERIFY_PROJECT=labreserve-verify-task004b-c10 make verify`.

Foundation coverage includes authentication, session rotation/revocation, CSRF and Origin checks, seed repeatability, resource protection/escaping, database-role privileges, and restart persistence. Task 2A/2B real-PostgreSQL tests cover booking invariants, replay, atomic activity events, locking/concurrency, and exclusion-based overlap enforcement. Task 3B real-PostgreSQL tests cover owner-only cancellation for Engineer and Coordinator self-service, strict/fresh time checks, retained metadata and creation replay, interval release, idempotent/concurrent calls, both cancellation/creation lock orders, event-failure rollback, and column-scoped runtime privileges. Task 3C HTTP/PostgreSQL tests cover cancellation PRG, owner-only authorization, CSRF/Origin rejection, stale-time responses, idempotent retries, inactive-resource cancellation, event-failure rollback, and read-only GET behavior; Playwright covers owner cancellation, forged non-owner requests, retained history, retry, and released-interval booking by another engineer. Task 2C Go/PostgreSQL HTTP tests assert no Booking/event mutation for rejected CSRF, malformed timestamps/IDs, Task 2B validation, unknown/inactive resources, overlap, changed request-ID reuse, and operational failure; successful Engineer and Coordinator creation, authenticated ownership, exact Jakarta instants, PRG, escaping, one creation event, replay counts, adjacency, and identical intervals on different Resources are also checked. Playwright runs in `America/New_York` with a separate Engineer B context and covers booking/replay, conflict with one visible contested record, adjacent schedule ordering, a different Resource, Coordinator self-booking, cross-midnight visibility on both Jakarta dates, escaped purpose text, and UTC `datetime` attributes matching Jakarta form values. The E2E app uses the verification-only controlled clock described above. Task 3A Go/PostgreSQL tests cover owner filtering before pagination, retained Cancelled bookings on inactive Resources, stable pages, controlled status boundaries, and unchanged database state for read-only views; Playwright covers owner isolation, the empty state, retained status display, Jakarta labels in America/New_York, pagination through more than 50 records, existing Booking detail, escaped purpose text, and unchanged Booking/Activity counts.

Task 4A service/unit and real-PostgreSQL tests cover stored Coordinator authorization, both creation states, Unicode canonicalization and long text, exact/case-variant duplicate rejection and independent creation races, immutable code assertions, canonical no-op without UPDATE, serialized before/after values and post-lock timestamps, Activity-failure rollback, and retained Booking identity/history across future, in-use, past, and Cancelled states. They also check trusted session identity, escaped service-created Resource text in existing list/schedule/detail/My Bookings views, guarded-capability effective privileges and migration-004 data preservation. The restart lifecycle creates and edits Active/Inactive Resources through the service, edits a seeded Resource, repeats seeding, preserves a related cancelled Booking and both histories over actual PostgreSQL/application restarts, and verifies post-restart no-op, duplicate, capability, and Booking replay behavior. Task 4B Chunk 10 adds disposable lifecycle regression coverage for retained Resource identity and Activity, Past Confirmed and Cancelled Booking/request/cancellation history, inactive replay and changed-request precedence, repeat-seed preservation, both durable state transitions, a post-restart matching-state no-op, an opposite-state transition, and an existing Coordinator session served by the restarted app.

The first `make verify` downloads the pinned Playwright browser image, which is large; later runs use the local image cache. A port conflict on the demo PostgreSQL port (default 54329) prevents demo startup; the verification database is private to its Compose network.

## Implementation defaults

- Go 1.26.0; PostgreSQL 17.7; `pgx` 5.7.6; Playwright 1.63.0/Chromium.
- Argon2id with 64 MiB memory, three iterations, one lane, and a random 16-byte salt.
- 32-byte random session and CSRF tokens; only the opaque session-token digest is stored.
- Embedded, numbered SQL migrations with transactional checksums and a single-runner advisory lock.
- UUID record IDs and a functional case-insensitive resource-code index.
- Login path `/login`, logout via CSRF-protected POST `/logout`, resource list `/resources`, resource schedule `/resources/{id}`, My Bookings `/my-bookings`, booking form `/resources/{id}/bookings/new`, booking detail `/bookings/{id}`, and owner cancellation via POST `/bookings/{id}/cancel`.
- Schedule date defaults to the current date in Asia/Jakarta. Schedule records intersecting that Jakarta day are sorted by `start_at ASC, id ASC`; page navigation uses validated page numbers and a 25-record SQL limit/offset.
- Booking forms use minute-resolution `datetime-local` values interpreted only in Asia/Jakarta. Successful submissions use POST/redirect/GET to the Booking detail page; replay is identified as an existing result rather than a second creation.

These defaults describe the current implementation and do not expand product scope. The PRD and approved architecture are unchanged. Task 2A implements persistence/database invariants; Task 2B implements the internal booking creation service; Task 2C exposes that service through the browser and populates Resource schedules.

## Current scope and deferred work

**Implemented — Task 2A persistence/database invariants:** booking and activity-event persistence; database-enforced `(owner_account_id, request_id)` uniqueness; static Booking invariants; PostgreSQL exclusion-based overlap enforcement for confirmed bookings; and real-PostgreSQL verification of these persistence guarantees.

**Implemented — Task 2B internal booking creation service:** canonical request handling; authoritative-time validation; Account serialization; post-lock replay lookup; Resource locking; idempotent replay; changed-request reuse detection; overlap conflict mapping; and atomic Booking + Activity creation.

**Implemented — Task 2C browser workflow:** authenticated CSRF-protected booking form; fresh stable request IDs; Jakarta-local transport parsing; service-outcome mapping; PRG booking detail; retained schedule display with derived states and stable 25-record pages; and cross-midnight Jakarta-day filtering.

**Implemented — Task 3A My Bookings:** authenticated, database-owner-filtered read-only views of retained bookings across resources; Upcoming/In use/Past/Cancelled labels derived from the authoritative clock; stable `start_at ASC, id ASC` pages of 25; escaped purpose text; and links to the existing Booking detail view. Pagination accepts one positive page number; invalid or repeated page values receive a safe HTTP 400 response.

**Implemented — Task 3B atomic owner cancellation:** the application service authorizes only the booking owner, permits only future Confirmed transitions, returns successful no-op for authorized Cancelled retries, locks Resource → Booking, captures fresh post-lock time, and atomically retains metadata/history while releasing the interval at commit. The runtime role has only the approved cancellation-column updates; direct SQL access to that role can bypass service authorization and Activity guarantees, an accepted development limitation.

**Implemented — Task 3C cancellation browser workflow:** owners can cancel future Confirmed Bookings from Booking detail using a CSRF/Origin-protected POST. The handler delegates to Task 3B; successful transitions and authorized idempotent retries use PRG. Non-owners receive the same 404 as unknown targets; stale cancellations receive a safe 409, operational failures a generic 503, and GET routes remain read-only. Cancelled records remain visible in detail, My Bookings, and schedules; the existing creation flow can reserve the released interval.

**Implemented — Task 4A internal Resource creation and descriptive editing:** the application service authorizes stored Coordinator identity, canonicalizes Unicode-edge text, explicitly persists Active or Inactive creation state, rejects duplicate/case-variant codes and attempted code changes, serializes descriptive edits on the Resource lock, and returns a successful no-op without UPDATE or Activity for unchanged text. Migration 005 grants the restricted runtime role only the matching guarded create/edit capabilities; each actual change and its Resource Activity event are atomic. This is an internal service/persistence capability, not a Resource-management browser workflow.

**Implemented — Task 4B internal Resource activation/deactivation:** the application service applies an explicit desired state for an existing Resource under stored Coordinator authority and the shared Resource lock. Deactivation checks all Confirmed Bookings against fresh post-lock time; blocking outcomes and genuine no-ops do not change Resource data or Activity. Migration 006 grants the runtime role only the matching guarded state capability; transitions preserve existing Resource identity/text and retained Booking history. This does not add a Resource-management HTTP or browser workflow.

**Still deferred:** Task 4C Resource-management navigation, HTTP handlers, forms, CSRF/Origin workflow, and browser acceptance; coordinator cancellation of another owner's Booking and its mandatory cancellation-reason workflow; booking rescheduling; activity-history UI; recurring bookings; and external notifications/integrations. External authentication, Redis, infrastructure access, and production deployment are also out of scope. Local demo credentials are intentionally fictional and **not suitable for any non-demo environment**.
