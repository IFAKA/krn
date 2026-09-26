# Research and evidence status

The papers behind each design decision, and what is and is not established by KRn's own tests and measurements.

[← README](../README.md)

## Research behind the architecture

KRn combines ideas supported by research across context retrieval, context optimization, and incremental computation.

These sources motivate individual design decisions in the settings they evaluate. They do not establish that KRn's particular combination is globally optimal or that KRn itself reduces model reasoning or token consumption.

KRn therefore treats its architecture as falsifiable and distinguishes research-supported principles from locally measured behavior and unproven hypotheses.

| KRn decision or feature                                 | Why                                                                                                                                                          | Evidence                                                                                                                                                                                                                                                                                                                                                                               |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bounded context and recoverable projections             | Retrieval quality is not the same as dumping more context into the model; KRn shows a small projection while retaining recoverable full evidence.            | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Evaluates 1,136 tasks across 66 repositories and reports recall/precision gaps plus a gap between explored and used context.                                                                                                          |
| Deterministic retrieval before model reasoning          | Repository facts that ordinary tools can establish reliably do not require probabilistic inference.                                                          | **[ContextBench: A Benchmark for Context Retrieval in Coding Agents — Han Li et al., 2026, arXiv preprint](https://arxiv.org/abs/2602.05892)** — Reports recall-over-precision behavior in evaluated models, motivating bounded retrieval rather than indiscriminate context accumulation.                                                                                             |
| Do not assume one retrieval family is universally best  | KRn uses ripgrep and explicit repository facts rather than embeddings or a vector database; it added a repository map only after a local ablation showed a gain. | **[Agent Retrieval Bench: Evaluating Repository Context Retrieval for Coding Agents — Bowen Qin and Yi Xie, 2026, arXiv preprint](https://arxiv.org/abs/2607.24882)** — Evaluates 427 samples from 25 repositories and reports that lexical, RepoMap, embedding, and agent-context approaches perform differently across tasks and metrics; no retrieval family dominates universally. |
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
* ranked natural-language search (`krn find`)
* token-budgeted, focus-ranked repository map with a blob-hash tag cache (`krn map`)
* pi extension install and marker-checked removal
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

It does not establish model-level efficiency. The agent-level measurements are in [Benchmarks](benchmarks.md). With pi and one local model, a first-turn map raised localization correctness from 22% to 81% (pooled 95% intervals 15–32% and 72–88%). A plain file list matched that correctness, and KRn's ranked map reached it about 40% faster. With Claude Code on Haiku 4.5, no variant changed correctness measurably, and the installed routing policy cost about 15% more.

### KRn-specific hypotheses

The broader hypothesis remains unproven:

> Moving deterministic or reconstructable work outside repeated model-driven execution may reduce the cognition or context required per verified useful coding outcome.

Outside the local pi evaluation, current measurements do **not** establish:

* fewer agent tokens
* less agent reasoning
* higher coding-task completion
* lower end-to-end agent wall time
* fewer human interventions
* better real-world coding-agent performance

Those claims require measurement on repeated real agent workloads.

`benchmark.sh` is therefore a local falsification harness for deterministic execution and cache behavior, not evidence that KRn is globally optimal or that it reduces model-token consumption.
