# Task 4B — Prepared Instruction for Chunk 4

**Instruction state: PREPARED_NOT_AUTHORIZED**
**Prepared from:** [`docs/handoffs/task-004b-chunk-3-to-4.md`](task-004b-chunk-3-to-4.md)
**Target:** A later implementation session, fresh if the harness supports it. Do not claim session isolation unless it is provided.

## Authorization and execution mode

- Use the `chunked-implementation` skill in **SINGLE-CHUNK** mode, only after explicit human authorization for Chunk 4 and all freshness gates below pass.
- The earlier authorization was for Chunk 3 only. The current request authorized updating the handoff artifacts; neither that request nor this prepared instruction authorizes Chunk 4.
- Execute exactly **Chunk 4 — Safe deactivation, occupancy boundaries and complete no-ops**. Do not start Chunk 5 or any other task.
- Do not stage, commit, push, merge, deploy, modify credentials, access persistent targets, or perform destructive cleanup without separate authorization.

## 1. Governing task and candidate identity

- **Approved task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md` — observed SHA-256 `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md` — observed current SHA-256 `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- **Completed-chunk evidence:** `docs/handoffs/task-004b-chunk-3-to-4.md`.
- **Relevant prior handoff:** `docs/handoffs/task-004b-chunk-2-to-3.md` for Migration 006, its tests, migration compatibility, and the earlier failed-attempt history.
- **Recorded implementation base:** `61e7d010205f24478654a1b37e47ae75aec14c76`.
- **Recorded committed Chunk 3 candidate:** `19566eeea5ec33944fba3740bf00ff54f0450b8c`, parent `bd231b22ed18e41942ef2001bec71dca6d15ffe4`, branch `feat/004b-resource-activation-deactivation` (the origin tracking ref pointed to the same commit when inspected).
- At handoff refresh, the worktree was clean at that commit. The refreshed evidence handoff and this instruction are now documentation-only working-tree changes; preserve them. Recheck the live repository state instead of assuming either identity remains current.
- Task and plan hashes above are freshly measured at the recorded candidate and match the previous evidence. The plan fingerprint at the original implementation base was `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b`; the documented delta is three absolute-path redactions. A prior handoff recorded a wrong historical plan hash; `task-004b-chunk-2-to-3.md` documents the correction. Do not rewrite that history.
- Chunk 3 recorded `PREFLIGHT_PASS` before implementation. This is historical gate evidence, not Chunk 4 authorization or a substitute for checking the current base/source set.
- **Migration fingerprints recorded at the current candidate:** 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`; 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`; 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`; 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`; 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`; 006 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`.

## 2. Mandatory authorization, repository and freshness gate — before edits

1. Confirm an explicit instruction authorizes **Chunk 4 only**. If absent, stop without editing implementation files.
2. Read repository `AGENTS.md`, the complete Task 4B contract, and the complete controlling plan through their endings. Read the entire Chunk 4 section and the relevant authoritative requirements/architecture it cites. Re-read the latest evidence handoff and this instruction; consult earlier handoffs for retained constraints/corrections.
3. Inspect the live branch, HEAD, upstream relation, complete staged/unstaged/untracked status and candidate diff. Protect the two documentation artifacts and any unrelated work. Record the actual Chunk 4 implementation base; it may differ from `19566ee`.
4. Recompute current full Task/plan SHA-256 identities and compare with this instruction. Confirm the task remains tracked/committed and the Chunk 4 objective, scope, tests and exit criteria are unchanged. Verify the state-service and Migration 006 source still match the recorded candidate or review actual intervening diffs.
5. Inspect the full current candidate from the actual implementation base, including untracked files. Confirm Chunk 3 activation remains present, tested source files are not silently changed, and deactivation still fails closed. Prior Chunk 3 test results are reported in the handoff; this instruction session did not rerun them.
6. Revalidate Migration 006's actual contents and unchanged migrations 001–005. Do not infer persistent-ledger compatibility from the disposable test history.
7. Resolve current test tooling and disposable verification-project ownership before running database tests. Keep credentials private and `.env` untouched.
8. Re-run the project specification-readiness/preflight gate appropriate to this implementation base. A prior pass or this instruction cannot silently be reused after material source/candidate drift.
9. Stop and report if authorization, source freshness, architecture/security constraints, a required test environment, or task scope cannot be established. Do not invent product behavior or edit authoritative sources to make implementation pass.

## 3. Exact Chunk 4 objective

**Controlling-plan title:** “Chunk 4 — Safe deactivation, occupancy boundaries and complete no-ops.”

**Plan objective:** complete the state service for V1–V3/V6/V7; cover FR-024–027/009/023, NFR-001, and AC-009/012/021/028–029/041/044 facets. See the complete Chunk 4 section in the controlling plan for dependencies, ordered tasks, validation, risks, and exit condition.

### Planned source scope

