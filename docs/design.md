# Design

Memory model, storage, deliberate non-goals, limits, and related work.

[← README](../README.md)

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

## Storage and privacy

KRn uses local files and existing repository tools. It has no daemon, cloud backend, network service, or telemetry service.

The installer writes only to `~/.local/bin/krn`, the managed Codex instruction block, the managed Claude Code instruction block (when Claude Code is detected), the pi extension (when pi is detected), and an ast-grep install when ast-grep is missing (user-local via npm, Cargo, or pip, otherwise Homebrew).

Repository-private KRn data is stored under the Git directory:

```text
<repo>/.git/krn/state.json
<repo>/.git/krn/metrics.jsonl
<repo>/.git/krn/cache/<content-key>.json
<repo>/.git/krn/cache/map/               tree-sitter tags for krn map, by blob hash
<repo>/.git/krn/runs/<timestamp>-<operation>.log
```

The optional team-owned verification configuration is stored at the repository root:

```text
<repo>/.kern/config.json
```

Logs and state are local and may contain command output or task text.

Metrics record measurements available to KRn. Model token counts are recorded as `unavailable` when they were not supplied.

Source files, Git, manifests, tests, compilers, CI, and package managers remain authoritative.

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

Aider's RepoMap builds a ranked symbol map and sends selected portions to the model. `krn map` follows the same idea (tree-sitter tags, a reference graph ranked with PageRank, a token budget), is computed on demand rather than maintained, and is delivered once, on pi's first prompt.

Otherwise KRn reconstructs repository facts using Git/filesystem tools, uses ripgrep for bounded search, and keeps recoverable evidence plus explicit-input cache records.

* [OpenAI Agents API architecture](https://developers.openai.com/api/docs/guides/agents-api/architecture) distinguishes the harness that runs the model/tool loop from the environment where work executes.
* [Aider repository map documentation](https://aider.chat/docs/repomap.html) describes its symbol map, dependency-graph ranking, and token-budgeted context selection.
