# Prepared instruction — Task 4B, Chunk 5

**Status: `PREPARED_NOT_AUTHORIZED`.**

**Current gate: `CHUNK4_RECOVERED; INDEPENDENT_REVIEW_PASS; CHUNK5_AUTHORIZATION_REQUIRED`.**

**Execution mode: unselected.**

This file prepares a future single-chunk task; it is not implementation authorization. The present user request authorizes updating only this instruction and the Chunk 4→5 handoff. Do not execute implementation, tests, or database operations from this document without separate explicit Chunk 5 authorization and a fresh preflight.

## Mandatory entry gate — review passed; Chunk 5 authorization remains required

Read the updated `docs/handoffs/task-004b-chunk-4-to-5.md` first. The previously missing plan-required equal-operation-microsecond regression was added in a separate Chunk 4-only recovery and passed against real PostgreSQL; inspect its test and evidence there. The Chunk 4 review and subsequent repair review were performed in separate sessions. The repair review status is **PASS**, with no blocking findings in the repair. It confirmed B1 resolved: only the deliberate Coordinator rejection maps to `ErrForbidden`; other `42501` failures remain operational, and the integration test verifies safe error text, unchanged Resource/Activity snapshots, and preservation of the genuine authorization rejection.

The review report identifies base `19566eeea5ec33944fba3740bf00ff54f0450b8c` and reviewed adapter/test SHA-256 prefixes `347f0997f7e5` (`internal/database/resource_state.go`) and `ce27290722f9` (`internal/database/resource_state_integration_test.go`). The current full hashes are recorded in the handoff. The report also records 17 Resource-state PostgreSQL tests passed without skips, 65 Resource tests/subtests passed, all-package compilation, focused `go vet`, formatting, and diff checks passed, and fault-trigger cleanup confirmed.

This PASS records the Chunk 4 review gate only; it is not Chunk 5 authorization or a fresh Chunk 5 preflight. At any separately authorized Chunk 5 execution, verify the review evidence and candidate hashes still match. If the review outcome or candidate identity cannot be confirmed, or any concern remains, stop and report it; do not fold a repair into Chunk 5. Obtain separate explicit Chunk 5 authorization and run a new specification preflight. Do not infer authorization or `PREFLIGHT_PASS` from this handoff, the earlier completion report, or another session.

## Contract and candidate identity to revalidate at authorized execution

