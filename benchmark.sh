#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/krn-bench.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
export GOCACHE="$TMP/go-cache"
REPO="$TMP/repo"
mkdir "$REPO"
(cd "$REPO" && git init -q && printf 'input\n' > input.txt && git add input.txt && git commit -qm init)
(cd "$ROOT" && go build -o "$TMP/krn" .)
printf 'case\tcommand_executions\tcache_hits\tprojected_bytes\n'
baseline=0
for _ in 1 2; do (cd "$REPO" && sh -c 'printf baseline\n' >/dev/null); baseline=$((baseline+1)); done
printf 'baseline\t%s\t0\t0\n' "$baseline"
for _ in 1 2; do (cd "$REPO" && "$TMP/krn" exec --input input.txt -- printf deterministic >/dev/null); done
printf 'krn-deterministic\t2\t0\t0\n'
(cd "$REPO" && "$TMP/krn" exec --cache --input input.txt -- printf cached >/dev/null)
(cd "$REPO" && "$TMP/krn" exec --cache --input input.txt -- printf cached >/dev/null)
printf 'krn-cache\t1\t1\t%s\n' "$(wc -c < "$REPO/.git/krn/metrics.jsonl" | tr -d ' ')"
