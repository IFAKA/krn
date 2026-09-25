#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"

install_ast_grep() {
	if command -v ast-grep >/dev/null 2>&1; then
		printf 'Using ast-grep at %s\n' "$(command -v ast-grep)"
		return
	fi
	if command -v npm >/dev/null 2>&1; then
		printf 'Installing ast-grep with npm into %s\n' "$HOME/.local"
		npm install --prefix "$HOME/.local" --global @ast-grep/cli
	elif command -v cargo >/dev/null 2>&1; then
		printf 'Installing ast-grep with Cargo into %s\n' "$HOME/.local"
		cargo install ast-grep --locked --root "$HOME/.local"
	elif command -v python3 >/dev/null 2>&1; then
		printf 'Installing ast-grep with pip into %s\n' "$HOME/.local"
		python3 -m pip install --prefix "$HOME/.local" ast-grep-cli
	elif command -v brew >/dev/null 2>&1; then
		printf 'Installing ast-grep with Homebrew\n'
		brew install ast-grep
	else
		cat >&2 <<'EOF'
Unable to install ast-grep automatically.
Install it with one of the supported package managers:
  npm install --prefix "$HOME/.local" --global @ast-grep/cli
  cargo install ast-grep --locked --root "$HOME/.local"
  python3 -m pip install --prefix "$HOME/.local" ast-grep-cli
  brew install ast-grep
Then rerun ./install.sh.
EOF
		exit 1
	fi
	if ! command -v ast-grep >/dev/null 2>&1; then
		printf 'ast-grep was installed but is not on PATH: %s\n' "$HOME/.local/bin" >&2
		exit 1
	fi
}

install_ast_grep
TMP=$(mktemp "${TMPDIR:-/tmp}/krn.XXXXXX")
trap 'rm -f "$TMP"' EXIT
(cd "$HERE" && GOCACHE="${TMP}.cache" go build -o "$TMP" ./cmd/krn)
install -m 0755 "$TMP" "$HOME/.local/bin/krn"
"$HOME/.local/bin/krn" integrate codex
printf '\nInstalled krn to %s\n' "$HOME/.local/bin/krn"
printf 'If krn is not found, add: export PATH="$HOME/.local/bin:$PATH"\n'
printf 'Then: cd any-git-project && codex\n'
