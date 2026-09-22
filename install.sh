#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$HOME/.local/bin"
TMP=$(mktemp "${TMPDIR:-/tmp}/krn.XXXXXX")
trap 'rm -f "$TMP"' EXIT
(cd "$HERE" && GOCACHE="${TMP}.cache" go build -o "$TMP" .)
install -m 0755 "$TMP" "$HOME/.local/bin/krn"
"$HOME/.local/bin/krn" integrate codex
printf '\nInstalled krn to %s\n' "$HOME/.local/bin/krn"
printf 'If krn is not found, add: export PATH="$HOME/.local/bin:$PATH"\n'
printf 'Then: cd any-git-project && codex\n'
