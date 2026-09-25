package eval

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"example.com/krn/internal/integrate"
	"example.com/krn/internal/workspace"
)

type suiteManifest struct {
	Schema      string      `json:"schema"`
	FixtureRoot string      `json:"fixture_root"`
	Ordering    string      `json:"ordering"`
	Tasks       []suiteTask `json:"tasks"`
}

type suiteTask struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Prompt   string `json:"prompt"`
	Verify   string `json:"verify"`
}

type suiteMetric struct {
	VerifiedCompletion  any             `json:"verified_completion"`
	TotalTokens         any             `json:"total_tokens"`
	InputTokens         any             `json:"input_tokens"`
	CachedInputTokens   any             `json:"cached_input_tokens"`
	UncachedInputTokens any             `json:"uncached_input_tokens"`
	OutputTokens        any             `json:"output_tokens"`
	ReasoningTokens     any             `json:"reasoning_output_tokens"`
	WallTimeMS          int64           `json:"wall_time_ms"`
	ToolExecutions      int             `json:"completed_tool_executions"`
	CommandExecutions   int             `json:"completed_command_executions"`
	AgentMessages       int             `json:"agent_messages"`
	FileChanges         int             `json:"file_change_events"`
	HumanInterventions  int             `json:"human_interventions"`
	KRnInvocations      []krnInvocation `json:"krn_invocations"`
	KRnOutputBytes      any             `json:"krn_output_bytes"`
	PerTurnUsage        any             `json:"per_turn_usage"`
	CodexExitCode       int             `json:"codex_exit_code"`
	VerificationExit    int             `json:"verification_exit_code"`
	Verification        string          `json:"verification"`
	Unavailable         []string        `json:"unavailable_metrics,omitempty"`
}

type krnInvocation struct {
	Command     string `json:"command"`
	EventID     string `json:"event_id"`
	Timestamp   string `json:"timestamp"`
	DurationMS  any    `json:"duration_ms"`
	StdoutBytes any    `json:"stdout_bytes"`
	StderrBytes any    `json:"stderr_bytes"`
}

type suiteRun struct {
	TaskID        string      `json:"task_id"`
	Variant       string      `json:"variant"`
	Order         int         `json:"order"`
	FixtureCommit string      `json:"fixture_commit"`
	Metric        suiteMetric `json:"metric"`
}

type suiteReport struct {
	Schema          string            `json:"schema"`
	Created         string            `json:"created"`
	KRnCommit       string            `json:"krn_commit"`
	CodexVersion    string            `json:"codex_version"`
	Model           string            `json:"model"`
	ReasoningEffort string            `json:"reasoning_effort"`
	ManifestSHA256  string            `json:"manifest_sha256"`
	FixtureSHA256   string            `json:"fixture_tree_sha256"`
	Runs            []suiteRun        `json:"runs"`
	Comparisons     []suiteComparison `json:"comparisons"`
	EvidenceRoot    string            `json:"evidence_root"`
	Unavailable     []string          `json:"unavailable_metrics"`
}

type suiteComparison struct {
	TaskID   string         `json:"task_id"`
	Category string         `json:"category"`
	A        suiteMetric    `json:"a"`
	B        suiteMetric    `json:"b"`
	Delta    map[string]any `json:"delta"`
}

