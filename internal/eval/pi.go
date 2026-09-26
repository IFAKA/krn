package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"example.com/krn/internal/workspace"
)

// eval-pi measures the pi integration variants on real repositories with a
// local model. Each run is a fresh clone, a single non-interactive pi prompt,
// and a deterministic grade: answer regexes for questions, a shell check for edits.

type piManifest struct {
	Schema string            `json:"schema"`
	Repos  map[string]piRepo `json:"repos"`
	Tasks  []piTask          `json:"tasks"`
}

type piRepo struct {
	Source  string `json:"source,omitempty"`  // Git repository to clone
	Commit  string `json:"commit,omitempty"`  // pinned commit
	Fixture string `json:"fixture,omitempty"` // or a directory inside the KRn repo
}

type piTask struct {
	ID       string   `json:"id"`
	Repo     string   `json:"repo"`
	Category string   `json:"category"`
	Prompt   string   `json:"prompt"`
	Answer   []string `json:"answer_patterns,omitempty"`
	Verify   string   `json:"verify,omitempty"`
}

type piVariant struct {
	Name             string
	Bound, Tool, Map bool
}

var piVariants = map[string]piVariant{
	"none": {Name: "none"},
	"A":    {Name: "A", Bound: true},
	"AB":   {Name: "AB", Bound: true, Tool: true},
	"ABC":  {Name: "ABC", Bound: true, Tool: true, Map: true},
	"B":    {Name: "B", Tool: true},
	"C":    {Name: "C", Map: true},
	"BC":   {Name: "BC", Tool: true, Map: true},
}

type piResult struct {
	Task         string         `json:"task"`
	Category     string         `json:"category"`
	Variant      string         `json:"variant"`
	Seed         int            `json:"seed"`
	Correct      bool           `json:"correct"`
	ExitCode     int            `json:"exit_code"`
	WallMS       int64          `json:"wall_ms"`
	Turns        int            `json:"turns"`
	UncachedIn   int64          `json:"uncached_input_tokens"`
	CachedIn     int64          `json:"cached_input_tokens"`
	Output       int64          `json:"output_tokens"`
	ToolCalls    map[string]int `json:"tool_calls"`
	FindCode     int            `json:"find_code_calls"`
	Final        string         `json:"final_answer"`
	GradeDetails string         `json:"grade"`
}

