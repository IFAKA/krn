package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type evalMetric struct {
	VerifiedCompletion any    `json:"verified_completion"`
	CodexTokens        any    `json:"codex_tokens"`
	PeakCodexContext   any    `json:"peak_codex_context"`
	WallTimeMS         int64  `json:"wall_time_ms"`
	ToolExecutions     int    `json:"tool_executions"`
	HumanInterventions int    `json:"human_interventions"`
	CodexExitCode      int    `json:"codex_exit_code"`
	VerificationExit   int    `json:"verification_exit_code"`
	Verification       string `json:"verification"`
}

type evalReport struct {
	Schema       string                `json:"schema"`
	Created      string                `json:"created"`
	SourceHead   string                `json:"source_head"`
	Task         string                `json:"task"`
	Verification string                `json:"verification_command"`
	Model        string                `json:"model"`
	Effort       string                `json:"reasoning_effort"`
	Runs         map[string]evalMetric `json:"runs"`
	Deltas       map[string]any        `json:"deltas"`
	EvidenceRoot string                `json:"evidence_root"`
}

type evalTelemetry struct {
	Tokens        int64
	TokensKnown   bool
	PeakContext   int64
	ContextKnown  bool
	Tools         int
	Interventions int
}

func evalCmd(args []string) error {
	f := flagSet("eval")
	taskPath := f.String("task", "", "path to the identical task prompt")
	verifyCommand := f.String("verify", "", "objective verification command")
	model := f.String("model", "", "Codex model (same for A and B)")
	effort := f.String("reasoning-effort", "", "Codex reasoning effort (same for A and B)")
	codexPath := f.String("codex", "codex", "Codex executable")
	output := f.String("output", "", "evidence directory")
	js := f.Bool("json", false, "print report JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *taskPath == "" || *verifyCommand == "" || *model == "" || *effort == "" {
		return errors.New("eval requires --task PATH, --verify COMMAND, --model MODEL, and --reasoning-effort EFFORT")
	}
	taskBytes, err := os.ReadFile(*taskPath)
	if err != nil {
		return err
	}
	r, err := discover()
	if err != nil {
		return err
	}
	head, err := git(r.Root, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve source HEAD: %w", err)
	}
	base, err := os.MkdirTemp("", "krn-eval-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(base)
	evidence := *output
	if evidence == "" {
		evidence = filepath.Join(r.Private, "evals", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		return err
	}
	report := evalReport{Schema: "1", Created: time.Now().UTC().Format(time.RFC3339Nano), SourceHead: strings.TrimSpace(head), Task: string(taskBytes), Verification: *verifyCommand, Model: *model, Effort: *effort, Runs: map[string]evalMetric{}, EvidenceRoot: evidence}
	for _, variant := range []string{"A-codex-alone", "B-codex-plus-krn"} {
		runDir := filepath.Join(base, variant)
		if err := cloneAtHead(r.Root, runDir); err != nil {
			return err
		}
		runEvidence := filepath.Join(evidence, variant)
		if err := os.MkdirAll(runEvidence, 0700); err != nil {
			return err
		}
		m, err := runEvalVariant(evalVariant{Variant: variant, Repo: runDir, Evidence: runEvidence, Task: string(taskBytes), Verify: *verifyCommand, Model: *model, Effort: *effort, Codex: *codexPath})
		if err != nil {
			return err
		}
		report.Runs[variant] = m
	}
	report.Deltas = evalDeltas(report.Runs["A-codex-alone"], report.Runs["B-codex-plus-krn"])
	if err := writeJSON(filepath.Join(evidence, "report.json"), report); err != nil {
		return err
	}
	if *js {
		return jsonPrint(report)
	}
	fmt.Printf("evaluation evidence: %s\n", evidence)
	for _, variant := range []string{"A-codex-alone", "B-codex-plus-krn"} {
		fmt.Printf("%s: %+v\n", variant, report.Runs[variant])
	}
	return nil
}

// flagSet keeps eval's flag parsing independent from the older command helpers.
func flagSet(name string) *flag.FlagSet { return flag.NewFlagSet(name, flag.ContinueOnError) }

func cloneAtHead(source, destination string) error {
	cmd := exec.Command("git", "clone", "--quiet", "--no-hardlinks", source, destination)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone %s: %w: %s", source, err, strings.TrimSpace(string(b)))
	}
	return nil
}

type evalVariant struct{ Variant, Repo, Evidence, Task, Verify, Model, Effort, Codex string }

func runEvalVariant(v evalVariant) (evalMetric, error) {
	_ = os.WriteFile(filepath.Join(v.Evidence, "prompt.txt"), []byte(v.Task), 0600)
	meta := map[string]any{"variant": v.Variant, "repo": v.Repo, "task": v.Task, "verification": v.Verify, "model": v.Model, "reasoning_effort": v.Effort, "started": time.Now().UTC().Format(time.RFC3339Nano)}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(v.Evidence, "metadata.json"), append(metaBytes, '\n'), 0600)
	cmdArgs := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--sandbox", "workspace-write", "--approve-for-me", "-C", v.Repo}
	if v.Model != "" {
		cmdArgs = append(cmdArgs, "--model", v.Model)
	}
	if v.Effort != "" {
		cmdArgs = append(cmdArgs, "-c", `model_reasoning_effort="`+v.Effort+`"`)
	}
	cmdArgs = append(cmdArgs, v.Task)
	cmd := exec.Command(v.Codex, cmdArgs...)
	cmd.Dir = v.Repo
	if v.Variant == "B-codex-plus-krn" {
		codexHome := filepath.Join(v.Evidence, "codex-home")
		if err := os.MkdirAll(codexHome, 0700); err != nil {
			return evalMetric{}, err
		}
		if err := os.WriteFile(filepath.Join(codexHome, "AGENTS.md"), []byte(codexText+"\n"), 0600); err != nil {
			return evalMetric{}, err
		}
		cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	} else {
		cmd.Env = append(os.Environ(), "CODEX_HOME="+filepath.Join(v.Evidence, "codex-home"))
	}
	start := time.Now()
	var stdout, stderrBuffer strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderrBuffer

	runErr := cmd.Run()
	out := []byte(stdout.String())
	wall := time.Since(start).Milliseconds()
	stderr := []byte(stderrBuffer.String())
	_ = os.WriteFile(filepath.Join(v.Evidence, "codex.stdout.jsonl"), out, 0600)
	_ = os.WriteFile(filepath.Join(v.Evidence, "codex.stderr.log"), stderr, 0600)
	telemetry := parseEvalTelemetry(out)
	verification, verificationCode := runVerification(v.Repo, v.Verify, v.Evidence)
	saveEvalWorkingTree(v.Repo, v.Evidence)
	codexCode := 0
	if runErr != nil {
		codexCode = 1
		if ee, ok := runErr.(*exec.ExitError); ok {
			codexCode = ee.ExitCode()
		}
	}
	m := evalMetric{VerifiedCompletion: verificationCode == 0 && codexCode == 0, CodexTokens: unavailableInt(telemetry.Tokens, telemetry.TokensKnown), PeakCodexContext: unavailableInt(telemetry.PeakContext, telemetry.ContextKnown), WallTimeMS: wall, ToolExecutions: telemetry.Tools, HumanInterventions: telemetry.Interventions, CodexExitCode: codexCode, VerificationExit: verificationCode, Verification: verification}
	return m, nil
}

