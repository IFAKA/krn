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

func TestCompilerRecognizesScopedRepetitionConservatively(t *testing.T) {
	records := []metric{
		{Operation: "exec", Parameters: []string{"npm", "test", "auth"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"npm", "test", "cart"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"npm", "test", "checkout"}, Result: "success", Verified: true},
	}
	got := parameterCandidates(records, 3)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want one", len(got))
	}
	if got[0].(map[string]any)["operation"] != "npm test(scope)" {
		t.Fatalf("unexpected candidate: %#v", got[0])
	}
}

func TestCompilerDoesNotMergeDifferentPrefixes(t *testing.T) {
	records := []metric{
		{Operation: "exec", Parameters: []string{"npm", "test", "auth"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"npm", "test", "cart"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"npm", "lint", "auth"}, Result: "success", Verified: true},
		{Operation: "exec", Parameters: []string{"npm", "lint", "cart"}, Result: "success", Verified: true},
	}
	got := parameterCandidates(records, 3)
	if len(got) != 0 {
		t.Fatalf("merged semantically different operations: %#v", got)
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
