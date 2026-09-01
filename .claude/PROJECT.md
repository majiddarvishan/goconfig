# goconfig — project notes for Claude

## What this is

`goconfig` (module `github.com/majiddarvishan/goconfig`, Go 1.18) is a **runtime, in-memory
dynamic configuration manager** for Go applications — not a "load config once into a struct"
library. It loads a JSON config document + JSON-Schema through a pluggable `ISource`, keeps it
as a mutable `Node` tree, re-validates on every mutation, tracks an audit history, and can
optionally be administered over HTTP via the separate `httpserver` sub-package.

There is no `main.go`/`cmd/` — this repo is a library only.

**Versioning:** git tags are the sole source of truth (e.g. `v1.4.2`, `v1.5.0`) — this is what Go
tooling (`go get module@vX.Y.Z`, `go.sum`, pkg.go.dev) actually resolves against. There is no
`VERSION` file (removed — it duplicated the tag and could drift out of sync) and no in-code
`Version` const. To pick the next tag: patch for fixes, minor for backward-compatible additions
(new exported API, no removals/behavior breaks), major for breaking changes. `git describe --tags`
shows how far HEAD is past the last release. Latest tag as of this writing: `v1.4.2`; `v1.5.0` is
the next one, covering the Manager/httpserver package split + `NewServer`/`AddRoute` additions.

**Architecture note:** `Manager` and the HTTP admin API were deliberately split into two packages
(root `goconfig` and `httpserver`) so `Manager` can be used with zero HTTP dependency, and so the
HTTP routes can be registered either on goconfig's own server or on a caller's own web service.
See "Package / file map" and "HTTP integration modes" below.

## Package / file map

Root package `goconfig` (flat layout) — the config engine, **no HTTP dependency at all**:
- `manager.go` — `Manager`: central type. Insert/Remove/Replace (mutation), history, validators,
  query wiring. Has no knowledge of HTTP — exposes everything an external HTTP layer (or any
  other consumer) needs purely through its exported API (see below).
- `routes.go` — `Route` struct + `Manager.GetRoutes() []Route`: the `/config` GET/POST handler
  logic as plain data (`net/http.HandlerFunc`), decoupled from any specific server. This is the
  *only* place `/config`'s request/response logic lives; both HTTP models in `httpserver` consume
  it. No apiKey/CORS here — those are transport concerns the caller's server owns.
- `node.go` — `Node`: tree value type (Object/Array/String/Int/Float/Bool/Null) with
  `GetString/GetBool/GetInt/GetFloat/GetObject/GetArray/At/DeepCopy`.
- `query.go` — `Manager.Query/QueryOne/FindAll/QueryExists/QueryCount`: small JSONPath-like DSL
  (`/a/b`, `/a/*/b`, `/a/[*]`, `/a/[0]`, `/a/[?age>18]`).
- `source.go` — `ISource` interface.
- `file_source.go` — `FileSource`: file-backed source, atomic write via temp+rename.
- `string_source.go` — `StrSource`: in-memory source, no persistence.
- `utilities.go` — JSON path traversal helpers (`jsonSetByPath`, `jsonInsertByPath`,
  `jsonRemoveByPath`, `parseNode`, `Clone`, `findNodePath`).
- `validation_service.go` — external HTTP validation client + built-in validator functions
  (`ValidateRange`, `ValidatePattern`, `ValidateEnum`, `ValidateRequired`, `ValidateUnique`).
- `validator.go` — JSON-Schema validation wrapper around `gojsonschema`.
- `history/change_event.go` (package `history`) — `ChangeEvent` struct + `ChangeHistory`
  fixed-size ring buffer (default max 1000).

