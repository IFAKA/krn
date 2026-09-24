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
		"Use it only when it is likely to reduce model context",
		"krn context --json",
		"krn find QUERY --json --max-files N",
		"krn verify --level fast|full --json",
		"krn exec --cache --input PATH",
		"krn state",
		"Skip KRn for trivial answers",
		"single known-file edits",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Fatalf("integration policy missing %q in:\n%s", want, s)
		}
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
