# goconfig

A runtime, in-memory dynamic configuration manager for Go applications. It loads a JSON config
document plus a JSON Schema, keeps it as a live, mutable tree, re-validates every change against
the schema, and can optionally expose itself over an HTTP admin API.

## Installation

```bash
go get -u github.com/majiddarvishan/goconfig
```

## Features

- Load config + JSON Schema from a file or an in-memory string (`FileSource`, `StrSource`)
- Live mutation - insert/remove/replace on registered paths, validated against the schema on
  every change, with automatic rollback on failure
- Optimistic-locking version counter and an in-memory change history (audit log)
- A small JSONPath-like query language for reading the config tree
- Custom validators and an optional external validation service
- An optional HTTP admin API (`GET`/`POST /config`, health check) in the separate `httpserver`
  package - `goconfig.Manager` has no HTTP dependency at all, so using it alone pulls in nothing
  HTTP-related

## Examples

Runnable, self-contained examples live under [`examples/`](examples/). Run any of them with
`go run ./examples/<name>`.

| Example | Demonstrates |
| --- | --- |
| [`examples/basic`](examples/basic) | `Manager` on its own, no HTTP: mutating config, querying, history |
| [`examples/history`](examples/history) | Change history in detail: `EnableHistory`, `GetHistory`, `GetHistoryByPath`, `ClearHistory` |
| [`examples/httpserver-embedded`](examples/httpserver-embedded) | An app's own web-service that starts out knowing nothing about goconfig, with goconfig's routes wired in only if/when needed - the recommended way to expose the admin API |
| [`examples/httpserver-fromnode`](examples/httpserver-fromnode) | Building the HTTP server from a config `Node` (`address`/`port`/`api_key` read from the managed config itself) |
| [`examples/httpserver-generic`](examples/httpserver-generic) | The generic `httpserver.NewServer` + `AddRoute`/`AddRoutes` model, independent of any `Manager` until routes are wired in |

## Thanks

This library's core idea - a live, schema-validated, mutable config tree - comes from Mohammad
Nejati's original C++ implementation. This is a Go take on that idea.
