# KRn

**Make Codex do less thinking.**

KRn is a research-informed deterministic optimization substrate for Codex. It reconstructs, filters, verifies, records, caches, and reuses work so recurring computation does not repeatedly consume model context and reasoning.

Codex remains the agent. KRn is the layer beneath and around it:

```text
GPT/model
    ↓
  Codex        agent / harness
    ↓
   KRn         deterministic optimization substrate
    ↓
repository + existing tools
```

The engineering thesis is: **reason once, verify, and compile recurring cognition downward until deterministic computation can replace it.** This is KRn's thesis, not a quotation from any research paper and not a claim that KRn is globally optimal.

## Install

The repository currently ships a source-based installer, not a release binary or a verified public `curl | sh` endpoint.

```sh
git clone https://github.com/IFAKA/krn.git
cd krn
./install.sh
```

`install.sh` requires Go 1.24+, builds KRn, installs it at `~/.local/bin/krn`, and adds a managed block to `$CODEX_HOME/AGENTS.md` or `~/.codex/AGENTS.md`. If `krn` is not on `PATH`, add `~/.local/bin` to it.

## Use

```sh
cd any-git-project
codex
```

There is no `krn init`. The integration is global; the commands operate on the Git repository containing the current directory.

## Why KRn?

An agent's context window is expensive working memory. Repository facts, command output, and repeated exploration should not enter that working memory in full when Git, the filesystem, ripgrep, tests, or a prior verified result can establish them deterministically.

KRn therefore tries to move work downward:

```text
Codex reasoning
      ↓
minimum evidence
      ↓
reusable abstraction
      ↓
deterministic operation
      ↓
dependency-aware cache
      ↓
    reuse
      ↓
   nothing
```

This does not make Codex passive or guarantee fewer tokens. It gives Codex small, recoverable interfaces for work that can be measured and repeated.

## How it works

```text
                         TASK
                           │
                           ▼
                    RECONSTRUCT
                           │
                           ▼
                  MINIMUM EVIDENCE
                    │             │
                    ▼             ▼
             DETERMINISTIC     CODEX
                WORK          REASONING
                    │             │
                    └──────┬──────┘
                           ▼
                         VERIFY
                           │
                           ▼
                         RECORD
                    │             │
                    ▼             ▼
                  CACHE        ABSTRACT
                                  │
                                  ▼
                           REVIEW CANDIDATE
```

- `context` reconstructs Git root, branch, commit, changed paths, detected ecosystems, and saved task state.
- `find` uses ripgrep, returns a bounded file/snippet projection, and saves the full search output for recovery.
- `verify` discovers safe project-native checks from `.kern/config.json`, `package.json`, Go, Cargo, or pytest. Unknown projects remain `unknown`; KRn does not invent a command.
- `exec` runs structured local commands, bounds output shown to the caller, records metrics, and can cache only executions with explicit input dependencies.
- `compile` reads successful trajectories and emits conservative candidates for repeated verified commands or a varying final scope parameter. It never promotes or executes candidates automatically.

The implementation is fail-open around uncertain reconstruction: it reports unavailable or unknown evidence rather than manufacturing a result.

## Memory model

Codex context is working memory, not KRn's persistent memory.

```text
Codex context             = expensive working memory
repository + Git          = reconstructable source-of-truth memory
.git/krn/state.json       = irreducible semantic/task memory
.git/krn/cache/           = computational memory
.git/krn/runs/            = recoverable command/search/verification evidence
.git/krn/metrics.jsonl    = local experience and measurement records
AGENTS.md                 = policy/instructions, not project memory
```

The governing rule is: **if KRn can cheaply reconstruct something from an authoritative source, it should not remember a duplicate.** State contains only fields such as objective, constraints, proven facts, open questions, and negative results. Cache records include command arguments, explicit dependencies, content fingerprints, result, and verification status.

## Commands

```text
krn context [--json]
krn find QUERY [--json] [--max-files N]
krn verify [--level fast|full] [--json]
krn state show
krn state set objective TEXT
krn state add constraint|proven|open|negative TEXT
krn state clear
krn exec [--verified] [--cache --input PATH ...] -- COMMAND ARGS...
krn compile [--min N] [--json]
krn integrate codex|remove-codex
krn doctor
krn uninstall
```

`--cache` requires at least one `--input`. A changed declared input changes the cache key; unknown side effects are never cached implicitly. `--verified` records that an externally verified successful execution may be considered by `compile`.

## Storage and privacy

KRn uses local files and existing repository tools. It has no daemon, cloud backend, network service, or telemetry service. The installer writes only to `~/.local/bin/krn` and the managed Codex instruction block. Repository-private data is stored under the Git directory:

```text
<repo>/.git/krn/state.json
<repo>/.git/krn/metrics.jsonl
<repo>/.git/krn/cache/<content-key>.json
<repo>/.git/krn/runs/<timestamp>-<operation>.log
<repo>/.kern/config.json                 optional team-owned checks
```

Logs and state are local and may contain command output or task text. Metrics record available measurements; Codex token counts are recorded as `unavailable` when they were not supplied. Source files, Git, manifests, tests, compilers, CI, and package managers remain authoritative.

## Research behind the architecture

