# Implementation Plan

## Phase 1: Restore the baseline

Status: completed.

- Add missing direct dependencies and tidy the module.
- Confirm the supported Go version.
- Establish repeatable commands for format, test, race test, vet, and static analysis.
- Add characterization tests for sources, nodes, mutations, paths, validation, query, history, and HTTP behavior.

Acceptance criteria:

- `.codex/check.sh` runs format verification, tests, race tests, and vet for the core packages from a clean checkout.
- Core desired behavior is protected by tests before refactoring.

`go test ./...` intentionally remains outside the core acceptance command because it includes the known-invalid `examples/` directory.

## Phase 2: Define contracts

Status: completed. Decisions are recorded in `.codex/CONTRACTS.md`, and canonical JSON Pointer parsing/escaping has executable tests.

- Adopt JSON Pointer semantics, including escaping and an explicit representation of root.
- Separate pre-commit validators from post-commit observers.
- Decide whether schema is optional or required.
- Define external-validation timeout and failure policy.
- Decide which API changes remain compatible and which require a future v2.

Acceptance criteria:

- Mutation ordering, callback behavior, errors, paths, and compatibility policy are documented and testable.

## Phase 3: Build a transactional mutation engine

Status: completed.

- Replace the three duplicated path mutators with one typed resolver and operation engine.
- Perform expected-version comparison inside the commit lock.
- Clone a single canonical snapshot and apply the operation to that snapshot.
- Run local schema, custom, and external validation against the candidate snapshot.
- Persist the candidate before publishing it as current state.
- Atomically update the in-memory snapshot, version, registered paths, and history.
- Invoke immutable post-commit notifications after releasing the lock.
- Remove or correctly implement path caching based on benchmarks.

Acceptance criteria:

- Failed validation or persistence never changes visible state or version.
- Concurrent operations cannot lose updates.
- Source, Node views, version, paths, and history always describe the same committed snapshot.

## Phase 4: Encapsulate Node and Source

Status: completed.

- Make `Manager.Config` return a safe snapshot.
- Prevent mutable maps, slices, and internal Source pointers from escaping.
- Normalize number decoding with `json.Number` or another explicit numeric model.
- Reject fractional and overflowing integer conversions.
- Return errors for unsupported input types instead of converting them to null.
- Redesign `ISource` as either a genuinely extensible exported interface or an intentionally private implementation detail.
- Compile and cache JSON Schema when the Manager is constructed.

Acceptance criteria:

- Public reads cannot mutate Manager state.
- Numeric behavior is explicit and covered across JSON and programmatic inputs.
- External packages can implement Source if extensibility is retained.

## Phase 5: Correct validation, persistence, and history

Status: completed.

- Export and rename validation types idiomatically.
- Validate complete candidate values consistently for insert, remove, and replace.
- Integrate external validation into the mutation pipeline with context support.
- Normalize enum comparisons and implement real regex/glob semantics or rename the validator.
- Make FileSource use a unique same-directory temporary file, preserve permissions, clean up failures, and fsync where appropriate.
- Deep-copy history payloads, validate limits, and make standalone history access thread-safe.
- Make history capacity configurable.

Acceptance criteria:

- Every mutation goes through the same validation policy.
- Persistence failure injection leaves both disk and memory in a defined state.
- History cannot be mutated indirectly by callers.

Implementation notes:

- `Validator`, `CustomValidator`, `ValidationService`, and correctly cased constructors are public; v1 compatibility adapters remain available.
- Insert/remove validators receive complete target arrays before and after mutation, while replace validators receive complete target values.
- `ValidatePattern` now has regex semantics with legacy `*` support; `ValidateRegexp` reports compilation errors at construction time.
- Numeric enum/unique comparisons normalize exact rational values rather than coercing all numbers to `float64`.
- File replacement uses a unique same-directory temporary file, preserves permissions, syncs content and the parent directory, and cleans temporary files on failure.
- Rename is the persistence commit point: pre-rename failures change neither disk nor memory; a post-rename directory-sync failure is treated as committed so Source and Manager snapshots cannot diverge.
- History owns deep copies, validates read limits, serializes standalone access, and has configurable Manager capacity.

## Phase 6: Redesign the HTTP boundary

Status: completed.

