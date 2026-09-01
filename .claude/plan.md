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
- [ ] Report summary to user; **do not commit** — ask first per user instruction
