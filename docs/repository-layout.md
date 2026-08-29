# Repository layout

The repository keeps the stable public import path at the module root while grouping its implementation in `internal/core`. A compact facade uses type aliases and constructor forwarding, so existing source code continues to import `github.com/majiddarvishan/goconfig` while implementation details stay out of the root directory.

| Location | Responsibility |
| --- | --- |
| `goconfig.go` | Stable public facade, exported aliases, constructors, options, constants, and errors |
| `internal/core/manager*.go`, `internal/core/transaction.go` | Manager state, options, mutations, validation, history, and HTTP lifecycle integration |
| `internal/core/*source.go` | Source contracts and built-in persistence implementations |
| `internal/core/node*.go`, path/query/codec files | JSON values, canonical paths, traversal, queries, and safe cloning |
| `internal/core/validator.go`, `internal/core/validation_service.go` | Schema, custom, and external validation |
| `internal/core/http_server.go`, `internal/core/route_registrar.go` | Reusable HTTP boundary and router integration |
| `history/` | Independently reusable bounded change-history package |
| `contract/` | Black-box compatibility tests against the root public facade |
| `examples/<topic>/` | Independent executable programs using only supported public APIs |
| `docs/` | User-facing design and integration documentation |
| `.codex/` | Review records, phased plan, verification commands, and benchmark baselines |
| `.github/workflows/` | Clean-checkout CI automation |

White-box tests and benchmarks stay beside `internal/core`, while `contract/` verifies that the facade remains usable by external consumers. Every example is a separate `main` package so the complete tree can be checked with `go test ./...`.
