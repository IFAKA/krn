package main

import (
	"os"
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
		"krn compile",
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

func TestCompilerRecognizesOnlyExactRepetition(t *testing.T) {
	records := []metric{
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"input.txt"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"input.txt"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"input.txt"}, Result: "success", Verified: true},
	}
	got := exactCandidates(records, 3, "/repo")
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want one", len(got))
	}
	if got[0].(map[string]any)["operation"] != "exec" {
		t.Fatalf("unexpected candidate: %#v", got[0])
	}
}

func TestCompilerDoesNotMergeDifferentCommandsOrDependencies(t *testing.T) {
	records := []metric{
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"a"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"b"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "different"}, Dependencies: []string{"a"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "different"}, Dependencies: []string{"b"}, Result: "success", Verified: true},
	}
	if got := exactCandidates(records, 3, "/repo"); len(got) != 0 {
		t.Fatalf("merged semantically different operations: %#v", got)
	}
}

func TestCompilerRequiresVerifiedExplicitDependencies(t *testing.T) {
	records := []metric{
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"input.txt"}, Result: "success", Verified: false},
		{Operation: "exec", Parameters: []string{"printf", "ok"}, Dependencies: []string{"input.txt"}, Result: "failure", Verified: true},
	}
	if got := exactCandidates(records, 1, "/repo"); len(got) != 0 {
		t.Fatalf("accepted unproven execution: %#v", got)
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

func TestFindAcceptsDocumentedFlagOrder(t *testing.T) {
	got := normalizeFindArgs([]string{"auth", "--json", "--max-files", "3"})
	want := []string{"--json", "--max-files", "3", "auth"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
