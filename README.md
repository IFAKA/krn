# KRn

**Make Codex repeat less deterministic work.**

KRn is a research-informed deterministic optimization substrate for Codex. It reconstructs repository facts, bounds retrieved evidence, verifies work, records irreducible state and execution evidence, and safely reuses deterministic computation when explicit dependencies establish that reuse remains valid.

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

`install.sh` requires Go 1.24+, builds KRn, installs it at `~/.local/bin/krn`, and adds a small managed routing policy to `$CODEX_HOME/AGENTS.md` or `~/.codex/AGENTS.md`. If `krn` is not on `PATH`, add `~/.local/bin` to it.

## Use

```sh
cd any-git-project
codex
```

There is no `krn init`. The integration is global; the commands operate on the Git repository containing the current directory.

## Why KRn?

A model context window is expensive working memory. Repository facts, command output, and repeated deterministic computation should not need to enter that working memory in full when Git, the filesystem, ripgrep, tests, or a valid cached result can establish the required evidence directly.

KRn therefore tries to move deterministic work downward:

```text
Codex reasoning
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

This does not make Codex passive and does not establish that KRn reduces model tokens or reasoning.

KRn provides small, recoverable interfaces for deterministic work that can be reconstructed, measured, verified, or safely reused.

## How it works

```text
TASK                         user asks Codex to do work
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
DETERMINISTIC WORK       CODEX REASONING
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
 VERIFIED REUSE               reuse only if command and inputs still match
       |
       v
SUCCESSFUL EXACT EXECUTIONS   repeated verified executions accumulate
       |
       v
     COMPILE                  find exact repeated patterns
       |
       v
REVIEW CANDIDATE ONLY         suggest inspection, never auto-promote
```

The main path is conservative: KRn reconstructs what it can, gathers bounded evidence, lets deterministic tools and Codex reasoning meet at verification, and records only verified work. The reuse path is narrower: cached results are reused only while their explicit dependencies still match, and repeated exact executions become review candidates rather than automatic abstractions.

* `context` reconstructs Git root, branch, commit, changed paths, detected ecosystems, and saved task state.
* `find` uses ripgrep, returns a bounded file/snippet projection, and saves the full search output for recovery.
* `verify` discovers safe project-native checks from `.kern/config.json`, `package.json`, Go, Cargo, or pytest. Unknown projects remain `unknown`; KRn does not invent a command.
* `exec` runs structured local commands, bounds output shown to the caller, records metrics, and can cache executions only when explicit input dependencies are supplied.
* Cached executions are reused only when their command, repository provenance, schema, declared dependencies, dependency fingerprints, metadata, and result integrity remain valid.
* `compile` reads successful trajectories and emits review-only candidates for repeated exact commands with explicit dependencies. It never infers semantic equivalence, parameterizes commands, promotes operations, or executes candidates automatically.

The implementation fails open around uncertain reconstruction or reuse: unavailable, malformed, stale, tampered, or mismatched evidence is rejected rather than treated as valid.

## Memory model

Codex context is working memory, not KRn's persistent memory.

```text
Codex context             = expensive working memory
repository + Git          = reconstructable source-of-truth memory
.git/krn/state.json       = irreducible semantic/task memory
.git/krn/cache/           = reusable deterministic computation
.git/krn/runs/            = recoverable command/search/verification evidence
.git/krn/metrics.jsonl    = local execution measurements
AGENTS.md                 = policy/instructions, not project memory
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

The Codex integration is intentionally instruction-based. It does not install prompt hooks, mutate Codex state databases, or force `krn context` on every task. Codex is instructed to use KRn when it is likely to reduce context, repeated exploration, or unreconstructable reasoning: `context` for repository orientation and saved task state, `find` before broad file reading, `verify` when discovered checks fit, `exec` only for deterministic repeated commands with explicit dependencies, `state` only for irreducible durable facts, and `compile` only to review repeated verified trajectories.

Codex should skip KRn for trivial answers, single known-file edits, direct user-specified commands, or when a normal tool call is cheaper than consulting KRn. This keeps the default integration small and fail-open while still making automatic use operationally clear.

