# Examples

Each subdirectory is an independent, buildable program using the supported public API:

- `string_source`: construct and read an in-memory configuration.
- `file_source`: persist an atomic file-backed mutation.
- `mutation_validation`: register mutation paths and custom validation.
- `history`: configure and inspect bounded change history.
- `http_handler`: mount and exercise the reusable HTTP handler without opening a listener.
- `external_validation`: validate a candidate through a self-contained HTTP validation service.

Run an example from the repository root, for example:

```bash
go run ./examples/string_source
```

All examples are compiled by `go test ./...` and intentionally avoid deprecated compatibility APIs.
