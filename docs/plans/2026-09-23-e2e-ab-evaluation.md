# End-to-End A/B Evaluation Harness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a separate, fixture-driven A/B harness that compares Codex alone with Codex plus the current KRn instructions while preserving raw evidence and reporting only available metrics.

**Architecture:** Add a `krn eval` command that snapshots the current committed repository into two fresh temporary run directories, invokes the same Codex CLI configuration for A and B, verifies each resulting checkout with the same explicit command, and writes a JSON report plus per-run prompt/metadata/stdout/stderr/verification artifacts. Pure helpers parse Codex JSONL usage events and compute report deltas; no KRn behavior or `benchmark.sh` behavior changes.

**Tech Stack:** Go standard library, existing `krn` CLI, Codex CLI JSONL output, shell-compatible verification commands.

---

### Task 1: Define the evaluation contract and failing tests

**Files:**
- Create: `eval_test.go`
- Modify: `main_test.go` only if shared test helpers are needed

**Steps:**

1. Add tests for a fake source repository being copied to independent A/B run directories, including reset after one run mutates a file.
2. Add tests that parse synthetic Codex JSONL usage events, preserve unavailable fields, and count tool/execution events and interventions deterministically.
3. Add tests for verified completion and A/B delta calculations, including unavailable metric deltas.
4. Run `go test ./...` and confirm the new tests fail because the evaluation helpers do not exist.

### Task 2: Implement isolated execution and report aggregation

**Files:**
- Create: `eval.go`
- Modify: `main.go`

**Steps:**

1. Register `krn eval` and validate required task/verification inputs.
2. Snapshot `HEAD` into a temporary evaluation directory and create independent A/B checkouts from that exact snapshot.
3. Run the same model, reasoning-effort, sandbox, and prompt task for both variants; inject only the current KRn routing policy into B through an isolated `CODEX_HOME`/`AGENTS.md`.
4. Capture raw prompt, metadata, Codex stdout JSONL, stderr, and verification output for each run.
5. Run the same objective verification command after Codex exits, calculate completion, wall time, tool/execution count, intervention count, token usage, and peak context when present, and otherwise emit `unavailable`.
6. Emit a machine-readable report containing A, B, and deltas without changing KRn runtime behavior.

### Task 3: Document usage and metric limitations

**Files:**
- Modify: `README.md`

**Steps:**

1. Document the separate `krn eval` workflow, required task/verification inputs, reset guarantee, and raw evidence location.
2. Explicitly distinguish measured metrics from unavailable metrics and explain that the existing `benchmark.sh` remains the deterministic KRn mechanism benchmark.

### Task 4: Verify and run what is possible

**Steps:**

1. Run focused evaluation tests.
2. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, `sh -n benchmark.sh`, and `./benchmark.sh`.
3. Build the CLI and attempt a live Codex evaluation only if the installed CLI can initialize; preserve the failure evidence and report unavailable live results when environment restrictions prevent it.
4. Inspect the final diff and push the completed changes to the configured remote without force-pushing.
