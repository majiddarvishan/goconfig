# goconfig

`goconfig` is a transactional, schema-validated JSON configuration manager for
Go. It supports in-memory and file-backed sources, optimistic versions, custom
and external validation, bounded history, JSON Pointer queries, and an optional
HTTP boundary.

## Installation

```bash
go get github.com/majiddarvishan/goconfig
```

The module currently targets Go 1.18 or newer. A non-empty JSON Schema is
required when constructing a Manager.

## Create a Manager

```go
package main

import (
    "fmt"

    "github.com/majiddarvishan/goconfig"
)

func main() {
    source, err := goconfig.NewStrSource(
        `{"name":"demo","port":8080}`,
        `{
          "type":"object",
          "required":["name","port"],
          "properties":{
            "name":{"type":"string"},
            "port":{"type":"integer"}
          }
        }`,
    )
    if err != nil {
        panic(err)
    }

    manager, err := goconfig.NewManager(source)
    if err != nil {
        panic(err)
    }

    name, err := manager.Lookup("/name")
    if err != nil {
        panic(err)
    }
    if err := manager.OnReplace(name.Node, nil); err != nil {
        panic(err)
    }
    if err := manager.Replace("/name", "production"); err != nil {
        panic(err)
    }

    value, _ := manager.Config().GetString("name")
    fmt.Println(value)
}
```

External source implementations should implement `Source` and use
`NewManagerFromSource`. `ISource` and `NewManager(ISource)` remain for v1
compatibility.

## Mutations

Mutation paths are RFC 6901 JSON Pointers. Paths must first be registered with
`OnInsert`, `OnRemove`, or `OnReplace`. The optional callback is a pre-commit
veto hook.

Use `Insert`, `Remove`, and `Replace` for simple calls, or `Mutate` when a
context and expected version are required:

```go
expected := manager.Version()
err := manager.Mutate(ctx, goconfig.Mutation{
    Operation:       goconfig.OperationReplace,
    Path:            "/port",
    Value:           9090,
    ExpectedVersion: &expected,
})
```

Every mutation uses the same schema, custom, external-validation, persistence,
version, history, and observer pipeline.

## Validation

```go
if err := manager.RegisterValidator("/port", goconfig.ValidateRange(1, 65535)); err != nil {
    return err
}

service := goconfig.NewValidationService("https://validator.example", 5*time.Second)
manager.SetValidationService(service)
```

Configured external validation fails closed and receives the complete candidate
configuration using the mutation context.

## Query

`Lookup` resolves an exact JSON Pointer. `Query` adds wildcard, legacy bracket
index, and filter extensions:

```go
one, err := manager.Lookup("/items/0/name")
many, err := manager.Query("/items/[*]/name")
filtered, err := manager.Query("/items/[?enabled==true]/name")
```

Returned paths are canonical escaped JSON Pointers, object traversal is lexical,
and returned nodes are independent snapshots.

## History and snapshots

```go
events := manager.History()
recentForPath := manager.HistoryByPath("/port", 10)
snapshot, err := manager.Snapshot()
```

History capacity is configurable with `WithHistoryCapacity` at construction.
History inputs and outputs are deep-copied.

## HTTP

```go
server, err := goconfig.NewHTTPServer(manager, goconfig.WithAPIKey("secret"))
handler := server.Handler()
```

The handler exposes `GET/POST /config` and optional `GET /health`. Standalone,
supplied `http.Server`, route-registrar, authentication, CORS, timeout, body-size,
and health configuration are documented in [HTTP_SERVER.md](HTTP_SERVER.md).

## Examples

The existing `examples/` tree is legacy and is not currently a supported source
of behavior. It will be replaced with buildable examples in the planned project
organization phase.
