# KRn

**Make coding agents repeat less deterministic work.**

KRn is a research-informed deterministic optimization substrate for coding agents such as Codex and Claude Code. It reconstructs repository facts, bounds retrieved evidence, verifies work, records irreducible state and execution evidence, and safely reuses deterministic computation when explicit dependencies establish that reuse remains valid.

Codex or Claude Code remains the agent. KRn is the layer beneath and around it:

```text
     model
       ↓
Codex / Claude Code   agent / harness
       ↓
      KRn              deterministic optimization substrate
       ↓
repository + existing tools
```

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

`install.sh` requires Go 1.24+, builds KRn, installs it at `~/.local/bin/krn`, installs the required `ast-grep` CLI when it is missing, and adds a small managed routing policy to `$CODEX_HOME/AGENTS.md` (default `~/.codex/AGENTS.md`). When Claude Code is detected (`claude` on `PATH` or an existing `$CLAUDE_CONFIG_DIR`/`~/.claude` directory), it also adds the same policy to `$CLAUDE_CONFIG_DIR/CLAUDE.md` (default `~/.claude/CLAUDE.md`). It tries npm, Cargo, and pip (installing into `~/.local` without sudo), then falls back to Homebrew, which installs into its own prefix. If either command is not on `PATH`, add `~/.local/bin` to it.

## Use

```sh
cd any-git-project
codex    # or: claude
```

There is no `krn init`. The integration is global (per agent, via `krn integrate codex|claude`); the commands operate on the Git repository containing the current directory.

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
* `find` uses ripgrep, returns a bounded file/snippet projection, and saves the full search output for recovery.
* `verify` discovers safe project-native checks from `.kern/config.json`, `package.json`, Go, Cargo, or pytest. Unknown projects remain `unknown`; KRn does not invent a command.
* `exec` runs structured local commands, bounds output shown to the caller, records metrics, and can cache executions only when explicit input dependencies are supplied.
* Cached executions are reused only when their command, repository provenance, schema, declared dependencies, dependency fingerprints, metadata, and result integrity remain valid.

The implementation fails open around uncertain reconstruction or reuse: unavailable, malformed, stale, tampered, or mismatched evidence is rejected rather than treated as valid.

## Memory model

Agent context is working memory, not KRn's persistent memory.

```text
agent context             = expensive working memory
repository + Git          = reconstructable source-of-truth memory
.git/krn/state.json       = irreducible semantic/task memory
.git/krn/cache/           = reusable deterministic computation
.git/krn/runs/            = recoverable command/search/verification evidence
.git/krn/metrics.jsonl    = local execution measurements
AGENTS.md / CLAUDE.md     = policy/instructions, not project memory
```

The governing rule is:

**If KRn can cheaply reconstruct something from an authoritative source, it should not remember a duplicate.**

State contains only fields such as objective, constraints, proven facts, open questions, and negative results.

Cache records contain the information required to conservatively determine whether a previously executed deterministic computation remains reusable.

KRn does not infer undeclared dependencies. Cache safety therefore depends on callers declaring the complete inputs that determine whether a command can be reused.

## Commands

```text
krn context [--json]
krn find QUERY [--json] [--max-files N]
krn code read|replace|insert-before|insert-after|remove --file PATH --pattern PATTERN [--lang LANG] [--content TEXT|--content-file PATH]
krn verify [--level fast|full] [--json]
krn state show
krn state set objective TEXT
krn state add constraint|proven|open|negative TEXT
krn state clear
krn exec [--verified] [--cache --input PATH ...] -- COMMAND ARGS...
krn eval --task PATH --verify COMMAND --model MODEL --reasoning-effort EFFORT [--output DIR] [--json]
krn eval-suite [--manifest PATH] --model MODEL --reasoning-effort EFFORT [--output DIR] [--freeze-only] [--json]
krn integrate codex|remove-codex|claude|remove-claude
krn doctor
krn uninstall
krn version
```

