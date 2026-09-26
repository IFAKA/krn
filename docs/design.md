# Design

Memory model, storage, deliberate non-goals, limits, and related work.

[← README](../README.md)

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

## Source layout and contributing

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
integrations/pi/      pi extension (first-turn map)
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
