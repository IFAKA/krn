#!/bin/sh
set -eu
go test ./...
test "$(go run ./cmd/summary | tr '\n' '|')" = 'food-001=1299|toy-002=899|'
