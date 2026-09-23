package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEvaluationClonesResetIndependentRepositories(t *testing.T) {
	source := t.TempDir()
	runTestCommand(t, source, "git", "init", "-q")
	runTestCommand(t, source, "git", "config", "user.email", "eval@example.invalid")
	runTestCommand(t, source, "git", "config", "user.name", "eval")
	if err := os.WriteFile(filepath.Join(source, "state.txt"), []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, source, "git", "add", "state.txt")
	runTestCommand(t, source, "git", "commit", "-qm", "initial")
	a, b := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	if err := cloneAtHead(source, a); err != nil {
		t.Fatal(err)
	}
	if err := cloneAtHead(source, b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "state.txt"), []byte("mutated by A\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(b, "state.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "initial\n" {
		t.Fatalf("B was not reset from the same initial state: %q", got)
	}
	headA := runTestCommand(t, a, "git", "rev-parse", "HEAD")
	headB := runTestCommand(t, b, "git", "rev-parse", "HEAD")
	if headA != headB {
		t.Fatalf("clones have different source heads: %q != %q", headA, headB)
	}
}

func TestEvalTelemetryAndUnavailableMetrics(t *testing.T) {
	data := []byte(`{"type":"item.completed","item":{"type":"command_execution"}}
{"type":"item.completed","item":{"type":"function_call"}}
{"type":"event_msg","payload":{"type":"approval_requested"}}
{"type":"turn.completed","usage":{"total_tokens":123,"peak_context_tokens":77}}`)
	got := parseEvalTelemetry(data)
	if !got.TokensKnown || got.Tokens != 123 || !got.ContextKnown || got.PeakContext != 77 || got.Tools != 2 || got.Interventions != 1 {
		t.Fatalf("unexpected telemetry: %+v", got)
	}
	if got := unavailableInt(0, false); got != "unavailable" {
		t.Fatalf("unavailable metric became %v", got)
	}
}

func TestEvalReportDeltasPreserveUnavailableValues(t *testing.T) {
	a := evalMetric{VerifiedCompletion: false, CodexTokens: "unavailable", PeakCodexContext: int64(40), WallTimeMS: 100, ToolExecutions: 3, HumanInterventions: 0}
	b := evalMetric{VerifiedCompletion: true, CodexTokens: int64(200), PeakCodexContext: "unavailable", WallTimeMS: 150, ToolExecutions: 5, HumanInterventions: 1}
	d := evalDeltas(a, b)
	want := map[string]any{"verified_completion": "improved", "codex_tokens": "unavailable", "peak_codex_context": "unavailable", "wall_time_ms": int64(50), "tool_executions": 2, "human_interventions": 1}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("delta mismatch: got %#v want %#v", d, want)
	}
	if _, err := json.Marshal(evalReport{Schema: "1", Runs: map[string]evalMetric{"A": a, "B": b}, Deltas: d}); err != nil {
		t.Fatal(err)
	}
}

func TestEvalVariantPreservesEvidenceAndVerifiesCompletion(t *testing.T) {
	source := t.TempDir()
	runTestCommand(t, source, "git", "init", "-q")
	runTestCommand(t, source, "git", "config", "user.email", "eval@example.invalid")
	runTestCommand(t, source, "git", "config", "user.name", "eval")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, source, "git", "add", "README")
	runTestCommand(t, source, "git", "commit", "-qm", "initial")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := cloneAtHead(source, repo); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"command_execution\"}}' '{\"type\":\"turn.completed\",\"usage\":{\"total_tokens\":9}}'\nprintf changed > changed.txt\n"
	if err := os.WriteFile(codex, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "evidence")
	if err := os.MkdirAll(evidence, 0700); err != nil {
		t.Fatal(err)
	}
	m, err := runEvalVariant(evalVariant{Variant: "A-codex-alone", Repo: repo, Evidence: evidence, Task: "make the change", Verify: "test -f changed.txt", Codex: codex})
	if err != nil {
		t.Fatal(err)
	}
	if !m.VerifiedCompletion.(bool) || m.CodexTokens != int64(9) || m.ToolExecutions != 1 {
		t.Fatalf("unexpected evaluation metric: %+v", m)
	}
	for _, name := range []string{"prompt.txt", "metadata.json", "codex.stdout.jsonl", "codex.stderr.log", "verification.log"} {
		if _, err := os.Stat(filepath.Join(evidence, name)); err != nil {
			t.Fatalf("missing raw evidence %s: %v", name, err)
		}
	}
	for _, name := range []string{"git-status.txt", "working-tree.patch"} {
		if _, err := os.Stat(filepath.Join(evidence, name)); err != nil {
			t.Fatalf("missing repository evidence %s: %v", name, err)
		}
	}
}

func runTestCommand(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, b)
	}
	return string(trimNewline(b))
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
