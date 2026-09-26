# Benchmarks

Full method, every run (including dropped experiments), pooled intervals, and the cache benchmark. The summary table is in the [README](../README.md#benchmarks); the reasoning behind each decision is in the [investigation log](plans/2026-09-26-pi-lean-investigation.md).

[← README](../README.md)

## Local model with pi: correctness per minute

`krn eval-pi`, 2026-09-26, M4 Pro (48 GB), NVIDIA-Nemotron-3.5-Lightning-30B-A3B-oQ4 served by oMLX (thinking off), pi 0.85.1. 12 tasks × 3 seeds = 36 runs per variant, each on a fresh detached clone:

* 9 localization questions ("where is X computed/stored/decided"), 3 each on a JS PWA, a TypeScript app, and this repository at a pinned commit, graded by regexes over the final answer against ground truth checked by reading the code;
* 3 edit tasks on `eval/fixture`, graded by their verify command.

Variants are parts of the [pi extension](pi.md): A bounds search output, B adds the `find_code` tool, C injects the first-turn map. A and B were later removed from the extension and the harness because they did not help; only `none`, `C` and `T` can be rerun. Within each seed, the variants of a task run back to back in a rotating order, so slow drift on the machine (thermals, cache state) spreads across variants instead of favouring one.

```text
manifest (task, repo@commit, answer regexes or verify command)
   |
   v
for each seed, task, variant ──> fresh detached clone ──> pi -p --mode json [-e krn.ts]
                                                              |
                                  results.jsonl <── grade final answer (regexes) or run verify
                                        |
                                        v
                          report.md: correct, correct/min, wall, turns, tokens, per task
```

| run | variant | correct | correct/min | median wall | mean wall | mean turns | mean uncached input tok | mean output tok |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| 1 | none | 16/36 (44%) | 0.97 | 14.7 s | 27.4 s | 7.4 | 6,115 | 1,036 |
| 1 | A | 16/36 (44%) | 0.87 | 17.2 s | 30.7 s | 8.8 | 6,579 | 1,179 |
| 1 | A+B | 22/36 (61%) | 1.11 | 21.6 s | 33.0 s | 9.8 | 7,420 | 1,193 |
| 1 | A+B+C | 29/36 (81%) | 1.64 | 23.7 s | 29.5 s | 7.9 | 6,968 | 1,069 |
| 2 | C | 31/36 (86%) | 1.61 | 28.7 s | 32.0 s | 7.7 | 8,042 | 1,094 |
| 2 | B+C | 30/36 (83%) | 1.41 | 27.7 s | 35.5 s | 7.6 | 7,893 | 1,117 |
| 3 | none | 15/36 (42%) | 1.00 | 16.3 s | 25.0 s | 6.3 | 6,340 | 856 |
| 3 | C | 29/36 (81%) | 1.50 | 27.6 s | 32.3 s | 7.1 | 7,397 | 981 |
| 4 ✗ | C + coverage note | 29/36 (81%) | 0.99 | 31.9 s | 48.6 s | 10.1 | 8,808 | 1,381 |
| 5 ✗ | C + gated coverage note | 34/36 (94%) | 1.63 | 29.9 s | 34.7 s | 9.1 | 8,485 | 1,161 |
| 6 | none | 14/36 (39%) | 0.94 | 17.7 s | 24.7 s | 6.4 | 5,546 | 881 |
| 6 | C | 32/36 (89%) | 1.50 | 28.1 s | 35.7 s | 8.6 | 8,375 | 1,220 |
| 6 | T (file list, no KRn) | 33/36 (92%) | 1.07 | 45.8 s | 51.2 s | 13.5 | 10,838 | 1,643 |

Run 1 is the ablation. Run 2 repeated it about 40 minutes later with C alone and B+C. Run 3 came after the map began ranking a definition named by the prompt above the code it calls; the benchmark prompts' maps barely changed, so it mostly measures noise. Runs 4 and 5 (✗) tested a line that lists question words the map does not show; neither shipped (see below). Run 6 added a baseline without KRn, T: the first prompt gets `git ls-files` cut to the map's byte budget, delivered the same way ([`eval/baselines/pi-tree.ts`](../eval/baselines/pi-tree.ts)).

Pooled over the three baseline runs (1, 3, 6) and the three map-only runs whose code is on `main` (2, 3, 6), with 95% Wilson intervals:

| | all tasks | localization only | correct/min |
|---|---:|---:|---:|
| none | 45/108 (42%, 33–51%) | 18/81 (22%, 15–32%) | 0.97 |
| C (map) | 92/108 (85%, 77–91%) | 66/81 (81%, 72–88%) | 1.53 |
| T (file list), run 6 only | 33/36 (92%, 78–97%) | 24/27 (89%, 72–96%) | 1.07 |

What this shows, and does not:

* Of KRn's three parts, the map (C) accounts for the gain: C alone matched A+B+C, so only C was kept. The model called `find_code` in about one run in ten when it was offered; bounding (A) alone changed nothing.
* The localization intervals for none and C do not overlap. Without any map, 40 of 54 localization runs in runs 1 and 3 answered after zero tool calls, and all 40 were wrong (invented files or functions); 13 of the other 14 were correct. A map puts real names in context, so the model searches instead of guessing.
* Any map does that. The plain file list (T) scored 33/36 to the ranked map's 32/36, within noise. It took more turns, since a list of paths still has to be opened and read, so the ranked map produced about 40% more correct answers per minute (1.50 to 1.07) and used about 40% fewer input tokens. KRn's ranking buys speed on this device, not correctness.
* Noise is large at this size. The map-only variant scored 31/36 and 29/36 in runs 2 and 3 with nearly identical maps. Run 5 scored 34/36 to run 4's 29/36, although only three tasks' inputs differed between them. Treat any difference of about five correct answers or less as noise.
* The three edit tasks were solved in nearly every variant; the difference is in localization.
* The median run is slower with the map (more correct answers take more turns reading code), but correct answers per minute rise from about 1.0 to about 1.55.
* One model, one machine, 36 runs per variant per run. Two of the three source repositories are private, so the exact manifest is not reproducible elsewhere; the harness is.

Negative result: the coverage note. Most remaining map failures are fast answers taken from the map when it left out the right file. For example, the answer to a question about sync conflicts is `mergeChanges`, but the question never names it. Runs 4 and 5 tested one fix: a last map line naming the question's rarer words that the map did not show, telling the model to search.

* Run 4 always showed the line. Correct stayed at 29/36, but mean wall time rose from 32 s to 49 s, and correct/min fell from 1.50 to 0.99.
* Run 5 showed the line only when most of those words were missing. It scored 34/36, but the line changed on only three tasks, and tasks with byte-identical inputs swung as much as the gain.
* Both versions listed prompt filler ("Answer", "chat") as missing words. Keeping only words that name definitions in the repository removed the filler. After that, the line appeared on one of nine questions, and still did not point at `mergeChanges`.

A map cannot report the omission of code the question never names, so the feature was dropped.

## Claude Code with Haiku 4.5

`krn eval-claude`, same 12 tasks, 3 seeds, Claude Code 2.1.283 with `claude-haiku-4-5-20251001`. Every run uses `claude -p --safe-mode`, so the user's CLAUDE.md, hooks, plugins, and MCP servers are not loaded; each variant adds only its own text to the system prompt. It ran alongside run 6.

| variant | correct | accuracy (95% CI) | correct/min | median wall | mean turns | mean uncached input tok | mean cached input tok | $ per task (list price) | krn calls per task |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| none | 35/36 | 97% (86–100%) | 3.56 | 15.3 s | 5.8 | 7,759 | 88,070 | 0.032 | 0 |
| policy (installed KRn routing policy) | 36/36 | 100% (90–100%) | 3.36 | 16.4 s | 6.0 | 9,540 | 113,602 | 0.037 | 2.0 |
| map (`krn map`, first prompt) | 33/36 | 92% (78–97%) | 4.52 | 11.2 s | 3.7 | 6,406 | 49,362 | 0.023 | 0 |
| tree (file list, no KRn) | 33/36 | 92% (78–97%) | 3.92 | 12.3 s | 5.2 | 7,454 | 70,515 | 0.028 | 0 |

* Vanilla Claude Code on Haiku 4.5 solves nearly all of these tasks, so there is no room to show a correctness gain. Unlike the local model, it searched before answering in every run.
* The policy is what `install.sh` added for Claude Code at the time; after this result it became opt-in (`krn integrate claude`). The model followed it (about two `krn` calls per task), which added turns and about 15% cost without changing correctness.
* The map cut turns by a third and cost by about 30%. Its three misses were all on `wk-rest`: the model read the timer code and stopped before finding `task-factory.js`, which sets the per-exercise rest time. The file-list runs missed the same task the same way, after more searching. With 36 runs, 33 vs 35 is within noise.
* These tasks are too easy to separate the variants on correctness for this model. Harder tasks and larger models remain unmeasured.

Raw results and per-task tables:

* runs 1–2: [`eval/results/2026-09-26-pi-local/`](../eval/results/2026-09-26-pi-local/)
* run 3: [`eval/results/2026-09-26-pi-local-rank/`](../eval/results/2026-09-26-pi-local-rank/)
* run 4: [`eval/results/2026-09-26-pi-local-coverage/`](../eval/results/2026-09-26-pi-local-coverage/)
* run 5: [`eval/results/2026-09-26-pi-local-coverage-gate/`](../eval/results/2026-09-26-pi-local-coverage-gate/)
* run 6 (file-list baseline): [`eval/results/2026-09-26-baselines-pi/`](../eval/results/2026-09-26-baselines-pi/)
* Claude Code: [`eval/results/2026-09-26-baselines-claude/`](../eval/results/2026-09-26-baselines-claude/)

Every run is listed there, including those that did not ship. The reasoning behind each decision, the dropped designs, and open questions are in [`docs/plans/2026-09-26-pi-lean-investigation.md`](plans/2026-09-26-pi-lean-investigation.md). Reproduce with your own manifest:

```sh
krn eval-pi --model MODEL --manifest eval/pi-local-manifest.json --variants none,C,T --seeds 3
krn eval-claude --model claude-haiku-4-5-20251001 --manifest eval/pi-local-manifest.json --seeds 3
```

## Deterministic execution cache

`benchmark.sh` builds KRn, runs `krn exec --cache` in a scratch repository, and checks each case by counting real executions. Output on 2026-09-26:

| case | executions | cumulative cache hits | result |
|---|---:|---:|---|
| unchanged dependency | 1 | 1 | PASS |
| changed declared dependency | 2 | 1 | PASS |
| unrelated file | 2 | 2 | PASS |
| failed execution | 2 | 2 | PASS |
| invalid applicability (different input) | 3 | 5 | PASS |
| malformed/stale cache | 4 | 2 | PASS |
| baseline repeated execution | 2 | 0 | PASS |
| KRn cache hits | 2 | 2 | PASS |
| unchanged workload saved executions | 1 | 1 | PASS |

This proves command reuse under the declared-dependency model (see [Current limitation](design.md#current-limitation)), not model-level savings.

```sh
./benchmark.sh
```
