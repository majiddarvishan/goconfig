# Benchmark Baseline

Recorded during Phase 8 on Linux/amd64 with Go's default benchmark settings except for `-benchtime=100x`.

Command:

```bash
go test -run '^$' -bench 'Benchmark(Clone|CompiledSchemaValidation|NodeAtJSONPointer)$' -benchmem -benchtime 100x .
```

Initial measurements:

| Benchmark | Time | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| Clone (250 items) | 135,658 ns/op | 112,522 | 1,256 |
| Compiled schema validation (250 items) | 219,570 ns/op | 121,929 | 2,022 |
| JSON Pointer lookup (`/items/200/name`) | 471.0 ns/op | 120 | 5 |

Hardware for this run: Intel Core i5-4460 at 3.20 GHz. These numbers are a comparison baseline, not cross-machine performance guarantees. Optimization should be driven by representative profiles and measured regressions rather than these values alone.