func RunSuite(args []string) error {
	f := workspace.FlagSet("eval-suite")
	manifestPath := f.String("manifest", "eval/task-manifest.json", "frozen task manifest")
	output := f.String("output", "", "evidence directory")
	model := f.String("model", "", "Codex model")
	effort := f.String("reasoning-effort", "", "Codex reasoning effort")
	codexPath := f.String("codex", "codex", "Codex executable")
	freezeOnly := f.Bool("freeze-only", false, "persist freeze evidence without running Codex")
	js := f.Bool("json", false, "print report JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if !*freezeOnly && (*model == "" || *effort == "") {
		return errors.New("eval-suite requires --model and --reasoning-effort")
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(r.Root, *manifestPath))
	if err != nil {
		return err
	}
	var manifest suiteManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if len(manifest.Tasks) == 0 {
		return errors.New("manifest has no tasks")
	}
	manifestHash := sha256Hex(manifestBytes)
	fixture := filepath.Join(r.Root, manifest.FixtureRoot)
	fixtureHash, err := fixtureTreeHash(fixture)
	if err != nil {
		return err
	}
	krnCommit, err := workspace.Git(r.Root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	version := commandOutput(*codexPath, "--version")
	evidence := *output
	if evidence == "" {
		evidence = filepath.Join(r.Private, "evals", "gate1-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(evidence, "task-manifest.json"), manifestBytes, 0600); err != nil {
		return err
	}
	meta := map[string]any{"manifest_sha256": manifestHash, "fixture_tree_sha256": fixtureHash, "krn_commit": strings.TrimSpace(krnCommit), "codex_version": version, "model": *model, "reasoning_effort": *effort, "ordering": manifest.Ordering, "created": time.Now().UTC().Format(time.RFC3339Nano)}
	if err := workspace.WriteJSON(filepath.Join(evidence, "freeze-metadata.json"), meta); err != nil {
		return err
	}
	if *freezeOnly {
		fmt.Printf("Gate-1 suite frozen at: %s\nmanifest_sha256: %s\nfixture_tree_sha256: %s\n", evidence, manifestHash, fixtureHash)
		return nil
	}
	report := suiteReport{Schema: "gate1.v1", Created: time.Now().UTC().Format(time.RFC3339Nano), KRnCommit: strings.TrimSpace(krnCommit), CodexVersion: version, Model: *model, ReasoningEffort: *effort, ManifestSHA256: manifestHash, FixtureSHA256: fixtureHash, EvidenceRoot: evidence, Unavailable: []string{"peak_context_tokens", "per_turn_usage_if_not_emitted", "interactive_human_interventions", "krn_duration_and_stream_bytes_if_not_emitted"}}
	for i, task := range manifest.Tasks {
		taskDir := filepath.Join(evidence, task.ID)
		if err := os.MkdirAll(taskDir, 0700); err != nil {
			return err
		}
		runs, err := runSuiteTask(r.Root, fixture, task, i, taskDir, *model, *effort, *codexPath)
		if err != nil {
			return fmt.Errorf("task %s: %w", task.ID, err)
		}
		report.Runs = append(report.Runs, runs...)
		var a, b suiteMetric
		for _, run := range runs {
			if run.Variant == "A-codex-alone" {
				a = run.Metric
			}
			if run.Variant == "B-codex-plus-krn" {
				b = run.Metric
			}
		}
		report.Comparisons = append(report.Comparisons, suiteComparison{TaskID: task.ID, Category: task.Category, A: a, B: b, Delta: suiteMetricDelta(a, b)})
	}
	if err := workspace.WriteJSON(filepath.Join(evidence, "report.json"), report); err != nil {
		return err
	}
	if err := writeSuiteMarkdown(filepath.Join(evidence, "report.md"), report); err != nil {
		return err
	}
	if *js {
		return workspace.PrintJSON(report)
	}
	fmt.Printf("Gate-1 evidence: %s\nmanifest_sha256: %s\nfixture_tree_sha256: %s\n", evidence, manifestHash, fixtureHash)
	return nil
}

func runSuiteTask(sourceRoot, fixture string, task suiteTask, index int, evidence string, model, effort, codexPath string) ([]suiteRun, error) {
	base := filepath.Join(evidence, "repos")
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	order := []string{"A-codex-alone", "B-codex-plus-krn"}
	if index%2 == 1 {
		order[0], order[1] = order[1], order[0]
	}
	var out []suiteRun
	for position, variant := range order {
		repo := filepath.Join(base, variant)
		commit, err := materializeFixture(fixture, repo)
		if err != nil {
			return nil, err
		}
		metric, err := runSuiteVariant(suiteVariant{Variant: variant, Repo: repo, Evidence: filepath.Join(evidence, variant), Task: task, Model: model, Effort: effort, Codex: codexPath, SourceRoot: sourceRoot})
		if err != nil {
			return nil, err
		}
		out = append(out, suiteRun{TaskID: task.ID, Variant: variant, Order: position, FixtureCommit: commit, Metric: metric})
	}
	return out, nil
}

type suiteVariant struct {
	Variant, Repo, Evidence, Model, Effort, Codex, SourceRoot string
	Task                                                      suiteTask
}

func runSuiteVariant(v suiteVariant) (suiteMetric, error) {
	if err := os.MkdirAll(v.Evidence, 0700); err != nil {
		return suiteMetric{}, err
	}
	if err := os.WriteFile(filepath.Join(v.Evidence, "prompt.txt"), []byte(v.Task.Prompt), 0600); err != nil {
		return suiteMetric{}, err
	}
	meta := map[string]any{"variant": v.Variant, "task_id": v.Task.ID, "category": v.Task.Category, "verification": v.Task.Verify, "model": v.Model, "reasoning_effort": v.Effort, "codex_args": evalCodexArgs(v.Repo, v.Model, v.Effort), "started": time.Now().UTC().Format(time.RFC3339Nano)}
	if err := workspace.WriteJSON(filepath.Join(v.Evidence, "metadata.json"), meta); err != nil {
		return suiteMetric{}, err
	}
	home, err := os.MkdirTemp("", "krn-gate1-codex-home-")
	if err != nil {
		return suiteMetric{}, err
	}
	defer os.RemoveAll(home)
	if err := prepareEvalCodexHome(home, os.Getenv("CODEX_HOME")); err != nil {
		return suiteMetric{}, err
	}
	if v.Variant == "B-codex-plus-krn" {
		if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte(integrate.CodexText+"\n"), 0600); err != nil {
			return suiteMetric{}, err
		}
	}
	cmd := exec.Command(v.Codex, evalCodexArgs(v.Repo, v.Model, v.Effort)...)
	cmd.Dir = v.Repo
	cmd.Stdin = strings.NewReader(v.Task.Prompt)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+home)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	wall := time.Since(start).Milliseconds()
	out := []byte(stdout.String())
	_ = os.WriteFile(filepath.Join(v.Evidence, "codex.stdout.jsonl"), out, 0600)
	_ = os.WriteFile(filepath.Join(v.Evidence, "codex.stderr.log"), []byte(stderr.String()), 0600)
	tel := parseDetailedTelemetry(out)
	verification, verificationCode := runVerification(v.Repo, v.Task.Verify, v.Evidence)
	saveEvalWorkingTree(v.Repo, v.Evidence)
	codexCode := exitCode(runErr)
	metric := suiteMetric{VerifiedCompletion: codexCode == 0 && verificationCode == 0, TotalTokens: optionalInt(tel.Total, tel.TotalKnown), InputTokens: optionalInt(tel.Input, tel.InputKnown), CachedInputTokens: optionalInt(tel.CachedInput, tel.CachedKnown), UncachedInputTokens: optionalInt(tel.UncachedInput, tel.UncachedKnown), OutputTokens: optionalInt(tel.Output, tel.OutputKnown), ReasoningTokens: optionalInt(tel.Reasoning, tel.ReasoningKnown), PerTurnUsage: tel.TurnUsages, WallTimeMS: wall, ToolExecutions: tel.ToolExecutions, CommandExecutions: tel.CommandExecutions, AgentMessages: tel.AgentMessages, FileChanges: tel.FileChanges, HumanInterventions: tel.Interventions, KRnInvocations: tel.KRn, KRnOutputBytes: "unavailable", CodexExitCode: codexCode, VerificationExit: verificationCode, Verification: verification, Unavailable: tel.Unavailable()}
	if err := workspace.WriteJSON(filepath.Join(v.Evidence, "timeline.json"), tel.Events); err != nil {
		return suiteMetric{}, err
	}
	if err := workspace.WriteJSON(filepath.Join(v.Evidence, "turn-usage.json"), tel.TurnUsages); err != nil {
		return suiteMetric{}, err
	}
	return metric, nil
}