Inspect and, if required by the current plan, change only:

- `internal/resources/state.go`
- `internal/resources/state_test.go`
- `internal/database/resource_state.go`
- `internal/database/resource_state_integration_test.go`

Migration 006 and its guarded capability are accepted Chunk 2 work. Keep it unchanged unless a concrete, reproducible defect makes a narrowly scoped in-scope repair necessary; never rewrite an applied migration for convenience. Do not add HTTP/application handlers, routes, Booking changes, migration infrastructure, or Chunk 5 privilege/session work.

### Required behavior and order

- Preserve trusted stored-Coordinator authorization before target-dependent success, including both matching-state no-ops. Engineers and unknown/malformed actors must not succeed through equality.
- Lock the Resource and read authoritative state after lock acquisition. A well-formed missing Resource remains `ErrUnknownResource`; service/adapter/database errors never become “unoccupied.”
- Compare locked current state with explicit desired state **before occupancy checks**. Both Active→Active and Inactive→Inactive are genuine no-ops: no clock read, occupancy query, UPDATE, `updated_at` rewrite, or Activity attempt; authorized no-op finalization is not success if commit fails.
- Remove the service and adapter's fail-closed inactive-state stubs only as part of implementing the complete Chunk 4 path. Preserve Chunk 3 activation and its security/error behavior.
- For an actual Active→Inactive transition only, sample fresh time after the Resource lock using the existing UTC-microsecond convention. Run a separate occupancy query exactly equivalent to `resource_id = target AND state = 'confirmed' AND end_at > operation_time`. Query/lock/context errors are operational. A true blocker yields the distinct occupancy domain result (`ErrResourceOccupied` or the approved existing equivalent), with no mutation or event. Do not filter by owner, start time, selected day, or derived UI label.
- If eligible, invoke the already-guarded capability with the same operation time; it must retain final Coordinator/state/occupancy/event guards. Persist only `active` and `updated_at`, atomically with exactly one deactivation Activity. A mutation or Activity error rolls back both; only a definite successful commit returns changed success. Never cancel or edit a Booking.
- Keep activation available: no deactivation occupancy gate applies to Inactive→Active, and retained Bookings/history/text stay unchanged. New Booking creation continues through the existing service rules.
- Do not infer last-request-wins or suppress a real opposite transition merely because timestamps are equal. No automatic retry/deactivation after a rejection.
- **Level 0/1 discretion:** ordinary implementation/debugging and technically reasonable helper/API/test-organization choices are permitted only within the approved contract and existing repository conventions. Record meaningful choices. **Level 2:** changing product precedence, authorization/trust, lock/isolation rules, security guarantees, migration history, or scope requires human approval; stop before dependent edits.

### Required real-PostgreSQL/service evidence

At minimum, add and execute Chunk 4 service tests for:

- Both desired-state no-ops; no UPDATE/event-attempt detectors; Active no-op and an Inactive no-op fixture with future occupancy; authorization before equality; no clock/occupancy call on no-op; no-op commit failure is operational.
- Upcoming Confirmed and in-use Confirmed Bookings (including exact start) blocking; all-owner/all-date scope; Cancelled and completed rows not blocking; other-Resource rows not blocking; mixed nonblockers plus one blocker.
- Strict end boundary: `end_at - 1µs` blocks, exact end and after permit, using canonical database precision.
- Actual Resource lock wait both before and across a Booking's end. Observe the wait, prove no eligibility clock read before release, then assert the post-lock time controls the outcome and is shared by `updated_at`/Activity on success.
- Activity failure and outer rollback for deactivation, unchanged Resource/Booking/history snapshots, then an explicit retry that produces one event.
- Existing activation, authorization, retention, ordinary Booking eligibility, inactive-creation precedence, and create/replay behavior still pass.

Use the real restricted runtime pool for application/service operations and independent PostgreSQL transactions for lock tests. Unit fakes alone do not prove the predicate, fresh database snapshot, grants, or wait ordering.

## 4. Verification obligations

The plan's Chunk 4 selectors are proposals: verify actual names with discovery before relying on them. Do not weaken selection until required cases execute.

- Discover state unit tests, then run the full state unit set, e.g. `^TestResourceState` in `./internal/resources`.
- Discover database state cases and run the Chunk 4 state/activation/capability subset, e.g. `^TestResourceState(Deactivation|NoOp|Occupancy|Activation|Capability)` in `./internal/database`.
- Also discover and run the existing inactive/replay and relevant Booking/cancellation time-boundary regressions. Current recorded names include `TestBookingCreationReplaySurvivesPastAndCancelledInactiveRecords`, `TestBookingCreationChangedDataWinsBeforeResourceEligibility`, `TestBookingCreationValidationAndEligibilityOutcomes`, `TestOwnerCancellationStrictTimeBoundaryAndCancelledNoOp`, and `TestOwnerCancellationUsesFreshTimeAfterResourceLockWait`; recheck they still exist and select only those required by the plan.
- Use uncached focused test invocations (`-count=1`; database package serialized with `-p 1`) through the repository's disposable Compose verification environment if needed. Example command shape, after verifying ownership and service configuration:

