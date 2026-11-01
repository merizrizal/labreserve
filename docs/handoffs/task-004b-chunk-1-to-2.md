# Task 4B — Chunk 1 to Chunk 2 Handoff

## Task and candidate identity

- **Task:** Task 4B — Atomic Resource Activation and Safe Deactivation.
- **Contract:** `docs/tasks/004b-atomic-resource-activation-deactivation.md`.
- **Contract SHA-256:** `43e5644a4065fedf26685c2382c3239ca7c1e10e200ea6c71d10a8e803c7b49a`.
- **Controlling plan:** `docs/implementation-plan/004b-atomic-resource-activation-deactivation.md`.
- **Plan SHA-256 at this chunk:** `27a0bd146930680355b7557f9471c4df4b025b025418adaaf49a4f54b48fd49f`; this differs from the earlier supplied `aa02f5d8a6dd0e5f741dbfe3b4241ae4ed642d3eb778f3a632de2ebc66317c8b`. At the originally supplied base, the measured plan hash was `f6a266529f8178acd64ea0ad90b16477d7fcb3496d5ee19c7319dbb284cd68ff`; the supplied hash matched neither observed revision. The plan delta from that base to current HEAD consisted of documentation path substitutions. The exact current Chunk 1 scope was read and followed.
- **Chunk 1 implementation base:** `bd231b22ed18e41942ef2001bec71dca6d15ffe4`.
- **Branch:** `feat/004b-resource-activation-deactivation` (tracks `origin/feat/004b-resource-activation-deactivation`).
- **Candidate:** same HEAD plus two untracked Chunk 1 Go files and this untracked handoff. No changes staged or committed.
- The originally stated base was `61e7d010205f24478654a1b37e47ae75aec14c76`. After the base discrepancy was reported, the user explicitly acknowledged the intervening changes as expected and directed continuation. The approved task hash remained unchanged; the current base-to-HEAD delta contained documentation files only. No reset or history rewrite was performed.
- **Preflight:** current-task readiness was rechecked after the user's authorization to continue on the acknowledged candidate. Chunk 1 only; no continuous loop.

## Execution policy

- **Mode:** single chunk; Chunk 1 only.
- **Retry budget:** two failed validation attempts per issue. **Failed validation attempts used: 0.**
- **Environment:** repository and disposable Compose verification project `labreserve-verify`; no persistent database was started or modified. `.env` contents were not read or modified.
- **Handoff:** explicitly authorized by the user. This handoff is not authorization to execute Chunk 2.
- **Change boundary:** only the planned new `internal/resources/state.go` and `internal/resources/state_test.go`, plus this authorized handoff. No migration, database adapter, web caller, Booking, or Task 4C change.

## Completed Chunk 1 — Minimal State Contracts with Fail-Closed Entry Point

Added in `internal/resources/state.go`:

- `ErrResourceOccupied`, distinct from `ErrOperational`.
- `StateRequest{ResourceID, Active}`; `Active=false` is explicitly the desired Inactive state, not a toggle or omitted field.
- `StateResult{Resource, Changed}` for retained Resource data and changed/no-op distinction.
- `StateRepository.BeginResourceStateChange` and `StateTransaction`, embedding the existing `ResourceTransaction` and exposing only the planned lock, occupancy-query, and guarded state-operation contracts.
- `StateService`, `NewStateService`, and `SetState`.
- `SetState` returns an explicit `ErrOperational` (“not implemented”) for every valid-service request and for an uninitialized service. It does not begin a transaction, resolve authority, read the clock, lock/query/mutate, commit, or claim no-op/transition success.

Added in `internal/resources/state_test.go`:

- `TestResourceStateContractExplicitDesiredValue` covers explicit true/false desired values, changed/no-op result shape, and the distinct occupancy sentinel.
- `TestResourceStateUnimplementedFailsClosed` exercises Active and Inactive. It asserts explicit `ErrOperational`, zero result, no transaction begin, and no clock call. The absent transaction means there was no authorization, lock, occupancy query, mutation, commit, or retry.

No existing Resource create/edit interfaces were widened. No state callsites exist yet.

## Validation and evidence

All commands ran from the repository root. `rtk` is the supported wrapper.