type detailedTelemetry struct {
	Total, Input, CachedInput, UncachedInput, Output, Reasoning, KRnBytes                          int64
	TotalKnown, InputKnown, CachedKnown, UncachedKnown, OutputKnown, ReasoningKnown, KRnBytesKnown bool
	ToolExecutions, CommandExecutions, AgentMessages, FileChanges, Interventions                   int
	KRn                                                                                            []krnInvocation
	TurnUsages                                                                                     []map[string]any
	Events                                                                                         []map[string]any
}

func parseDetailedTelemetry(data []byte) detailedTelemetry {
	var t detailedTelemetry
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var value map[string]any
		if json.Unmarshal(scanner.Bytes(), &value) != nil {
			continue
		}
		event := map[string]any{"line": line, "type": value["type"]}
		if id := eventString(value, "id"); id != "" {
			event["id"] = id
		}
		t.Events = append(t.Events, event)
		t.walk(value, fmt.Sprintf("line-%d", line))
	}
	return t
}

func (t *detailedTelemetry) walk(value any, lineID string) {
	switch x := value.(type) {
	case map[string]any:
		for k, v := range x {
			key := strings.ToLower(k)
			if n, ok := numberValue(v); ok {
				switch key {
				case "total_tokens":
					t.Total, t.TotalKnown = n, true
				case "input_tokens":
					t.Input, t.InputKnown = n, true
				case "cached_input_tokens", "cache_read_input_tokens":
					t.CachedInput, t.CachedKnown = n, true
				case "uncached_input_tokens":
					t.UncachedInput, t.UncachedKnown = n, true
				case "output_tokens":
					t.Output, t.OutputKnown = n, true
				case "reasoning_output_tokens", "reasoning_tokens":
					t.Reasoning, t.ReasoningKnown = n, true
				case "stdout_bytes":
					t.KRnBytes, t.KRnBytesKnown = t.KRnBytes+n, true
				}
			}
			t.walk(v, lineID)
		}
		typ := strings.ToLower(eventString(x, "type"))
		if strings.Contains(typ, "turn.completed") {
			if usage, ok := x["usage"].(map[string]any); ok {
				t.TurnUsages = append(t.TurnUsages, usage)
			}
		}
		completed := strings.Contains(typ, "completed")
		item, _ := x["item"].(map[string]any)
		itemType := strings.ToLower(eventString(item, "type"))
		command := eventString(item, "command")
		if command == "" {
			command = eventString(x, "command")
		}
		if completed && (itemType == "command_execution" || itemType == "function_call" || strings.Contains(itemType, "tool_call")) {
			t.ToolExecutions++
			if itemType == "command_execution" {
				t.CommandExecutions++
			}
			if strings.Contains(command, "krn ") || strings.HasPrefix(command, "krn ") {
				timestamp := eventString(x, "timestamp")
				if timestamp == "" {
					timestamp = "unavailable"
				}
				t.KRn = append(t.KRn, krnInvocation{Command: command, EventID: lineID, Timestamp: timestamp})
			}
		}
		if itemType == "message" || itemType == "agent_message" {
			t.AgentMessages++
		}
		if strings.Contains(itemType, "file") && strings.Contains(itemType, "change") {
			t.FileChanges++
		}
		if strings.Contains(typ, "approval") || strings.Contains(typ, "intervention") || strings.Contains(typ, "user_input") {
			t.Interventions++
		}
	case []any:
		for _, v := range x {
			t.walk(v, lineID)
		}
	}
}

