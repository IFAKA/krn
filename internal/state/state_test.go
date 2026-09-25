package state

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"example.com/krn/internal/workspace"
)

func TestMalformedStateFailsClosedAtReader(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state.json")
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	var s File
	if err := workspace.ReadJSON(p, &s); err == nil {
		t.Fatal("malformed state accepted")
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
	if err := Run([]string{"set", "objective", "keep", "it", "small"}); err != nil {
		t.Fatal(err)
	}
	var got File
	if err := workspace.ReadJSON(filepath.Join(d, ".git", "krn", "state.json"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Objective != "keep it small" {
		t.Fatalf("objective = %q", got.Objective)
	}
	if err := Run([]string{"add", "negative", "no", "compiler"}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.ReadJSON(filepath.Join(d, ".git", "krn", "state.json"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Negative) != 1 || got.Negative[0] != "no compiler" {
		t.Fatalf("negative = %#v", got.Negative)
	}
}
