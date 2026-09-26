package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/krn/internal/integrate"
	"example.com/krn/internal/workspace"
)

// eval-claude runs the eval-pi manifest through Claude Code (`claude -p`) so the same
// tasks and grades compare vanilla Claude Code, the installed KRn routing policy, the
// map, and a plain file list. Every run uses --safe-mode, so the user's CLAUDE.md,
// hooks, plugins, and MCP servers never reach the model; each variant adds only its
// own text through --append-system-prompt.

var claudeVariants = map[string]bool{
	"none":   true, // vanilla Claude Code
	"policy": true, // the routing policy `krn integrate claude` installs, krn on PATH
	"map":    true, // `krn map --focus PROMPT` appended to the system prompt
	"tree":   true, // `git ls-files` cut to the same budget (baseline, no KRn)
}

const mapTokens = 800

var krnCommand = regexp.MustCompile(`(^|[;&|(]\s*)krn\s`)

func RunClaude(args []string) error {
	f := workspace.FlagSet("eval-claude")
	sf := addSuiteFlags(f, "none,policy,map,tree", 2)
	model := f.String("model", "claude-haiku-4-5-20251001", "Claude model")
	krnBin := f.String("krn", "", "krn binary for the policy and map variants (default: this executable)")
	runBudget := f.Float64("max-budget-usd", 0.5, "per-run cap passed to claude --max-budget-usd")
	totalBudget := f.Float64("total-budget-usd", 10, "stop starting new runs once reported cost reaches this")
	timeout := f.Duration("timeout", 10*time.Minute, "per-run timeout")
	if err := f.Parse(args); err != nil {
		return err
	}
	s, err := sf.load("claude", func(v string) bool { return claudeVariants[v] })
	if err != nil {
		return err
	}
	defaultKrn(krnBin)
	// The policy tells the model to run `krn`; put exactly this binary first on PATH.
	binDir := filepath.Join(s.workdir, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(binDir, "krn"))
	if err := os.Symlink(*krnBin, filepath.Join(binDir, "krn")); err != nil {
		return err
	}
	defer os.RemoveAll(binDir)
	var spent float64
	all, err := s.run(map[string]any{"agent": "claude", "model": *model, "krn": *krnBin, "max_budget_usd": *runBudget, "total_budget_usd": *totalBudget},
		func(repo string, task piTask, variant string, seed int, evidence string) piResult {
			res := piResult{Task: task.ID, Category: task.Category, Variant: variant, Seed: seed, ToolCalls: map[string]int{}}
			if spent >= *totalBudget {
				res.ExitCode, res.GradeDetails = -1, "skipped: total budget reached"
				return res
			}
			runClaude(repo, task, variant, *model, *krnBin, binDir, *runBudget, *timeout, evidence, &res)
			spent += res.CostUSD
			return res
		})
	if err != nil {
		return err
	}
	return s.report(all, claudeCost(all, s.variants))
}

func runClaude(repo string, task piTask, variant, model, krnBin, binDir string, budget float64, timeout time.Duration, evidence string, res *piResult) {
	var appendix string
	switch variant {
	case "policy":
		appendix = integrate.ClaudeText
	case "map":
		c := exec.Command(krnBin, "map", "--tokens", strconv.Itoa(mapTokens), "--focus", task.Prompt)
		c.Dir = repo
		if m, err := c.Output(); err == nil {
			appendix = "Repository map (ranked by relevance to the request; signatures only):\n" + string(m)
		}
	case "tree":
		appendix = "Repository files (git ls-files):\n" + fileList(repo, 4*mapTokens)
	}
	args := []string{"-p", "--safe-mode", "--model", model, "--no-session-persistence", "--output-format", "stream-json", "--verbose",
		"--permission-mode", "dontAsk", "--allowedTools", "Bash,Read,Grep,Glob,Edit,Write",
		"--max-budget-usd", strconv.FormatFloat(budget, 'f', 2, 64)}
	if appendix != "" {
		args = append(args, "--append-system-prompt", appendix)
	}
	args = append(args, task.Prompt)
	cmd := exec.Command("claude", args...)
	cmd.Dir = repo
	cmd.Env = claudeEnv(binDir)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	if err := cmd.Start(); err != nil {
		res.ExitCode, res.GradeDetails = -1, err.Error()
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		runErr = <-done
		res.GradeDetails = "timeout; "
	}
	res.WallMS = time.Since(start).Milliseconds()
	res.ExitCode = exitCode(runErr)
	_ = os.WriteFile(evidence+".jsonl", []byte(stdout.String()), 0600)
	if stderr.Len() > 0 {
		_ = os.WriteFile(evidence+".stderr", []byte(stderr.String()), 0600)
	}
	parseClaudeEvents(stdout.String(), res)
	res.Correct, res.GradeDetails = gradePi(repo, task, res.Final, res.GradeDetails)
}

