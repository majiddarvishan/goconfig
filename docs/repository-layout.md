# Repository layout

The repository follows Go's convention of keeping the public package at the module root. Moving the implementation behind forwarding wrappers would add import cycles or duplicate a large public surface, so focused root files remain the ownership boundary for `goconfig`.

| Location | Responsibility |
| --- | --- |
| `manager*.go`, `transaction.go` | Manager state, options, mutations, validation, history, and HTTP lifecycle integration |
| `source.go`, `string_source.go`, `file_source.go` | Public source contracts and built-in persistence implementations |
| `node*.go`, `path.go`, `mutation_path.go`, `query.go`, `json_codec.go` | JSON values, canonical paths, traversal, queries, and safe cloning |
| `validator.go`, `validation_service.go` | Schema, custom, and external validation |
| `http_server.go`, `route_registrar.go` | Reusable HTTP boundary and router integration |
| `history/` | Independently reusable bounded change-history package |
| `examples/<topic>/` | Independent executable programs using only supported public APIs |
| `docs/` | User-facing design and integration documentation |
| `.codex/` | Review records, phased plan, verification commands, and benchmark baselines |
| `.github/workflows/` | Clean-checkout CI automation |

White-box tests and benchmarks stay adjacent to the package they verify. This keeps unexported behavior testable without creating artificial test-only packages. Every example is a separate `main` package so the complete tree can be checked with `go test ./...`.