The Codex and Claude Code integrations are instruction-based and install the same routing policy, addressed to each agent: a managed block in the Codex `AGENTS.md` or the Claude Code `CLAUDE.md`. The agent makes a routing decision before its first shell or file-reading tool call. When a task requires learning the repository before answering or editing, the first operation is `krn context --json`; this includes repository-orientation questions such as what the project is, how it is structured, and where behavior is implemented. If more evidence is needed, the agent prefers `krn find QUERY --json --max-files N` before broad traversal or repeated search. `verify`, `exec`, and `state` retain their narrower roles described above.

The orientation rule remains bypassable for trivial answers, exact known-file/content requests, explicit user-requested shell commands, one clearly sufficient cheap direct operation, or unavailable KRn. The integration does not install prompt hooks, mutate agent state databases or settings, force KRn on every task, or claim that KRn reduces model tokens, reasoning, or wall time without measurements. Re-running integration replaces all well-formed KRn-managed blocks with the current policy and leaves unrelated `AGENTS.md`/`CLAUDE.md` content in place.

`--cache` requires at least one `--input`.

### Structural code edits

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

### End-to-end A/B evaluation

`krn eval` is a separate experiment from `benchmark.sh`. It currently drives Codex only; there is no Claude Code eval harness yet. It evaluates the same task twice from fresh clones of the same committed `HEAD`:

* A: Codex alone, with isolated Codex state.
* B: the same Codex invocation plus the current KRn routing policy in isolated Codex instructions.

Example:

```sh
krn eval --task task.txt --verify 'go test ./...' --model MODEL --reasoning-effort high --json
```

The harness preserves each prompt, metadata, Codex JSONL stdout, stderr, verification output, and a `report.json` under `.git/krn/evals/` (or `--output DIR`). The report measures verified completion, wall time, tool/execution events, and human-intervention events. Codex token totals and peak context are reported as `unavailable` unless the CLI JSONL contains explicit usage fields; the harness never estimates them from output size. Deltas involving unavailable values are also `unavailable`.

The verifier is run after Codex exits in each fresh checkout. A run is verified complete only when both Codex and the explicit verifier succeed. The command is non-interactive and uses automatic approval, so human interventions are counted from explicit intervention events in the event stream; this is not a substitute for measuring an interactive operator.

`benchmark.sh` remains the deterministic KRn cache-mechanism benchmark and is not replaced by this end-to-end experiment.

The cache key includes the exact argv, repository root, cache schema, canonicalized dependencies, and content fingerprints of every declared input.

A changed declared input, command, repository root, malformed record, stale schema, tampered result, or provenance mismatch produces a cache miss.

Unknown side effects are never inferred safe. Callers must declare the complete inputs that make a command reusable.

`--verified` is an external assertion recorded with an execution. It is not proof of command purity or complete dependency coverage.

## Storage and privacy

KRn uses local files and existing repository tools. It has no daemon, cloud backend, network service, or telemetry service.

The installer writes only to `~/.local/bin/krn`, the managed Codex instruction block, the managed Claude Code instruction block (when Claude Code is detected), and an ast-grep install when ast-grep is missing (user-local via npm, Cargo, or pip, otherwise Homebrew).

Repository-private KRn data is stored under the Git directory:

```text
<repo>/.git/krn/state.json
<repo>/.git/krn/metrics.jsonl
<repo>/.git/krn/cache/<content-key>.json
<repo>/.git/krn/runs/<timestamp>-<operation>.log
```

The optional team-owned verification configuration is stored at the repository root:

```text
<repo>/.kern/config.json
```

Logs and state are local and may contain command output or task text.

Metrics record measurements available to KRn. Model token counts are recorded as `unavailable` when they were not supplied.

Source files, Git, manifests, tests, compilers, CI, and package managers remain authoritative.

## Research behind the architecture

KRn combines ideas supported by research across context retrieval, context optimization, and incremental computation.

These sources motivate individual design decisions in the settings they evaluate. They do not establish that KRn's particular combination is globally optimal or that KRn itself reduces model reasoning or token consumption.