Sub-package `httpserver` (import path `github.com/majiddarvishan/goconfig/httpserver`) — the
optional HTTP transport layer. It imports `goconfig` (for `*goconfig.Manager`/`goconfig.Node`/
`goconfig.Route` types); `goconfig` never imports it. Two independent models live here (see "HTTP
integration modes" below):
- `http_server.go` — `HttpServer`: address/port, API-key auth (SHA-256 + constant-time compare),
  CORS, `GET /health`, optimistic-locking via `version` (enforced inside `goconfig.Route`'s
  handler). Manager-bound constructors (`NewHttpServer`/`NewHttpServerFromNode` +
  `RegisterRoutes`/`Start`) build `/config` from `manager.GetRoutes()`; the generic constructor
  (`NewServer` + `AddRoute`/`AddRoutes`) has no `Manager` dependency at all until routes are added.
- `mux_adapter.go` — adapter making `*http.ServeMux` satisfy `RouteRegistrar`.
- `route_registrar.go` — `RouteRegistrar` interface (decouples the HTTP layer from a specific
  router/framework).

`examples/` — four runnable programs (previous `example_usage.go` / `http_server_examples.go`
were broken and removed):
- `examples/basic/` — `Manager` only, zero HTTP.
- `examples/httpserver-embedded/` — the app's own web-service (`newAppWebService()`, no goconfig
  import at all) with goconfig's routes optionally wired in later via `RegisterRoutes`; this is the
  intended real-world shape, not a second server goconfig owns on its own port.
- `examples/httpserver-fromnode/` — `httpserver.NewHttpServerFromNode`, reading `address`/`port`/
  `api_key` from a `*goconfig.Node` (a config section) instead of functional options; this one does
  run its own dedicated server, since that's the actual point of loading address/port from config.
- `examples/httpserver-generic/` — `httpserver.NewServer(ip, port, apiKey, baseAPI)` with no
  `Manager` at construction time; a custom `/health` route added via `AddRoute`, then the manager's
  routes wired in via `AddRoutes(manager.GetRoutes())`. `apiKey` protects everything added this way,
  including the custom route.

## Actual public API (verified against source, not README)

```go
// Manager
func NewManager(source ISource) (*Manager, error)
func (m *Manager) Config() *Node
func (m *Manager) Source() ISource
func (m *Manager) Version() int64

func (m *Manager) EnableHistory(enabled bool)
func (m *Manager) GetHistory() []history.ChangeEvent
func (m *Manager) GetHistoryByPath(path string, limit int) []history.ChangeEvent
func (m *Manager) ClearHistory()

func (m *Manager) AddValidator(path string, validator validatorFunc) // validatorFunc unexported
func (m *Manager) GetCustomValidator() *customValidator
func (m *Manager) SetValidationService(service *validationService)   // *validationService unexported

func (m *Manager) ConfigJSON() string // raw JSON text of the persisted config
func (m *Manager) SchemaJSON() string // raw JSON schema text

func (m *Manager) OnInsert(node *Node, handler func(*Node) error) error
func (m *Manager) OnRemove(node *Node, handler func(*Node) error) error
func (m *Manager) OnReplace(node *Node, handler func(*Node) error) error

func (m *Manager) Insert(path string, index int, value interface{}) error
func (m *Manager) Remove(path string, index int) error
func (m *Manager) Replace(path string, value interface{}) error
func (m *Manager) InsertablePaths() []string
func (m *Manager) RemovablePaths() []string
func (m *Manager) ReplaceablePaths() []string

type Route struct {
    Path    string
    Methods []string
    Handler http.HandlerFunc
}
func (m *Manager) GetRoutes() []Route // currently just /config (GET/POST/OPTIONS)

func (m *Manager) Query(query string) ([]QueryResult, error)
func (m *Manager) QueryOne(query string) (*QueryResult, error)
func (m *Manager) FindAll(predicate func(*Node) bool) []QueryResult
func (m *Manager) QueryExists(query string) bool
func (m *Manager) QueryCount(query string) (int, error)

// Sources
func NewFileSource(configPath string, schema string) (*FileSource, error)
func NewStrSource(config, schema string) (*StrSource, error)

// httpserver package (github.com/majiddarvishan/goconfig/httpserver)
type HttpServerOption func(*HttpServer)
func WithAddress(address string) HttpServerOption
func WithPort(port int) HttpServerOption
func WithAPIKey(apiKey string) HttpServerOption
func WithRouteRegistrar(r RouteRegistrar) HttpServerOption
func WithServer(server *http.Server) HttpServerOption
func NewHttpServer(m *goconfig.Manager, opts ...HttpServerOption) (*HttpServer, error)
func NewHttpServerFromNode(m *goconfig.Manager, conf *goconfig.Node) (*HttpServer, error)
func (hs *HttpServer) RegisterRoutes(r RouteRegistrar) error // requires manager-bound hs

func NewServer(ip string, port int, apiKey string, baseAPI string) (*HttpServer, error) // no Manager needed
func (hs *HttpServer) AddRoute(path string, handler http.HandlerFunc, methods ...string) error  // requires NewServer-built hs
func (hs *HttpServer) AddRoutes(routes []goconfig.Route) error                                  // requires NewServer-built hs

func (hs *HttpServer) Start() error
func (hs *HttpServer) Shutdown(ctx context.Context) error
func HashSHA256(s string) string

type RouteRegistrar interface {
    HandleFunc(path string, handler http.HandlerFunc, methods ...string)
}

// Validators (built-in factories)
func ValidateRange(min, max float64) validatorFunc
func ValidatePattern(pattern string) validatorFunc // literal match only, not real regex/glob
func ValidateEnum(allowed ...interface{}) validatorFunc
func ValidateRequired() validatorFunc
func ValidateUnique(field string) validatorFunc
func NewCustomValidator() *customValidator

// history package
type ChangeEvent struct {
    Timestamp time.Time; Operation string; Path string
    Index *int; OldValue, NewValue interface{}; User string; Version int64
}
func NewChangeHistory(maxSize int) *ChangeHistory
func (ch *ChangeHistory) Add(event ChangeEvent)
func (ch *ChangeHistory) GetAll() []ChangeEvent
func (ch *ChangeHistory) GetByPath(path string, limit int) []ChangeEvent
func (ch *ChangeHistory) GetRecent(limit int) []ChangeEvent
func (ch *ChangeHistory) Clear()
func (ch *ChangeHistory) ExportJSON() ([]byte, error)
```