func saveEvalWorkingTree(repo, evidence string) {
	status := exec.Command("git", "status", "--short")
	status.Dir = repo
	if b, err := status.CombinedOutput(); err == nil {
		_ = os.WriteFile(filepath.Join(evidence, "git-status.txt"), b, 0600)
	}
	diff := exec.Command("git", "diff", "--binary", "HEAD")
	diff.Dir = repo
	if b, err := diff.CombinedOutput(); err == nil {
		_ = os.WriteFile(filepath.Join(evidence, "working-tree.patch"), b, 0600)
	}
}

func runVerification(repo, command, evidence string) (string, int) {
	c := exec.Command("sh", "-c", command)
	c.Dir = repo
	b, err := c.CombinedOutput()
	_ = os.WriteFile(filepath.Join(evidence, "verification.log"), b, 0600)
	if err == nil {
		return "passed", 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return "failed", ee.ExitCode()
	}
	return "failed", 1
}

func unavailableInt(n int64, known bool) any {
	if known {
		return n
	}
	return "unavailable"
}

func parseEvalTelemetry(data []byte) evalTelemetry {
	var t evalTelemetry
	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		var value any
		if json.Unmarshal(s.Bytes(), &value) != nil {
			continue
		}
		walkEvalJSON(value, &t, "")
	}
	return t
}

func walkEvalJSON(value any, t *evalTelemetry, parent string) {
	switch x := value.(type) {
	case map[string]any:
		for k, v := range x {
			key := strings.ToLower(k)
			walkEvalJSON(v, t, key)
			if n, ok := numberValue(v); ok {
				switch key {
				case "total_tokens":
					t.Tokens, t.TokensKnown = n, true
				case "input_tokens":
					if !t.TokensKnown {
						t.Tokens += n
					}
				case "output_tokens":
					if !t.TokensKnown {
						t.Tokens += n
					}
				case "peak_context_tokens", "context_used_tokens", "context_tokens":
					if n > t.PeakContext {
						t.PeakContext, t.ContextKnown = n, true
					}
				}
			}
		}
		if typ, ok := x["type"].(string); ok {
			lower := strings.ToLower(typ)
			if strings.Contains(lower, "tool") || strings.Contains(lower, "command_execution") || strings.Contains(lower, "function_call") {
				t.Tools++
			}
			if strings.Contains(lower, "approval") || strings.Contains(lower, "intervention") || strings.Contains(lower, "user_input") {
				t.Interventions++
			}
		}
	case []any:
		for _, v := range x {
			walkEvalJSON(v, t, parent)
		}
	}
}

func numberValue(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		i, e := n.Int64()
		return i, e == nil
	case string:
		i, e := strconv.ParseInt(n, 10, 64)
		return i, e == nil
	}
	return 0, false
}

func evalDeltas(a, b evalMetric) map[string]any {
	out := map[string]any{"verified_completion": boolDelta(a.VerifiedCompletion, b.VerifiedCompletion), "wall_time_ms": b.WallTimeMS - a.WallTimeMS, "tool_executions": b.ToolExecutions - a.ToolExecutions, "human_interventions": b.HumanInterventions - a.HumanInterventions}
	out["codex_tokens"] = numericDelta(a.CodexTokens, b.CodexTokens)
	out["peak_codex_context"] = numericDelta(a.PeakCodexContext, b.PeakCodexContext)
	return out
}

func boolDelta(a, b any) string {
	av, aok := a.(bool)
	bv, bok := b.(bool)
	if !aok || !bok {
		return "unavailable"
	}
	if av == bv {
		return "unchanged"
	}
	if bv {
		return "improved"
	}
	return "regressed"
}
func numericDelta(a, b any) any {
	an, aok := numberValue(a)
	bn, bok := numberValue(b)
	if !aok || !bok {
		return "unavailable"
	}
	return bn - an
}
