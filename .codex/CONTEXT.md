# Project Context

## Scope

Review and improve the `goconfig` module. The originally supplied `examples/` were not used as behavioral references; Phase 9 replaced them with executable programs derived from the finalized public API.

## Current architecture

1. `Manager` owns one canonical ordered configuration snapshot and a compiled JSON Schema.
2. The public `Node` tree is a derived view. `Manager.Config`, object getters, array getters, query results, handlers, and observers expose independent or structurally immutable snapshots.
3. `OnInsert`, `OnRemove`, and `OnReplace` register canonical JSON Pointer paths. Snapshot nodes retain private owner/path metadata so registration does not require an internal mutable pointer.
4. A mutation clones the committed snapshot, applies one typed path operation, runs schema/custom/external validation, and invokes the compatibility pre-commit handler without holding a Manager lock.
5. Commit reacquires the Manager lock, rechecks the base version, persists the candidate, and atomically publishes config, Node view, paths, version, and history.
6. Post-commit observers run after the Manager lock is released and cannot veto the committed change.
7. `FileSource` and `StrSource` implement both the legacy v1 source contract and the new externally implementable `Source` API.
8. Custom validation receives complete path values before/after each mutation; external validation receives the complete candidate configuration and caller context.
9. `FileSource` commits through a unique same-directory temporary file and atomic rename, preserving mode and syncing file/directory state.
10. History is an independently synchronized bounded circular log whose inputs and outputs are deep-copied.
11. HTTP exposes one reusable handler across standalone, supplied-server, and route-registrar ownership modes with strict input and coherent responses.
12. Query and `FindAll` traverse independent snapshots, return canonical JSON Pointer paths, and use deterministic lexical object order.
13. Public direct mutation, HTTP mutation, validation, persistence, history, and observers share the same transaction engine.
14. The module root is a stable facade; cohesive implementation and white-box tests live in `internal/core`.

## Working constraints

- Preserve user changes and avoid unrelated edits.
- Establish tests before structural refactoring.
- Prefer backward-compatible API evolution; breaking changes require an explicit major-version decision.
- Treat one immutable configuration snapshot as the desired source of truth.
- Run formatting, unit tests, race tests, vet, and static analysis after implementation.

## Baseline verification

- The missing direct `orderedmap` and `cors` dependencies have been added to `go.mod`.
- The supported module language baseline remains Go 1.18; Phase 1 was verified with Go 1.26.0.
- The root and `history` packages have characterization tests and pass full verification.
- Each `examples/<topic>` directory is an independent `main` package compiled by `go test ./...`.
- The installed `staticcheck` binary is incompatible with the environment's Go standard library. `.codex/check.sh` runs it only when `RUN_STATICCHECK=1` and a compatible binary is available.
- Phases 3 through 9 are complete with 89 deterministic tests, 4 fuzz targets, 3 benchmarks, buildable examples, and passing repeated, race, and vet checks.
