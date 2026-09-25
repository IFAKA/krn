package code

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/krn/internal/testutil"
)

func TestCodeUsesAstGrepByteRangesAndPreservesMode(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "main.ts")
	source := "const keep = 1;\nfunction target() { return 1; }\n"
	if err := os.WriteFile(path, []byte(source), 0750); err != nil {
		t.Fatal(err)
	}
	testutil.FakeAstGrep(t, `[ {"text":"function target() { return 1; }","range":{"byteOffset":{"start":16,"end":47}}} ]`)
	testutil.Chdir(t, d)
	if err := Run([]string{"replace", "--file", "main.ts", "--pattern", "function target() { $$$BODY }", "--lang", "typescript", "--content", "function target() { return 2; }"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "const keep = 1;\nfunction target() { return 2; }\n" {
		t.Fatalf("edited source = %q", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0750 {
		t.Fatalf("mode changed to %o", info.Mode().Perm())
	}
}

func TestCodeRejectsZeroMultipleInvalidAndStaleMatches(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "main.js")
	original := "function target() {}\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, d)
	for _, fixture := range []string{"[]", `[{"text":"function target() {}","range":{"byteOffset":{"start":0,"end":21}}},{"text":"function target() {}","range":{"byteOffset":{"start":0,"end":21}}}]`, `[{"text":"wrong","range":{"byteOffset":{"start":0,"end":21}}}]`, `[{"text":"function target() {}","range":{"byteOffset":{"start":0,"end":99}}}]`} {
		testutil.FakeAstGrep(t, fixture)
		if err := Run([]string{"replace", "--file", "main.js", "--pattern", "function target() {}", "--content", "function target() { return 1; }"}); err == nil {
			t.Fatalf("expected fixture to fail: %s", fixture)
		}
		got, _ := os.ReadFile(path)
		if string(got) != original {
			t.Fatalf("source changed after rejected edit: %q", got)
		}
	}
}

func TestCodeValidationFailureAndIdempotentInsertion(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "script.sh")
	original := "function target() {}\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, d)
	testutil.FakeAstGrep(t, `[{"text":"function target() {}","language":"Bash","range":{"byteOffset":{"start":0,"end":20}}}]`)
	if err := Run([]string{"replace", "--file", "script.sh", "--pattern", "function target() {}", "--content", "function target() { broken"}); err == nil {
		t.Fatal("expected validation failure")
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatal("validation failure changed source")
	}
	inserted := "function helper() {}"
	if err := Run([]string{"insert-after", "--file", "script.sh", "--pattern", "function target() {}", "--content", inserted}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if err := Run([]string{"insert-after", "--file", "script.sh", "--pattern", "function target() {}", "--content", inserted}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("identical insertion was not idempotent")
	}
}

func TestCodeMissingAstGrepIsActionable(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", bin)
	t.Cleanup(func() { t.Setenv("PATH", oldPath) })
	testutil.Chdir(t, d)
	path := filepath.Join(d, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err = Run([]string{"read", "--file", "main.go", "--pattern", "package $NAME"})
	if err == nil || !strings.Contains(err.Error(), "install it separately") {
		t.Fatalf("error = %v", err)
	}
}
