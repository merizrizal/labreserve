# Task 4B — Chunk 3 to Chunk 4 Handoff

**Status: CONTINUABLE.** Chunk 3 exit criteria were met in its implementation session; the candidate is now committed. Chunk 4 has not started, Task 4B remains incomplete, and this handoff does not authorize the next chunk.

**Handoff mode:** `ENGINEERING_CHUNK_HANDOFF`, refreshed after commit.
**Paired next-session instruction:** [`instruction-chunk-4-task-004b-atomic-resource-activation-deactivation.md`](instruction-chunk-4-task-004b-atomic-resource-activation-deactivation.md) — `PREPARED_NOT_AUTHORIZED`.

## 1. Task, source and candidate identity

- **Task:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`
- **Observed task SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Sole controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`
- **Observed current plan SHA-256:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`.
- **Original approved implementation base:** `61e7d010205f24478654a1b37e47ae75aec14c76`.
- **Committed Chunk 3 candidate:** `19566eeea5ec33944fba3740bf00ff54f0450b8c`, parent `bd231b22ed18e41942ef2001bec71dca6d15ffe4`.
- **Observed branch:** `feat/004b-resource-activation-deactivation`; the branch and its `origin` tracking ref pointed to `19566ee` at inspection.
- **Commit contents observed:** 14 files, 2,149 insertions and 13 deletions. It contains the accepted Task 4B Chunk 1–3 implementation/tests, Migration 006 and its tests, verification updates, and the three chunk handoffs. The commit subject is `feat(resources): add guarded activation state service`.
- At the start of this post-commit handoff refresh, `git status --porcelain=v1` was empty at `19566ee`. This documentation update creates the only working-tree delta: this existing handoff is modified and the paired instruction is new/untracked. No implementation source, tests, migration, or configuration was changed; nothing was staged or committed during this refresh.
- The task and plan are tracked/committed and their current hashes match the previously recorded values. The plan hash at the original base is `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b`; the documented difference is three path redactions, with no change to scope, requirements, ladder, or exit criteria.
- **Preflight provenance:** `PREFLIGHT_PASS` was reported for the Task 4B implementation source/base before Chunk 3. This refresh re-read the complete current Task 4B contract and controlling plan and measured their hashes; it did not run an implementation preflight for Chunk 4. A new session must revalidate its own authority, base and preflight gate.
- The committed handoff version immediately before this refresh had SHA-256 `633bd54d9c4b4fba0a352e6b966029a4a86cea73e469e22656bbd4b5252e168a`. It described the same candidate while still in the pre-commit dirty state. That historical blob remains available in Git; this working copy corrects the candidate/commit status without rewriting history.

## 2. Execution mode and authorization

- Chunk 3 was executed in **SINGLE-CHUNK** mode. The prior user request explicitly authorized Chunk 3 and its handoff; it did not authorize Chunk 4.
- The current user request authorizes updating this handoff after commit. The paired instruction is only a prepared continuation packet. **Chunk 4 authorization is NOT GRANTED by this handoff or instruction.**
- The Task 4B document itself is a bounded contract; the prior explicit Chunk 3 request supplied the applicable implementation authority. Do not infer future authority from the commit, branch, `CONTINUABLE` status, or this handoff.
- **Retry policy:** two failed validation attempts per issue. Chunk 2's fixture-typing issue used one failed attempt and was repaired with explicit casts. Chunk 3's fake begin-call expectation used one failed attempt and was corrected. They are distinct resolved issues; neither history is reset. The over-anchored regression selector passed but selected only one test; it was corrected and is not a failed validation attempt.
- **Current refresh activity:** read-only repository/source inspection and documentation authoring only. No tests, builds, Docker, migrations, database operations, or persistent-target access were performed in this refresh. The prior handoff reported cleanup of its disposable Compose project; current Docker state was not checked. Initial documentation checks found trailing Markdown spaces in both artifacts; they were removed. Final `rtk git diff --check` and the whitespace scan passed. These were documentation-format corrections, not implementation-test failures.

## 3. Completed Chunk 3 — Coordinator activation vertical slice

**Controlling-plan title:** “Chunk 3 — Coordinator activation vertical slice.”
**Objective:** “inactive→active and authorized active→active no-op via trusted service/runtime database, V1/V2/V6/V7; FR-002/009/023/026–027, AC-016/021/028/041/044 state facets.”

The committed slice implements:

- `internal/resources/state.go`: `StateService.SetState` canonicalizes the trusted actor and Resource UUID; resolves stored Coordinator authority before target validation or successful no-op; locks and separately reads the authoritative Resource; supports Inactive→Active and an authorized Active→Active no-op.
- Active no-op commits unchanged without reading the clock, checking occupancy, updating the Resource, or attempting Activity. Actual activation samples UTC microsecond operation time after the Resource lock, invokes the guarded capability, and commits once.
- Deactivation remains deliberately fail-closed in the service and adapter. An authorized Inactive request returns an operational error; the incomplete path performs no occupancy check or mutation. No web/application caller was added.
- `internal/database/resource_state.go`: narrow READ COMMITTED adapter reuses existing stored-role, Resource-lock/read, commit, and rollback behavior. Activation uses Migration 006's guarded function through the actual runtime pool; no direct Resource write grant or repository widening was added.
- `internal/resources/state_test.go`: explicit desired-state contract and activation authorization/order/no-op/time/error/rollback coverage, with deactivation kept fail-closed.
- `internal/database/resource_state_integration_test.go`: real-PostgreSQL service/adapter activation, authorization, retained data and history, ordinary Booking after activation, failure rollback/retry, no-op detectors, and observed post-lock clock behavior.
- Chunk 2's Migration 006 capability remains able to guard both directions at the database boundary. No migration was edited in Chunk 3.

**Not added:** HTTP routes/forms/browser workflow, Booking changes, or Chunk 4+ application behavior. `cmd/labreserve` and web composition do not call the state service yet. The next chunk must preserve this compile-safe intermediate state until deactivation is complete.

## 4. Verification evidence and limits

The following results were **executed and reported in the Chunk 3 implementation session**, before commit. This refresh did not rerun them. The listed source hashes match the fingerprints recorded for that tested candidate. `19566ee` has `bd231b2` as its parent; this refresh changed no implementation source, and tests remain prior-session evidence rather than post-commit reruns.

| Prior command/check | Reported result and limits |
| --- | --- |
| `rtk go version && rtk go test ./internal/resources` baseline | PASS; host Go 1.27.3 and 43 pre-existing Resource tests. |
| First post-edit `rtk go test ./internal/resources` | One failed attempt for a test expectation: `begin calls = 1, want 0`. The fake records the begin method invocation even when begin returns an error; expectation corrected to one. |
| `rtk gofmt -w internal/resources/state_test.go && rtk go test ./internal/resources` | PASS; 58 Resource package tests. Current code fingerprints match this candidate. |
| `rtk proxy go test -run '^$' ./...` and application `go build` | PASS; compile-only across packages, no behavior tests selected by the first command; application build succeeded. |
| Verification-image build, disposable PostgreSQL 17.7 startup, explicit migration and seed | PASS, reported from project `labreserve-verify`; the prior handoff reports removal of its container, volume and network. This refresh did not verify current Docker state. |
| State unit test discovery and `go test -count=1 -v -run '^TestResourceState' ./internal/resources` | PASS; seven state unit tests discovered and executed. |
| Database focused test discovery and `go test -count=1 -v -p 1 -run '^(TestResource(Creation|Edit|Capabilities)|TestResourceState(Activation|Capability)|TestBookingCreationValidationAndEligibilityOutcomes)' ./internal/database` | PASS; 29 top-level real-PostgreSQL tests and subtests reported, no skips. Included new activation, Migration 006 capability/privilege, Resource regressions and Booking eligibility. |
| `rtk gofmt -l` on changed Go files and `rtk git diff --check` | PASS, reported before commit. |
| Full `make verify`, browser tests and later-chunk race/commit/lifecycle suites | NOT_RUN for Chunk 3; not its required focused gate or were assigned to later chunks. Full delivery verification is reserved for Chunk 11. |

An initially end-anchored regression selector passed while selecting only its exact Booking test. Test discovery exposed the narrow selection; the corrected prefix selector above then ran the intended 29. No required selected test was skipped. The historical Chunk 2 failed run remains recorded in `task-004b-chunk-2-to-3.md`.

### Current source fingerprints observed

These match the source fingerprints recorded for the tested Chunk 3 candidate:

- `internal/resources/state.go`: `a4b52bb771b49f0bac658557355db6ed3d21f1c921fa325a048dd4d973b63c35`
- `internal/resources/state_test.go`: `8b8a145095999ed970ba6d2cb697fa1f562456e7ffa0bf5f43598540b252eaa6`
- `internal/database/resource_state.go`: `ce2206785e3e7d34f098241a43f9cb26ac7df4693321362ffcf8621dff205ad4`
- `internal/database/resource_state_integration_test.go`: `7a7a1bed055ec00c748258707be7dbf71b34630588126ab621f3c35484d16d3d`
- `internal/database/resource_state_capability_integration_test.go`: `4923397dd1d305915da39f65dafd6b447ab74bcdc4006e66e2623a7b56171c14`
- `internal/database/resource_state_migration_integration_test.go`: `b9acefac467ad644cc73c5d244d6125a86d2b67090dd7c056ac88ad199de01ed`

Migrations observed at the committed candidate:

- 001 `2ba81c1dd6b9c31784b5912134a9159406f69d322d4edd13340ab612b9cd719f`
- 002 `958b1149717b93226d6d09feb0c023bf2a95f4859b5f3df482fd309625de45e4`
- 003 `c357dce16cad611752c847c5db4e30261949c9f69bc39e1a9edd6215fefedcbf`
- 004 `1a61db4263980090b2074ff6b3d9b220a40d045929a1d8786d63c1cf090f7d45`
- 005 `ea30417512fec6da02ed40d90502a5bbd4991a8e226f12ff56bce3504cd38ec7`
- 006 `84330d7bd227f56e004da61ed3169e76deb3e77a0478e749c4374cb744dd8d7f`

## 5. Candidate, diff and preservation audit

- At refresh start, the worktree was clean at `19566ee`; the latest commit contains the complete 14-file Task 4B candidate described above. The current documentation-only delta is limited to the modified evidence handoff and its paired new instruction. There are no staged changes and no implementation delta from HEAD.
- This refresh re-read the full Task 4B contract, full controlling plan including its ending, the current handoff and relevant prior handoffs. It inspected the commit inventory/stat, complete tracked Chunk 2 edits, Migration 006, service/adapter, unit and PostgreSQL integration tests. No source/plan drift or unrelated file was found.
- The plan and task are unchanged; migration hashes 001–006 remain as recorded. Chunk 1 contracts, Chunk 2's guarded capability/migration tests, and accepted Resource create/edit and Booking behavior remain in the commit.
- Corrected historical plan-hash evidence in `task-004b-chunk-2-to-3.md` remains preserved; do not edit that historical handoff to erase its earlier error.

## 6. Risks, gaps, decisions and rollback

- **Functional:** deactivation is intentionally unavailable in the service until Chunk 4. It must remain fail-closed; no success may be inferred from an occupancy error.
- **Integration:** no HTTP/session-to-state-service route exists; Task 4C remains out of scope. Current refresh did not start Docker or inspect a database.
- **Validation gap:** Chunk 3's tests are reported from the prior execution and tied to matching current source fingerprints, but were not rerun after commit. No independent implementation review has occurred. Full `make verify`, deactivation boundaries/races, actual COMMIT rejection, ambiguous acknowledgement, and restart lifecycle remain unverified in this chunk or assigned to later chunks.
- **Migration/data integrity:** Migration 006 is additive and unchanged. Compatibility with an uninspected persistent Migration 005 ledger remains unresolved; no persistent target was inspected or changed. Disposable migration evidence is not persistent-target certification.
- **Level 2 decisions:** none identified for Chunk 4 by the current task/plan. Any required change to product precedence, security/trust, locking/isolation, migration history, or task scope requires escalation rather than silent discretion.
- **Rollback:** no rollback was performed. The code candidate is committed; do not reset or rewrite history. Any source correction requires a separately authorized scoped change. These handoff documents are documentation-only working-tree edits; do not discard or overwrite them as cleanup.

## 7. Exact next boundary and transfer instructions

**Next planned chunk:** **Chunk 4 — Safe deactivation, occupancy boundaries and complete no-ops**, exactly as section 9 of the controlling plan specifies.

The paired instruction describes the plan-grounded scope and required evidence. Before implementation, a separately authorized session must revalidate current authorization, branch/HEAD/status, task/plan identities and candidate diff. Preserve Chunk 3 activation and Migration 006. Implement both real no-ops before occupancy evaluation; for actual Active→Inactive only, use fresh post-lock time and a separate all-owner Confirmed query with `end_at > operation_time`. Do not rewrite Migration 006 for convenience, modify Bookings, add HTTP/browser work, start Chunk 5, or treat the prepared instruction as authorization.

Persistent Migration 005 compatibility is a prerequisite only before any persistent-target operation. Stop on a divergent historical checksum; do not rewrite a ledger or reset a target.

## 8. Suggested skills

- `chunked-implementation` — SINGLE-CHUNK mode, only after separate Chunk 4 authorization and preflight.
- `pre-read-discipline`, `pre-edit-discipline`, and `post-edit-discipline` — current source/diff discovery and bounded implementation verification.
- `rtk-command-prefix` — repository and test commands.
- `specification-readiness-authority` — only if the current authorization/preflight or source-freshness gate needs revalidation.
- `handoff` — only for a separately authorized next transfer.

**Chunk 3 status: CONTINUABLE.**
