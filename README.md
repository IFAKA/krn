# KRn

**A small, local CLI that gives coding agents a ranked map of your repository and takes deterministic work out of the model's hands.**

KRn sits between a coding agent (pi, Claude Code, Codex) and your repository. It does the jobs a program can do exactly and cheaply: rank the code relevant to a question, search by plain words, make structural edits, run the project's checks, and cache repeated commands. The agent keeps doing the reasoning. Nothing runs in the background, and nothing leaves your machine.

```text
     model
       ↓
  agent (pi, Claude Code, Codex)    reasons, decides, edits
       ↓
      KRn                           maps, searches, edits, verifies, caches
       ↓
  repository, Git, ripgrep, tree-sitter, ast-grep, your test commands
```

## Does it help?

It depends on the agent and the model. Measured on 12 tasks (9 "where is X implemented" questions, 3 small edits), 3 seeds each:

| setup | without KRn | with KRn | takeaway |
|---|---:|---:|---|
| **pi + local 30B model** (M4 Pro) | 42% correct | **85% correct**, about 1.6× the correct answers per minute | Worth using. The small model guesses without looking; the map makes it search. |
| same, a plain `git ls-files` list instead of KRn (one run) | | 92% correct, about 1.1× per minute | Any file list stops the guessing. KRn's ranking reaches the same accuracy about 40% faster. |
| **Claude Code + Haiku 4.5** | 97% correct | 100% with the routing policy, at about 15% more cost | No measurable gain. These tasks are too easy for this model, so the policy is opt-in. |
| Claude Code with Sonnet or Opus, Codex, real bug fixes | | | Not measured yet. |

At this sample size, differences of up to about 5 correct answers out of 36 are noise. Every run is in [docs/benchmarks.md](docs/benchmarks.md), including experiments that failed and were dropped, and the raw data is in [eval/results/](eval/results/). The harness is included, so you can run it on your own tasks.

## Quick start

Requires Go 1.24+ and Git. There is no release binary yet.

```sh
git clone https://github.com/IFAKA/krn.git
cd krn
./install.sh
```

The installer:

