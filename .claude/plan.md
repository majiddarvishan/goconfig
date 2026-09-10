# Plan: decouple Manager from HttpServer + rewrite examples

Goal: `goconfig.Manager` and the HTTP admin API must be fully independent — Manager usable with
zero HTTP code, and the HTTP routes registrable either on goconfig's own server or on a caller's
own web service. Implemented as a package split: `httpserver` sub-package depends on `goconfig`,
never the reverse. See `.claude/PROJECT.md` for the full architecture notes once updated.

Legend: `[x]` done, `[ ]` not done yet.

## 1. Explore & document existing library

- [x] Explore repo structure, actual public API, and doc drift between README/HTTP_SERVER.md and source
- [x] Write `.claude/PROJECT.md` with project overview + known issues

## 2. Decouple Manager from HttpServer (package split)

- [x] Remove `httpServer *HttpServer` field from `Manager` (manager.go)
- [x] Remove `NewHttpServerFromNode` / `NewHttpServer` / `StartHttpServer` / `StopHttpServer` / `SetupRoutes` methods from `Manager`
- [x] Export mutation methods: `insert`→`Insert`, `remove`→`Remove`, `replace`→`Replace`
- [x] Export path getters: `getInsertablePaths`→`InsertablePaths`, `getRemovablePaths`→`RemovablePaths`, `getReplaceablePaths`→`ReplaceablePaths`
- [x] Add `Manager.ConfigJSON()` / `Manager.SchemaJSON()` passthroughs (so the HTTP layer doesn't need `ISource`'s unexported methods)
- [x] Create new `httpserver/` package: move `http_server.go`, `mux_adapter.go`, `route_registrar.go`, update to use `*goconfig.Manager` and the new exported API
- [x] Export `NewHttpServer` / `NewHttpServerFromNode` as top-level `httpserver` package constructors returning `(*HttpServer, error)`
- [x] Export `registerRoutes` → `RegisterRoutes(r RouteRegistrar) error` for standalone route registration onto a caller's own web service
- [x] Delete old `http_server.go` / `mux_adapter.go` / `route_registrar.go` from package root
- [x] `go mod tidy` (fixes pre-existing missing `iancoleman/orderedmap` / `rs/cors` requires)
- [x] `go build` + `go vet` on `.`, `./httpserver`, `./history` — passing
- [x] Scratch program compiled to verify all 3 usage modes (Manager-only, goconfig-owned server, embedded into caller's server)

## 3. Update project docs

- [x] Update `.claude/PROJECT.md`: reflect the two-package architecture, close out the now-resolved
      known issues (#3 doc drift on HTTP API shape, #4 no exported mutation API, coupling notes)

## 4. Rewrite `examples/`

Current `examples/` is broken (two files with conflicting package clauses, fabricated APIs that
don't exist in source) — per user instruction it was left untouched during the refactor above.
Now rewriting it from scratch, one runnable example per usage mode (each needs its own `main()`,
so each gets its own subdirectory/binary):

- [x] `examples/basic/main.go` — Manager only, no HTTP: `NewStrSource`/`NewManager`, `OnInsert/OnRemove/OnReplace`, `Insert/Remove/Replace`, `Query`, history
- [x] `examples/httpserver-embedded/main.go` — caller owns the web service (plain `*http.ServeMux`), registers goconfig's routes via `RegisterRoutes`
- [x] Remove the old broken `examples/example_usage.go` and `examples/http_server_examples.go`
- [x] `go build ./...` and `go vet ./...` succeed across the whole module, including `examples/...`
- [x] Ran the examples for real: `go run ./examples/basic` (mutations/query/history all correct),
      `httpserver-embedded` (`curl /ping` on the caller's own route + `curl /health`/`curl /config`
      from goconfig) — all responded correctly

## 4b. Revisions after user feedback

- [x] Added `examples/httpserver-fromnode/main.go` — demonstrates `httpserver.NewHttpServerFromNode`,
      reading `address`/`port`/`api_key` from a `*goconfig.Node` (a `"http_server"` section inside
      the managed config) instead of functional options. Runs its own dedicated server on the
      address/port taken from that node — that's the actual reason to read them from config.
      Verified with `curl /health` and `curl -H X-API-Key /config`.
- [x] User flagged `examples/httpserver-standalone` as conceptually wrong: the real intended
      architecture is "the app owns one web-service that starts out knowing nothing about
      `goconfig`; `goconfig`'s routes get added to *that* web-service only if/when needed" — not a
      second, fully separate server goconfig owns on its own port. Removed
      `examples/httpserver-standalone/` entirely.
- [x] Restructured `examples/httpserver-embedded/main.go` to make that separation explicit:
      `newAppWebService()` builds the app's `*http.ServeMux` with zero goconfig import/knowledge;
      `main()` then conditionally (an `enableConfigAPI` flag standing in for "if needed") builds the
      `Manager` + `HttpServer` and calls `RegisterRoutes` on the *same* mux. Re-verified with
      `curl /ping`, `curl /health`, `curl -H X-API-Key /config`.

## 6. Generic `httpserver.NewServer` model (user prototype request)

User asked for a second, simpler `httpserver` shape: `NewServer(ip, port, apiKey, baseAPI)` +
`AddRoute`/`AddRoutes`, with `Manager` handing out its own routes as data via `GetRoutes()`.
Clarified via questions: **keep the existing manager-bound API as-is** (not a replacement) and
have apiKey protect **all** routes added through `AddRoute`, including the caller's own (e.g.
`/health`).

- [x] Added `goconfig/routes.go`: `type Route struct{ Path string; Methods []string; Handler http.HandlerFunc }`
      and `Manager.GetRoutes() []Route` (currently just `/config`). Moved the GET/POST handler logic
      (previously `httpserver`'s `onGet`/`onPost`/`buildConfigState`/`getString`/`getIndex`) into this
      file as unexported `Manager` methods, using the already-exported `Insert/Remove/Replace/
      ConfigJSON/SchemaJSON/InsertablePaths/RemovablePaths/ReplaceablePaths/Version`. No apiKey/CORS
      logic here — those stay transport concerns owned by whatever serves the routes.
- [x] Refactored `httpserver/http_server.go`:
      - `RegisterRoutes` (manager-bound model) now registers `/health` + loops `manager.GetRoutes()`,
        wrapping each in a new `protect()` middleware (apiKey check) instead of hand-rolling
        `handleConfig`/`onGet`/`onPost` — same external behavior, single source of truth for `/config`.
      - Added `NewServer(ip, port, apiKey, baseAPI) (*HttpServer, error)` — no `*goconfig.Manager`
        dependency at construction time.
      - Added `AddRoute(path, handler, methods...) error` and `AddRoutes(routes []goconfig.Route) error`
        on `*HttpServer`, valid only for `NewServer`-built instances (`hs.mux != nil`); every route
        goes through `protect()`, so apiKey (if set) guards everything added this way.
      - `Start()` now branches three ways: external registrar / already-populated `hs.mux` (generic
        model) / manager-owned local mux (old standalone model) — old behavior unchanged.
      - `httpserver` no longer imports `iancoleman/orderedmap` (that logic moved to `goconfig`).
- [x] Added `examples/httpserver-generic/main.go`: `NewServer("localhost", 8084, "secret", "/api/v1")`,
      `AddRoute("/health", ...)` for a custom route, then `AddRoutes(manager.GetRoutes())` wired in
      only once the manager exists — matches the user's sketch.
- [x] `go build ./...` + `go vet ./...` clean.
- [x] Re-verified old model unchanged: `httpserver-embedded` (`/ping`, `/health` open, `/config` 401
      without key / 200 with key) and `httpserver-fromnode` (`/health`, `/config` with key) both
      behave identically to before the refactor.
- [x] Verified new generic model: `httpserver-generic` — `/api/v1/health` and `/api/v1/config` both
      401 without `X-API-Key`, both 200 with it (uniform protection, base path applied).

## 7. Final checks

- [x] Update `.claude/PROJECT.md`'s example list/architecture note to match (dropped standalone
      example reference, added fromnode example, reframed the integration modes)
- [x] Update `.claude/PROJECT.md` again: added `Manager.GetRoutes()`/`Route` to the API list,
      documented `NewServer`/`AddRoute`/`AddRoutes`, added `examples/httpserver-generic`, updated
      the `/config`+`/health`+auth-ownership description, updated "Practical guidance"'s build note
- [x] `go build ./...` + `go vet ./...` clean after all revisions
- [x] `git status` / `git diff` reviewed — only intended files changed
- [x] Report summary to user; **do not commit** — ask first per user instruction

## 8. Versioning cleanup

User asked to move to the standard Go versioning approach (git tags) and drop the `VERSION` file
if not needed. Discovered the repo already had real semver git tags (`v1.0.0`..`v1.4.2`) — the
`VERSION` file was a redundant duplicate that could (and did, per commit history) drift in and out
of sync with the tags on its own.

- [x] Removed `VERSION`.
- [x] Documented the tag-based convention (patch/minor/major rules, `git describe --tags`) in
      `.claude/PROJECT.md`, replacing the "`VERSION` currently says..." line.
- [x] Committed locally (`dc4eb63`). User explicitly said not to tag `v1.5.0` yet and did not ask
      to push — commit sits local-only; tagging and pushing are separate follow-ups to do only when
      the user asks.

## 9. README / HTTP_SERVER.md cleanup + history example

User asked: (a) whether `HTTP_SERVER.md` is still needed, (b) rewrite `README.md` with no Go code
in it, referencing `examples/` instead, keep a "Thanks" section at the end with better wording,
(c) add a new example demonstrating change history.

- [x] Added `examples/history/main.go` — `EnableHistory`, mutations across multiple paths,
      `GetHistory`, `GetHistoryByPath`, `ClearHistory`. While building it, found that
      `history.ChangeEvent.OldValue`/`NewValue` leak the internal `map[string]*Node` representation
      for object-typed values (prints raw pointers) — documented as a known issue in
      `.claude/PROJECT.md` rather than fixed (out of scope for this doc task); worked around in the
      example by not printing Old/NewValue.
- [x] Answered the `HTTP_SERVER.md` question: recommended deletion — it documented a `GetHandler`/
      `GetServer`/`StartTLS`/`SetupRoutes(mux)`/package-level-`NewHttpServer` API that never existed
      in source (already flagged as stale in `.claude/PROJECT.md`'s known issues), and everything
      useful in it is now covered by the `httpserver-*` examples. Deleted `HTTP_SERVER.md`.
- [x] Rewrote `README.md`: no Go code blocks (kept only the `go get` install command); a features
      list in prose; a table linking to all five `examples/*` directories; improved "Thanks" wording
      kept at the end.
- [x] Updated `.claude/PROJECT.md`: added `examples/history` to the file map, rewrote "Known
      issues" (dropped the now-resolved build/examples/docs-drift items, added the new
      OldValue/NewValue finding), updated "Practical guidance".
- [x] `go build ./...` + `go vet ./...` clean; ran `go run ./examples/history` to confirm output.
- [x] Report summary to user; **do not commit** — ask first per user instruction

## 10. Full codebase review — bugs, performance, cleanliness (NOT STARTED — plan only)

User asked for a full review pass (not a diff review) of the whole module for bugs, performance
issues, and cleanliness/structure, phased for later implementation. **User explicitly said not to
start implementing yet** — this section is a TODO list to come back to when told to. Every finding
was verified against current source, not assumed. Nothing below is done.

### Phase 1 — Bug fixes — ✅ DONE

- [x] **B1** `utilities.go` (`jsonSetByPath`/`jsonRemoveByPath`/`jsonInsertByPath`):
      `reflect.TypeOf(found).Kind()` panicked when `found` is `nil` (a JSON `null` along the path) —
      confirmed empirically in isolation before fixing (`reflect.TypeOf(nil).Kind()` → "invalid
      memory address or nil pointer dereference"). Added a `found == nil` check before the reflect
      call in all three functions, returning a clean error instead. Regression tests:
      `TestJsonByPath_NilInPath_DoesNotPanic`, `TestJsonByPath_NilArrayElement_DoesNotPanic`
      (`utilities_test.go`) — call the unexported functions directly (an external `Manager`-level
      test can't reach this path: `findModifiableLocked` rejects an unregistered path before ever
      reaching `jsonSetByPath`, so the nil-in-path case is only reachable by exercising these
      functions directly, or when a registered path's Node-tree location and the freshly-cloned JSON
      copy briefly disagree).
- [x] **B2** `utilities.go` (`parseNode`): extended the type switch to cover
      `int64/int32/int16/int8/uint/uint64/uint32/uint16/uint8` (normalized to `int64`) and `float32`
      (normalized to `float64`) instead of falling through to the `default: Null` case. `node.go`'s
      `Type()` `case int64` is no longer dead code. Regression test:
      `TestParseNode_NumericTypeCoverage` (`utilities_test.go`) — one subtest per Go numeric type.
- [x] **B3** `manager.go` (`removeLocked`'s `OldValue: removedNode.value` and `replaceLocked`'s
      `oldValue := oldNode.value`): both stored the raw internal `*Node.value` for object/array
      values — unexported `map[string]*Node`/`[]*Node`, printing as pointer addresses and
      serializing to `{}` via `ChangeHistory.ExportJSON()`. Added `nodeValueToPlain(*Node)
      interface{}` (`utilities.go`) — recursively unwraps to plain `map[string]interface{}`/
      `[]interface{}`/scalar — and used it at both call sites. (`insertLocked`'s `NewValue`/
      `replaceLocked`'s `NewValue` were already fine — they store the caller's original raw value,
      never a `*Node`.) Regression test: `TestManager_History_OldValueIsPlain` (new
      `manager_test.go`, added as part of this phase) — asserts `OldValue` is a
      `map[string]interface{}`, not a `map[string]*goconfig.Node`.
- [x] **B4** `validation_service.go` (`customValidator`): added its own `sync.RWMutex` (independent
      of `Manager.mu`, since `GetCustomValidator()` hands this pointer out for use outside any
      Manager-held lock) — `AddValidator` takes the write lock, `Validate` takes a read lock around
      the map read only (releases before calling validator functions, so a validator itself calling
      back into `AddValidator` can't deadlock). Kept `GetCustomValidator()` rather than removing it
      (would have been a breaking API change) — the mutex makes the getter safe regardless of how
      it's used. Verified with `go run -race` on a scratch program hammering concurrent
      `AddValidator`/`Validate` — clean, no race reported (confirmed the race existed before this
      fix by reasoning about the unguarded map, consistent with Go's documented map-concurrency
      rules).
- [x] **B5** `manager.go` (`Config()`): rewrote the doc comment to describe actual behavior (returns
      the live internal `*Node`, required for `OnInsert`/`OnRemove`/`OnReplace` pointer-identity
      lookups; treat as read-only) instead of the stale "returns a deep copy" claim next to a
      commented-out `DeepCopy()` call. No behavior change, ties into the Phase 2 concurrency
      contract this comment now references.

Verified: `go build ./...` + `go vet ./...` clean; `go test ./...` (new tests) passing;
`go run ./examples/basic` re-run as a behavioral regression check (Phase 1 didn't touch
`httpserver`/`routes.go`, so the four HTTP examples were not re-run individually).

### Phase 2 — Concurrency — ✅ DONE

- [x] **C1** `manager.go`: `m.mu` is unlocked around the `OnInsert`/`OnRemove`/`OnReplace` handler
      call, but the tree is already mutated before that unlock — a concurrent second mutation on the
      same array during that window could have its change silently discarded if the first mutation's
      handler failed and rolled back. **Asked the user which fix they wanted** (per the note this
      section carried) — chose **per-path serialization** over documentation-only.

      Implemented: `Manager` gained `pathLocks sync.Map` (lazily-created `*sync.Mutex` per distinct
      path string, via new `pathLock(path)`); `Insert`/`Remove`/`Replace` (the exported entry
      points) now acquire that path's lock *before* the global `m.mu.Lock()` and hold it for the
      entire call, including the handler-call window where `m.mu` is released — so two
      Insert/Remove/Replace calls on the *same* path can never interleave, while calls on
      *different* paths remain fully concurrent. `routes.go`'s HTTP handlers call the exported
      `m.Insert`/`m.Remove`/`m.Replace` (never the `*Locked` internals directly), so the fix covers
      the HTTP-driven path too, not just direct `Manager` API use. Documented the full concurrency
      contract in a new doc comment on the `Manager` type itself (including the one real caveat:
      a handler that calls back into Insert/Remove/Replace on the *same* path it was invoked for
      will deadlock — the per-path lock isn't reentrant, and that pattern isn't supported).
      Also noted as a caveat, not fixed: `pathLocks` entries are never evicted, so a workload with
      unbounded/ever-changing path strings would leak one mutex per distinct path forever — fine for
      this library's intended use (a bounded, largely-static set of registered modifiable paths).

      Regression test `TestManager_PerPathLocking_ConcurrentSamePath` (`manager_test.go`): goroutine
      A inserts a value whose handler sleeps then fails (forcing a rollback); goroutine B inserts a
      different value on the *same* path while A is still mid-handler. Asserts the final array is
      exactly B's value alone — proving B's successful insert survives A's rollback rather than
      being wiped or corrupted. **Verified the test is meaningful**: reverted only the `manager.go`
      locking change (test file untouched) and reran — failed consistently, 5/5 runs; reapplied the
      fix — passes consistently under both plain `go test` and `go test -race`.

Verified: `go build ./...` + `go vet ./...` clean; `go test -race ./...` passing (full suite,
including Phase 1's tests).

### Phase 3 — Performance — ✅ DONE

- [x] **P1** Every Insert/Remove/Replace did up to 3 full-document JSON marshals: `Clone()`
      (1 marshal + 1 unmarshal), `validateJSONAgainstSchema` (a 2nd marshal), and
      `source.setConfig`'s own internal `json.MarshalIndent` (a 3rd). Consolidated the 2nd and 3rd:
      `insertLocked`/`removeLocked`/`replaceLocked` now marshal the proposed config exactly once
      (`json.MarshalIndent`) right after mutating the cloned copy, validate schema from those same
      bytes (new `validateJSONBytesAgainstSchema`, replacing the old marshal-then-validate helper),
      and pass the same bytes to `source.setConfig` for persistence. Changed `ISource.setConfig`'s
      signature to `setConfig(conf *orderedmap.OrderedMap, data []byte) error` so `FileSource`/
      `StrSource` write `data` directly instead of re-marshaling `conf` themselves — safe to change
      since `ISource`'s methods are unexported (Known Issue #1), so only in-package implementations
      exist. Net: 3 marshals → 2 per mutation.

      **Honest performance finding, not oversold**: ran the "quick before/after timing comparison"
      the plan itself asked for (500-item array, 200 insert+remove cycles) — result was
      **~12.3-13.3ms/op both before and after**, no measurable difference. Traced why: `Clone()`'s
      own marshal+unmarshal round-trip and, more significantly, `gojsonschema`'s *own* internal
      re-parse of the validated JSON string (independent of anything `validateJSONBytesAgainstSchema`
      does — `gojsonschema.Validate` takes a loader and parses the document itself) dominate the
      per-call cost far more than the one marshal this phase removed. The fix is still correct and
      worth keeping (it's a real, verified reduction from 3 marshals to 2, with zero behavior
      change), but **do not claim a measured speedup from it alone** — a real improvement here would
      need to also avoid `gojsonschema`'s internal reparse (e.g. `gojsonschema.NewGoLoader` fed the
      already-in-memory value instead of a string/bytes loader) and/or replace `Clone()`'s JSON
      round-trip with a native `orderedmap` deep-copy — both bigger, riskier changes than what P1
      was scoped for, noted here but not implemented.
- [x] **P2** `routes.go`'s `buildConfigState()` called `m.ConfigJSON()` (a string) then
      `json.Unmarshal`ed it into a fresh `orderedmap.OrderedMap`, and did the same for the schema —
      on *every single* `GET`/`POST /config` request — even though that string was itself produced by
      marshaling an `orderedmap.OrderedMap` moments earlier (inside `setConfig`). Replaced both
      round-trips with `json.RawMessage(configStr)`/`json.RawMessage(schemaStr)`, confirming first
      (by reading `orderedmap`'s own `MarshalJSON`, which calls `encoder.Encode` per value) that a
      `json.RawMessage` value embeds correctly without re-parsing. Kept a cheap `json.Valid()` check
      (well-formedness scan, no allocation of a parsed tree) rather than dropping validation
      entirely, since a corrupted stored string should still surface as a clean error, not garbage
      embedded in the response. Verified against a **real running example**
      (`examples/httpserver-embedded`): `GET /config`, `POST /config` (insert), and a follow-up
      `GET /config` all produced byte-identical response shapes to before this change, with the
      version bump and new data correctly reflected.

Verified: `go build ./...` + `go vet ./...` clean; `go test -race ./...` passing; real HTTP example
run end-to-end for P2 (GET/POST/GET); timing comparison run for P1 (see honest finding above —
correct fix, not a measured win on its own).

### Phase 3 — Performance

- [ ] **P1** Every Insert/Remove/Replace does up to 3 full-document JSON marshals + 1 unmarshal:
      `Clone()` (`utilities.go:392`), `validateJSONAgainstSchema` (`manager.go:627`), and
      `source.setConfig`'s internal marshal (`file_source.go`/`string_source.go`). Consolidate
      where reasonable (e.g. reuse the validation marshal's bytes for persistence).
- [ ] **P2** `routes.go`'s `buildConfigState` calls `m.ConfigJSON()` (string) then
      `json.Unmarshal`s it back into an `orderedmap.OrderedMap` on every `/config` request, even
      though `ISource.getConfigObject()` already holds a live parsed one. Avoid the round-trip.
- [ ] *(Noted, not scheduled)* path cache (`manager.go` `rebuildPathCache`) fully rebuilds on every
      mutation rather than incrementally — only worth acting on if profiling shows it matters.

### Phase 4 — Cleanliness & structure

- [ ] **S1** `utilities.go`: `jsonSetByPath` (14), `jsonRemoveByPath` (87), `jsonInsertByPath`
      (177) share ~90% duplicated "walk to parent map" code. Extract a shared
      `navigateToParentMap(jsonMap, path) (parent *orderedmap.OrderedMap, lastKey string, err error)`.
- [ ] **S2** Remove dead commented-out code in `manager.go` (lines 86, 281, 368, 448 — `// return
      m.config.DeepCopy()` and three `// handlerNode := ...DeepCopy()` lines), or replace with a
      one-line comment explaining why DeepCopy is intentionally skipped (ties into B5/C1).
- [ ] **S3** Rename `NewvalidationService` → `NewValidationService` (`validation_service.go:36`,
      casing typo, currently undiscoverable/unused so low-risk).
- [ ] **S4** Add a baseline `go test` suite (currently zero `*_test.go` files anywhere) — prioritize
      `utilities.go` path functions (as B1/B2 regression tests), `Manager` insert/remove/replace +
      rollback + history (B3 regression), and the `query.go` DSL. Best done *after* Phase 1 so the
      new tests can assert the fixed behavior.
- [ ] *(Optional, not recommended to schedule)* S5: consider splitting `httpserver.HttpServer`'s
      two modes (manager-bound vs generic) into separate types — user previously asked to keep both
      on one type, so only revisit if that changes.
- [ ] *(Optional, not recommended to schedule)* S6: no CI (`.github/workflows`) running
      `go build`/`go vet`/`go test` — process/infra, not a code defect.

### Verification (once a phase is actually started)

- `go build ./...` + `go vet ./...` across the whole module.
- Re-run all five `examples/*` programs to confirm no behavioral regressions.
- Phase 1/4: add regression tests for each fixed bug (nil-path input, non-float64 numeric insert,
  history export round-trip for object values).
- Phase 3: a quick before/after timing comparison on a moderately sized config.