HTTP endpoints for `/config` (defined once, in `goconfig.Manager.GetRoutes()`; served by whichever
`httpserver` model wires it in):
- `GET /config` → `{success, data:{modifiable_paths:{insertable,removable,replaceable}, config, schema, version}}`
- `POST /config` body `{"op":"insert"|"remove"|"replace","path":"/...","index":N,"value":...,"version":N}`,
  409 on version mismatch.
- `GET /health` is provided by `httpserver` itself (manager-bound model) or is just another route
  the caller adds via `AddRoute` (generic model) — it is not part of `Manager.GetRoutes()`.
- `X-API-Key` header, checked with SHA-256 + constant-time compare, is enforced by `httpserver`
  (`protect()`/`checkAccess`), never by `goconfig` — `Manager.GetRoutes()`'s handler has no auth of
  its own, by design, so it works under any auth scheme the caller's server wants to apply.

## HTTP integration modes (`httpserver` package)

Four independent ways to use `Manager`, none forcing the others:

```go
// 1. Manager only, zero HTTP. (examples/basic)
source, _ := goconfig.NewStrSource(configJSON, schemaJSON)
manager, _ := goconfig.NewManager(source)
manager.Query("/users/[?active==true]")

// 2. The app owns its web-service, built with zero knowledge of goconfig;
//    goconfig's routes are wired into it only if/when needed, and goconfig
//    never listens on anything of its own. This is the intended real-world
//    shape. (examples/httpserver-embedded)
hs, _ := httpserver.NewHttpServer(manager, httpserver.WithAPIKey("secret"))
hs.RegisterRoutes(myRouteRegistrarAdapter) // wraps gorilla/mux, chi, *http.ServeMux, etc.

// 3. goconfig owns and runs its own dedicated HTTP server. Useful when the
//    server's address/port genuinely belongs to goconfig rather than an app
//    web-service - e.g. built via NewHttpServerFromNode, reading
//    address/port/api_key straight from the managed config.
//    (examples/httpserver-fromnode)
hs, _ := httpserver.NewHttpServerFromNode(manager, manager.Config().At("http_server"))
go hs.Start()
defer hs.Shutdown(ctx)

// 4. Generic server, no Manager dependency at construction time. apiKey (if
//    set) protects every route added via AddRoute/AddRoutes uniformly,
//    including the caller's own; baseAPI prefixes every route path and may
//    be empty. Manager's routes are wired in only if/when needed.
//    (examples/httpserver-generic)
hs, _ := httpserver.NewServer("localhost", 8080, "secret", "/api/v1")
hs.AddRoute("/health", myHealthHandler, "GET")
hs.AddRoutes(manager.GetRoutes()) // only once a manager actually exists
go hs.Start()
defer hs.Shutdown(ctx)
```

