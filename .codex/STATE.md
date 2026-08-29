# Work State

## Current status

- [x] Repository structure inspected.
- [x] Core package files reviewed.
- [x] `examples/` excluded from review.
- [x] Baseline build issue confirmed.
- [x] Temporary dependency-assisted compile and vet completed outside the workspace.
- [x] Improvement plan prepared.
- [x] Implementation authorized.
- [x] Baseline dependencies fixed.
- [x] Characterization tests added.
- [x] Phase 1 completed.
- [x] Phase 2 contracts defined.
- [x] Core refactoring started.
- [x] Phase 3 transactional mutation engine completed.
- [x] Phase 4 Node and Source encapsulation completed.
- [x] Phase 5 validation, persistence, and history hardening completed.

## Phase 2 decisions

1. Preserve source compatibility during v1; incompatible removals are deferred to v2.
2. JSON Schema remains mandatory in v1. Optional validation requires an explicit future option.
3. Existing handlers remain pre-commit veto hooks; a separate post-commit observer API may be added.
4. Configured external validation fails closed by default.
5. Serialized configuration preserves order where supported; object-query results become lexically deterministic.

Full contracts are in `CONTRACTS.md`.

## Implementation order

Phases 1 through 5 are complete. The next implementation work is Phase 6: HTTP boundary redesign.

## Phase 1 verification

- Core test packages: `.` and `./history`
- Characterization and contract tests: 61
- Statement coverage after Phase 5: 66.6% for the root package and 93.9% for `history`
- `go test`, `go test -race`, and `go vet`: passing
- Formatting check: passing
- `go test ./...`: intentionally not an acceptance command because the excluded `examples/` directory contains mixed packages and a placeholder import
- `staticcheck`: command is wired behind `RUN_STATICCHECK=1`; the currently installed binary is incompatible with the environment's Go standard library

## Phase 3 and 4 implementation summary

- One canonical Manager-owned ordered snapshot with a derived Node tree
- Unified RFC 6901 resolver for object, direct array-element, and nested-array mutations
- Typed path, validation, version-conflict, and persistence errors
- Atomic expected-version check during commit
- Private candidate validation and pre-commit callbacks without Manager locks
- Persistence before atomic publication of state, version, paths, and history
- Post-commit observer API with independent change snapshots
- Removal of the unsafe legacy mutation implementations and ineffective path cache
- Deep-copy boundaries for Manager, Source, object, and array reads
- Exact JSON integer decoding with checked integer conversion
- Strict rejection of unsupported and non-finite mutation values
- Public `Source`/`SourceData` contract and `NewManagerFromSource` for external implementations
- Compiled-once JSON Schema reuse
- Coherent HTTP config responses from one Manager snapshot

## Phase 5 implementation summary

- Public idiomatic validation API with deprecated-compatible legacy names
- Complete candidate values for insert, remove, and replace custom validation
- Context cancellation propagated through configured external validation
- Real regular-expression validation and exact normalized numeric enum comparisons
- Unique same-directory FileSource temporary files with permission preservation, cleanup, file sync, atomic rename, and directory sync
- Explicit pre-rename failure and post-rename commit behavior preventing disk/Manager divergence
- Standalone thread-safe history with deep-copied payloads and safe non-positive limits
- Configurable history capacity through `WithHistoryCapacity`