`--cache` requires at least one `--input`.

The cache key includes the exact argv, repository root, cache schema, canonicalized dependencies, and content fingerprints of every declared input.

A changed declared input, command, repository root, malformed record, stale schema, tampered result, or provenance mismatch produces a cache miss.

Unknown side effects are never inferred safe. Callers must declare the complete inputs that make a command reusable.

`--verified` is an external assertion recorded for review candidates. It is not proof of command purity, complete dependencies, semantic equivalence, or generalized applicability.

## Storage and privacy

KRn uses local files and existing repository tools. It has no daemon, cloud backend, network service, or telemetry service.

The installer writes only to `~/.local/bin/krn` and the managed Codex instruction block.

Repository-private data is stored under the Git directory:

```text
<repo>/.git/krn/state.json
<repo>/.git/krn/metrics.jsonl
<repo>/.git/krn/cache/<content-key>.json
<repo>/.git/krn/runs/<timestamp>-<operation>.log
<repo>/.kern/config.json                 optional team-owned checks
```

Logs and state are local and may contain command output or task text.

Metrics record measurements available to KRn. Codex token counts are recorded as `unavailable` when they were not supplied.

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
| Do not infer semantic abstraction from repetition alone | Repeated shell commands do not establish semantic equivalence or safe parameterization. KRn therefore reports exact repetition only as a review signal.      | **[DreamCoder: Bootstrapping Inductive Program Synthesis with Wake-Sleep Library Learning — Kevin Ellis et al., 2021, PLDI](https://doi.org/10.1145/3453483.3454080)** — Studies library learning inside a defined synthesis language and domain. It does not establish that arbitrary Codex execution trajectories can be safely generalized from repetition alone.                   |
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
* marker-based idempotent Codex integration
* exact-command review candidates
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

* fewer Codex tokens
* less Codex reasoning
* higher coding-task completion
* lower end-to-end agent wall time
* fewer human interventions
* better real-world coding-agent performance

Those claims require measurement on repeated real Codex workloads.

`benchmark.sh` is therefore a local falsification harness for deterministic execution and cache behavior, not evidence that KRn is globally optimal or that it reduces model-token consumption.

## `compile`

`compile` is intentionally conservative.

It examines successful recorded executions and may report repeated **exact commands** with explicit dependencies as review candidates.

A candidate means only:

> this exact deterministic execution pattern has occurred repeatedly and may deserve human inspection.

It does **not** mean the executions are semantically equivalent beyond what their recorded structure establishes.

`compile` does not:

* infer parameterized operations
* infer command purity
* infer undeclared dependencies
* infer semantic equivalence
* create reusable operations
* automatically promote candidates
* automatically execute candidates
* use an LLM similarity decision

Repetition is evidence of repetition, not proof of abstraction.

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

Codex is the agent/harness that reasons and uses tools; KRn is a deterministic optimization substrate around that workflow.

Aider's RepoMap uses a different architecture: it builds a ranked symbol map and sends selected portions to the model.

KRn currently reconstructs repository facts using Git/filesystem tools, uses ripgrep for bounded search, and keeps recoverable evidence plus explicit-input cache records instead of maintaining a universal repository map.

* [OpenAI Agents API architecture](https://developers.openai.com/api/docs/guides/agents-api/architecture) distinguishes the harness that runs the model/tool loop from the environment where work executes.
* [Aider repository map documentation](https://aider.chat/docs/repomap.html) describes its symbol map, dependency-graph ranking, and token-budgeted context selection.

## Uninstall

```sh
krn uninstall
```

This removes KRn's managed block from the Codex `AGENTS.md`.

When invoked from the installed `~/.local/bin/krn`, it also removes that binary.

It does not remove repository-private `.git/krn` records.

## Contributing

Keep changes small, deterministic, inspectable, and falsifiable.

Add or update tests for behavior.

Run the project checks.

Document only capabilities that source, tests, measurements, or explicitly cited external evidence establish.

Do not convert architectural hypotheses into product claims.

Additional complexity requires a reproducible workload showing that it improves the relevant frontier.

## License

[MIT](LICENSE)
