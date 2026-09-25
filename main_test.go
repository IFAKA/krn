package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileFingerprintChangesOnlyDeclaredInput(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a.txt")
	b := filepath.Join(d, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	one, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	two, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("unrelated file changed fingerprint: %s != %s", one, two)
	}
	if err := os.WriteFile(a, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	three, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if one == three {
		t.Fatal("declared input change did not change fingerprint")
	}
}

func TestCacheKeyCanonicalizesDependencyOrderAndDuplicates(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a.txt")
	b := filepath.Join(d, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	one, fpsOne, err := cacheKeyAtRoot(d, []string{"printf", "ok"}, []string{"b.txt", "a.txt", "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	two, fpsTwo, err := cacheKeyAtRoot(d, []string{"printf", "ok"}, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if one != two || len(fpsOne) != 2 || len(fpsTwo) != 2 {
		t.Fatalf("dependency canonicalization failed: %q %q %#v %#v", one, two, fpsOne, fpsTwo)
	}
}

func TestMalformedOrMismatchedCacheRecordIsRejected(t *testing.T) {
	fps := map[string]string{"/repo/input.txt": "hash"}
	base := cacheRecord{
		Key: "key", Primitive: "exec", PrimitiveVersion: cacheSchema,
		Parameters: []string{"printf", "ok"}, Dependencies: []string{"/repo/input.txt"},
		DependencyFingerprints: fps, Result: "ok", ResultFingerprint: digest([]byte("ok")), Log: "/repo/run.log", ExitCode: 0, Created: "now",
	}
	if !validCacheRecord(base, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("valid cache record rejected")
	}
	bad := base
	bad.DependencyFingerprints = map[string]string{"/repo/input.txt": "wrong"}
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("stale dependency fingerprint accepted")
	}
	bad = base
	bad.Parameters = []string{"printf", "different"}
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("different command accepted")
	}
	bad = base
	bad.PrimitiveVersion = "1"
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("old cache schema accepted")
	}
	bad = base
	bad.Result = "tampered"
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("tampered cached result accepted")
	}
}

