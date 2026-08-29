#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

packages=(./...)

mapfile -t go_files < <(rg --files -g '*.go')
unformatted=$(gofmt -l -- "${go_files[@]}")
if [[ -n "$unformatted" ]]; then
  echo "The following Go files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go test -count=1 "${packages[@]}"
go test -race "${packages[@]}"
go vet "${packages[@]}"

if [[ "${RUN_FUZZ:-0}" == "1" ]]; then
  fuzz_time="${FUZZ_TIME:-5s}"
  fuzz_targets=(
    FuzzJSONPointerRoundTrip
    FuzzQueryParsingAndTraversal
    FuzzHTTPMutationRequest
    FuzzApplyMutation
  )
  for target in "${fuzz_targets[@]}"; do
    go test -run '^$' -fuzz "^${target}$" -fuzztime "$fuzz_time" ./internal/core
  done
fi

if [[ "${RUN_BENCHMARKS:-0}" == "1" ]]; then
  go test -run '^$' -bench . -benchmem -benchtime "${BENCH_TIME:-1x}" ./internal/core
fi

if [[ "${RUN_STATICCHECK:-0}" == "1" ]]; then
  staticcheck "${packages[@]}"
fi
