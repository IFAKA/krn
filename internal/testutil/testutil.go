package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func Chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func InitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
}

func FakeAstGrep(t *testing.T, fixture string) {
	t.Helper()
	d := t.TempDir()
	tool := filepath.Join(d, "ast-grep")
	// scan reports one ERROR node per "broken" line on stdin, mimicking
	// tree-sitter error recovery; run prints the fixture.
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'ast-grep 0.1.0'; exit 0; fi\nif [ \"$1\" = scan ]; then\n  n=$(grep -c broken)\n  printf '['; i=0; while [ \"$i\" -lt \"$n\" ]; do [ \"$i\" -gt 0 ] && printf ','; printf '{\"text\":\"broken\"}'; i=$((i+1)); done; printf ']'\n  exit 0\nfi\nprintf '%s' \"$FAKE_AST_JSON\"\n"
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	old := os.Getenv("PATH")
	t.Setenv("PATH", d+string(os.PathListSeparator)+old)
	t.Setenv("FAKE_AST_JSON", fixture)
}

// ModuleRoot returns the directory containing go.mod, where install.sh lives.
func ModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