- Expose a reusable `Handler() http.Handler` and make standalone serving a thin wrapper.
- Return startup and shutdown errors instead of panicking or printing.
- Correctly integrate user-provided servers and route registrars.
- Decode requests into explicit structs using strict integer validation.
- Map syntax, authentication, conflict, size, validation, and internal failures to appropriate HTTP statuses.
- Build each response from one coherent Manager snapshot.
- Add configurable CORS, logger, authentication, timeouts, and health policy.
- Retain only the API-key hash.

Acceptance criteria:

- HTTP integration works with standalone, supplied-server, and supplied-router modes.
- Concurrent requests with one expected version result in exactly one successful commit.

Implementation notes:

- `HTTPServer.Handler` and `Manager.Handler` expose the reusable boundary.
- `Start`, `Shutdown`, `StartHTTPServer`, and `ShutdownHTTPServer` return lifecycle errors; legacy Manager methods remain deprecated adapters without panic/printing.
- Supplied `http.Server` handlers are preserved as fallbacks, and route-registrar mode mounts the same handler without opening a listener.
- POST uses an explicit strict request type, exact JSON integers, unknown/trailing-field rejection, operation-specific fields, and configurable body limits.
- CORS, logger, authenticator/API key, timeouts, maximum body size, and health policy are options.
- The HTTP server retains only an API-key digest and maps public errors without exposing persistence internals.

## Phase 7: Query, structure, and cleanup

Status: completed.

- Use the canonical escaped path implementation in query and mutation code.
- Make wildcard/filter error behavior explicit and deterministic.
- Never invoke predicates with mutable internal nodes under a Manager lock.
- Split Manager responsibilities across focused files.
- Rename public APIs idiomatically while providing compatible aliases or deprecations where required.
- Remove dead constants, stale comments, duplicate code, and unreachable features.
- Synchronize core documentation with implemented behavior; do not use `examples/` as a source of truth.

Acceptance criteria:

- Public API documentation matches behavior.
- Core files have focused responsibilities and no duplicated path traversal logic.

Implementation notes:

- Direct query segments and result paths use canonical escaped JSON Pointer semantics; plain array indexes are supported alongside compatible bracket syntax.
- `Lookup` provides exact, non-wildcard JSON Pointer access for otherwise ambiguous keys.
- Wildcard/filter traversal is lexical and fail-fast on the first deterministic branch error instead of silently discarding it.
- `FindAll` traverses an independent snapshot and never invokes predicates under a Manager lock.
- Manager history, validation, HTTP, and mutation responsibilities are split into focused files.
- Public `Mutation`/`Mutate` and `Insert`/`Remove`/`Replace` APIs expose the transactional engine outside HTTP.
- Idiomatic `History`, `HistoryByPath`, `CustomValidator`, `RegisterValidator`, `Snapshot`, `HTTPServer`, and related names coexist with deprecated v1 adapters.
- The stale root README and dead schema-validation wrapper were replaced/removed.

## Phase 8: Verification and hardening

- Add integration tests for FileSource and HTTP.
- Add concurrency and race tests.
- Add fuzz tests for paths, query expressions, JSON requests, and mutations.
- Add failure-injection tests for persistence and validation.
- Benchmark cloning, schema validation, and path lookup before optimizing.
- Run formatting, all tests, race detector, vet, and compatible static analysis in CI.

Acceptance criteria:

- All checks pass in a clean environment.
- Critical mutation and concurrency behavior is covered by deterministic tests.

## Phase 9: Organize the project and rebuild examples

- Group implementation files into focused packages/directories where doing so improves ownership without introducing import cycles or unnecessary public API breaks.
- Establish clear locations for core configuration, sources, validation, history, HTTP integration, internal helpers, tests, and documentation.
- Keep the root public package stable through compatible forwarding APIs where files or implementations move.
- Remove obsolete, duplicate, and misleading files after confirming their replacements.
- Replace the known-invalid `examples/` tree with small, buildable examples that use the finalized public API.
- Add examples for string and file sources, mutation/validation, history, HTTP handler integration, and external validation where practical.
- Make examples part of `go test ./...` and CI verification.
- Document the resulting repository layout and how each example is run.

Acceptance criteria:

- Every production file has a clear responsibility and package ownership.
- `go test ./...` succeeds with all examples included.
- Examples compile, run without placeholder imports, and demonstrate supported behavior rather than legacy internals.
