#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

packages=(. ./history)

unformatted=$(gofmt -l -- *.go history/*.go)
if [[ -n "$unformatted" ]]; then
  echo "The following Go files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go test "${packages[@]}"
go test -race "${packages[@]}"
go vet "${packages[@]}"

if [[ "${RUN_STATICCHECK:-0}" == "1" ]]; then
  staticcheck "${packages[@]}"
fi
