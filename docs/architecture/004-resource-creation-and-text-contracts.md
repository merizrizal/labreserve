# 004 — Resource creation and text contracts

- **Status:** Product decisions D4-001, D4-002, and D4-003 approved by the fictional client in the Delivery 4 documentation-authoring request. This ADR records those approvals. At authoring, this ADR and Task 4A awaited human review and commit; their reviewed versions are now committed at `a4f820054aa3f20f03fd56be490b469e0165bb25`. Git commitment does not constitute explicit human document/task approval or a passing formal implementation preflight. This documentation-only correction supplies neither gate and does not authorize application implementation.
- **Product authority:** [PRD](../prd.md), especially FR-005, FR-023, FR-026–FR-027, AC-028, and AC-041–AC-044; [baseline section 13](../baseline.md#13-delivery-4-client-request--resource-management); the three explicit client decisions reproduced below.
- **Architectural authority:** Extends [architecture 001](001-initial-architecture.md), particularly sections 2, 7, 9, 10, and 15. Preserves [architecture 002](002-my-bookings-and-cancellation.md) and the narrowly bounded limitation in [architecture 003](003-cancellation-privilege-boundary.md).
- **Related contract:** [Task 4A — Atomic Resource creation and descriptive editing](../tasks/004a-atomic-resource-creation-editing.md), a draft requiring separate implementation approval.
- **Documentation base:** `77605c28782c0b6883d6670d4cc17ce7bf8ff28f`; working tree clean before authoring. Existing requirements, architecture, and Tasks 2A–3C were tracked and committed. This records inspection context, not re-acceptance or new verification of Deliveries 1–3.

## 1. Context

Resources are already browsable and referenced by retained Bookings. The current schema has stable UUID identity, `TEXT NOT NULL` code/name/description, `BOOLEAN NOT NULL DEFAULT TRUE` active state, and creation/update timestamps. Migration 001 enforces case-insensitive code uniqueness with `resources_code_case_insensitive_uq` on `lower(code)`; migrations 002–004 provide Bookings, Activity, locking helpers, and cancellation-column privileges.

The PRD's Delivery 4 form-contract question and architecture 001 section 15 deliberately left text validation and creation-state policy unsettled. The client has now resolved those questions below. Earlier documents retain their historical open-question wording; for these specific questions, read them together with this ADR. No unrelated product or architectural decision is superseded.

## 2. D4-001 — Resource name and description

### Decision

- **Name:** Required. Trim surrounding Unicode whitespace and reject an empty-after-trim value.
- **Description:** Optional. Trim surrounding Unicode whitespace. Empty string is valid; an omitted description is represented by the existing non-null empty string.
- **Both:** Preserve all internal characters and whitespace. Do not case-fold, apply Unicode normalization, or introduce arbitrary product-level maximum lengths.
- **Unchanged edit:** If the submitted canonical name and description equal the current canonical values, return successful no-op. Do not execute a Resource update, rewrite `updated_at`, or append an Activity event. Authorization still applies before returning no-op.

### Rationale

A Resource needs an understandable name, but additional explanatory text is not mandatory. Trimming accidental edge whitespace gives deterministic persisted values and edit comparison without changing intentional internal text. Exact comparison preserves differences in casing, internal spacing, and Unicode composition; those differences must not be silently erased. Unspecified length limits are not client requirements. A no-op is not a business change and must not manufacture history or recency.

### Consequences

Canonicalization belongs at the application boundary before persistence and must agree with any guarded database capability. Reuse Go's established Unicode-whitespace trimming semantics; PostgreSQL's default space-only trimming is not an equivalent implementation. Compare exact canonical text, not a case-insensitive or Unicode-normalized approximation. An edit changing only one descriptive field still records one descriptive-edit event, with relevant before/after values.

`name` and `description` remain PostgreSQL `TEXT`; optional description does not require nullable storage or a schema maximum. Existing retained rows are not an invitation to run a bulk normalization/backfill or rewrite timestamps/history. Any discovered legacy value incompatible with safe application of this contract must be investigated; destructive remediation is not implicitly approved.

## 3. D4-002 — Resource code

### Decision

- Required and nonempty after surrounding Unicode-whitespace trimming.
- Trim that surrounding whitespace before persistence.
- Preserve submitted casing and all internal characters.
- Preserve the existing database-enforced case-insensitive uniqueness contract.
- Code is immutable after creation. A case-only code edit is still an attempted change and is rejected, not treated as a rename or descriptive no-op.
- Do not impose uppercase-only input, a pattern, or an arbitrary product-level length restriction.
- Reject duplicate creation under the existing uniqueness contract, including concurrent case variants. A duplicate is not a successful creation replay or an update of the existing Resource.

### Rationale

Code is stable human-readable identity, not mutable display metadata or the primary key. Existing case-insensitive identity prevents confusing duplicates while retaining the spelling selected at creation. Trimming edge whitespace avoids accidental identity variants; forced uppercase and extra validation patterns would introduce unapproved restrictions.

### Consequences

Keep the existing `lower(code)` unique index and its current PostgreSQL semantics. Do not replace it with application-only checking, another case-folding definition, or an upsert that rewrites retained data. The initial uppercase seed codes are examples/bootstrap choices, not a requirement for later input.

A descriptive-edit API has no code mutation authority. If it carries a submitted code assertion, reject a value whose trimmed spelling differs from the persisted code, including casing; alternatively use an explicit rejected-field boundary. Do not silently ignore a requested code change. Exact API shape is a Task 4A engineering choice, but rejection must be observable and tested. Descriptive editing retains Resource ID/code, state, creation instant, existing Booking data/request identity, and prior Activity details.

## 4. D4-003 — Chosen initial active state

### Decision

- Resource creation permits either Active or Inactive.
- The browser creation form must visibly default to Active.
- The application service explicitly persists the chosen state, rather than accidentally relying on the SQL default.
- Creation appends exactly one Resource-created Activity event containing the selected initial state along with relevant creation values.
- Creation must not generate a separate activation event.

### Rationale

A coordinator may prepare a Resource before making it reservable. Active is the visible form default for ordinary creation, not a hidden database fallback. Initial state is part of creation; recording it as a second transition would misrepresent what occurred.

### Consequences

The existing non-null boolean and its SQL default need not change. The creation command and persistence call carry an explicit state, including `false`; no layer may drop Inactive as a missing/default value. A typed command may express the choice as a boolean or equivalent explicit representation. The existing browse/schedule/booking paths consume the resulting state: Active permits otherwise-valid reservations; Inactive does not.

The form-default decision is recorded now for the later browser task. Task 4A implements and tests only the internal creation contract, not a Resource-management form or route. Selecting initial Inactive/Active does not authorize changing the state of an existing Resource.

## 5. Atomicity, privilege, and architectural continuity

The decisions above extend the existing Resource record, not the Booking state model or database trust model. Preserve:

- trusted session-derived identity and stored Coordinator authorization at the application service;
- one explicit READ COMMITTED transaction and one commit for a mutation plus its required Activity;
- Resource `FOR UPDATE` serialization for edits, authoritative locked before-values, and fresh operation time after lock waits;
- creation's explicit timestamps and edit's unchanged `created_at`, with the operation timestamp also used by its event;
- Booking creation's Account `FOR NO KEY UPDATE` → separate replay lookup → Resource protocol, owner cancellation's Resource → Booking order, and the immediate Confirmed-only overlap constraint;
- retained identities, immutable Booking request data, seed bootstrap history exemption, escaped plain-text rendering, and durable restart behavior.

Task 4A must introduce only narrowly guarded, migration-managed Resource creation/descriptive-edit capabilities. Prefer operation-specific SECURITY DEFINER functions, consistent with migration 003's scoped capability pattern, with final guards and mandatory corresponding Activity insertion coupled to the mutation. A runtime caller must not be able to commit a Resource mutation by invoking the new capability but omitting its event. Do not grant direct Resource INSERT/UPDATE/DELETE, including descriptive-column UPDATE, merely because the service normally appends history. Do not expose a generic privileged SQL/column updater or accept caller-authored Activity details as authoritative before/after state.

Function names, decomposition, and an equivalently restricted implementation are Level 1 choices if they meet Task 4A's guards, safe ownership/search-path/EXECUTE constraints, shared lock protocol, and effective-permission tests. This is not permission to weaken those properties. The existing runtime Activity SELECT/INSERT grants support Booking behavior and are not broadened here. No claim of arbitrary-runtime-SQL impersonation resistance or administrator-proof history is introduced: authenticated identity remains a trusted application responsibility. Database role checks do not authenticate a supplied account ID.

Architecture 003 accepts a development-only bypass for existing Booking cancellation columns. It is not precedent for unaudited Resource writes, mutable codes, or Resource state toggles through the new capability. Do not redesign cancellation privileges in Task 4A. If safe Resource capability implementation would require a new material security/Activity-integrity exception or architecture change, stop for a precise Level 2 decision rather than accepting that risk.

## 6. Scope and verification consequences

Task 4A covers internal Coordinator creation, descriptive editing, validation, immutable codes, initial state, no-op behavior, atomic history, and minimal database capabilities. Its contract requires real-PostgreSQL authorization, uniqueness/concurrency, rollback, permissions, migration, retention, restart, and regression evidence through the existing verification infrastructure.

Existing-Resource activation/deactivation, deactivation eligibility/races, and all Resource-management HTTP routes/forms remain outside Task 4A. The intended downstream ownership is:

- **Task 4B:** Existing-Resource activation/deactivation; state-transition service and database operations; fresh-time occupancy validation; Resource locking and concurrency for those transitions; corresponding atomic Activity recording; service/database verification.
- **Task 4C:** Coordinator Resource-management browser navigation; Resource creation/edit/state forms; HTTP authorization and CSRF/Origin protection; safe outcomes and redirects; browser integration and acceptance.

The full Delivery 4 browser/state requirements remain obligations for separately approved tasks; this ADR and Task 4A do not approve or authorize Task 4B or Task 4C implementation. Coordinator cross-owner cancellation and Activity-history inspection remain outside Delivery 4. Deletion, Booking mutation, and new infrastructure remain outside Task 4A.

## 7. Decision authority and review boundary

D4-001–D4-003 are resolved product decisions, not remaining questions for an implementation agent. No unresolved Level 2 decision is identified for the bounded Task 4A design. Internal package/API shape, safe helper design, event detail encoding, deterministic test seams, and non-destructive migration details retain Level 1 discretion within existing architecture and the task contract.

Human review and approval of these documents as applicable, separate explicit Task 4A implementation approval, and a passing formal implementation preflight remain prerequisites to implementation. The reviewed documents' Git commitment is recorded above and does not satisfy those approval/preflight gates. Neither this ADR nor successful documentation checks certify application behavior, grant final acceptance, or authorize commit, push, merge, deployment, or credential changes.
