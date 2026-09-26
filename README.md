# KRn

**Make coding agents repeat less deterministic work.**

KRn is a research-informed deterministic optimization substrate for coding agents such as Codex, Claude Code, and pi. It reconstructs repository facts, bounds retrieved evidence, verifies work, records irreducible state and execution evidence, and safely reuses deterministic computation when explicit dependencies establish that reuse remains valid.

The agent (Codex, Claude Code, or pi) stays in charge. KRn is the layer beneath and around it:

```text
          model
            ↓
Codex / Claude Code / pi      agent / harness
            ↓
           KRn                deterministic optimization substrate
            ↓
repository + existing tools
```

Measured so far (method, noise, and limits in [Benchmarks](docs/benchmarks.md)):

* **pi with a local 30B model:** a repository map on the first prompt raised correct answers from 42% to 85% (pooled over three runs). A plain `git ls-files` list did as well on correctness; KRn's ranked map reached it about 40% faster.
* **Claude Code with Haiku 4.5:** vanilla Claude Code already answered 35 of 36 tasks. The KRn routing policy that `install.sh` adds made no measurable difference to correctness and cost about 15% more per task.
* **Not measured:** Codex, Claude Code with larger models, and real bug-fix or feature work.

Features that did not help were left off or dropped, and those runs are published too.

The engineering thesis is:

**When work can be reconstructed or safely reused deterministically, move it out of repeated model-driven execution.**

This is KRn's thesis, not a quotation from any research paper and not a claim that KRn reduces model reasoning, reduces tokens, or is globally optimal.

## Install

The repository currently ships a source-based installer, not a release binary or a verified public `curl | sh` endpoint.

```sh
git clone https://github.com/IFAKA/krn.git
cd krn
./install.sh
```

`install.sh` requires Go 1.24+, builds KRn, installs it at `~/.local/bin/krn`, installs the required `ast-grep` CLI when it is missing, and adds a small managed routing policy to `$CODEX_HOME/AGENTS.md` (default `~/.codex/AGENTS.md`). When Claude Code is detected (`claude` on `PATH` or an existing `$CLAUDE_CONFIG_DIR`/`~/.claude` directory), it also adds the same policy to `$CLAUDE_CONFIG_DIR/CLAUDE.md` (default `~/.claude/CLAUDE.md`). When pi is detected (`pi` on `PATH` or an existing `$PI_CODING_AGENT_DIR`/`~/.pi/agent` directory), it copies the pi extension to `extensions/krn/index.ts` there. It tries npm, Cargo, and pip (installing into `~/.local` without sudo), then falls back to Homebrew, which installs into its own prefix. If either command is not on `PATH`, add `~/.local/bin` to it.

## Use

```sh
cd any-git-project
codex    # or: claude, or: pi
```

There is no `krn init`. The integration is global: an instruction block per agent for Codex and Claude Code (`krn integrate codex|claude`), and an extension for pi (copied by `install.sh`). The commands operate on the Git repository containing the current directory. pi's model and provider configuration is pi's own and outside KRn; see [Local models with pi](docs/pi.md).

## Benchmarks

### At a glance

| if you use | measured | correct (of 36) | speed and cost | verdict |
|---|---|---|---|---|
| pi + local model (30B, M4 Pro) | no KRn, KRn map, plain file list (run 6) | 14 / 32 / 33 | correct per minute 0.94 / 1.50 / 1.07 | Use the map. Most of the gain comes from giving the model any map; KRn's ranking makes it about 40% faster than a file list. |
| Claude Code + Haiku 4.5 | no KRn, KRn policy, KRn map, plain file list | 35 / 36 / 33 / 33 | $ per task 0.032 / 0.037 / 0.023 / 0.028 | No correctness gain; vanilla is near the ceiling on these tasks. The installed policy costs about 15% more. The map was fastest and cheapest but is not part of the Claude Code integration. |
| Claude Code with Sonnet or Opus, Codex | not measured | | | unknown |
| bug fixes and features | 3 small edit tasks, solved in nearly every variant | | | unknown |
| `krn exec --cache` | correct reuse and invalidation (`benchmark.sh`) | | time saved not measured | mechanism only |

