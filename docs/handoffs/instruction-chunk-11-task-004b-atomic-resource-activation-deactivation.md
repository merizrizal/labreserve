# LabReserve Task 4B — Prepared Instruction for Chunk 11

- **Instruction state:** `PREPARED_NOT_AUTHORIZED`
- **Source handoff:** [`task-004b-chunk-10-to-11.md`](task-004b-chunk-10-to-11.md)
- **Controlling ladder:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`, §9, Chunk 11 — **Delivery-Wide Verification, Documentation, and Candidate Review**.
- **Execution:** Not authorized by this instruction. Do not begin Chunk 11 unless a later user authorization separately names Chunk 11 and its execution mode.

## 1. Authority and entry gates

The user authorized Task 4B **Chunk 10 only**, in SINGLE-CHUNK mode, including its plan-required category-F `make verify` when confined to verified disposable resources. The user also authorized creation of the Chunk 10→11 handoff and this `PREPARED_NOT_AUTHORIZED` instruction. Those authorizations did not accept Chunk 10, authorize Chunk 11, or permit merge/deployment/Task 4C/integration. A later, separate user request authorized the Chunk 9 and Chunk 10 source commits followed by the handoff-document commit; the resulting implementation candidate is recorded below.

Before any later authorized Chunk 11 work:

1. Obtain explicit Chunk 10 acceptance and separate Chunk 11 authorization. A passing full gate and this `CONTINUABLE` handoff are evidence, not acceptance or execution authority.
2. Use `chunked-implementation` in the explicitly authorized mode; execute Chunk 11 only, then stop. Re-read current repository instructions, the complete Task 4B and controlling plan including endings, this instruction, the complete Chunk 10→11 handoff, prior handoffs, relevant PRD/architecture, and the Chunk 2/5 migration evidence.
3. Revalidate branch, HEAD/tree/upstream, staged/unstaged/untracked state, task/plan/migration fingerprints, source ancestry, all Chunk 0–10 evidence and exact candidate files. Preserve the dirty Chunk 9/10 work listed in the handoff; do not assume the current checkout is unchanged.
4. Inspect the complete migration ledger only on verified disposable targets; prove all normal operations use `labreserve_app`, setup/migration/inspection use allowed disposable test credentials, and never connect to a demo/persistent target. Migrations 001–006 are immutable. Do not inspect or print secrets.
5. Treat the earlier repository default project `labreserve-verify` and all retained Chunk 8/9/10 projects as protected until current ownership and isolation are rechecked. The successful Chunk 10 gate used and cleaned only `labreserve-verify-task004b-c10-b6c1fcc`; the separately retained focused Chunk 10 project remains in use. Select an unused, prefixed disposable project if needed.
6. The user’s category-F clarification is **Chunk-10-only**. It does not authorize Chunk 11. If Chunk 11 is later explicitly authorized, its controlling plan requires `F`; inspect project/volume effects again and run it only against a verified disposable project, preferably with a unique `LABRESERVE_VERIFY_PROJECT=labreserve-verify-...` value. Do not run the default against an existing retained `labreserve-verify` volume.

## 2. Chunk 11 objective and bounded scope

Complete the controlling plan’s delivery-wide V1–V9 audit, verification, accurate documentation/evidence and candidate review. No new feature is authorized. The plan’s Chunk 11 changes are limited to accurate README/evidence/plan corrections or removal of task-created accidental artifacts. Do not change product behavior, approved architecture, migrations, security boundaries, test expectations, or task scope to make verification pass. Do not add Task 4C routes/forms/browser acceptance, Coordinator intervention/cross-owner cancellation, Activity-history APIs/UI, or new infrastructure.

If a real defect appears, substantiate it, apply only a bounded in-scope repair authorized by the controlling plan and the future Chunk 11 authorization, then rerun every affected check and the required full gate. Escalate for any Level 2/product/security/architecture decision. Do not conceal required failures, skips, environmental gaps, or unresolved risks.

## 3. Required delivery-wide audit

Use the exact controlling plan §9 Chunk 11 checklist and §10 acceptance obligations. At minimum:

- Reconcile every V1–V9 requirement and each PRD/acceptance row against implementation and actual evidence. Check explicit desired state, authorization-before-no-op, Resource→Booking/Account lock compatibility, exact all-owner `confirmed AND end_at > fresh post-lock time` occupancy, separate READ COMMITTED query, no-op-without-UPDATE/event-attempt, correct atomic events, rollback/COMMIT/ambiguous-ack distinctions, retained raw identity/text/Booking/request/cancellation/history, and trusted actor/time limitations.
- Inspect effective table/column/function/schema ACLs, SECURITY DEFINER ownership/search path, direct-write/replace/alter/assume-owner/shadow abuse tests, and preserved architecture-003 cancellation limitation. Do not broaden grants or claim arbitrary SQL actor/time authentication.
- Verify fresh installation, compatible 005→006 additive upgrade, preserved 004→005 evidence, exact ledger rejection, repeated migration/seed, unchanged migrations 001–006, and both existing/new Resource and Booking lifecycle fixtures. Never migrate a persistent target; an earlier/different 005 checksum remains a stop condition for any separately authorized persistent upgrade.
- Discover focused unit, database, web and old 4A/2A–3C regression tests before executing exact selectors. Inspect skips and prove the separately flagged restart fixture/replay phases actually ran. A test skipped in the general suite is not lifecycle evidence.
- Run formatting, all-package compile, application build, shell syntax if relevant, `git diff --check`, full `make verify` (`F`) and complete candidate diff/scope audit. Run F only in a reverified disposable Compose project; preserve other volumes. The prior successful Chunk 10 result is not a substitute for a future authorized Chunk 11 gate if the candidate changes.
- Finalize README/evidence with actual commands/results, candidate/base/fingerprints, choices/risks, migration/permission/recovery limits, and explicit Task 4C/intervention/history deferrals. Do not describe the internal state capability as a browser-management feature.

## 4. Known Chunk 10 candidate/evidence at handoff

See the source handoff for the complete changed-path inventory, exact source/migration hashes, test selectors/results, candidate and environment details. In summary, Chunk 10 added only the retention/lifecycle database tests, an existing read-path session test, `scripts/verify.sh` lifecycle/project wiring, and README documentation. It did not change production service code or migrations. The final authorized `make verify` passed, including 8/8 Playwright tests and explicit state fixture/replay phases around real PostgreSQL/application restarts; focused migration selectors and compilation/format/syntax checks also passed.

The Chunk 10 verification base was `b6c1fcc7a55fdc7b3542fc0ce9068a864eeddb5c`, branch `feat/004b-resource-activation-deactivation`. After verification, the authorized Chunk 9 test commit is `c1655be9ab0b45dbb8279f4fcdc99afeeda80bfb`, followed by the Chunk 10 implementation/test commit **`b59c8acd2bb1813513d8dbcce1430b5052b7739a`** (tree `54a25d599009ef671a502b0f3549547143efaaef`). That is the implementation candidate hash; the `docs/handoffs` commit is intentionally separate and last. Revalidate everything before use. No current Chunk 11 check has run.

## 5. Environment and safety

- Never run verification against demo/persistent DBs. Do not run `make reset-demo`, `docker compose down -v` on retained projects, ledger edits/rechecksumming, migration rollback, or destructive compatibility repair.
- The full-gate script removes the Compose project selected for its run. Prove that exact name is unused and disposable before executing it. Preserve the default `labreserve-verify` volume unless separately authorized.
- Do not read/log `.env`, credentials, passwords, or token values. Never commit local `.env`.
- No staging, commit, push, merge, rebase, deploy, or credential modification without separate explicit authorization.
- Failed validation budget is two attempts per issue and is cumulative across sessions; use the handoff’s failure/repair history and do not reset counts. Diagnose and repair only within authority. Required unavailable tooling/access is a blocker, not a pass.
- Independent implementation review must use a fresh session; self-check is not independent review or human acceptance.

## 6. Exact stop condition

Stop after Chunk 11. Report `READY_FOR_REVIEW` only when every mandatory acceptance item and delivery-wide verification actually pass, the complete relevant diff is scoped, and no required evidence is skipped/unresolved. Otherwise report `BLOCKED` or `INCOMPLETE` with substantiated evidence. Do not proceed to acceptance, commit, merge, deploy, Task 4C, or any later work.