* builds `krn` into `~/.local/bin` (add it to `PATH` if needed);
* installs [ast-grep](https://ast-grep.github.io/) if it is missing, for `krn code`;
* **pi:** copies the extension to `~/.pi/agent/extensions/krn/`, which adds the map to the first prompt of each session.

It does not touch Claude Code or Codex. For them, `krn` is a set of commands the agent can call, and there is an opt-in routing policy that tells the agent when to call them: `krn integrate claude` or `krn integrate codex` adds a short, marked block to `~/.claude/CLAUDE.md` or `~/.codex/AGENTS.md`. It is opt-in because on Claude Code with Haiku 4.5 it added about 15% cost without a measured gain; Codex hasn't been measured.

After that, use your agent as usual inside any Git repository; there is no per-project setup. `krn uninstall` removes everything the installer added. Details are in [docs/commands.md](docs/commands.md#installation-and-integration).

## What it looks like

`krn map --focus "where is the exec cache key computed" --tokens 300`, run on this repository:

```text
project: A small, local CLI that gives coding agents a ranked map of your repository and takes ...
layout: eval/results/(16) eval/fixture/(7) internal/eval/(7) docs/(5) ...

internal/exec/exec.go:
  19│ const cacheSchema = "2"
  21│ type cacheRecord struct
  77│ func cacheKeyAtRoot(root string, command []string, inputs []string) (string, map[string]string, error)
  177│ func validCacheRecord(rec cacheRecord, key string, command, inputs []string, fps map[string]string) bool
internal/workspace/repo.go:
  29│ func Discover() (Repo, error)
  ...
```

The map lists function and type signatures only. Files are ranked toward the question using PageRank over the references between them, and the output is cut to a token budget. It supports JS/TS/TSX, Python, and Go.

`krn find cache key fingerprint` searches by plain words and returns the best-matching files, with the matching lines in context:

```text
terms: cache, key, fingerprint (271 matching lines)

internal/exec/exec.go
> 77  func cacheKeyAtRoot(root string, command []string, inputs []string) (string, map[string]string, error) {
  78  	h := sha256.New()
  ...
full_log: .git/krn/runs/20260926T153058Z-find.log
```

The agent sees a short result, and the full output is saved to a log it can open if it needs more.

## Commands

| command | does |
|---|---|
| `krn map [--focus TEXT] [--tokens N]` | ranked, signatures-only repository map |
| `krn find WORDS... [--max-files N]` | plain-word search, ranked, with context (`--regex` for raw patterns) |
| `krn context` | repository root, branch, commit, changed files, detected ecosystems, saved task state |
| `krn code read\|replace\|insert-before\|insert-after\|remove` | structural edit through ast-grep; the pattern must match exactly once, and the file is written only if the result still parses |
| `krn verify [--level fast\|full]` | runs the project's own checks (Go, npm, Cargo, pytest, or `.kern/config.json`); reports `unknown` if it finds none, rather than inventing a command |
| `krn exec [--cache --input PATH...] -- CMD` | runs a command with bounded output; with `--cache`, reuses the last result while the declared inputs are unchanged |
| `krn state show\|set\|add\|clear` | a few durable task facts: objective, constraints, proven facts, open questions, dead ends |
| `krn eval-pi`, `krn eval-claude` | benchmark harness: runs a task manifest through pi or Claude Code, variant by variant, and grades the answers |
| `krn integrate`, `krn doctor`, `krn uninstall` | add or remove the agent integrations; check the setup |

Most commands take `--json`. Full flags and semantics are in [docs/commands.md](docs/commands.md). For how `map`, `find` and the pi extension work, see [docs/pi.md](docs/pi.md).

## Design rules

* **Deterministic work only.** KRn does only what a program can do exactly. Judgement stays with the model.
* **Small output, full evidence on disk.** What the agent sees is bounded; the full output goes to `.git/krn/runs/`.
* **Fail open.** Outside Git, or on a parse error or a stale or tampered cache record, KRn steps aside instead of guessing.
* **Don't store what can be rebuilt.** Git and the files are the source of truth. KRn stores only task facts that can't be reconstructed, plus cache records that are checked before every reuse.
* **Measure before shipping.** A feature stays only if it raises correct answers per minute in the benchmark. That is why the pi extension contains only the map: bounded search output and a `find_code` tool were removed because neither helped, and the Claude Code and Codex routing policy is opt-in. The standalone commands (`find`, `code`, `verify`, `exec`, `state`) are tested for correctness but not yet shown to help an agent; see [Limits](#limits).

It deliberately has no daemon, embeddings, vector database, MCP server, cloud service, or telemetry. The reasoning is in [docs/design.md](docs/design.md).

## Limits

* The benchmark is small: 12 tasks, one local model and one Claude model. Two of the three task repositories are private.
* On Claude Code with Haiku, the routing policy added cost without a measured gain, so `install.sh` no longer adds it. Whether it helps larger models or harder tasks is unknown.
* Only the pi map has been measured on its own. The benchmark tasks are short lookups and small edits, which never need `code`, `verify`, `exec --cache` or `state`, so their effect on an agent is unmeasured.
* The map shows where functions start, not which line inside them does the work, so agents can still cite the wrong line.
* `exec --cache` is only as safe as the inputs you declare. KRn checks that they haven't changed, but it can't know about inputs you left out.

## Documentation

| | |
|---|---|
| [docs/benchmarks.md](docs/benchmarks.md) | method, every run, confidence intervals, dropped experiments, Claude Code results, cache benchmark |
| [docs/pi.md](docs/pi.md) | how `find` and `map` work, the pi extension and its settings, using pi with a local model |
| [docs/commands.md](docs/commands.md) | installation, command reference, routing policy, cache semantics, structural edits, Codex A/B harness |
| [docs/design.md](docs/design.md) | why and how it works, memory model, storage and privacy, non-goals, related work, source layout |
| [docs/evidence.md](docs/evidence.md) | the research behind each design decision, and what is and isn't proven |
| [docs/plans/](docs/plans/) | design plans and the [investigation log](docs/plans/2026-09-26-pi-lean-investigation.md) |

## Uninstall

```sh
krn uninstall
```

This removes the managed blocks from `CLAUDE.md` and `AGENTS.md`, the pi extension, and the installed binary. Per-repository data in `.git/krn/` stays; delete it yourself if you want it gone.

## Contributing

Each command is its own package under `internal/`, so changing one command means reading one package. Add tests with behavior changes, run `go test ./...`, and back any new feature or claim with a measurement. The source layout is described in [docs/design.md](docs/design.md#source-layout-and-contributing).

## License

[MIT](LICENSE)
