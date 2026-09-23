package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const version = "0.3.0"
const cacheSchema = "2"
const markerStart = "<!-- KRN:BEGIN -->"
const markerEnd = "<!-- KRN:END -->"
const codexText = `KRn is available globally. Use it only when it is likely to reduce model context, repeated exploration, or unreconstructable reasoning.

Routing:
- For repository/code-change tasks that need orientation or may benefit from prior KRn state, start with ` + "`krn context --json`" + `.
- Before broad file reading or repeated search, prefer ` + "`krn find QUERY --json --max-files N`" + ` for bounded evidence with a recoverable full log.
- For verification, prefer ` + "`krn verify --level fast|full --json`" + ` when its discovered checks fit the task; otherwise run the project-native focused command directly.
- Use ` + "`krn exec --cache --input PATH ... -- COMMAND ...`" + ` only for deterministic repeated commands with explicit input dependencies; add ` + "`--verified`" + ` only after an external check verified the result.
- Use ` + "`krn state`" + ` only for irreducible durable facts: objective, constraints, proven facts, open questions, or negative results that are not cheaply reconstructable from Git/files/tests.
- Use ` + "`krn compile`" + ` only when reviewing repeated verified trajectories for reusable deterministic operations.

Skip KRn for trivial answers, single known-file edits, direct user-specified commands, or when a normal tool call is cheaper than consulting KRn. Do not dump large KRn logs into context; use bounded projections and paths to recover details only when needed.`

type repo struct{ Root, GitDir, Private string }
type stateFile struct {
	Objective   string   `json:"objective,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	Proven      []string `json:"proven,omitempty"`
	Open        []string `json:"open,omitempty"`
	Negative    []string `json:"negative,omitempty"`
}
type metric struct {
	Operation       string   `json:"operation"`
	Parameters      []string `json:"parameters,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Result          string   `json:"result"`
	Verified        bool     `json:"verified"`
	Cached          bool     `json:"cached"`
	DurationMS      int64    `json:"duration_ms"`
	ProjectionBytes int      `json:"projection_bytes,omitempty"`
	Tokens          string   `json:"codex_tokens,omitempty"`
	Timestamp       string   `json:"timestamp"`
	Version         string   `json:"version"`
}
type cacheRecord struct {
	Key, Primitive, PrimitiveVersion string
	Parameters                       []string
	Dependencies                     []string
	DependencyFingerprints           map[string]string
	Result, ResultFingerprint, Log   string
	ExitCode                         int
	Verified                         bool
	Created                          string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	var err error
	switch os.Args[1] {
	case "context":
		err = contextCmd(os.Args[2:])
	case "find":
		err = findCmd(os.Args[2:])
	case "verify":
		err = verifyCmd(os.Args[2:])
	case "state":
		err = stateCmd(os.Args[2:])
	case "exec":
		err = execCmd(os.Args[2:])
	case "compile":
		err = compileCmd(os.Args[2:])
	case "eval":
		err = evalCmd(os.Args[2:])
	case "integrate":
		err = integrateCmd(os.Args[2:])
	case "doctor":
		err = doctorCmd()
	case "uninstall":
		err = uninstallCmd()
	case "version":
		fmt.Println(version)
	default:
		usage()
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "krn:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("krn " + version + "\nusage: krn context|find|verify|state|exec|compile|eval|integrate|doctor|uninstall")
}

func discover() (repo, error) {
	p, err := os.Getwd()
	if err != nil {
		return repo{}, err
	}
	root, err := git(p, "rev-parse", "--show-toplevel")
	if err != nil {
		return repo{}, errors.New("not inside a git repository")
	}
	root = strings.TrimSpace(root)
	gd, err := git(p, "rev-parse", "--git-dir")
	if err != nil {
		return repo{}, err
	}
	gd = strings.TrimSpace(gd)
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(root, gd)
	}
	r := repo{Root: root, GitDir: filepath.Clean(gd)}
	r.Private = filepath.Join(r.GitDir, "krn")
	return r, nil
}
func git(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, err := c.Output()
	return string(b), err
}
func ensure(r repo) error { return os.MkdirAll(filepath.Join(r.Private, "runs"), 0700) }
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func ecosystem(r repo) []string {
	names := map[string]string{"package.json": "node", "pnpm-lock.yaml": "node", "yarn.lock": "node", "bun.lockb": "node", "go.mod": "go", "Cargo.toml": "rust", "pom.xml": "maven", "build.gradle": "gradle", "build.gradle.kts": "gradle", "pyproject.toml": "python", "requirements.txt": "python", "MODULE.bazel": "bazel"}
	seen := map[string]bool{}
	var out []string
	for n, e := range names {
		if _, err := os.Stat(filepath.Join(r.Root, n)); err == nil && !seen[e] {
			out = append(out, e)
			seen[e] = true
		}
	}
	sort.Strings(out)
	return out
}

