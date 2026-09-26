Run 6: pi with NVIDIA-Nemotron-3.5-Lightning-30B-A3B-oQ4 (oMLX, thinking off), M4 Pro 48 GB, 2026-09-26.
Variants none, C (KRn map), T (plain `git ls-files` list at the map's byte budget, `eval/baselines/pi-tree.ts`), 3 seeds.
The Claude Code run (`../2026-09-26-baselines-claude/`) ran at the same time; it uses the API, not the local GPU.

    krn eval-pi --model NVIDIA-Nemotron-3.5-Lightning-30B-A3B-oQ4 --agent-dir PI_AGENT_DIR --variants none,C,T --seeds 3
