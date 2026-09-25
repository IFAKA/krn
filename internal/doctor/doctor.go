package doctor

import (
	"fmt"
	"os/exec"
	"strings"

	"example.com/krn/internal/workspace"
)

func Run() error {
	r, e := workspace.Discover()
	if e != nil {
		fmt.Printf("repository: unavailable (fail open)\nast-grep: %s\n", astGrepStatus())
		return nil
	}
	fmt.Printf("repository: %s\nprivate state: %s\nrg: %s\ngit: %s\nast-grep: %s\n", r.Root, r.Private, look("rg"), look("git"), astGrepStatus())
	return nil
}

func astGrepStatus() string {
	path, err := exec.LookPath("ast-grep")
	if err != nil {
		return "unavailable"
	}
	b, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s (version unavailable)", path)
	}
	version := strings.TrimSpace(string(b))
	if version == "" {
		version = "version unavailable"
	}
	return fmt.Sprintf("%s (%s)", path, version)
}

func look(s string) string {
	if p, e := exec.LookPath(s); e == nil {
		return p
	}
	return "unavailable"
}