Differences of about five correct answers or fewer out of 36 are noise at this size (see [docs/benchmarks.md](docs/benchmarks.md)). Other tools (Aider's repo map, LSP or MCP code-navigation servers) were not run; the plain file list stands in for "any map". Two of the three task repositories are private, so the numbers cannot be reproduced exactly elsewhere; the harness can.

Full method, per-run tables, the dropped experiments, and the cache benchmark: [docs/benchmarks.md](docs/benchmarks.md).

## Why KRn?

A model context window is expensive working memory. Repository facts, command output, and repeated deterministic computation should not need to enter that working memory in full when Git, the filesystem, ripgrep, tests, or a valid cached result can establish the required evidence directly.

KRn therefore tries to move deterministic work downward:

```text
agent reasoning
      ↓
minimum evidence
      ↓
deterministic work
      ↓
explicit dependencies
      ↓
verified cache
      ↓
safe reuse
```

This does not make the agent passive and does not establish that KRn reduces model tokens or reasoning.

KRn provides small, recoverable interfaces for deterministic work that can be reconstructed, measured, verified, or safely reused.

## How it works

```text
TASK                         user asks the agent to do work
  |
  v
RECONSTRUCT                  rebuild repo facts from Git, files, and state
  |
  v
MINIMUM EVIDENCE             retrieve only the evidence needed now
  |
  +---------------------------+
  |                           |
  v                           v
DETERMINISTIC WORK       AGENT REASONING
tools prove facts        model judges, plans, and synthesizes
  |                           |
  +-------------+-------------+
                |
                v
              VERIFY          run checks or reject uncertain evidence
                |
                v
              RECORD          save local evidence from verified work
                |
       +--------+--------+
       |                 |
       v                 v
     CACHE             METRICS
 reusable results      local measurements
       |
       v
 VERIFIED REUSE               reuse only while command and inputs still match
```

The main path is conservative: KRn reconstructs what it can, gathers bounded evidence, lets deterministic tools and agent reasoning meet at verification, and records only verified work. The reuse path is narrower: cached results are reused only while their explicit dependencies still match.

* `context` reconstructs Git root, branch, commit, changed paths, detected ecosystems, and saved task state.
* `find` turns plain words or identifiers into ripgrep searches, ranks files, returns a bounded file/snippet projection, and saves the full search output for recovery.
* `map` parses source files with tree-sitter and prints a ranked, signatures-only repository map fitted to a token budget.
* `code` applies ast-grep structural edits whose pattern must match exactly once.
* `verify` discovers safe project-native checks from `.kern/config.json`, `package.json`, Go, Cargo, or pytest. Unknown projects remain `unknown`; KRn does not invent a command.
* `exec` runs structured local commands, bounds output shown to the caller, records metrics, and can cache executions only when explicit input dependencies are supplied.
* Cached executions are reused only when their command, repository provenance, schema, declared dependencies, dependency fingerprints, metadata, and result integrity remain valid.

The implementation fails open around uncertain reconstruction or reuse: unavailable, malformed, stale, tampered, or mismatched evidence is rejected rather than treated as valid.

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
krn eval-pi --model MODEL [--provider NAME] [--manifest PATH] [--variants none,A,AB,ABC,C,T] [--seeds N] [--tasks IDS]
krn eval-claude [--model MODEL] [--manifest PATH] [--variants none,policy,map,tree] [--seeds N] [--tasks IDS] [--total-budget-usd N]
            [--agent-dir DIR] [--extension PATH] [--krn PATH] [--workdir DIR] [--timeout DURATION] [--output DIR]
krn integrate codex|remove-codex|claude|remove-claude
krn doctor
krn uninstall
krn version
```

Details: [docs/commands.md](docs/commands.md). pi specifics: [docs/pi.md](docs/pi.md).

## Uninstall

```sh
krn uninstall
```

This removes KRn's managed blocks from the Codex `AGENTS.md` and the Claude Code `CLAUDE.md`, and the pi extension directory `~/.pi/agent/extensions/krn` when its `index.ts` is KRn's.

When invoked from the installed `~/.local/bin/krn`, it also removes that binary.

It does not remove repository-private `.git/krn` records.

## Documentation

| doc | contents |
|---|---|
| [docs/benchmarks.md](docs/benchmarks.md) | method, every run and dropped experiment, pooled intervals, Claude Code results, cache benchmark |
| [docs/pi.md](docs/pi.md) | `krn find`, `krn map`, the pi extension, its settings, and `krn eval-pi`/`eval-claude` |
| [docs/commands.md](docs/commands.md) | integration policy, cache semantics, structural edits, Codex A/B harness |
| [docs/design.md](docs/design.md) | memory model, storage and privacy, non-goals, limits, related work |
| [docs/evidence.md](docs/evidence.md) | research behind the architecture and what is and is not established |
| [docs/plans/](docs/plans/) | design plans and the [investigation log](docs/plans/2026-09-26-pi-lean-investigation.md) of the evaluations |
| [eval/results/](eval/results/) | raw results for every run |


## Contributing

Keep changes small, deterministic, inspectable, and falsifiable.

The source tree is organized by feature: each command lives in its own package, so a change to one command only requires reading that package.

```text
cmd/krn/              command dispatch and end-to-end tests of the built binary
internal/workspace/   shared core: repository discovery, .git/krn storage, bounded output, metrics
internal/context/     krn context
internal/find/        krn find (ranked natural-language search)
internal/repomap/     krn map (tree-sitter tags + personalized PageRank)
internal/code/        krn code (ast-grep structural edits)
internal/verify/      krn verify
internal/state/       krn state
internal/exec/        krn exec and the explicit-input cache
internal/eval/        krn eval (A/B harness), krn eval-suite, krn eval-pi, krn eval-claude
eval/                 eval task manifests and fixture repository
integrations/pi/      pi extension (find_code tool, first-turn map, bounded search output)
internal/integrate/   Codex/Claude Code routing policy, krn integrate, krn uninstall
internal/doctor/      krn doctor
internal/testutil/    helpers shared by tests
```

Feature packages depend on `workspace`; `context` also reads `state`, and `eval` uses the policy from `integrate`.

Add or update tests for behavior.

Run the project checks.

Document only capabilities that source, tests, measurements, or explicitly cited external evidence establish.

Do not convert architectural hypotheses into product claims.

Additional complexity requires a reproducible workload showing that it improves the relevant frontier.

## License

[MIT](LICENSE)