// claudeEnv drops the variables that mark a nested Claude Code session, and puts the
// eval's krn first on PATH. Authentication is left to the user's normal login.
func claudeEnv(binDir string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case k == "CLAUDECODE", k == "CLAUDE_PID", k == "CLAUDE_EFFORT", k == "CLAUDE_JOB_DIR",
			strings.HasPrefix(k, "CLAUDE_CODE_") && k != "CLAUDE_CODE_OAUTH_TOKEN":
			continue
		case k == "PATH":
			kv = "PATH=" + binDir + string(os.PathListSeparator) + v
		}
		env = append(env, kv)
	}
	return env
}

// fileList is `git ls-files` cut at a line boundary to budget bytes, like eval/baselines/pi-tree.ts.
func fileList(repo string, budget int) string {
	out, err := exec.Command("git", "-C", repo, "ls-files").Output()
	if err != nil {
		return ""
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	var b strings.Builder
	n := 0
	for _, f := range files {
		if b.Len()+len(f)+1 > budget {
			break
		}
		b.WriteString(f + "\n")
		n++
	}
	if n < len(files) {
		fmt.Fprintf(&b, "… %d more files\n", len(files)-n)
	}
	return b.String()
}

func parseClaudeEvents(s string, res *piResult) {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var e struct {
			Type    string  `json:"type"`
			Result  string  `json:"result"`
			Turns   int     `json:"num_turns"`
			Cost    float64 `json:"total_cost_usd"`
			Message struct {
				Content []struct {
					Type  string `json:"type"`
					Name  string `json:"name"`
					Input struct {
						Command string `json:"command"`
					} `json:"input"`
				} `json:"content"`
			} `json:"message"`
			Usage struct {
				Input         int64 `json:"input_tokens"`
				CacheCreation int64 `json:"cache_creation_input_tokens"`
				CacheRead     int64 `json:"cache_read_input_tokens"`
				Output        int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		switch e.Type {
		case "assistant":
			for _, c := range e.Message.Content {
				if c.Type == "tool_use" {
					res.ToolCalls[c.Name]++
					if c.Name == "Bash" && krnCommand.MatchString(c.Input.Command) {
						res.KrnCalls++
					}
				}
			}
		case "result":
			res.Final, res.Turns, res.CostUSD = e.Result, e.Turns, e.Cost
			res.UncachedIn = e.Usage.Input + e.Usage.CacheCreation
			res.CachedIn, res.Output = e.Usage.CacheRead, e.Usage.Output
		}
	}
}

func claudeCost(all []piResult, variants []string) string {
	var b strings.Builder
	b.WriteString("\nCost at list price, as reported by Claude Code (total_cost_usd):\n\n| variant | total USD | mean USD/run | krn calls/run |\n|---|---:|---:|---:|\n")
	var sum float64
	for _, v := range variants {
		var c float64
		var n, krnCalls int
		for _, r := range all {
			if r.Variant == v {
				c += r.CostUSD
				n++
				krnCalls += r.KrnCalls
			}
		}
		sum += c
		if n > 0 {
			fmt.Fprintf(&b, "| %s | %.2f | %.3f | %.1f |\n", v, c, c/float64(n), float64(krnCalls)/float64(n))
		}
	}
	fmt.Fprintf(&b, "\nTotal: %.2f USD\n", sum)
	return b.String()
}
