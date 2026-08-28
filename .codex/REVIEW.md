# Technical Review

## Resolution status

Phases 3 and 4 resolve the original findings concerning temporary publication of mutations, optimistic-locking races, direct array replacement, duplicated reflection-based traversal, ineffective path caching, mutable Manager/Source snapshots, silent numeric truncation, unsupported-value coercion, repeated schema compilation, and the externally unimplementable source abstraction.

The remaining active work starts at Phase 5: validator API cleanup, FileSource durability details, history deep-copy/thread-safety guarantees, and the broader HTTP lifecycle redesign.

## Critical findings

### Build is broken

`go.mod` does not declare these direct dependencies:

- `github.com/iancoleman/orderedmap`
- `github.com/rs/cors`

### Internal state is exposed

- `Manager.Config` returns the internal root despite claiming to return a deep copy.
- `Node.GetObject` and `Node.GetArray` return mutable internal maps and slices.
- Source implementations return an internal `OrderedMap` pointer after releasing their lock.

These APIs allow unsynchronized mutation, stale path metadata, and divergence between the Manager tree and Source state.

### Mutation is not transactional

Insert, remove, and replace mutate the live Node tree and then release the Manager lock while invoking handlers. Persistence happens afterward. Concurrent operations can observe or overwrite uncommitted state, rollback can erase another operation, and handler side effects cannot be rolled back after persistence failure.

### Optimistic locking has a TOCTOU race

The HTTP layer checks `Version()` before acquiring the mutation lock. Multiple requests with the same expected version can pass the check and commit sequentially.

### Source and Node can diverge

The package maintains an `OrderedMap` in Source and a separate Node tree in Manager. Path mutation logic does not correctly support all array cases, including direct replacement of an array element and nested arrays. A mutation can update the two models differently.

## High-priority findings

### Path implementation

- Set, insert, and remove traversal code is duplicated.
- `reflect.TypeOf(nil).Kind()` can panic.
- Keys containing `/` cannot be addressed.
- Empty path is used for both root and not-found.
- The path cache is initialized invalid and never enters a normal rebuild path.

### Validation

- The configured external validation service is never called.
- Remove does not run custom validation.
- Insert validates only the inserted item, while validators such as `ValidateUnique` expect the full array.
- Numeric enum comparison is inconsistent across `int` and `float64`.
- `ValidatePattern` implements equality except for the special value `*`.
- Validator internals can be exposed without synchronization.
- JSON Schema is reparsed for every mutation.

### Node model

- JSON integers become `float64` and are reported as floating point.
- `GetInt` silently truncates fractional values and does not check overflow.
- `parseNode` silently turns unsupported values, including some otherwise recognized numeric types, into null.
- Object ordering is discarded when converting from `orderedmap` to a Go map.

### HTTP server

- Startup errors panic in a goroutine instead of being returned.
- Start, stop, and route setup can dereference a nil server.
- HTTP server state is accessed without Manager synchronization.
- `WithServer` does not attach the goconfig handler to the supplied server.
- Version and index accept fractional JSON numbers and truncate them.
- Oversized bodies, validation failures, conflicts, and persistence errors are mapped to inaccurate status codes.
- Response state is assembled using multiple independent locks and may be inconsistent.
- CORS, logging, authentication behavior, and health behavior are largely hard-coded.
- The raw API key is retained even though a hash is also stored.

### Source abstraction and file persistence

- `ISource` is exported but has unexported methods, so external packages cannot implement it.
- The interface says Source performs validation, while validation is actually owned by Manager.
- FileSource uses a fixed temporary path, forces mode `0644`, does not preserve permissions, and does not fsync the file and parent directory around rename.

## Medium-priority findings

### Query

- Query paths are not JSON Pointer compatible and cannot escape special keys.
- Wildcard and filter branches silently discard traversal errors.
- Object result ordering is nondeterministic.
- `FindAll` invokes user code with an internal node while holding a read lock, enabling mutation or deadlock.
- Root result paths are represented inconsistently as `/` and an empty string.

### History

- `ChangeHistory` is not independently thread-safe.
- Negative limits can cause incorrect behavior or panic.
- Event payloads are shallow copies and may change after insertion.
- Version starts from one again after process restart.

### Code quality

- Large Manager and utility files mix several responsibilities.
- Naming is not idiomatic in places: `ISource`, `HttpServer`, `handler_t`, and `NewvalidationService`.
- There are stale comments, commented-out behavior, unused constants, and documented features not implemented by the core API.
- There is no automated test suite.