func TestIntegrationPreservesUserContentAndIsIdempotent(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CODEX_HOME", d)
	p := filepath.Join(d, "AGENTS.md")
	if err := os.WriteFile(p, []byte("user instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := integrateCmd([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if strings.Count(string(first), markerStart) != 1 || strings.Count(string(first), markerEnd) != 1 {
		t.Fatalf("fresh integration did not install exactly one managed block: %q", first)
	}
	if err := integrateCmd([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Fatal("integration is not idempotent")
	}
	if string(second)[:len("user instructions")] != "user instructions" {
		t.Fatal("user content changed")
	}
	if err := integrateCmd([]string{"remove-codex"}); err != nil {
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
	if err := integrateCmd([]string{"codex"}); err != nil {
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
	if strings.Contains(s, "old policy") || !strings.Contains(s, codexText) || !strings.Contains(s, "before\n") || !strings.Contains(s, "after\n") {
		t.Fatalf("stale or unrelated content was mishandled: %q", s)
	}
	if err := integrateCmd([]string{"remove-codex"}); err != nil {
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
	if err := integrateCmd([]string{"codex"}); err != nil {
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
	if err := integrateCmd([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	direct, err := os.ReadFile(filepath.Join(directHome, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	installHome := t.TempDir()
	installCodexHome := t.TempDir()
	cmd := exec.Command("sh", "install.sh")
	cmd.Dir, _ = os.Getwd()
	cmd.Env = append(os.Environ(), "HOME="+installHome, "CODEX_HOME="+installCodexHome)
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

func TestMalformedStateFailsClosedAtReader(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state.json")
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	var s stateFile
	if err := readJSON(p, &s); err == nil {
		t.Fatal("malformed state accepted")
	}
}

func TestProjectionIsBounded(t *testing.T) {
	got := projection([]byte(strings.Repeat("x", 20000)), "/tmp/full.log")
	if len(got) > 12100 {
		t.Fatalf("projection too large: %d", len(got))
	}
	if !strings.Contains(got, "full.log") {
		t.Fatal("projection lost recoverable log reference")
	}
}

func TestStateUsesDocumentedArgumentOrder(t *testing.T) {
	d := t.TempDir()
	if err := exec.Command("git", "-C", d, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := stateCmd([]string{"set", "objective", "keep", "it", "small"}); err != nil {
		t.Fatal(err)
	}
	var got stateFile
	if err := readJSON(filepath.Join(d, ".git", "krn", "state.json"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Objective != "keep it small" {
		t.Fatalf("objective = %q", got.Objective)
	}
	if err := stateCmd([]string{"add", "negative", "no", "compiler"}); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(d, ".git", "krn", "state.json"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Negative) != 1 || got.Negative[0] != "no compiler" {
		t.Fatalf("negative = %#v", got.Negative)
	}
}

func TestFindAcceptsDocumentedFlagOrder(t *testing.T) {
	got := normalizeFindArgs([]string{"auth", "--json", "--max-files", "3"})
	want := []string{"--json", "--max-files", "3", "auth"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCodeEditsTopLevelGoDeclarationAndPreservesUnrelatedSource(t *testing.T) {
	d := t.TempDir()
	initTestRepo(t, d)
	file := filepath.Join(d, "odd name.go")
	original := "package sample\n\nimport \"fmt\"\n\nvar keep = 1\n\nfunc Target() { fmt.Println(\"old\") }\n"
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := codeCmd([]string{"read", "--file", "odd name.go", "--entity", "Target"}); err != nil {
		t.Fatal(err)
	}
	if err := codeCmd([]string{"replace", "--file", "odd name.go", "--entity", "Target", "--content", "func Target() { fmt.Println(\"new\") }"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "package sample\n\nimport \"fmt\"\n\nvar keep = 1\n\nfunc Target() { fmt.Println(\"new\") }\n"
	if string(got) != want {
		t.Fatalf("edited source = %q, want %q", got, want)
	}
	if err := codeCmd([]string{"insert-before", "--file", "odd name.go", "--entity", "Target", "--content", "func Before() {}"}); err != nil {
		t.Fatal(err)
	}
	if err := codeCmd([]string{"insert-after", "--file", "odd name.go", "--entity", "Target", "--content", "func After() {}"}); err != nil {
		t.Fatal(err)
	}
	if err := codeCmd([]string{"remove", "--file", "odd name.go", "--entity", "Before"}); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "func Before") || !strings.Contains(string(got), "func After") || !strings.Contains(string(got), "var keep = 1") {
		t.Fatalf("unexpected final source: %q", got)
	}
	if err := codeCmd([]string{"insert-after", "--file", "odd name.go", "--entity", "Target", "--content", "func After() {}"}); err != nil {
		t.Fatal(err)
	}
	gotAgain, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotAgain) != string(got) {
		t.Fatal("repeating an identical insertion was not idempotent")
	}
}

func TestCodeRejectsAmbiguousMissingMalformedAndUnsupportedEditsWithoutCorruption(t *testing.T) {
	d := t.TempDir()
	initTestRepo(t, d)
	file := filepath.Join(d, "main.go")
	original := "package sample\n\nfunc A() {}\nfunc A() {}\nfunc B() {}\n"
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	for _, args := range [][]string{
		{"replace", "--file", "main.go", "--entity", "A", "--content", "func A() {"},
		{"replace", "--file", "main.go", "--entity", "Missing", "--content", "func Missing() {}"},
		{"replace", "--file", "main.go", "--entity", "A", "--content", "func C() {}"},
	} {
		if err := codeCmd(args); err == nil {
			t.Fatalf("expected code edit to fail: %v", args)
		}
		got, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != original {
			t.Fatalf("failed edit corrupted source: %q", got)
		}
	}
	if err := codeCmd([]string{"replace", "--file", "main.go", "--entity", "B", "--content", "const C = 1"}); err == nil {
		t.Fatal("expected unsupported replacement kind to fail")
	}
}

func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
}