```sh
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState' ./internal/resources
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^TestResourceState' ./internal/resources
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^TestResourceState(Deactivation|NoOp|Occupancy|Activation|Capability)' ./internal/database
rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -p 1 -run '^TestResourceState(Deactivation|NoOp|Occupancy|Activation|Capability)' ./internal/database
```

- Run formatting (`gofmt -l`/format only changed Go files), compile/type check and application build as appropriate; inspect symbols and selector discovery. Run existing Booking/cancellation regressions with their actual test names.
- Inspect persisted Resource, Booking, and Activity snapshots for each reject/no-op/rollback; capture observed lock-wait evidence. Record every actual command and distinguish `PASS`, `FAIL`, `SKIP`, `NOT_RUN`, and `UNVERIFIED`.
- Run the plan's required focused checks and complete diff/risk review. Do not claim full `make verify` is a Chunk 4 requirement; the plan reserves delivery-wide `make verify` for Chunk 11. Run additional verification only when required by a material integration boundary and report it accurately.
- At session end inspect `git status`, staged/unstaged/untracked content and the complete relevant diff. Keep the Chunk 4 delta separate from the cumulative Task 4B commit and the documentation-only handoff changes. Do not stage or commit.

## 5. Preserved invariants and explicit exclusions

- Preserve Migration 006's guarded SECURITY DEFINER boundary, stored-role check, Resource locking, safe fixed search path and runtime-only capability. No direct Resource UPDATE/column privilege.
- Preserve Resource identity, code, raw name/description (including legacy edge whitespace/HTML-like text), `created_at`; preserve all Bookings, request identities, cancellation metadata and prior Activity.
- Preserve Account `FOR NO KEY UPDATE` → replay lookup → Resource order for new Booking creation, owner cancellation's Resource → Booking order, and state operations' stored-role read → Resource lock order. No Resource→Account coordination lock or Booking mutation.
- Preserve accepted Task 4A behavior, existing creation/edit contracts and Task 2A–3C behavior. Do not introduce a UI caller or new externally visible API.
- Explicitly out of scope: Task 4C HTTP/browser work; cross-owner cancellation/reason; history UI/API; Booking changes; new schema/migration/privileges; deployment; production/persistent database operations; unrelated refactoring.

## 6. Retry, environment and risk controls

- **Budget:** two failed validation attempts per issue, from Task 4B/chunked-implementation policy. Chunk 2's resolved fixture-typing issue used one attempt; Chunk 3's resolved fake begin-call expectation used one attempt. Those histories remain recorded and are not reset, but they are distinct resolved issues. Track any Chunk 4 failures by root issue and do not exceed the finite budget.
- Use only a confirmed disposable `labreserve-verify` project or another explicitly isolated project if ownership conflicts; re-build source images after code edits. The previous Chunk 3 handoff reports its disposable database/container/volume/network were removed, but this instruction-preparation session did not inspect current Docker state.
- Before any persistent target operation, stop until its 001–005 migration provenance/checksums are checked non-destructively. Older/different Migration 005 checksums require escalation; do not rewrite a ledger, reset a volume, or treat disposable migration success as persistent compatibility.
- Functional risk: wrong `>` boundary, stale pre-lock clock/snapshot, unauthorized no-op, or no-op accidentally executing SQL/event. Integration risk: migration capability error classification, shared Resource locking, test-order nondeterminism. Repair only in-scope issues with persisted evidence; a product/security/locking/architecture change is a Level 2 stop.

## 7. Mandatory post-work and stop rule

After the authorized single chunk: format, compile, execute focused discovered tests, diagnose/repair in-scope failures within budget, inspect complete cumulative and chunk-specific diffs, assess functional/integration/security/data risk and rollback, and report actual evidence, failures, skips, and gaps. Write/update a Chunk 4-to-5 handoff only if separately authorized by that invocation or the governing execution mode; do not make one automatically from this prepared document.

Stop after Chunk 4. Do not start Chunk 5, run deployment/integration, commit, or imply Task 4B is complete. If authorization/source freshness is missing, or a required test fails/exhausts its budget, stop with a precise `BLOCKED` or `INCOMPLETE` report rather than proceeding.

## 8. Suggested skills

- `chunked-implementation` — SINGLE-CHUNK only, after explicit authorization.
- `pre-read-discipline` and `pre-edit-discipline` — current source/diff discovery before implementation.
- `post-edit-discipline` — required verification and full diff review.
- `rtk-command-prefix` — repository and test commands.
- `specification-readiness-authority` — only if the current contract, plan, or preflight gate requires revalidation.
- `handoff` — only for an independently authorized next transfer.
