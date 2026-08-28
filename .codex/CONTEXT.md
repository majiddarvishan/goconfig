# Project Context

## Scope

Review and improve the core `goconfig` package. The code under `examples/` is known to be incorrect and must not be used as a behavioral reference.

## Current architecture

1. `Manager` owns one canonical ordered configuration snapshot and a compiled JSON Schema.
2. The public `Node` tree is a derived view. `Manager.Config`, object getters, array getters, query results, handlers, and observers expose independent or structurally immutable snapshots.
3. `OnInsert`, `OnRemove`, and `OnReplace` register canonical JSON Pointer paths. Snapshot nodes retain private owner/path metadata so registration does not require an internal mutable pointer.
4. A mutation clones the committed snapshot, applies one typed path operation, runs schema/custom/external validation, and invokes the compatibility pre-commit handler without holding a Manager lock.
5. Commit reacquires the Manager lock, rechecks the base version, persists the candidate, and atomically publishes config, Node view, paths, version, and history.
6. Post-commit observers run after the Manager lock is released and cannot veto the committed change.
7. `FileSource` and `StrSource` implement both the legacy v1 source contract and the new externally implementable `Source` API.
8. Query, history, validation, and HTTP are consumers of coherent Manager snapshots.

## Working constraints

- Preserve user changes and avoid unrelated edits.
- Establish tests before structural refactoring.
- Prefer backward-compatible API evolution; breaking changes require an explicit major-version decision.
- Treat one immutable configuration snapshot as the desired source of truth.
- Run formatting, unit tests, race tests, vet, and static analysis after implementation.

## Baseline verification

- The missing direct `orderedmap` and `cors` dependencies have been added to `go.mod`.
- The supported module language baseline remains Go 1.18; Phase 1 was verified with Go 1.26.0.
- The root and `history` packages have characterization tests and pass core compilation.
- `examples/` is intentionally excluded from core checks because it contains mixed packages and a placeholder import.
- The installed `staticcheck` binary is incompatible with the environment's Go standard library. `.codex/check.sh` runs it only when `RUN_STATICCHECK=1` and a compatible binary is available.
- Phases 3 and 4 were completed with 50 tests, 64.7% root-package statement coverage, and passing race and vet checks.
