package context

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"example.com/krn/internal/state"
	"example.com/krn/internal/workspace"
)

func ecosystem(r workspace.Repo) []string {
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

func Run(args []string) error {
	f := flag.NewFlagSet("context", flag.ContinueOnError)
	js := f.Bool("json", false, "json")
	if err := f.Parse(args); err != nil {
		return err
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	workspace.Ensure(r)
	head, _ := workspace.Git(r.Root, "rev-parse", "HEAD")
	branch, _ := workspace.Git(r.Root, "branch", "--show-current")
	changes, _ := workspace.Git(r.Root, "status", "--porcelain=v1")
	paths := []string{}
	for _, line := range strings.Split(strings.TrimRight(changes, "\n"), "\n") {
		if len(line) > 3 {
			paths = append(paths, strings.TrimSpace(line[3:]))
		}
	}
	s := state.File{}
	stateErr := workspace.ReadJSON(filepath.Join(r.Private, "state.json"), &s)
	v := map[string]any{"root": r.Root, "git_dir": r.GitDir, "head": strings.TrimSpace(head), "branch": strings.TrimSpace(branch), "changed": paths, "ecosystems": ecosystem(r), "state": s}
	if stateErr != nil && !os.IsNotExist(stateErr) {
		v["state"] = nil
		v["state_status"] = "unknown"
	}
	if *js {
		return workspace.PrintJSON(v)
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