KRn combines ideas supported by research across context optimization, retrieval, incremental computation, program abstraction, and agent engineering. These sources motivate individual design decisions; they do not prove that KRn's particular combination is globally optimal. KRn therefore treats its architecture as falsifiable and measures its own behavior.

| KRn decision or feature | Why | Evidence |
| --- | --- | --- |
| Bounded context and recoverable projections | Retrieval quality is not the same as dumping more context into the model; KRn shows a small projection while retaining a local full log. | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Evaluates 1,136 tasks across 66 repositories and reports recall/precision gaps plus a gap between explored and used context, motivating bounded evidence before Codex reasoning. |
| Deterministic retrieval before model reasoning | Search and repository inspection should be paid for by ordinary tools when they can recover the fact reliably. | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Reports that evaluated language models favor recall over precision, motivating a bounded `find` projection rather than indiscriminate output. |
| Do not assume one retrieval family is universally best | KRn uses ripgrep and explicit repository facts by default; it does not install embeddings, a vector database, or a repository map without workload evidence. | **[Agent Retrieval Bench: Evaluating Repository Context Retrieval for Coding Agents — Bowen Qin and Yi Xie, 2026, arXiv preprint](https://arxiv.org/abs/2607.24882)** — On 427 samples from 25 repositories, lexical, RepoMap, embedding, and agent-context methods win on different metrics and tasks; no family dominates. |
| Context externalization and compression | Long trajectories make irrelevant history expensive; KRn stores full evidence locally and exposes bounded projections. | **[ACON: Optimizing Context Compression for Long-horizon LLM Agents — Minki Kang et al., 2026, Lifelong Agent @ ICLR 2026 workshop](https://openreview.net/forum?id=x0alNh5o8v)** — Reports 26–54% lower peak tokens on AppWorld, OfficeBench, and Multi-objective QA while largely preserving task performance; KRn does not claim those results for itself. |
| Dependency-aware caching and invalidation | Recompute only when declared inputs change, while keeping dependency structure explicit. | **[Build Systems à la Carte — Andrey Mokhov, Neil Mitchell, and Simon Peyton Jones, 2018, ICFP](https://doi.org/10.1145/3236774)** — Separates dependency and rebuild decisions and analyzes persistent build information, motivating explicit input fingerprints and conservative cache reuse. |
| Conservative reusable abstractions | Repeated verified operations are candidates for parameterization, but generalization must remain reviewable. | **[DreamCoder: Bootstrapping Inductive Program Synthesis with Wake-Sleep Library Learning — Kevin Ellis et al., 2021, PLDI](https://doi.org/10.1145/3453483.3454080)** — Shows library learning can capture recurring program structure and improve later synthesis; it does not show that KRn can generalize arbitrary enterprise Codex trajectories. |
| Minimal scaffolding | Additional agent machinery is not automatically an improvement, so KRn keeps the default substrate small and local. | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Finds sophisticated scaffolding gives only marginal retrieval gains in its evaluated setting, supporting a measured-complexity policy rather than a universal claim against scaffolding. |

## Evidence status

### Research-supported principles

The sources above support bounded context, careful retrieval evaluation, context compression, explicit dependency structure, and reusable abstractions as useful design directions in their evaluated settings.

### Implemented KRn mechanisms

The source and tests establish bounded projections, local recoverable logs, fail-open verification discovery, explicit-input content-keyed caching, JSONL metrics, marker-based idempotent Codex integration, and conservative compiler candidates. `go test ./...` is the executable check for these local behaviors.

### KRn-specific hypotheses

KRn's central hypothesis remains unproven until a reproducible benchmark covers repeated real coding work:

> Moving recurring verified operations from model cognition into deterministic or reusable computation will reduce cognition and context required per verified useful outcome.

`benchmark.sh` is a small local falsification harness for observable command executions and cache reuse. It is not evidence that KRn is optimal, nor evidence of a particular token reduction.

## What KRn deliberately does not do

KRn does not add these by default:

- vector database or embeddings
- daemon or cloud backend
- planner/reviewer/scout agent swarm
- universal repository graph
- workflow engine or custom DSL

These technologies are not inherently bad. KRn admits complexity only after a measured workload demonstrates that it improves the relevant frontier.

## Related work

Codex is the agent/harness that reasons and uses tools; KRn is a deterministic optimization substrate around that workflow. Aider's RepoMap is a different design: it builds a ranked symbol map and sends selected portions of it to the model. KRn currently reconstructs repository facts with Git/filesystem tools, uses ripgrep for bounded search, and keeps local evidence and explicit-input cache records instead of maintaining a universal repository map.

- [OpenAI Agents API architecture](https://developers.openai.com/api/docs/guides/agents-api/architecture) distinguishes the harness that runs the model/tool loop from the environment where work executes, matching KRn's substrate positioning.
- [Aider repository map documentation](https://aider.chat/docs/repomap.html) describes its symbol map, dependency graph ranking, and token-budgeted context selection.

## Uninstall

```sh
krn uninstall
```

This removes KRn's managed block from the Codex `AGENTS.md`. When invoked from the installed `~/.local/bin/krn`, it also removes that binary. It does not remove repository-private `.git/krn` records.

## Contributing

Keep changes small, deterministic, and inspectable. Add or update tests for behavior, run the project checks, and document only capabilities that the source and tests establish. Benchmark claims must include a reproducible workload.

## License

[MIT](LICENSE)
