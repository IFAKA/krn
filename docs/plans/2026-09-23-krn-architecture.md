# KRn Architecture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep KRn a small, fail-open, deterministic substrate with bounded context, recoverable evidence, authoritative verification, explicit-input cache reuse, and no unsupported semantic compilation.

**Architecture:** A standard-library-only Go CLI reconstructs repository facts from Git/filesystem, projects bounded evidence, and keeps full command evidence under `.git/krn/runs`. Explicit-input execution uses schema-versioned, content-keyed cache records with checked provenance and result integrity; state and trajectory records remain repo-private. The compiler emits only exact-command review candidates from repeated records with explicit dependencies and never promotes or executes them.

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

### Task 4: Implement exact-candidate reporting and benchmark

**Files:** `internal/compiler.go`, `benchmark.sh`, `tests`

Recognize repeated successful exact commands with matching explicit dependencies, reject semantic mismatches and malformed cache records, and provide a reproducible local benchmark that measures observable work.

### Task 5: Test architecture and document actual behavior

**Files:** `*_test.go`, `README.md`

Cover storage scope, bounded output/recoverability, cache invalidation, provenance, verification fail-open behavior, lifecycle/idempotence, compiler safety, and malformed/interrupted operations. Update README to describe the implemented behavior.
