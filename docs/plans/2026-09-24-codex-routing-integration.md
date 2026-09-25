# Codex Routing Integration Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make repository orientation an explicit default KRn routing case while preserving cheap-task bypasses and idempotent managed installation.

**Architecture:** Keep Codex as the agent and KRn as a deterministic CLI substrate. Use the supported global `AGENTS.md` instruction channel because the installed Codex CLI exposes no routing hook; strengthen the policy wording and make source-generated marker replacement remove stale duplicates.

**Tech Stack:** Go standard library, shell installer, Codex CLI JSONL evaluation harness.

---

### Task 1: Establish integration boundary evidence

**Files:** `main.go`, `install.sh`, `main_test.go`, `eval.go`, `README.md`

1. Inspect source and installed `AGENTS.md`.
2. Confirm current Codex CLI instruction discovery and lack of a supported routing hook.
3. Record the installed policy drift and current evaluation capabilities.

### Task 2: Add failing integration regressions

**Files:** `main_test.go`

1. Assert fresh integration has exactly one managed block.
2. Assert stale duplicate blocks are replaced, unrelated content survives, and removal removes only KRn content.
3. Assert the installer and direct integration produce the same generated policy.

### Task 3: Implement the minimal source-of-truth fix

**Files:** `main.go`

1. Rewrite policy as an explicit pre-tool routing decision with mandatory `context` for repository orientation.
2. Preserve explicit bypass cases and fail-open behavior.
3. Replace all well-formed KRn blocks while preserving unrelated content and keeping removal scoped.

### Task 4: Document and verify

**Files:** `README.md`

1. Document actual orientation and bypass semantics.
2. Run focused tests, `go test ./...`, `go test -race ./...`, build/install smoke checks, and applicable evals.
3. Reinstall the checkout and inspect the installed block, `krn doctor`, and Codex profiles without changing configuration.