KRn therefore treats its architecture as falsifiable and distinguishes research-supported principles from locally measured behavior and unproven hypotheses.

| KRn decision or feature                                 | Why                                                                                                                                                          | Evidence                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bounded context and recoverable projections             | Retrieval quality is not the same as dumping more context into the model; KRn shows a small projection while retaining recoverable full evidence.            | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Evaluates 1,136 tasks across 66 repositories and reports recall/precision gaps plus a gap between explored and used context.                                                                                                          |
| Deterministic retrieval before model reasoning          | Repository facts that ordinary tools can establish reliably do not require probabilistic inference.                                                          | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Reports recall-over-precision behavior in evaluated models, motivating bounded retrieval rather than indiscriminate context accumulation.                                                                                             |
| Do not assume one retrieval family is universally best  | KRn uses ripgrep and explicit repository facts by default rather than adding embeddings, a vector database, or a repository map without workload evidence.   | **[Agent Retrieval Bench: Evaluating Repository Context Retrieval for Coding Agents — Bowen Qin and Yi Xie, 2026, arXiv preprint](https://arxiv.org/abs/2607.24882)** — Evaluates 427 samples from 25 repositories and reports that lexical, RepoMap, embedding, and agent-context approaches perform differently across tasks and metrics; no retrieval family dominates universally. |
| Context externalization and compression                 | Long trajectories can accumulate irrelevant history; KRn retains full evidence outside active model context and exposes bounded projections.                 | **[ACON: Optimizing Context Compression for Long-horizon LLM Agents — Minki Kang et al., 2026, Lifelong Agent @ ICLR 2026 workshop](https://openreview.net/forum?id=x0alNh5o8v)** — Reports 26–54% lower peak tokens in its evaluated agent settings while largely preserving task performance. KRn does not claim those results for itself.                                           |
| Dependency-aware caching and invalidation               | Previously computed results should be reused only while the inputs determining them remain valid.                                                            | **[Build Systems à la Carte — Andrey Mokhov, Neil Mitchell, and Simon Peyton Jones, 2018, ICFP](https://doi.org/10.1145/3236774)** — Separates dependency structure from rebuild decisions and analyzes persistent build information, motivating explicit dependencies and conservative invalidation.                                                                                  |
| Do not infer semantic abstraction from repetition alone | Repeated shell commands do not establish semantic equivalence or safe parameterization. KRn therefore reports exact repetition only as a review signal.      | **[DreamCoder: Bootstrapping Inductive Program Synthesis with Wake-Sleep Library Learning — Kevin Ellis et al., 2021, PLDI](https://doi.org/10.1145/3453483.3454080)** — Studies library learning inside a defined synthesis language and domain. It does not establish that arbitrary agent execution trajectories can be safely generalized from repetition alone.                   |
| Minimal scaffolding                                     | Additional agent machinery is not automatically an improvement, so KRn keeps its substrate small unless measured workload evidence justifies more machinery. | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Reports only marginal retrieval gains from more sophisticated scaffolding in its evaluated setting; this supports measuring additional complexity rather than assuming it is beneficial.                                              |

## Evidence status

### Research-supported principles

The cited work supports, within its evaluated settings:

* careful and bounded context retrieval
* context externalization/compression as a potentially useful optimization
* evaluating retrieval strategies rather than assuming one universally dominates
* explicit dependency and rebuild decisions for deterministic computation

These results motivate KRn's architecture. They do not establish KRn's effectiveness.

### Implemented and locally verified KRn mechanisms

The source, tests, and benchmark establish:

* Git/filesystem reconstruction
* bounded projections with recoverable full logs
* fail-open verification discovery
* explicit semantic state
* structured command execution
* canonicalized explicit dependencies
* dependency content fingerprinting
* provenance-checked cache records
* cache-result integrity validation
* conservative cache invalidation
* malformed/stale/tampered cache rejection
* JSONL metrics
* marker-based idempotent Codex and Claude Code integration
* argument validation

`go test ./...`, `go test -race ./...`, `go vet ./...`, shell syntax checks, and the benchmark provide executable checks for these local behaviors.

### Directly measured efficiency

The benchmark demonstrates a narrow deterministic optimization:

```text
same command
+ same repository
+ same declared dependencies
+ same dependency contents
+ valid cache provenance
        ↓
previous result can be reused
        ↓
one repeated command execution avoided
```

It also verifies:

```text
changed declared dependency  → cache miss
unrelated file change        → cache remains applicable
failed execution             → not cached
different dependency         → cache miss
malformed/stale record       → cache miss
tampered/mismatched record   → cache miss
```

This establishes command-execution reuse under the declared dependency model.

It does not establish model-level efficiency.

### KRn-specific hypotheses

The broader hypothesis remains unproven:

> Moving deterministic or reconstructable work outside repeated model-driven execution may reduce the cognition or context required per verified useful coding outcome.

Current measurements do **not** establish:

* fewer agent tokens
* less agent reasoning
* higher coding-task completion
* lower end-to-end agent wall time
* fewer human interventions
* better real-world coding-agent performance

Those claims require measurement on repeated real agent workloads.

`benchmark.sh` is therefore a local falsification harness for deterministic execution and cache behavior, not evidence that KRn is globally optimal or that it reduces model-token consumption.

## What KRn deliberately does not do

KRn does not add these by default:

* vector database or embeddings
* semantic operation router
* autonomous operation promotion
* generalized trajectory synthesis
* daemon or cloud backend
* planner/reviewer/scout agent swarm
* universal repository graph
* workflow engine or custom DSL
* generic long-term AI memory
* telemetry backend

These technologies are not inherently bad.

They are outside KRn until a demonstrated workload shows that adding one improves the relevant efficiency frontier enough to justify its context, runtime, maintenance, and failure-surface cost.

## Current limitation

The most important limitation of deterministic cache reuse is explicit dependency completeness.

KRn can establish:

```text
declared dependency unchanged
```

It cannot establish:

```text
caller declared every dependency that can affect this command
```

Therefore a cache hit proves validity only under KRn's declared-input model.

Likewise, `--verified` records an external assertion. It does not prove semantic equivalence or complete applicability.

KRn deliberately fails open where it cannot establish those properties.

## Related work

Codex and Claude Code are the agents/harnesses that reason and use tools; KRn is a deterministic optimization substrate around that workflow.

Aider's RepoMap uses a different architecture: it builds a ranked symbol map and sends selected portions to the model.

KRn currently reconstructs repository facts using Git/filesystem tools, uses ripgrep for bounded search, and keeps recoverable evidence plus explicit-input cache records instead of maintaining a universal repository map.

* [OpenAI Agents API architecture](https://developers.openai.com/api/docs/guides/agents-api/architecture) distinguishes the harness that runs the model/tool loop from the environment where work executes.
* [Aider repository map documentation](https://aider.chat/docs/repomap.html) describes its symbol map, dependency-graph ranking, and token-budgeted context selection.

## Uninstall

```sh
krn uninstall
```

This removes KRn's managed blocks from the Codex `AGENTS.md` and the Claude Code `CLAUDE.md`.

When invoked from the installed `~/.local/bin/krn`, it also removes that binary.

It does not remove repository-private `.git/krn` records.

## Contributing

Keep changes small, deterministic, inspectable, and falsifiable.

The source tree is organized by feature: each command lives in its own package, so a change to one command only requires reading that package.

```text
cmd/krn/              command dispatch and end-to-end tests of the built binary
internal/workspace/   shared core: repository discovery, .git/krn storage, bounded output, metrics
internal/context/     krn context
internal/find/        krn find
internal/code/        krn code (ast-grep structural edits)
internal/verify/      krn verify
internal/state/       krn state
internal/exec/        krn exec and the explicit-input cache
internal/eval/        krn eval (A/B harness) and krn eval-suite
eval/                 eval-suite task manifest and fixture repository
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
