package code

import (
	"fmt"
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

func TestCodeLiteralReplaceTargetsUniqueTextInEmbeddedSyntax(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "index.html")
	source := "<script>\nif (e.key === \"ArrowRight\") next();\n</script>\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	testutil.FakeAstGrep(t, "[]")
	testutil.Chdir(t, d)
	if err := Run([]string{
		"replace",
		"--file", "index.html",
		"--pattern", `e.key === "ArrowRight"`,
		"--literal",
		"--lang", "html",
		"--content", `e.key === "ArrowRight" || e.key === "j"`,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "<script>\nif (e.key === \"ArrowRight\" || e.key === \"j\") next();\n</script>\n"
	if string(got) != want {
		t.Fatalf("edited source = %q, want %q", got, want)
	}
}

func TestCodeLiteralReplaceRejectsAmbiguousText(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "notes.txt")
	original := "same\nsame\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, d)
	err := Run([]string{
		"replace",
		"--file", "notes.txt",
		"--pattern", "same",
		"--literal",
		"--lang", "text",
		"--content", "changed",
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous (2 matches)") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("source changed after ambiguous match: %q", got)
	}
}

func TestLiteralNoMatchHintsWhenDecodedEscapesMatchExactlyOnce(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		pattern string
	}{
		{name: "unicode apostrophes", source: `'open'`, pattern: `\u0027open\u0027`},
		{name: "escaped quotes", source: `say "hello"`, pattern: `say \"hello\"`},
		{name: "escaped backslash", source: `path\name`, pattern: `path\\name`},
		{name: "newline", source: "first\nsecond", pattern: `first\nsecond`},
		{name: "tab", source: "key\tvalue", pattern: `key\tvalue`},
		{name: "hex and unicode", source: "A λ 🙂", pattern: `\x41 \u03bb \U0001F642`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveLiteralMatch([]byte(tt.source), tt.pattern, "text")
			if err == nil || !strings.Contains(err.Error(), "after decoding backslash escapes, the pattern matches exactly once; pass the decoded characters directly or adjust shell quoting") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLiteralNoMatchKeepsOrdinaryErrorWhenDecodedCandidateIsNotUnique(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		pattern string
	}{
		{name: "malformed escape", source: `'open'`, pattern: `\uZZZZ`},
		{name: "unrelated decoded pattern", source: `'open'`, pattern: `\u0027closed\u0027`},
		{name: "ambiguous decoded pattern", source: "'open' and 'open'", pattern: `\u0027open\u0027`},
		{name: "unchanged pattern", source: `'open'`, pattern: `missing`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveLiteralMatch([]byte(tt.source), tt.pattern, "text")
			want := fmt.Sprintf("literal pattern %q matched no text", tt.pattern)
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestLiteralEscapeHintDoesNotChangeFailedMutation(t *testing.T) {
	d := t.TempDir()
	testutil.InitRepo(t, d)
	path := filepath.Join(d, "notes.txt")
	original := "status = 'open'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, d)
	err := Run([]string{
		"replace",
		"--file", "notes.txt",
		"--pattern", `\u0027open\u0027`,
		"--literal",
		"--lang", "text",
		"--content", "closed",
	})
	if err == nil || !strings.Contains(err.Error(), "after decoding backslash escapes, the pattern matches exactly once") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("source changed after failed escaped match: %q", got)
	}
}

func TestLiteralValidPatternStillMatchesUnchanged(t *testing.T) {
	match, err := resolveLiteralMatch([]byte("before 'open' after"), `'open'`, "text")
	if err != nil {
		t.Fatal(err)
	}
	if match.Text != `'open'` || match.Range.ByteOffset.Start != 7 || match.Range.ByteOffset.End != 13 {
		t.Fatalf("match = %+v", match)
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
