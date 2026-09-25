package verify

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"example.com/krn/internal/workspace"
)

func discoverChecks(r workspace.Repo, level string) [][]string {
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

func Run(args []string) error {
	f := flag.NewFlagSet("verify", flag.ContinueOnError)
	level := f.String("level", "fast", "fast or full")
	js := f.Bool("json", false, "json")
	if e := f.Parse(args); e != nil {
		return e
	}
	if *level != "fast" && *level != "full" {
		return errors.New("level must be fast or full")
	}
	r, e := workspace.Discover()
	if e != nil {
		return e
	}
	workspace.Ensure(r)
	checks := discoverChecks(r, *level)
	out := map[string]any{"level": *level, "checks": []any{}, "verified": false}
	if len(checks) == 0 {
		out["status"] = "unknown"
		out["message"] = "no verification commands safely discovered"
		if *js {
			return workspace.PrintJSON(out)
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
		log := workspace.SaveRun(r, "verify", b)
		ok := err == nil
		all = all && ok
		x := map[string]any{"command": a, "passed": ok, "status": map[bool]string{true: "PASS", false: "FAIL"}[ok], "duration_ms": time.Since(start).Milliseconds(), "full_log": log}
		workspace.RecordMetric(r, workspace.Metric{Operation: "verify", Parameters: a, Result: map[bool]string{true: "success", false: "failure"}[ok], Verified: ok, DurationMS: time.Since(start).Milliseconds(), ProjectionBytes: len(workspace.Tail(b, 4000))})
		if !ok {
			x["evidence"] = workspace.Tail(b, 4000)
		}
		list = append(list, x)
	}
	out["checks"] = list
	out["verified"] = all
	out["status"] = map[bool]string{true: "passed", false: "failed"}[all]
	if *js {
		return workspace.PrintJSON(out)
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