func RunPi(args []string) error {
	f := workspace.FlagSet("eval-pi")
	manifestPath := f.String("manifest", "eval/pi-local-manifest.json", "task manifest")
	output := f.String("output", "", "evidence directory")
	model := f.String("model", "", "pi model id (provider omlx)")
	provider := f.String("provider", "omlx", "pi provider")
	agentDir := f.String("agent-dir", "", "PI_CODING_AGENT_DIR for isolated pi config")
	extension := f.String("extension", "integrations/pi/krn.ts", "KRn pi extension")
	krnBin := f.String("krn", "", "krn binary used by the extension (default: this executable)")
	workdir := f.String("workdir", "", "where clones are created (must not be under a directory with CLAUDE.md/AGENTS.md)")
	variantList := f.String("variants", "none,A,AB,ABC", "comma-separated variants")
	seeds := f.Int("seeds", 3, "repetitions per task and variant")
	only := f.String("tasks", "", "comma-separated task ids (default all)")
	timeout := f.Duration("timeout", 10*time.Minute, "per-run timeout")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *model == "" {
		return errors.New("eval-pi requires --model")
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(r.Root, *manifestPath))
	if err != nil {
		return err
	}
	var m piManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	var variants []piVariant
	for _, name := range strings.Split(*variantList, ",") {
		v, ok := piVariants[strings.TrimSpace(name)]
		if !ok {
			return fmt.Errorf("unknown variant %q", name)
		}
		variants = append(variants, v)
	}
	wanted := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	var tasks []piTask
	for _, t := range m.Tasks {
		if len(wanted) == 0 || wanted[t.ID] {
			tasks = append(tasks, t)
		}
	}
	if *krnBin == "" {
		if exe, e := os.Executable(); e == nil {
			*krnBin = exe
		}
	}
	ext, _ := filepath.Abs(filepath.Join(r.Root, *extension))
	evidence := *output
	if evidence == "" {
		evidence = filepath.Join(r.Private, "evals", "pi-"+time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		return err
	}
	wd := *workdir
	if wd == "" {
		wd = filepath.Join(os.TempDir(), fmt.Sprintf("krn-eval-pi-%d", os.Getpid()))
	}
	if err := os.MkdirAll(wd, 0700); err != nil {
		return err
	}
	resultsPath := filepath.Join(evidence, "results.jsonl")
	out, err := os.OpenFile(resultsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_ = workspace.WriteJSON(filepath.Join(evidence, "config.json"), map[string]any{"model": *model, "provider": *provider, "variants": *variantList, "seeds": *seeds, "manifest": *manifestPath, "krn": *krnBin, "extension": ext, "started": time.Now().UTC().Format(time.RFC3339)})
	var all []piResult
	total := *seeds * len(tasks) * len(variants)
	n := 0
	for seed := 0; seed < *seeds; seed++ {
		for ti, task := range tasks {
			// Rotate variant order per task and seed so warm prefix caches and
			// server state are not systematically in one variant's favor.
			for k := range variants {
				v := variants[(k+ti+seed)%len(variants)]
				n++
				repo, err := materialize(r.Root, m.Repos[task.Repo], filepath.Join(wd, fmt.Sprintf("%s-%s-%d", task.ID, v.Name, seed)))
				if err != nil {
					return fmt.Errorf("%s: %w", task.ID, err)
				}
				res := runPi(repo, task, v, seed, *model, *provider, *agentDir, ext, *krnBin, *timeout, filepath.Join(evidence, fmt.Sprintf("%s.%s.%d", task.ID, v.Name, seed)))
				_ = os.RemoveAll(repo)
				line, _ := json.Marshal(res)
				_, _ = out.Write(append(line, '\n'))
				all = append(all, res)
				fmt.Printf("[%d/%d] %-14s %-4s seed=%d correct=%-5v wall=%5.1fs turns=%2d uncached=%6d find_code=%d\n", n, total, task.ID, v.Name, seed, res.Correct, float64(res.WallMS)/1000, res.Turns, res.UncachedIn, res.FindCode)
			}
		}
	}
	_ = os.Remove(wd)
	report := piReport(all, variants)
	if err := os.WriteFile(filepath.Join(evidence, "report.md"), []byte(report), 0600); err != nil {
		return err
	}
	fmt.Print(report)
	fmt.Printf("evidence: %s\n", evidence)
	return nil
}

func materialize(root string, repo piRepo, dest string) (string, error) {
	_ = os.RemoveAll(dest)
	if repo.Fixture != "" {
		_, err := materializeFixture(filepath.Join(root, repo.Fixture), dest)
		return dest, err
	}
	if repo.Source == "" || repo.Commit == "" {
		return "", errors.New("repo needs source and commit, or fixture")
	}
	for _, args := range [][]string{{"clone", "-q", "--no-checkout", repo.Source, dest}, {"-C", dest, "checkout", "-q", "--detach", repo.Commit}} {
		if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %v: %w: %s", args, err, b)
		}
	}
	return dest, nil
}

func runPi(repo string, task piTask, v piVariant, seed int, model, provider, agentDir, ext, krnBin string, timeout time.Duration, evidence string) piResult {
	res := piResult{Task: task.ID, Category: task.Category, Variant: v.Name, Seed: seed, ToolCalls: map[string]int{}}
	tools := "read,bash,edit,write,grep,find,ls"
	args := []string{"-p", "--mode", "json", "--no-session", "--no-extensions", "--provider", provider, "--model", model}
	if v.Bound || v.Tool || v.Map {
		args = append(args, "-e", ext)
		if v.Tool {
			tools += ",find_code"
		}
	}
	args = append(args, "--tools", tools, task.Prompt)
	// cmd.Stdin stays nil (the null device): pi -p waits for EOF on a piped stdin.
	cmd := exec.Command("pi", args...)
	cmd.Dir = repo
	env := append(os.Environ(), "PI_OFFLINE=1", "KRN_BIN="+krnBin, "KRN_PI_BOUND="+flag01(v.Bound), "KRN_PI_TOOL="+flag01(v.Tool), "KRN_PI_MAP="+flag01(v.Map))
	if agentDir != "" {
		env = append(env, "PI_CODING_AGENT_DIR="+agentDir)
	}
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	if err := cmd.Start(); err != nil {
		res.ExitCode, res.GradeDetails = -1, err.Error()
		return res
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
	parsePiEvents(stdout.String(), &res)
	res.Correct, res.GradeDetails = gradePi(repo, task, res.Final, res.GradeDetails)
	return res
}

func flag01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func parsePiEvents(s string, res *piResult) {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var e struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
					Name string `json:"name"`
				} `json:"content"`
				Usage *struct {
					Input     int64 `json:"input"`
					Output    int64 `json:"output"`
					CacheRead int64 `json:"cacheRead"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Type != "message_end" || e.Message.Role != "assistant" {
			continue
		}
		res.Turns++
		if u := e.Message.Usage; u != nil {
			res.UncachedIn += u.Input
			res.CachedIn += u.CacheRead
			res.Output += u.Output
		}
		text := ""
		for _, c := range e.Message.Content {
			switch c.Type {
			case "toolCall":
				res.ToolCalls[c.Name]++
				if c.Name == "find_code" {
					res.FindCode++
				}
			case "text":
				text += c.Text
			}
		}
		if strings.TrimSpace(text) != "" {
			res.Final = text
		}
	}
}

func gradePi(repo string, task piTask, final, prefix string) (bool, string) {
	var fails []string
	for _, p := range task.Answer {
		re, err := regexp.Compile("(?is)" + p)
		if err != nil || !re.MatchString(final) {
			fails = append(fails, "missing /"+p+"/")
		}
	}
	if task.Verify != "" {
		c := exec.Command("sh", "-c", task.Verify)
		c.Dir = repo
		if b, err := c.CombinedOutput(); err != nil {
			fails = append(fails, "verify failed: "+workspace.Tail(b, 300))
		}
	}
	if len(fails) == 0 {
		return prefix == "", prefix + "ok"
	}
	return false, prefix + strings.Join(fails, "; ")
}

func piReport(all []piResult, variants []piVariant) string {
	var b strings.Builder
	b.WriteString("| variant | correct | accuracy | correct/min | median wall s | mean wall s | mean turns | mean uncached in | mean cached in | mean out | find_code/run |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, v := range variants {
		var rs []piResult
		for _, r := range all {
			if r.Variant == v.Name {
				rs = append(rs, r)
			}
		}
		if len(rs) == 0 {
			continue
		}
		var correct int
		var wall, turns, unc, cached, outTok, fc float64
		walls := make([]float64, 0, len(rs))
		for _, r := range rs {
			if r.Correct {
				correct++
			}
			w := float64(r.WallMS) / 1000
			wall += w
			walls = append(walls, w)
			turns += float64(r.Turns)
			unc += float64(r.UncachedIn)
			cached += float64(r.CachedIn)
			outTok += float64(r.Output)
			fc += float64(r.FindCode)
		}
		sort.Float64s(walls)
		k := float64(len(rs))
		fmt.Fprintf(&b, "| %s | %d/%d | %.0f%% | %.2f | %.1f | %.1f | %.1f | %.0f | %.0f | %.0f | %.1f |\n", v.Name, correct, len(rs), 100*float64(correct)/k, float64(correct)/(wall/60), walls[len(walls)/2], wall/k, turns/k, unc/k, cached/k, outTok/k, fc/k)
	}
	b.WriteString("\nPer task (correct runs / runs, mean wall s):\n\n| task |")
	for _, v := range variants {
		b.WriteString(" " + v.Name + " |")
	}
	b.WriteString("\n|---|" + strings.Repeat("---:|", len(variants)) + "\n")
	var ids []string
	seen := map[string]bool{}
	for _, r := range all {
		if !seen[r.Task] {
			seen[r.Task] = true
			ids = append(ids, r.Task)
		}
	}
	for _, id := range ids {
		b.WriteString("| " + id + " |")
		for _, v := range variants {
			var c, n int
			var w float64
			for _, r := range all {
				if r.Task == id && r.Variant == v.Name {
					n++
					w += float64(r.WallMS) / 1000
					if r.Correct {
						c++
					}
				}
			}
			if n == 0 {
				b.WriteString(" - |")
				continue
			}
			fmt.Fprintf(&b, " %d/%d, %.0fs |", c, n, math.Round(w/float64(n)))
		}
		b.WriteString("\n")
	}
	return b.String()
}
