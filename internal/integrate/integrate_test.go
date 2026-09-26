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

// The routing policy showed no measured gain on Claude Code, so it is opt-in:
// install.sh installs the pi extension and leaves AGENTS.md and CLAUDE.md alone.
func TestInstallScriptInstallsPiExtensionAndNoPolicy(t *testing.T) {
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
	// HOME is redirected, so keep Go's module cache on the real one: a module
	// cache under the temp HOME is read-only and breaks TempDir cleanup.
	modCache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	piDir := filepath.Join(installHome, ".pi", "agent")
	if err := os.MkdirAll(piDir, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "install.sh")
	cmd.Dir = testutil.ModuleRoot(t)
	cmd.Env = append(os.Environ(), "GOMODCACHE="+strings.TrimSpace(string(modCache)), "HOME="+installHome, "CODEX_HOME="+installCodexHome, "CLAUDE_CONFIG_DIR="+filepath.Join(installHome, ".claude"), "PI_CODING_AGENT_DIR="+piDir, "PATH="+localBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, output)
	}
	for _, p := range []string{filepath.Join(installCodexHome, "AGENTS.md"), filepath.Join(installHome, ".claude", "CLAUDE.md")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("install.sh wrote %s; the routing policy is opt-in (err=%v)", p, err)
		}
	}
	ext, err := os.ReadFile(filepath.Join(piDir, "extensions", "krn", "index.ts"))
	if err != nil || !strings.Contains(string(ext), piExtensionMarker) {
		t.Fatalf("install.sh did not install the pi extension: %v", err)
	}
}

func TestUninstallRemovesOnlyKRnPiExtension(t *testing.T) {
	d := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", d)
	ext := filepath.Join(d, "extensions", "krn")
	if err := os.MkdirAll(ext, 0700); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(ext, "index.ts")
	if err := os.WriteFile(index, []byte("// a user's own extension\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removePiExtension(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("removed an extension that is not KRn's: %v", err)
	}
	if err := os.WriteFile(index, []byte("/**\n * "+piExtensionMarker+" — test\n */\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removePiExtension(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ext); !os.IsNotExist(err) {
		t.Fatalf("KRn pi extension still present: %v", err)
	}
}

func TestClaudeIntegrationUsesClaudeConfigAndIsReversible(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", d)
	p := filepath.Join(d, "CLAUDE.md")
	if err := os.WriteFile(p, []byte("user instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"claude"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if strings.Count(s, markerStart) != 1 || !strings.Contains(s, ClaudeText) || !strings.HasPrefix(s, "user instructions\n") {
		t.Fatalf("unexpected Claude integration: %q", s)
	}
	if strings.Contains(ClaudeText, "Codex") || !strings.Contains(ClaudeText, "krn context --json") {
		t.Fatalf("Claude policy is not addressed to Claude Code: %q", ClaudeText)
	}
	if err := Run([]string{"remove-claude"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if string(b) != "user instructions\n" {
		t.Fatalf("removal changed user content: %q", b)
	}
}
