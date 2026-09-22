# KRn Architecture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the unrecoverable binary with a small, fail-open, deterministic KRn implementation that matches the documented storage, context, verification, caching, provenance, and compiler boundaries.

**Architecture:** A standard-library-only Go CLI will reconstruct repository facts from Git/filesystem, project bounded projections, and keep full command evidence under `.git/krn/runs`. Explicit-input execution uses content keyed cache records with provenance; state and trajectory records remain repo-private. The compiler only emits conservative candidates from repeated verified records and never promotes code automatically.

**Tech Stack:** Go 1.24+, standard library, Git, ripgrep when available, project-native commands.

---

### Task 1: Establish a buildable source baseline

**Files:** `go.mod`, `main.go`, `krn`, `install.sh`

Write the CLI dispatcher and repository discovery helpers, replace the binary with a source launcher, and make installation build a temporary native binary before installing it.

### Task 2: Implement bounded reconstruction, search, state, and integration

**Files:** `internal/*.go`, `main.go`

Implement context reconstruction, bounded ripgrep evidence with recoverable full output, repo-private state, and marker-based idempotent Codex integration/removal.

### Task 3: Implement verification, execution, cache, provenance, and telemetry

**Files:** `internal/*.go`, `main.go`

Add project-native verification discovery, structured process execution, explicit-input cache keys, compact provenance, redacted metrics, and fail-open handling.

### Task 4: Implement conservative compilation and benchmark

**Files:** `internal/compiler.go`, `benchmark.sh`, `tests`

Recognize repeated successful normalized commands, reject semantic mismatches and duplicate primitives, and provide a reproducible local benchmark that measures observable work.

### Task 5: Test architecture and document actual behavior

**Files:** `*_test.go`, `README.md`

Cover storage scope, bounded output/recoverability, cache invalidation, provenance, verification fail-open behavior, lifecycle/idempotence, compiler safety, and malformed/interrupted operations. Update README to describe the implemented behavior.