func (t detailedTelemetry) Unavailable() []string {
	var u []string
	if !t.TotalKnown {
		u = append(u, "total_tokens")
	}
	if !t.InputKnown {
		u = append(u, "input_tokens")
	}
	if !t.CachedKnown {
		u = append(u, "cached_input_tokens")
	}
	if !t.UncachedKnown {
		u = append(u, "uncached_input_tokens")
	}
	if !t.OutputKnown {
		u = append(u, "output_tokens")
	}
	if !t.ReasoningKnown {
		u = append(u, "reasoning_output_tokens")
	}
	return u
}

func materializeFixture(source, destination string) (string, error) {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return "", err
	}
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, _ := filepath.Rel(source, path)
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if d.Type()&0111 != 0 {
			mode = 0700
		}
		return os.WriteFile(target, b, mode)
	})
	if err != nil {
		return "", err
	}
	run := func(args ...string) {}
	_ = run
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "gate1@example.invalid"}, {"config", "user.name", "gate1"}, {"add", "."}} {
		c := exec.Command("git", args...)
		c.Dir = destination
		if b, e := c.CombinedOutput(); e != nil {
			return "", fmt.Errorf("git %v: %w: %s", args, e, b)
		}
	}
	c := exec.Command("git", "commit", "-qm", "fixture")
	c.Dir = destination
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=gate1", "GIT_AUTHOR_EMAIL=gate1@example.invalid", "GIT_COMMITTER_NAME=gate1", "GIT_COMMITTER_EMAIL=gate1@example.invalid", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z")
	if b, e := c.CombinedOutput(); e != nil {
		return "", fmt.Errorf("git commit: %w: %s", e, b)
	}
	return strings.TrimSpace(commandOutputIn(destination, "git", "rev-parse", "HEAD")), nil
}

