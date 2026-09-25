package integrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/krn/internal/testutil"
)

func TestIntegrationPreservesUserContentAndIsIdempotent(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CODEX_HOME", d)
	p := filepath.Join(d, "AGENTS.md")
	if err := os.WriteFile(p, []byte("user instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if strings.Count(string(first), markerStart) != 1 || strings.Count(string(first), markerEnd) != 1 {
		t.Fatalf("fresh integration did not install exactly one managed block: %q", first)
	}
	if err := Run([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Fatal("integration is not idempotent")
	}
	if string(second)[:len("user instructions")] != "user instructions" {
		t.Fatal("user content changed")
	}
	if err := Run([]string{"remove-codex"}); err != nil {
		t.Fatal(err)
	}
	third, _ := os.ReadFile(p)
	if string(third) != "user instructions\n" {
		t.Fatalf("managed block removal changed user content: %q", third)
	}
}

func TestCodexReintegrationReplacesAllStaleManagedBlocks(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CODEX_HOME", d)
	p := filepath.Join(d, "AGENTS.md")
	stale := markerStart + "\nold policy\n" + markerEnd
	content := "before\n" + stale + "\nafter\n" + stale + "\n"
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Count(s, markerStart) != 1 || strings.Count(s, markerEnd) != 1 {
		t.Fatalf("reintegration left duplicate managed blocks: %q", s)
	}
	if strings.Contains(s, "old policy") || !strings.Contains(s, CodexText) || !strings.Contains(s, "before\n") || !strings.Contains(s, "after\n") {
		t.Fatalf("stale or unrelated content was mishandled: %q", s)
	}
	if err := Run([]string{"remove-codex"}); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before\nafter\n" {
		t.Fatalf("removal did not preserve unrelated content: %q", got)
	}
}

func TestCodexIntegrationInstallsOperationalRoutingPolicy(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CODEX_HOME", d)
	p := filepath.Join(d, "AGENTS.md")
	if err := Run([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	checks := []string{
		"Decide routing before the first shell or file-reading tool call",
		"the first operation MUST be `krn context --json`",
		"Repository orientation includes questions about what the project is",
		"krn context --json",
		"krn find QUERY --json --max-files N",
		"krn code read|replace|insert-before|insert-after|remove",
		"krn verify --level fast|full --json",
		"krn exec --cache --input PATH",
		"krn state",
		"Bypass the orientation rule only when repository inspection is unnecessary",
		"an exact known file/content named by the user",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Fatalf("integration policy missing %q in:\n%s", want, s)
		}
	}
}

func TestInstallScriptAndDirectIntegrationUseTheSamePolicy(t *testing.T) {
	directHome := t.TempDir()
	t.Setenv("CODEX_HOME", directHome)
	if err := Run([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	direct, err := os.ReadFile(filepath.Join(directHome, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	installHome := t.TempDir()
	installCodexHome := t.TempDir()
	localBin := filepath.Join(installHome, ".local", "bin")
	if err := os.MkdirAll(localBin, 0700); err != nil {
		t.Fatal(err)
	}
	fakeAstGrep := filepath.Join(localBin, "ast-grep")
	if err := os.WriteFile(fakeAstGrep, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "install.sh")
	cmd.Dir = testutil.ModuleRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+installHome, "CODEX_HOME="+installCodexHome, "PATH="+localBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(installCodexHome, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(direct) {
		t.Fatalf("install.sh and direct integration installed different policy:\ndirect=%q\nscript=%q", direct, installed)
	}
}