`WithRouteRegistrar(r)` + `Start()` is a convenience alias for mode 2, for callers who prefer the
option-based style over calling `RegisterRoutes` directly. Modes 2/3 (`NewHttpServer`/
`NewHttpServerFromNode`) and mode 4 (`NewServer`) are two parallel, independently maintained
constructors on the same `HttpServer` type — kept side by side deliberately (user explicitly asked
for the simpler model in addition to, not instead of, the original one). `AddRoute`/`AddRoutes`
only work on a `NewServer`-built instance; `RegisterRoutes` only works on a manager-bound one.

## Known issues / doc drift (important — read before trusting README.md / HTTP_SERVER.md)

1. **Build was broken, now fixed.** `go.mod` originally only required `xeipuuv/gojsonschema`
   while the code imported `github.com/iancoleman/orderedmap` and `github.com/rs/cors` without
   declaring them. Fixed via `go mod tidy`; both are now proper `go.mod` requires.
2. **`examples/` was rewritten from scratch** (see `.claude/plan.md` for the history) — it
   previously had two files with conflicting `package main`/`package examples` clauses and
   referenced APIs that never existed (`Batch`/`Transaction`, `CreateSnapshot`/`Restore`/
   `StartAutoBackup`, `ConditionalReplace`/`CompareAndSwap`, a package-level
   `config.NewHttpServer(...)` with `GetHandler`/`GetServer`/`StartTLS`). It's now three runnable
   programs matching the real API — see "Package / file map" above.
3. **`README.md` and `HTTP_SERVER.md` are stale** — written for an API that doesn't match the
   source (predates the `httpserver` package split too). Notably README's example calls
   `manager.history.ExportJSON()` (unexported field, won't compile — use `manager.GetHistory()`
   + marshal manually), and HTTP_SERVER.md documents constructors/methods that don't exist.
   These docs have not been rewritten yet; trust `.go` source and this file over them.
4. **`ISource` has unexported methods**, so only types inside package `goconfig` can implement it.
   External code cannot supply a custom source (e.g. etcd/Consul-backed) — only `FileSource` and
   `StrSource` ship today.
5. **Likely constructor bug**: the external validation service constructor is
   `NewvalidationService` (lowercase `v`), while README calls it `NewValidationService`. As
   written, `SetValidationService`/`AddValidator` take unexported param types
   (`*validationService`, `validatorFunc`) that external callers can't construct meaningfully.
6. **Zero automated tests.** No `*_test.go` files anywhere in the module.
7. `ValidatePattern`'s "pattern" matching is literal `*`/exact-match only, not real regex/glob —
   don't assume regex semantics despite the name.

## Practical guidance for future work here

- Trust the actual `.go` source over `README.md`/`HTTP_SERVER.md` when they conflict — those two
  files have not been updated for the `httpserver` package split and are stale.
- `go build ./...` / `go vet ./...` cover the whole module cleanly, including all four
  `examples/*` programs (each is its own subdirectory/`package main`, since Go only allows one
  `main()` per package).
- Concurrency: `Manager` is guarded by a single `sync.RWMutex`; mutation methods
  (`insertLocked`/`removeLocked`/`replaceLocked`) unlock around the user-supplied `OnInsert` etc.
  handler call (to avoid deadlock/reentrancy) and re-lock after — be aware state can change
  underneath during that handler call.
- Path caching: `Manager` keeps a `map[*Node]string` node→path cache, invalidated on every
  mutation, rebuilt lazily (`manager.go`).
