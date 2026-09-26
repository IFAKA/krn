# KRn with pi and local models

How `krn find`, `krn map`, the pi extension, and its settings work. Results are in [benchmarks.md](benchmarks.md).

[← README](../README.md)

`krn find` takes plain words or identifiers. It splits identifiers (camelCase, snake_case), drops stopwords, weights terms by rarity, prefers definition lines and files that match several terms, and prints at most `--max-files` files (default 5), each with up to three hits: line number, enclosing symbol, and two lines of context. Output stays under `--budget` bytes (default 2400). `--regex` restores the old raw-pattern behaviour.

`krn map` prints a signatures-only repository map for JS/TS/TSX, Python, and Go: tree-sitter definitions and references, a reference graph ranked with personalized PageRank toward `--focus` terms and dirty files, fitted to `--tokens` (default 800). PageRank passes a file's rank to the definitions it references, so a definition whose name contains focus terms also gets a boost from its own file's relevance and is listed above the code it calls. It starts with a short header (the README's first paragraph or the manifest description, and the top-level directory layout) that takes at most a third of the budget and is cut at a word boundary; a line cut down to its bare label is dropped. Tags are cached in `.git/krn/cache/map/` by blob hash.

`integrations/pi/krn.ts` is a [pi](https://github.com/earendil-works/pi) extension. `install.sh` copies it to `~/.pi/agent/extensions/krn/index.ts` when pi is detected; `krn uninstall` removes it. It has three parts. Only the map is on by default; set a variable to `1` to enable a part or `0` to disable it:

```text
first prompt in a Git repo (and no file path in it)
   |
   v
krn map --focus PROMPT --tokens 800
   |  source files ── tree-sitter ──> definitions + references  (tags cached by blob hash)
   |  references between files ────> graph ── personalized PageRank toward prompt terms, dirty files
   |  header (README / manifest, layout) + top signatures, cut to the budget
   v
appended as a message after the system prompt  ──>  model answers or reads/greps further
(system prompt untouched: the KV prefix cache stays valid across turns)

optional: find_code tool (B) ── krn find       bash search output > 6000 bytes (A) ── 40 lines + saved log
```

* `KRN_PI_MAP` (C, default on): on the first prompt of a session, appends `krn map --focus PROMPT` as a message. It never edits the system prompt, so the KV prefix cache stays valid. It is skipped when the prompt already names a file path.
* `KRN_PI_TOOL` (B, default off): registers a `find_code` tool backed by `krn find`.
* `KRN_PI_BOUND` (A, default off): output from grep/find/ls or search commands run in bash, over 6000 bytes, is cut to 40 lines; the full text is saved under `.git/krn/runs/`. An empty search gets `krn find` hits for the same words appended.

Every part fails open outside a Git repository or when `krn` errors. Further variables: `KRN_PI_MAP_TOKENS` (map budget, default 800), `KRN_PI_BOUND_BYTES` (bounding threshold, default 6000), and `KRN_BIN` (the `krn` executable, default `krn` on `PATH`).

`krn eval-pi` runs pi non-interactively (`pi -p --mode json --no-session --no-extensions`, plus `-e` with the extension for KRn variants) on fresh detached clones, once per variant, task, and seed. It grades the final answer against regexes and optional verify commands, and writes `results.jsonl` and `report.md`. Variant T is a baseline without KRn: [`eval/baselines/pi-tree.ts`](../eval/baselines/pi-tree.ts) gives the first prompt a plain `git ls-files` list at the map's byte budget. `krn eval-claude` runs the same manifest through `claude -p --safe-mode` (Haiku 4.5 by default) and adds the variant's text with `--append-system-prompt`; it stops starting runs once the reported cost reaches `--total-budget-usd` (default 10).

Measured results are in [Benchmarks](benchmarks.md). The map alone matched all three parts there, so it is the only default. A plain file list matched the map's correctness but took about 40% longer per correct answer. The hardest task was solved in only 4 of 18 map runs: asked where a rest timer's end time is computed, the model usually cited the first line of `startRest` instead of the assignment three lines below it, and the default constant instead of the per-exercise value. Signatures show where a function starts, not which line inside it does the work; this is a known weakness in precise line citation.

To use it interactively, pi's `models.json` needs the served model and `settings.json` needs it as `defaultModel` (the eval passes `--provider`/`--model` and an isolated `PI_CODING_AGENT_DIR` instead). With `"reasoning": false` and a `contextWindow` no larger than the server's limit, `pi` in any Git repository gets the map on its first prompt. When scripting `pi -p`, redirect stdin (`pi -p '...' </dev/null`): pi reads piped stdin as extra prompt input and waits for it to close. `krn eval-pi` runs pi with stdin on the null device.