func fixtureTreeHash(root string) (string, error) {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			paths = append(paths, rel)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return "", err
		}
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Hex(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func optionalInt(n int64, ok bool) any {
	if ok {
		return n
	}
	return "unavailable"
}
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode()
	}
	return 1
}
func commandOutput(name string, args ...string) string {
	return strings.TrimSpace(commandOutputIn("", name, args...))
}
func commandOutputIn(dir, name string, args ...string) string {
	c := exec.Command(name, args...)
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return "unavailable"
	}
	return string(b)
}
func eventString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func suiteMetricDelta(a, b suiteMetric) map[string]any {
	out := map[string]any{
		"verified_completion":          boolDelta(a.VerifiedCompletion, b.VerifiedCompletion),
		"wall_time_ms":                 b.WallTimeMS - a.WallTimeMS,
		"completed_tool_executions":    b.ToolExecutions - a.ToolExecutions,
		"completed_command_executions": b.CommandExecutions - a.CommandExecutions,
		"human_interventions":          b.HumanInterventions - a.HumanInterventions,
	}
	for name, av := range map[string]any{"total_tokens": a.TotalTokens, "input_tokens": a.InputTokens, "cached_input_tokens": a.CachedInputTokens, "uncached_input_tokens": a.UncachedInputTokens, "output_tokens": a.OutputTokens, "reasoning_output_tokens": a.ReasoningTokens} {
		out[name] = numericDelta(av, metricField(b, name))
	}
	return out
}

func metricField(m suiteMetric, name string) any {
	switch name {
	case "total_tokens":
		return m.TotalTokens
	case "input_tokens":
		return m.InputTokens
	case "cached_input_tokens":
		return m.CachedInputTokens
	case "uncached_input_tokens":
		return m.UncachedInputTokens
	case "output_tokens":
		return m.OutputTokens
	case "reasoning_output_tokens":
		return m.ReasoningTokens
	}
	return "unavailable"
}

func writeSuiteMarkdown(path string, report suiteReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Gate-1 KRn evaluation\n\n- KRn commit: `%s`\n- Codex: `%s`, effort `%s`\n- Manifest SHA-256: `%s`\n- Fixture tree SHA-256: `%s`\n\n", report.KRnCommit, report.CodexVersion, report.ReasoningEffort, report.ManifestSHA256, report.FixtureSHA256)
	b.WriteString("| Task | Category | A verified | B verified | A total tokens | B total tokens | token delta | A wall ms | B wall ms | A commands | B commands |\n|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, c := range report.Comparisons {
		fmt.Fprintf(&b, "| %s | %s | %v | %v | %v | %v | %v | %d | %d | %d | %d |\n", c.TaskID, c.Category, c.A.VerifiedCompletion, c.B.VerifiedCompletion, c.A.TotalTokens, c.B.TotalTokens, c.Delta["total_tokens"], c.A.WallTimeMS, c.B.WallTimeMS, c.A.CommandExecutions, c.B.CommandExecutions)
	}
	b.WriteString("\nMetrics not exposed by Codex remain `unavailable`; this report makes no statistical-significance claim. Raw JSONL, timelines, prompts, verification logs, patches, and Git status are retained per task and variant.\n")
	return os.WriteFile(path, []byte(b.String()), 0600)
}
