Claude Code 2.1.283 with claude-haiku-4-5-20251001, `claude -p --safe-mode` (no user CLAUDE.md, hooks, plugins, or MCP), 2026-09-26.
Variants: none; policy (the routing policy `krn integrate claude` installs, appended to the system prompt, krn on PATH);
map (`krn map --tokens 800 --focus PROMPT` appended); tree (`git ls-files` at the same byte budget appended). 3 seeds.
Cost is Claude Code's reported total_cost_usd at list price.

    krn eval-claude --model claude-haiku-4-5-20251001 --variants none,policy,map,tree --seeds 3
