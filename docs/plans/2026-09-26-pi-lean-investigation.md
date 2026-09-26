# Investigation log: KRn for pi on a local model, and Claude Code baselines (2026-09-26)

This is the lab notebook for the work that produced `krn find`, `krn map`, the pi extension, `krn eval-pi`, and
`krn eval-claude`. The README and `docs/benchmarks.md` hold the current results; this file keeps the reasoning, the dead ends, and the
open questions so they are not lost with the chat history. Raw data for every run is in `eval/results/2026-09-26-*`.

## Starting point

Before this work KRn added little over grep for a model:

- `krn context` returned only Git metadata.
- `krn find` was a raw regex `rg`, which fails on natural-language queries.
- Integration was about ten rules of routing prose in the system prompt.

Goal: a local pi setup where KRn raises **correct answers per wall-clock minute on this Mac**. Every part must win
an ablation; anything neutral is cut.

## Device facts (measured from the oMLX log, not assumed)

- M4 Pro, 12 cores, 48 GB. Wired-memory limit untouched.
- Only model on disk: NVIDIA-Nemotron-3.5-Lightning-30B-A3B-oQ4 (17 GB), thinking forced off. pi's configured
  default model is not on disk, so every eval passes `--model` explicitly.
- Cold prefill about 450 tok/s (60,487 tokens in 135.6 s); decode 30–55 tok/s. Warm turns with a prefix-cache
  hit take 1–7 s.
- Cost model: about 2.2 ms per uncached input token, about 25 ms per output token, and a changed prompt prefix
  costs a full re-prefill.

Design consequences: keep the prompt prefix byte-stable and append-only; keep bulky tool output out of context;
cut wasted tool rounds.

## Research that bounded the design (papers, 2024–2026)

- Always-on context files do not reliably improve frontier agents (Do Context Files Help?, 2026). A map had to win
  the ablation, not be assumed useful.
- LSP-style semantic tools often cost more tokens than grep for localization, and agents pick grep almost always.
  So KRn should work through grep-shaped habits rather than compete with them.
- Output shape decides adoption: returning source lines raised success and cut file reads.
- Structural ranking helps on multi-file work (RepoGraph, RepoAtlas, "Code Isn't Memory").
- The metric that matters is cache-adjusted cost per solved task, not pass rate alone.

## Design: three switchable parts

| part | what | env toggle |
|---|---|---|
| A | bound oversized grep/find/ls/cat output to a projection plus a recoverable log path | `KRN_PI_BOUND` |
| B | `find_code` tool backed by the rewritten `krn find` (term split, camel/snake variants, ranked by term coverage and definition lines, ±2 lines of context, about 600 tokens) | `KRN_PI_TOOL` |
| C | `krn map`: tree-sitter tags for JS/TS/Python/Go, def→ref graph with personalized PageRank boosted by focus terms, signatures only, 800 tokens, appended to the first prompt (never the system prompt, so the prefix cache holds) | `KRN_PI_MAP` |

Dropped before measuring, as negative or zero value on this device: MCP (13–18k tokens of schemas), per-turn
system-prompt edits (break the KV prefix), `krn context` injection, routing prose for state/exec/verify/code, and a
scout subagent (serial second model call on one GPU).

## Harness

`krn eval-pi` (`internal/eval/pi.go`):

- manifest `eval/pi-local-manifest.json`: 9 localization questions (3 on a JS PWA, 3 on a TypeScript app, 3 on this
  repository at a pinned commit) graded by regexes against ground truth checked by reading the code, plus 3 edit
  tasks on `eval/fixture` graded by their verify command;
- a fresh detached clone per run; variants of a task run back to back in a rotating order per seed so machine
  drift spreads across variants;
- ship rule: keep a part only if it raises correct answers per minute without lowering correctness beyond noise.

## Runs and decisions

| run | variants | result | decision |
|---|---|---|---|
| 1 | none, A, A+B, A+B+C | 16 / 16 / 22 / 29 of 36 | A alone changes nothing; the gain arrives with C |
| 2 | C, B+C | 31 / 30 | C alone matches B+C; `find_code` was called in about one run in ten. **Only C is on by default**; A and B stay behind their toggles |
| — | map header fix | a header line clipped to a bare label (`layout:`) | fixed (`clipHeader`, 821b71e) |
| — | focus ranking | a definition named by the prompt now ranks above the code it calls | shipped (107feab) |
| 3 | none, C | 15 / 29 | confirms the gap; C 31→29 with nearly identical maps shows the noise level (about ±5 of 36) |
| 4 ✗ | C + coverage note (always) | 29, wall 32→49 s | slower, no gain: dropped |
| 5 ✗ | C + gated coverage note | 34, but only 3 tasks' inputs changed | within noise. A definition-name filter removed prompt filler, then the note fired on 1 of 9 prompts and still missed `mergeChanges`. **Dropped**: a map cannot report the omission of code the question never names |
| 6 | none, C, T (plain `git ls-files` at the same byte budget) | 14 / 32 / 33 | any map fixes the "answer without searching" failure; KRn's ranking makes it about 40% faster (1.50 vs 1.07 correct/min, about 40% fewer input tokens) |

Pooled (runs 1, 3, 6 for none; 2, 3, 6 for C): none 45/108 (42%), C 92/108 (85%). On localization alone: 18/81
vs 66/81, with non-overlapping 95% intervals.

Key failure mode without a map: 40 of 54 localization runs in runs 1 and 3 answered after zero tool calls, and all
40 were wrong (invented files or functions).

## Claude Code with Haiku 4.5

`krn eval-claude` (`internal/eval/claude.go`) runs the same manifest through `claude -p --safe-mode`, so the
user's CLAUDE.md, hooks, plugins, and MCP servers are not loaded; each variant adds only its own text via
`--append-system-prompt`. Haiku 4.5 was chosen to keep subscription usage low. Per-run cap $0.50, total cap $10;
actual spend $4.31 at list price.

| variant | correct | $ per task | note |
|---|---:|---:|---|
| none | 35/36 | 0.032 | searched before answering in every run |
| policy (what `install.sh` installs) | 36/36 | 0.037 | about 2 `krn` calls per task; +15% cost, no measurable gain |
| map | 33/36 | 0.023 | a third fewer turns; all 3 misses on `wk-rest` (never found `task-factory.js`) |
| tree | 33/36 | 0.028 | same `wk-rest` misses, after more searching |

These tasks are too easy for Haiku to separate the variants on correctness.

## What shipped

- `krn find`: natural-language ranked search (the old regex behaviour stays behind `--regex`).
- `krn map`: ranked, focus-aware, signature-only repository map with a tag cache in `.git/krn/cache/map/`.
- `integrations/pi/krn.ts`: map on the first prompt by default; bounding and `find_code` optional. Installed by
  `install.sh`, removed by `krn uninstall`.
- `krn eval-pi`, `krn eval-claude`, and the file-list baseline `eval/baselines/pi-tree.ts`.
- README: "At a glance" decision table, every run including the dropped ones, pooled intervals, and noise notes.

## Open questions

1. **Claude Code integration.** The installed routing policy cost more and helped nothing on Haiku. Candidates:
   replace it with the first-prompt map, or keep it until harder tasks show a difference.
2. **Harder tasks.** Real bug fixes and features across files; the three edit tasks are solved by every variant.
3. **Other agents and models.** Codex, Sonnet, Opus: not measured.
4. **Public reproducibility.** Two of the three task repositories are private; a public-repo manifest would let
   others reproduce the numbers.
5. **Other maps.** Aider's repo map and LSP/MCP navigation servers were not run; the file list stands in for them.