- Task: `docs/tasks/004b-atomic-resource-activation-deactivation.md`
- Sole controlling ladder: `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, **Chunk 5 — Effective runtime privileges and trusted-session boundary**.
- Recorded task SHA-256: `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- Recorded plan SHA-256: `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- Recorded Chunk 4 implementation base/current `HEAD`: `19566eeea5ec33944fba3740bf00ff54f0450b8c`; branch `feat/004b-resource-activation-deactivation`, upstream at the same commit at handoff time.
- The candidate at handoff time is **dirty and uncommitted**. It includes the four Chunk 4 source/test files and protected pre-existing handoff edits listed in `task-004b-chunk-4-to-5.md`. Current hashes recorded there are evidence for that candidate, not an assertion they remain current later.
- Migration 006 current recorded hash: `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`; it was not changed in Chunk 4.

At the start of any separately authorized Chunk 5 execution session:

1. Re-read repository `AGENTS.md`, the complete Task 4B contract, relevant PRD/architecture and the **complete** controlling plan including its ending, plus the updated Chunk 4 handoff and independent review outcome. Confirm all authoritative documents remain tracked/committed and structurally complete.
2. Recheck explicit user authorization, current branch/HEAD/upstream, task/plan/migration hashes, full staged/unstaged/untracked state, and candidate source hashes. Preserve every pre-existing change; never assume the recorded base/candidate is still current.
3. Confirm the equal-time regression and its real-PostgreSQL evidence, and that independent review accepted the complete Chunk 4 candidate. Resolve/report the plan’s execution policy: two failed validation attempts per issue; execution mode remains unselected until explicitly chosen; local verification environment only. Do not let session changes reset the failure budget.
4. Report `PREFLIGHT_PASS` only if the current authorized Chunk 5 contract, base/candidate, plan and prior-chunk evidence all pass the required gates. Otherwise report `PREFLIGHT_BLOCKED` or `INCOMPLETE` and stop before editing.

## Authorized Chunk 5 objective and scope (future only)

**Objective:** establish the final-write privilege boundary and prove that trusted application sessions supply the Coordinator identity to the existing internal state service. Preserve approved cancellation privileges, existing safe reads, and the Task 4B state behavior. This chunk must not introduce production behavior, a management route, or broader grants.

The controlling plan proposes these test-only files:

- `internal/database/resource_state_privilege_integration_test.go`
- `internal/web/resource_state_session_integration_test.go`

Inspect and reuse existing seams before editing:

- DB permissions/shadow-object tests: `internal/database/resource_mutations_integration_test.go`, especially `TestResourceCapabilitiesExposeOnlyGuardedRuntimeWrites` and its permission-denial helpers.
- Existing state capability coverage and snapshots: `internal/database/resource_state_capability_integration_test.go` and `internal/database/resource_state.go`.
- Role/bootstrap and capability history: `deploy/postgres/01-roles.sql`, migrations 003–006, especially `internal/database/migrations/006_resource_state.sql`.
- Authentication/session: `internal/auth/service.go` (`Login`, `Resolve`, `Logout`), `internal/database/store.go` (session lookup/current stored account role), `internal/web/middleware.go`, and `internal/web/foundation_integration_test.go` (`TestFoundationHTTPAuthenticationAndResourceBrowsing`, test pool safety and existing login/session seams).
- Existing read/HTML regression: `internal/web/security_test.go` (`TestResourceTemplateEscapesHTMLLikeContent`) and the current list/schedule/detail/My Bookings read paths.

Do not add any production symbol, route, form, status mapping, endpoint, configuration, role, migration, dependency, or generic/raw state setter. The plan permits a guarded capability/test defect to be repaired only when it is substantiated and repair fits task scope and applied-history constraints. Do not relax tests, broaden grants, or edit immutable migration history to make tests pass. If correct completion requires any of those or another security/architecture change, stop and escalate with evidence/options.

## Required work and acceptance evidence

### 1. Actual runtime effective privilege boundary

Use the actual `labreserve_app` runtime login for all ordinary capability and denial tests; assert `current_user = 'labreserve_app'` in the tested runtime connection. Do not substitute `labreserve_migrator` or an administrator for runtime evidence. The migrator may be used only for migration/disposable-fixture setup, catalog inspection, and scoped fault setup as the plan allows.

Prove effective ownership and privileges from PostgreSQL catalogs, including table, every-column, function, schema, `PUBLIC`, inherited-role, grant-option, and relevant role membership effects. Check fixed capability ownership and search path, `SECURITY DEFINER` configuration, revoked `PUBLIC` execution, and runtime-only execute grant. Include actual denials—not catalog assertions alone—for the plan’s attempted Resource INSERT/every-column UPDATE/DELETE/TRUNCATE, capability replace/alter/owner change, `SET ROLE`, schema DDL, and unrelated writes. Run each denial in an isolated transaction and assert the observed denial/SQLSTATE without leaving partial state. Preserve existing approved cancellation-column permissions and Activity read/write privileges exactly; first inspect current expected behavior instead of assuming all Activity writes are denied.

Prove the ungranted invocation path using either an already available ungranted test role or the plan’s serialized, owned disposable fixture: temporarily revoke runtime EXECUTE, invoke through a **fresh runtime connection**, verify the actual denial, and reliably restore/assert the exact original ACL. Verify the fixture is not shared or persistent before changing ACLs. If the test cannot guarantee exact restoration or would require unapproved role provisioning, stop; the accepted bootstrap does not give the migrator `CREATE ROLE`. Do not leave a privilege change behind, and do not attempt this on a persistent/demo target.

Exercise the guarded state capability directly with Engineer, unknown and null actor values; matching-state no-ops; an occupancy blocker; and valid transitions. Verify the stored Coordinator check, locked state, qualified occupancy lookup and internally derived Activity data; no Resource UPDATE or Activity attempt on a no-op; no writable code/name/description, Booking/history, skip-event, or skip-occupancy input. Verify shadow objects for `accounts`, `resources`, `bookings`, and Activity plus caller search path cannot bypass authority/occupancy or redirect writes. Confirm no generic raw setter overload exists. Record catalog facts, attempted operations, actual SQLSTATEs and before/after Resource/Booking/history/Activity snapshots.

### 2. Trusted live session → existing internal service

Use the existing Login/Resolve/Logout and test wiring to resolve a **current live session**, take the account identity from that resolved session, and invoke the internal state service. Do not authenticate from a role claim, cookie payload, or cached `Actor` alone.

Prove:

- A live Coordinator session can reach the existing service’s authorized behavior.
- Both Engineer fixtures are rejected; changing/tampering with a claimed role does not elevate them.
- Unknown actors are rejected, and after logout a fresh Resolve fails before invoking the service. A previously retained account ID/Actor is not proof of a live session.
- Current stored-role changes are reflected safely by the existing Resolve/service path; do not invent session semantics.
- Existing safe Resource/list/schedule/detail/My Bookings reads continue to work and preserve legacy/raw HTML-like text appropriately.

Use the existing authentication/session seam and internal service only. Add no management HTTP endpoint, route, form, HTTP status mapping, or user-facing workflow.

## Environment and integrity constraints

- Use only repository-local isolated verification services after confirming the selected Compose project is not another worker’s active environment and that the test helper’s safety guard confirms the exact test database (currently the helper is intended to reject anything other than `labreserve_test`). Re-read the helper rather than relying on this note.
- At the end of Chunk 4 recovery, `labreserve-recovery-c4-19566ee` was stopped and its named volume retained. The separate `labreserve-verify` project had been observed running with a named volume and was left untouched; its ownership was not established. Do not assume either volume is disposable, run `down -v`, reset, or clean without confirming ownership and authorization. Never touch a persistent/demo target or production credentials.
- Do not reveal or modify `.env`, passwords, tokens, or credentials. Avoid printing secrets in test logs.
- Do not stage, commit, push, merge, deploy, or perform destructive cleanup.
- Preserve the Chunk 4 dirty work and pre-existing handoff edits. Do not overwrite or “clean” them.
- Keep Migration 006 and prior migration history unchanged unless an actual defect is reproduced and an in-scope, applied-history-safe repair is established. No down migration or privilege broadening is authorized.

## Focused validation and evidence

Re-discover test names before selecting them; proposed new names are not evidence until the tests exist and run. Use uncached PostgreSQL tests with serial package execution and actual runtime connections. The plan’s focused selectors are:

- Database: `^TestResourceStatePrivilege` and existing `^TestResourceCapabilitiesExposeOnlyGuardedRuntimeWrites`.
- Web: `^(TestResourceStateSession|TestFoundationHTTPAuthenticationAndResourceBrowsing|TestResourceTemplateEscapesHTMLLikeContent)`.
- Discovery: run the corresponding `go test -list` patterns first and report every selected test, skip and result. Required DB tests must not silently skip for a missing database/password environment.
- Compile/type check and build, formatter, `git diff --check`, and full relevant diff review including pre-existing staged/unstaged/untracked work.

Use the repository’s documented RTK-prefixed Compose verification commands from plan section 8, with the current authorized project identity and rebuilt images. The prescribed patterns are approximately:

```sh
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceStatePrivilege' ./internal/database
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceCapabilitiesExposeOnlyGuardedRuntimeWrites$' ./internal/database
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^(TestResourceStateSession|TestFoundationHTTPAuthenticationAndResourceBrowsing|TestResourceTemplateEscapesHTMLLikeContent)' ./internal/web
```

Before relying on these, verify current test names, build the candidate image(s), and confirm the local verification project is owned and uses the safe test DB. `make verify` and browser/lifecycle delivery-wide work are reserved for Chunk 11; do not run them as a substitute for Chunk 5’s focused evidence or claim them passed.

## Exit report and stop condition

After completing an explicitly authorized Chunk 5 session, report the exact base and dirty candidate identity; task/plan/source fingerprints; actual current_user and ACL/denial evidence; trusted session results; tests discovered, run, skipped and their results; commands; complete diff; any test fixture/ACL restoration proof; security/integration risks; limitations; and every pre-existing change preserved. Use only the non-final chunk status `CONTINUABLE`, `INCOMPLETE`, or `BLOCKED` as the actual evidence supports. Do not report `READY_FOR_REVIEW` for this non-final chunk.

Stop immediately after Chunk 5. Do not begin Chunk 6 or any later chunk. A future chunk requires a separate explicit authorization, fresh session, new source/base preflight, and the accepted Chunk 5 evidence. Create a successor handoff only if the future invocation’s selected execution mode/policy authorizes it; do not infer continuous-loop mode.