func contextCmd(args []string) error {
	f := flag.NewFlagSet("context", flag.ContinueOnError)
	js := f.Bool("json", false, "json")
	if err := f.Parse(args); err != nil {
		return err
	}
	r, err := discover()
	if err != nil {
		return err
	}
	ensure(r)
	head, _ := git(r.Root, "rev-parse", "HEAD")
	branch, _ := git(r.Root, "branch", "--show-current")
	changes, _ := git(r.Root, "status", "--porcelain=v1")
	paths := []string{}
	for _, line := range strings.Split(strings.TrimRight(changes, "\n"), "\n") {
		if len(line) > 3 {
			paths = append(paths, strings.TrimSpace(line[3:]))
		}
	}
	s := stateFile{}
	stateErr := readJSON(filepath.Join(r.Private, "state.json"), &s)
	v := map[string]any{"root": r.Root, "git_dir": r.GitDir, "head": strings.TrimSpace(head), "branch": strings.TrimSpace(branch), "changed": paths, "ecosystems": ecosystem(r), "state": s}
	if stateErr != nil && !os.IsNotExist(stateErr) {
		v["state"] = nil
		v["state_status"] = "unknown"
	}
	if *js {
		return jsonPrint(v)
	}
	fmt.Printf("root: %s\nbranch: %s\nhead: %s\necosystems: %s\n", r.Root, strings.TrimSpace(branch), strings.TrimSpace(head), strings.Join(ecosystem(r), ", "))
	if len(paths) > 0 {
		fmt.Println("changed:")
		for _, p := range paths {
			fmt.Println("  " + p)
		}
	}
	if s.Objective != "" {
		fmt.Println("objective: " + s.Objective)
	}
	if stateErr != nil && !os.IsNotExist(stateErr) {
		fmt.Println("state: unknown (malformed state file)")
	}
	return nil
}

func findCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("find requires a query")
	}
	f := flag.NewFlagSet("find", flag.ContinueOnError)
	js := f.Bool("json", false, "json")
	max := f.Int("max-files", 12, "maximum files")
	args = normalizeFindArgs(args)
	if err := f.Parse(args); err != nil {
		return err
	}
	if *max < 0 {
		return errors.New("max-files must be non-negative")
	}
	if f.NArg() == 0 {
		return errors.New("find requires a query")
	}
	r, err := discover()
	if err != nil {
		return err
	}
	q := f.Arg(0)
	ensure(r)
	start := time.Now()
	c := exec.Command("rg", "-n", "--no-heading", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**", q, ".")
	c.Dir = r.Root
	raw, e := c.CombinedOutput()
	log := saveRun(r, "find", raw)
	type hit struct {
		File string `json:"file"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	counts := map[string]int{}
	hits := map[string][]hit{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		n, _ := strconv.Atoi(parts[1])
		file := strings.TrimPrefix(parts[0], "./")
		counts[file]++
		if len(hits[file]) < 2 {
			hits[file] = append(hits[file], hit{file, n, parts[2]})
		}
	}
	files := make([]string, 0, len(counts))
	for p := range counts {
		files = append(files, p)
	}
	sort.Slice(files, func(i, j int) bool {
		if counts[files[i]] != counts[files[j]] {
			return counts[files[i]] > counts[files[j]]
		}
		return files[i] < files[j]
	})
	if len(files) > *max {
		files = files[:*max]
	}
	if *js {
		selected := map[string][]hit{}
		for _, p := range files {
			selected[p] = hits[p]
		}
		return jsonPrint(map[string]any{"query": q, "matches": len(counts), "files": files, "hits": selected, "full_log": log, "duration_ms": time.Since(start).Milliseconds()})
	}
	fmt.Printf("query: %s\nfiles: %d\nfull_log: %s\n", q, len(files), log)
	for _, p := range files {
		fmt.Printf("%d %s\n", counts[p], p)
		for _, h := range hits[p] {
			fmt.Printf("  %d: %s\n", h.Line, h.Text)
		}
	}
	if e != nil && len(counts) == 0 {
		return nil
	}
	return nil
}

func normalizeFindArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			flags = append(flags, a)
		case a == "--max-files":
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case strings.HasPrefix(a, "--max-files="):
			flags = append(flags, a)
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

func stateCmd(args []string) error {
	r, err := discover()
	if err != nil {
		return err
	}
	p := filepath.Join(r.Private, "state.json")
	s := stateFile{}
	if e := readJSON(p, &s); e != nil && !os.IsNotExist(e) {
		return e
	}
	if len(args) == 0 || args[0] == "show" {
		return jsonPrint(s)
	}
	if len(args) == 1 && args[0] == "clear" {
		if err := ensure(r); err != nil {
			return err
		}
		return writeJSON(p, stateFile{})
	}
	if len(args) < 3 {
		return errors.New("state set|add|clear <field> [text]")
	}
	field, op, val := args[0], args[1], strings.Join(args[2:], " ")
	if op == "clear" {
		s = stateFile{}
	} else if op == "set" && field == "objective" {
		s.Objective = val
	} else if op == "add" {
		switch field {
		case "constraint":
			s.Constraints = append(s.Constraints, val)
		case "proven":
			s.Proven = append(s.Proven, val)
		case "open":
			s.Open = append(s.Open, val)
		case "negative":
			s.Negative = append(s.Negative, val)
		default:
			return errors.New("unknown state field")
		}
	} else {
		return errors.New("unknown state operation")
	}
	if err := ensure(r); err != nil {
		return err
	}
	return writeJSON(p, s)
}

func saveRun(r repo, name string, b []byte) string {
	if err := ensure(r); err != nil {
		return ""
	}
	p := filepath.Join(r.Private, "runs", time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+name+".log")
	if len(b) > 0 {
		_ = os.WriteFile(p, b, 0600)
	}
	return p
}
func tail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}
func projection(b []byte, log string) string {
	const limit = 12000
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + fmt.Sprintf("\n... output bounded; full_log: %s\n", log)
}
func jsonPrint(v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		fmt.Println(string(b))
	}
	return e
}

func discoverChecks(r repo, level string) [][]string {
	var out [][]string
	if b, e := os.ReadFile(filepath.Join(r.Root, ".kern", "config.json")); e == nil {
		var c struct {
			Verify map[string][]string `json:"verify"`
		}
		if json.Unmarshal(b, &c) == nil {
			for _, s := range c.Verify[level] {
				if a := splitCommand(s); len(a) > 0 {
					out = append(out, a)
				}
			}
		}
	}
	if b, e := os.ReadFile(filepath.Join(r.Root, "package.json")); e == nil {
		var p struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &p) == nil {
			names := []string{"typecheck", "lint", "check"}
			if level == "full" {
				names = append(names, "test", "build")
			}
			for _, n := range names {
				if s := p.Scripts[n]; s != "" {
					out = append(out, []string{"sh", "-c", s})
				}
			}
		}
	}
	if _, e := os.Stat(filepath.Join(r.Root, "go.mod")); e == nil {
		if level == "full" {
			out = append(out, []string{"go", "test", "./..."})
		} else {
			out = append(out, []string{"go", "test", "./...", "-run", "^$"})
		}
	}
	if _, e := os.Stat(filepath.Join(r.Root, "Cargo.toml")); e == nil {
		out = append(out, []string{"cargo", "check"})
	}
	if _, e := os.Stat(filepath.Join(r.Root, "pyproject.toml")); e == nil {
		if _, x := exec.LookPath("pytest"); x == nil {
			out = append(out, []string{"pytest", "--collect-only", "-q"})
		}
	}
	return out
}
func splitCommand(s string) []string { f := strings.Fields(s); return f }
func verifyCmd(args []string) error {
	f := flag.NewFlagSet("verify", flag.ContinueOnError)
	level := f.String("level", "fast", "fast or full")
	js := f.Bool("json", false, "json")
	if e := f.Parse(args); e != nil {
		return e
	}
	if *level != "fast" && *level != "full" {
		return errors.New("level must be fast or full")
	}
	r, e := discover()
	if e != nil {
		return e
	}
	ensure(r)
	checks := discoverChecks(r, *level)
	out := map[string]any{"level": *level, "checks": []any{}, "verified": false}
	if len(checks) == 0 {
		out["status"] = "unknown"
		out["message"] = "no verification commands safely discovered"
		if *js {
			return jsonPrint(out)
		}
		fmt.Println(out["message"])
		return nil
	}
	var list []any
	all := true
	for _, a := range checks {
		start := time.Now()
		c := exec.Command(a[0], a[1:]...)
		c.Dir = r.Root
		b, err := c.CombinedOutput()
		log := saveRun(r, "verify", b)
		ok := err == nil
		all = all && ok
		x := map[string]any{"command": a, "passed": ok, "status": map[bool]string{true: "PASS", false: "FAIL"}[ok], "duration_ms": time.Since(start).Milliseconds(), "full_log": log}
		recordMetric(r, metric{Operation: "verify", Parameters: a, Result: map[bool]string{true: "success", false: "failure"}[ok], Verified: ok, DurationMS: time.Since(start).Milliseconds(), ProjectionBytes: len(tail(b, 4000))})
		if !ok {
			x["evidence"] = tail(b, 4000)
		}
		list = append(list, x)
	}
	out["checks"] = list
	out["verified"] = all
	out["status"] = map[bool]string{true: "passed", false: "failed"}[all]
	if *js {
		return jsonPrint(out)
	}
	for _, x := range list {
		m := x.(map[string]any)
		fmt.Printf("%s %v (%vms)\n", m["status"], m["command"], m["duration_ms"])
	}
	if !all {
		return errors.New("verification failed")
	}
	return nil
}

func fileFingerprint(path string) (string, error) {
	h := sha256.New()
	info, e := os.Stat(path)
	if e != nil {
		return "", e
	}
	if !info.IsDir() {
		b, e := os.ReadFile(path)
		if e != nil {
			return "", e
		}
		h.Write(b)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	var files []string
	if e := filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if i.IsDir() {
			if p != path && (i.Name() == ".git" || i.Name() == "node_modules" || i.Name() == "dist" || i.Name() == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if i.Mode().IsRegular() {
			files = append(files, p)
		}
		return nil
	}); e != nil {
		return "", e
	}
	sort.Strings(files)
	for _, p := range files {
		rel, _ := filepath.Rel(path, p)
		h.Write([]byte(rel))
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func cacheKeyAtRoot(root string, command []string, inputs []string) (string, map[string]string, error) {
	h := sha256.New()
	h.Write([]byte("exec/" + cacheSchema + "\x00"))
	h.Write([]byte(filepath.Clean(root) + "\x00"))
	for _, a := range command {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	fps := map[string]string{}
	canonical := canonicalInputs(root, inputs)
	for _, abs := range canonical {
		fp, e := fileFingerprint(abs)
		if e != nil {
			return "", nil, e
		}
		fps[abs] = fp
		h.Write([]byte(abs + "\x00" + fp))
	}
	return hex.EncodeToString(h.Sum(nil)), fps, nil
}

func canonicalInputs(root string, inputs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range inputs {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, p)
		}
		abs = filepath.Clean(abs)
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	sort.Strings(out)
	return out
}
func recordMetric(r repo, m metric) {
	_ = ensure(r)
	if m.Tokens == "" {
		m.Tokens = "unavailable"
	}
	m.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	m.Version = version
	b, _ := json.Marshal(m)
	f, e := os.OpenFile(filepath.Join(r.Private, "metrics.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e == nil {
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
}
func execCmd(args []string) error {
	f := flag.NewFlagSet("exec", flag.ContinueOnError)
	cache := f.Bool("cache", false, "cache with explicit inputs")
	verified := f.Bool("verified", false, "record that an external check verified this result")
	var inputs multiFlag
	f.Var(&inputs, "input", "declared dependency")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() == 0 {
		return errors.New("exec [--cache --input PATH ...] -- command [args]")
	}
	command := f.Args()
	r, e := discover()
	if e != nil {
		return e
	}
	ensure(r)
	if *cache && len(inputs) == 0 {
		return errors.New("cache requires at least one --input")
	}
	canonical := canonicalInputs(r.Root, inputs)
	key, fps, e := cacheKeyAtRoot(r.Root, command, canonical)
	if e != nil {
		return e
	}
	cp := filepath.Join(r.Private, "cache", key+".json")
	if *cache {
		var rec cacheRecord
		if readJSON(cp, &rec) == nil && validCacheRecord(rec, key, command, canonical, fps) {
			fmt.Print(projection([]byte(rec.Result), rec.Log))
			recordMetric(r, metric{Operation: "exec", Parameters: command, Dependencies: canonical, Result: "success", Verified: rec.Verified, Cached: true})
			return nil
		}
	}
	start := time.Now()
	c := exec.Command(command[0], command[1:]...)
	c.Dir = r.Root
	b, e := c.CombinedOutput()
	log := saveRun(r, "exec", b)
	code := 0
	if e != nil {
		code = 1
		if x, ok := e.(*exec.ExitError); ok {
			code = x.ExitCode()
		}
	}
	fmt.Print(projection(b, log))
	recordMetric(r, metric{Operation: "exec", Parameters: command, Dependencies: canonical, Result: map[bool]string{true: "success", false: "failure"}[e == nil], Verified: *verified && e == nil, DurationMS: time.Since(start).Milliseconds(), ProjectionBytes: len(projection(b, log))})
	if *cache && e == nil {
		_ = os.MkdirAll(filepath.Dir(cp), 0700)
		result := string(b)
		_ = writeJSON(cp, cacheRecord{Key: key, Primitive: "exec", PrimitiveVersion: cacheSchema, Parameters: command, Dependencies: canonical, DependencyFingerprints: fps, Result: result, ResultFingerprint: digest([]byte(result)), Log: log, ExitCode: code, Verified: *verified, Created: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	return e
}

func validCacheRecord(rec cacheRecord, key string, command, inputs []string, fps map[string]string) bool {
	if rec.Key != key || rec.Primitive != "exec" || rec.PrimitiveVersion != cacheSchema || rec.ExitCode != 0 || rec.Created == "" || rec.Log == "" {
		return false
	}
	if rec.ResultFingerprint == "" || rec.ResultFingerprint != digest([]byte(rec.Result)) {
		return false
	}
	if !sameStrings(rec.Parameters, command) || !sameStrings(rec.Dependencies, inputs) {
		return false
	}
	if len(rec.DependencyFingerprints) != len(fps) {
		return false
	}
	for p, fp := range fps {
		if rec.DependencyFingerprints[p] != fp {
			return false
		}
	}
	return true
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

func compileCmd(args []string) error {
	f := flag.NewFlagSet("compile", flag.ContinueOnError)
	min := f.Int("min", 3, "minimum successful repetitions")
	js := f.Bool("json", false, "json")
	if e := f.Parse(args); e != nil {
		return e
	}
	if *min < 1 {
		return errors.New("min must be positive")
	}
	r, e := discover()
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(r.Private, "metrics.jsonl"))
	if e != nil {
		if os.IsNotExist(e) {
			fmt.Println("no trajectories")
			return nil
		}
		return e
	}
	var records []metric
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		var m metric
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Operation != "exec" || m.Result != "success" {
			continue
		}
		records = append(records, m)
	}
	out := exactCandidates(records, *min, r.Root)
	if *js {
		return jsonPrint(map[string]any{"candidates": out, "promoted": false})
	}
	if len(out) == 0 {
		fmt.Println("no conservative candidates")
		return nil
	}
	fmt.Println("candidates (review before turning into reusable primitives):")
	for _, x := range out {
		fmt.Println(x)
	}
	return nil
}

func exactCandidates(records []metric, min int, root string) []any {
	type group struct {
		Command      []string
		Count        int
		Dependencies []string
	}
	groups := map[string]*group{}
	for _, m := range records {
		if m.Operation != "exec" || m.Result != "success" || !m.Verified {
			continue
		}
		deps := canonicalInputs(root, m.Dependencies)
		if len(deps) == 0 {
			continue
		}
		keyBytes, _ := json.Marshal(struct {
			Command      []string `json:"command"`
			Dependencies []string `json:"dependencies"`
		}{m.Parameters, deps})
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			g = &group{Command: append([]string(nil), m.Parameters...), Dependencies: deps}
			groups[key] = g
		}
		g.Count++
	}
	var out []any
	for _, g := range groups {
		if g.Count >= min {
			out = append(out, map[string]any{"status": "candidate", "operation": "exec", "parameters": g.Command, "repetitions": g.Count, "dependencies": g.Dependencies, "reason": "repeated exact command with explicit dependencies; review required; no automatic promotion"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]) < fmt.Sprint(out[j]) })
	return out
}

func integrateCmd(args []string) error {
	if len(args) != 1 || (args[0] != "codex" && args[0] != "remove-codex") {
		return errors.New("integrate requires codex|remove-codex")
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		home = os.Getenv("HOME")
		if home == "" {
			return errors.New("HOME unavailable")
		}
		home = filepath.Join(home, ".codex")
	}
	p := filepath.Join(home, "AGENTS.md")
	b, _ := os.ReadFile(p)
	s := string(b)
	start, end := strings.Index(s, markerStart), strings.Index(s, markerEnd)
	if args[0] == "remove-codex" {
		if start >= 0 && end > start {
			s = s[:start] + s[end+len(markerEnd):]
			s = strings.Replace(s, "\n\n", "\n", 1)
			return os.WriteFile(p, []byte(s), 0600)
		}
		return nil
	}
	block := markerStart + "\n" + codexText + "\n" + markerEnd
	if start >= 0 && end > start {
		s = s[:start] + block + s[end+len(markerEnd):]
	} else {
		if len(s) > 0 && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += block + "\n"
	}
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		return e
	}
	fmt.Println("Codex integration installed")
	return nil
}
func doctorCmd() error {
	r, e := discover()
	if e != nil {
		fmt.Println("repository: unavailable (fail open)")
		return nil
	}
	fmt.Printf("repository: %s\nprivate state: %s\nrg: %s\ngit: %s\n", r.Root, r.Private, look("rg"), look("git"))
	return nil
}

func uninstallCmd() error {
	if err := integrateCmd([]string{"remove-codex"}); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	exe, _ = filepath.EvalSymlinks(exe)
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	target := filepath.Join(home, ".local", "bin", "krn")
	if filepath.Clean(exe) == filepath.Clean(target) {
		_ = os.Remove(target)
		fmt.Println("krn uninstalled")
	}
	return nil
}
func look(s string) string {
	if p, e := exec.LookPath(s); e == nil {
		return p
	}
	return "unavailable"
}
