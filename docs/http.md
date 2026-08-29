# HTTP integration

`goconfig` exposes `/config` and `/health` through a reusable `http.Handler`.
The same boundary can be mounted in an existing application, registered on a
router adapter, or served by a standalone/supplied `http.Server`.

## Reusable handler

```go
server, err := goconfig.NewHTTPServer(manager,
    goconfig.WithAPIKey(os.Getenv("CONFIG_API_KEY")),
)
if err != nil {
    return err
}

mux := http.NewServeMux()
mux.Handle("/admin/", http.StripPrefix("/admin", server.Handler()))
```

`manager.Handler()` is a shortcut that installs safe defaults when no HTTP
server has been configured.

## Standalone lifecycle

`Start` blocks and returns listener errors. `Shutdown` returns shutdown errors;
neither method panics or prints errors.

```go
server, err := goconfig.NewHTTPServer(manager,
    goconfig.WithAddress("127.0.0.1"),
    goconfig.WithPort(8080),
    goconfig.WithLogger(log.Default()),
)
if err != nil {
    return err
}

go func() {
    if err := server.Start(); err != nil {
        log.Printf("configuration server stopped: %v", err)
    }
}()

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := server.Shutdown(ctx); err != nil {
    return err
}
```

The Manager equivalents are `StartHTTPServer`, `ShutdownHTTPServer`, and
`RegisterHTTPRoutes`. The older `StartHttpServer`, `StopHttpServer`, and
`SetupRoutes` methods remain deprecated best-effort adapters.

## Supplied server

```go
owned := &http.Server{
    Addr:              ":8080",
    Handler:           applicationHandler,
    ReadHeaderTimeout: 5 * time.Second,
}

server, err := goconfig.NewHTTPServer(manager, goconfig.WithServer(owned))
if err != nil {
    return err
}
return server.Start()
```

Existing routes on `owned.Handler` remain available. `/config` and `/health`
are mounted ahead of the existing fallback handler. Existing non-zero timeout
values are preserved.

## Route registrar

Framework adapters can implement:

```go
type RouteRegistrar interface {
    HandleFunc(path string, handler http.HandlerFunc, methods ...string)
}
```

Use `server.RegisterRoutes(registrar)` directly, or configure
`WithRouteRegistrar(registrar)` and call `Start`; registrar mode registers the
routes and returns without opening a listener.

## Options

- `WithAddress` and `WithPort` configure standalone listening.
- `WithServer` selects a supplied `http.Server`.
- `WithRouteRegistrar` selects router registration mode.
- `WithAPIKey` stores only a SHA-256 digest and authenticates `X-API-Key` using
  a constant-time comparison.
- `WithAuthenticator` installs custom request authentication.
- `WithCORS` configures allowed origins, methods, headers, and max age.
- `WithLogger` configures lifecycle logging; `nil` disables it.
- `WithHTTPTimeouts` configures default read/write/idle timeouts.
- `WithMaxBodySize` configures the POST body limit.
- `WithHealthCheck` supplies a health policy; `WithHealthEnabled(false)` removes
  the health route.

Supplying both `WithServer` and `WithRouteRegistrar` is rejected because their
ownership models are ambiguous.

## GET /config

Authentication is checked first. The successful response is built from one
coherent Manager snapshot:

```json
{
  "success": true,
  "data": {
    "modifiable_paths": {
      "insertable": ["/items"],
      "removable": ["/items"],
      "replaceable": ["/settings/timeout"]
    },
    "config": {},
    "schema": {},
    "version": 4
  }
}
```

## POST /config

Requests are decoded into an explicit structure. Unknown fields, trailing JSON,
fractional/exponent integer forms, and operation-specific extra fields are
rejected. The empty path is valid and identifies the document root.

```json
{
  "op": "replace",
  "path": "/settings/timeout",
  "value": 60,
  "version": 4
}
```

Insert requires `index` and `value`; remove requires `index` and forbids
`value`; replace requires `value` and forbids `index`. `version` is optional and
is checked atomically during commit.

Status mapping:

- `400` malformed or semantically invalid request
- `401` failed authentication
- `403` path is not registered for the requested mutation
- `404` configuration path not found
- `409` expected-version conflict
- `413` request body too large
- `415` unsupported content type
- `422` schema, custom, external, or compatibility-handler rejection
- `500` persistence or internal failure

## GET /health

The default response is `200` with `{"status":"ok"}`. A configured health
check failure returns `503` with `{"status":"unavailable"}`. Health is public
by default and can be removed with `WithHealthEnabled(false)`.
