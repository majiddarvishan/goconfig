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
- [x] Phase 6 HTTP boundary redesign completed.
- [x] Phase 7 query, structure, and cleanup completed.
- [x] Phase 8 verification and hardening completed.
- [x] Phase 9 repository-organization and executable-examples phase added to the roadmap.

## Phase 2 decisions

1. Preserve source compatibility during v1; incompatible removals are deferred to v2.
2. JSON Schema remains mandatory in v1. Optional validation requires an explicit future option.
3. Existing handlers remain pre-commit veto hooks; a separate post-commit observer API may be added.
4. Configured external validation fails closed by default.
5. Serialized configuration preserves order where supported; object-query results become lexically deterministic.

Full contracts are in `CONTRACTS.md`.

## Implementation order

Phases 1 through 8 are complete. The next implementation work is Phase 9: reorganize the repository and rebuild `examples/` as supported, executable programs.

## Phase 1 verification

- Core test packages: `.` and `./history`
- Deterministic tests after Phase 8: 89, plus 4 fuzz targets and 3 benchmarks
- Statement coverage after Phase 8: 75.0% for the root package and 93.9% for `history`
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

## Phase 6 implementation summary

- Reusable `HTTPServer.Handler` and zero-configuration `Manager.Handler`
- Public idiomatic `HTTPServer` API with v1 spelling aliases
- Error-returning standalone and supplied-server lifecycle
- Correct supplied-handler fallback and external route-registrar integration
- Strict operation-specific request decoding with exact integer semantics
- Coherent snapshot responses and explicit 400/401/404/409/413/415/422/500 mappings
- Configurable CORS, logging, authentication, timeouts, body size, and health policy
- Digest-only API-key retention and constant-time comparison
- Deterministic HTTP expected-version concurrency coverage

## Phase 7 implementation summary

- Canonical escaped JSON Pointer query paths and plain array indexes
- Exact `Lookup` API for keys that overlap query-extension syntax
- Lexically ordered wildcard/object traversal with explicit branch errors
- Snapshot-based `FindAll` predicates with no Manager lock held
- Exact rational numeric filter comparison and stricter filter parsing
- Focused Manager history, validation, HTTP, mutation, and core files
- Public transactional mutation API with expected-version/context support
- Typed invalid/denied mutation and query errors
- Idiomatic history, validation, HTTP, and snapshot APIs with compatibility adapters
- Root README synchronized with implemented public behavior

## Phase 8 implementation summary

- End-to-end HTTP transport, transaction, FileSource persistence, history, and read integration coverage
- Deterministic persistence failure injection with rollback and temporary-file cleanup assertions
- External-validation transport, status, decoding, and rejection failure coverage
- Fuzz targets for JSON Pointer, query traversal, HTTP decoding, and mutation application
- Benchmarks for cloning, compiled-schema validation, and path lookup, with recorded baselines
- CI on minimum and stable Go versions with format, tests, race, vet, repetition, bounded fuzzing, and benchmark smoke runs
