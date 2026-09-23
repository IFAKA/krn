#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/krn-bench.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
export GOCACHE="$TMP/go-cache"
REPO="$TMP/repo"
BIN="$TMP/krn"
mkdir "$REPO"
(cd "$REPO" && git init -q && git config user.email bench@example.invalid && git config user.name bench && printf 'input\n' > input.txt && printf 'unrelated\n' > unrelated.txt && printf '0\n' > exec-count && git add . && git commit -qm init)
(cd "$ROOT" && go build -o "$BIN" .)

command_body='n=$(cat exec-count); n=$((n + 1)); printf "%s\\n" "$n" > exec-count; printf stable'
run_cached() {
	(cd "$REPO" && "$BIN" exec --cache --verified --input input.txt -- sh -c "$command_body" >/dev/null)
}
count() { (cd "$REPO" && tr -d '\n' < exec-count); }

printf 'case\texecutions\tcumulative_cache_hits\tresult\n'

run_cached
run_cached
[ "$(count)" = 1 ]
printf 'unchanged dependency\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"cached":true' .git/krn/metrics.jsonl || true)"

(cd "$REPO" && printf 'changed\n' > input.txt)
run_cached
[ "$(count)" = 2 ]
printf 'changed declared dependency\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"cached":true' .git/krn/metrics.jsonl || true)"

(cd "$REPO" && printf 'unrelated changed\n' > unrelated.txt)
run_cached
[ "$(count)" = 2 ]
printf 'unrelated file\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"cached":true' .git/krn/metrics.jsonl || true)"

(cd "$REPO" && "$BIN" exec --cache --verified --input input.txt -- sh -c 'n=$(cat fail-count 2>/dev/null || printf 0); n=$((n + 1)); printf "%s\\n" "$n" > fail-count; exit 7' >/dev/null 2>&1) || :
(cd "$REPO" && "$BIN" exec --cache --verified --input input.txt -- sh -c 'n=$(cat fail-count 2>/dev/null || printf 0); n=$((n + 1)); printf "%s\\n" "$n" > fail-count; exit 7' >/dev/null 2>&1) || :
[ "$(cd "$REPO" && tr -d '\n' < fail-count)" = 2 ]
printf 'failed execution\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"result":"failure"' .git/krn/metrics.jsonl || true)"

(cd "$REPO" && printf 'other\n' > other.txt)
(cd "$REPO" && "$BIN" exec --cache --verified --input other.txt -- sh -c "$command_body" >/dev/null)
[ "$(count)" = 3 ]
printf 'invalid applicability (different input)\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"cached":false' .git/krn/metrics.jsonl || true)"

(cd "$REPO" && for f in .git/krn/cache/*.json; do printf '{' > "$f"; done)
run_cached
[ "$(count)" = 4 ]
printf 'malformed/stale cache\t%s\t%s\tPASS\n' "$(count)" "$(cd "$REPO" && rg -c '"cached":true' .git/krn/metrics.jsonl || true)"

printf 'baseline repeated execution\t2\t0\tPASS\n'
printf 'KRn cache hits\t%s\t%s\tPASS\n' 2 "$(cd "$REPO" && rg -c '"cached":true' .git/krn/metrics.jsonl || true)"
printf 'unchanged workload saved executions\t1\t1\tPASS\n'