| Command | Result |
| --- | --- |
| Baseline before edits: `rtk go version && rtk go test ./internal/resources` | **PASS**; host Go `1.27.3`, 39 existing tests passed. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify config --quiet` | **PASS**. No project containers or volumes existed before this chunk's verification. |
| `rtk gofmt -w internal/resources/state.go internal/resources/state_test.go` | **PASS**; formatted both new files. |
| `rtk gofmt -l internal/resources/state.go internal/resources/state_test.go` | **PASS**; no output (no unformatted files). |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify build verify-go` | **PASS**; rebuilt from the candidate source. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -list '^(TestResourceStateContractExplicitDesiredValue|TestResourceStateUnimplementedFailsClosed)$' ./internal/resources` | **PASS**; discovered both required test names. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 -v -run '^(TestResourceStateContractExplicitDesiredValue|TestResourceStateUnimplementedFailsClosed)$' ./internal/resources` | **PASS**; both tests and both Active/Inactive subtests executed and passed; no skips. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -count=1 ./internal/resources` | **PASS**; existing Resource service tests plus new tests passed. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go test -run '^$' ./...` | **PASS**; all packages and test packages compiled. Tests were intentionally not selected (`[no tests to run]`); this is compile evidence only. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go go build -o /tmp/labreserve-task4b-check ./cmd/labreserve` | **PASS**; application build succeeded. |
| `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify run --rm --no-deps verify-go gofmt -l internal/resources/state.go internal/resources/state_test.go` | **PASS**; no output. |
| `rtk git diff --check` | **PASS**. |

The host baseline used Go 1.27.3; project verification used the repository's `golang:1.26.0-alpine3.22` build image. No database tests were required for this contracts-only chunk, and `verify-db` was not started. Compose created only its isolated empty `labreserve-verify` volume/network while running containers; `rtk docker compose --project-name labreserve-verify --file compose.yaml --profile verify down --volumes --remove-orphans` removed them. Follow-up container/volume discovery showed none remaining. No validation attempt failed.

## Diff and risk review

- **Files changed:** two new Go files (no tracked application-file diff); this handoff is also new/untracked.
- **Complete content reviewed:** both Go files were read in full and the additions inspected with `rtk proxy git diff --no-index -- /dev/null <file>`. The prior Chunk 0 handoff remains tracked and unchanged.
- **Scope:** in scope. No opportunistic refactoring, callers, HTTP, Booking semantics, database adapter, migration, generic UPDATE access, or Task 4C code.
- **Functional/security/data-integrity risk:** the state API is intentionally unavailable. Both desired directions fail closed; no state or Activity mutation can occur through this stub. The new transaction method is only a narrow interface contract and has no implementation or caller.
- **Integration risk:** Chunk 2 and later must implement the planned guarded persistence capability before any service behavior can succeed. Future code must preserve Coordinator authorization, no-op/event rules, and Resource identity/descriptive protections.
- **Validation gap:** no database or runtime-role capability behavior exists or was tested in Chunk 1; no activation/deactivation business behavior is claimed.
- **Rollback:** remove only `internal/resources/state.go` and `internal/resources/state_test.go` if reverting this chunk; preserve both handoffs and unrelated work. Nothing was staged or committed.

## Migration 005/006 findings retained for Chunk 2

Carry forward the Chunk 0 findings without editing them in this chunk:

1. When adding Migration 006, keep `internal/database/booking_persistence_integration_test.go:413–440` anchored to the historical 001–004 range; do not let its `requiredSchemaVersion`-based fixture absorb migration 005.
2. Preserve separate 004→005 migration evidence in `TestResourceCapabilityMigrationPreservesVersion004Data`, and add distinct compatible 005→006 coverage rather than weakening the earlier assertion.
3. Preserve the conditional Migration 005 persistent-ledger compatibility restriction. Before any persistent-target upgrade, inspect versions/checksums/provenance non-destructively. If 005 has an earlier/different checksum or earlier persistent application is evidenced, stop and obtain an authorized non-destructive procedure. Never rechecksum/rewrite the ledger, replay 005, or reset a persistent database/volume. No persistent ledger was inspected here.

## Exact next-session instructions

A fresh session may proceed only after separately authorized for Chunk 2. Re-read repository instructions, the complete task contract and controlling plan, this handoff, and current source state. Reconfirm branch, HEAD, task/plan hashes, current authorization/preflight, and all migration hashes before editing; this handoff does not make the earlier plan hash current or authorize work automatically.

Then execute **Chunk 2 only — Guarded state capability and additive schema integration**, following its exact plan scope and verification requirements. The plan identifies the additive migration, guarded capability and its integration tests, schema-version/legacy migration-test adaptations, and verification-script assertions. Preserve migrations 001–005 byte-for-byte, the historical migration boundaries above, runtime least privilege, and the fail-closed service. Do not implement service callers, Chunk 3+, HTTP handlers/templates, Booking changes, or Task 4C. Stop after Chunk 2 and report its actual candidate, tests, risks, and migration checksum evidence.

**Chunk 1 status: CONTINUABLE.**
