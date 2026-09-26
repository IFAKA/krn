# Command reference

Details behind the command list in the README: agent integration policy, cache semantics, structural edits, and the Codex A/B harness.

[← README](../README.md)

## Installation and integration

The repository currently ships a source-based installer, not a release binary or a verified public `curl | sh` endpoint.

```sh
git clone https://github.com/IFAKA/krn.git
cd krn
./install.sh
```

`install.sh` requires Go 1.24+, builds KRn, installs it at `~/.local/bin/krn`, installs the required `ast-grep` CLI when it is missing, and does not add the Codex or Claude Code routing policy: that is opt-in with `krn integrate codex` (writes `$CODEX_HOME/AGENTS.md`, default `~/.codex/AGENTS.md`) or `krn integrate claude` (writes `$CLAUDE_CONFIG_DIR/CLAUDE.md`, default `~/.claude/CLAUDE.md`), because on Claude Code with Haiku 4.5 it added cost without a measured gain ([Benchmarks](benchmarks.md#claude-code-with-haiku-45)). When pi is detected (`pi` on `PATH` or an existing `$PI_CODING_AGENT_DIR`/`~/.pi/agent` directory), it copies the pi extension to `extensions/krn/index.ts` there. It tries npm, Cargo, and pip (installing into `~/.local` without sudo), then falls back to Homebrew, which installs into its own prefix. If either command is not on `PATH`, add `~/.local/bin` to it.

```sh
cd any-git-project
codex    # or: claude, or: pi
```

There is no `krn init`. The integration is global: an extension for pi (copied by `install.sh`), and an opt-in instruction block per agent for Codex and Claude Code (`krn integrate codex|claude`). The commands operate on the Git repository containing the current directory. pi's model and provider configuration is pi's own and outside KRn; see [Local models with pi](pi.md).

## Commands

```text
krn context [--json]
krn find QUERY... [--json] [--max-files N] [--budget BYTES] [--regex]
krn map [--tokens N] [--focus TEXT]
krn code read|replace|insert-before|insert-after|remove --file PATH --pattern PATTERN [--lang LANG] [--content TEXT|--content-file PATH]
krn verify [--level fast|full] [--json]
krn state show
krn state set objective TEXT
krn state add constraint|proven|open|negative TEXT
krn state clear
krn exec [--verified] [--cache --input PATH ...] -- COMMAND ARGS...
krn eval --task PATH --verify COMMAND --model MODEL --reasoning-effort EFFORT [--codex PATH] [--output DIR] [--json]
krn eval-suite [--manifest PATH] --model MODEL --reasoning-effort EFFORT [--codex PATH] [--output DIR] [--freeze-only] [--json]
krn eval-pi --model MODEL [--provider NAME] [--manifest PATH] [--variants none,C,T] [--seeds N] [--tasks IDS]
krn eval-claude [--model MODEL] [--manifest PATH] [--variants none,policy,map,tree] [--seeds N] [--tasks IDS] [--total-budget-usd N]
            [--agent-dir DIR] [--extension PATH] [--krn PATH] [--workdir DIR] [--timeout DURATION] [--output DIR]
krn integrate codex|remove-codex|claude|remove-claude
krn doctor
krn uninstall
krn version
```

The Codex and Claude Code integrations are instruction-based and install the same routing policy, addressed to each agent: a managed block in the Codex `AGENTS.md` or the Claude Code `CLAUDE.md`. The agent makes a routing decision before its first shell or file-reading tool call. When a task requires learning the repository before answering or editing, the first operation is `krn context --json`; this includes repository-orientation questions such as what the project is, how it is structured, and where behavior is implemented. If more evidence is needed, the agent prefers `krn find QUERY --json --max-files N` before broad traversal or repeated search. `verify`, `exec`, and `state` retain their narrower roles described above.

The orientation rule remains bypassable for trivial answers, exact known-file/content requests, explicit user-requested shell commands, one clearly sufficient cheap direct operation, or unavailable KRn. The integration does not install prompt hooks, mutate agent state databases or settings, force KRn on every task, or claim that KRn reduces model tokens, reasoning, or wall time without measurements. Re-running integration replaces all well-formed KRn-managed blocks with the current policy and leaves unrelated `AGENTS.md`/`CLAUDE.md` content in place.

`--cache` requires at least one `--input`.

The cache key includes the exact argv, repository root, cache schema, canonicalized dependencies, and content fingerprints of every declared input.

A changed declared input, command, repository root, malformed record, stale schema, tampered result, or provenance mismatch produces a cache miss.

Unknown side effects are never inferred safe. Callers must declare the complete inputs that make a command reusable.

`--verified` is an external assertion recorded with an execution. It is not proof of command purity or complete dependency coverage.

## Structural code edits

`krn code` is a language-independent mechanical interface backed by the externally maintained `ast-grep` CLI. The installer provisions ast-grep into the user-local environment when it is missing. If KRn is installed another way, install ast-grep separately and put it on `PATH`. Patterns are structural ast-grep patterns, and language is inferred from the file extension unless `--lang` is supplied:

```sh
krn code read --file src/user.ts --pattern 'function CreateUser() { $$$BODY }'
krn code replace --file src/user.ts --pattern 'function CreateUser() { $$$BODY }' --content 'function CreateUser() { return 42 }'
krn code insert-before --file script.sh --pattern 'function deploy() { $$$BODY }' --content 'function log() { echo ok; }'
krn code insert-after --file .gitlab-ci.yml --lang yaml --pattern 'build: { $$$JOB }' --content 'test: { script: npm test }'
krn code remove --file styles.scss --lang css --pattern '$COLOR: red;'
```

The pattern must resolve to exactly one match. Missing, ambiguous, unsupported, unavailable-grammar, malformed, stale, or invalid targets fail with a non-zero status and leave the file unchanged. KRn verifies ast-grep's UTF-8 byte range against the local source, rejects the edit if the candidate has more tree-sitter `ERROR` nodes than the original (so files with pre-existing errors stay editable), additionally parses `.go` results with Go's standard parser, generates a unified diff, and writes atomically only after validation. Tree-sitter recovers silently from some truncated input, such as a missing closing brace in TypeScript or an unterminated `if` in Bash, so this check is not a full syntax guarantee outside Go. Repeating an insert with identical adjacent content is a no-op. It preserves unrelated source bytes and does not format or semantically interpret code.

This boundary is intentional: the model decides the structural pattern, content, and semantic intent; KRn resolves the external match, performs the byte-span transformation, validates the candidate, and exposes the resulting diff. Examples can target TypeScript, JavaScript, Bash, YAML/GitLab CI, Go, CSS, and CSS-compatible SCSS. The installed ast-grep grammar set determines the available language names; for example, current ast-grep releases provide `css` but may not provide a separate `scss` grammar, so unsupported `--lang` values fail cleanly. KRn does not maintain language grammars or syntax-version tables; parser and grammar updates belong to ast-grep and Tree-sitter tooling. Semantic validation, compilation, formatting, and type checking remain repository-native concerns. Run `go test ./...` and the repository's verification checks after edits.

## End-to-end A/B evaluation

`krn eval` is a separate experiment from `benchmark.sh`. It drives Codex; `krn eval-claude` runs the pi task manifest through Claude Code (see [Benchmarks](benchmarks.md#claude-code-with-haiku-45)). `krn eval` evaluates the same task twice from fresh clones of the same committed `HEAD`:

* A: Codex alone, with isolated Codex state.
* B: the same Codex invocation plus the current KRn routing policy in isolated Codex instructions.

Example:

```sh
krn eval --task task.txt --verify 'go test ./...' --model MODEL --reasoning-effort high --json
```

The harness preserves each prompt, metadata, Codex JSONL stdout, stderr, verification output, and a `report.json` under `.git/krn/evals/` (or `--output DIR`). The report measures verified completion, wall time, tool/execution events, and human-intervention events. Codex token totals and peak context are reported as `unavailable` unless the CLI JSONL contains explicit usage fields; the harness never estimates them from output size. Deltas involving unavailable values are also `unavailable`.

The verifier is run after Codex exits in each fresh checkout. A run is verified complete only when both Codex and the explicit verifier succeed. The command is non-interactive and uses automatic approval, so human interventions are counted from explicit intervention events in the event stream; this is not a substitute for measuring an interactive operator.

`benchmark.sh` remains the deterministic KRn cache-mechanism benchmark and is not replaced by this end-to-end experiment.
