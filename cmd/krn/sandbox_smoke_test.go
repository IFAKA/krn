package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/krn/internal/integrate"
	"example.com/krn/internal/testutil"
)

func TestSandboxedCLIEndToEndWithFakeAstGrep(t *testing.T) {
	root := t.TempDir()
	testutil.InitRepo(t, root)
	toolDir := t.TempDir()
	tool := filepath.Join(toolDir, "ast-grep")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "ast-grep 0.1.0"; exit 0; fi
if [ "$1" = "scan" ]; then cat >/dev/null; printf '[]'; exit 0; fi
last=""
for last do :; done
case "$last" in
  */replace.js)
    if grep -q 'return 2' "$last"; then text='function target() { return 2; }'; else text='function target() {}'; fi
    start=$(grep -bo -m1 "$text" "$last" | cut -d: -f1); end=$((start + ${#text}))
    printf '[{"text":"%s","language":"JavaScript","range":{"byteOffset":{"start":%s,"end":%s}}}]' "$text" "$start" "$end" ;;
  */insert.js)
    text='function target() {}'; start=$(grep -bo -m1 "$text" "$last" | cut -d: -f1); end=$((start + 20))
    printf '[{"text":"%s","language":"JavaScript","range":{"byteOffset":{"start":%s,"end":%s}}}]' "$text" "$start" "$end" ;;
  */remove.js) printf '%s' '[{"text":"function deleteMe() {}","language":"JavaScript","range":{"byteOffset":{"start":0,"end":22}}}]' ;;
  *) echo unknown-file >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	krn := filepath.Join(t.TempDir(), "krn")
	build := exec.Command("go", "build", "-o", krn, ".")
	build.Dir = mustGetwd(t)
	build.Env = append(os.Environ(), "GOCACHE="+t.TempDir())
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}
	pathEnv := toolDir + string(os.PathListSeparator) + os.Getenv("PATH")
	run := func(args ...string) string {
		cmd := exec.Command(krn, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+pathEnv)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("krn %v failed: %v\n%s", args, err, output)
		}
		return string(output)
	}
	write := func(name, source string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	replace := write("replace.js", "function target() {}\n")
	if got := run("code", "read", "--file", "replace.js", "--pattern", "function target() {}"); got != "function target() {}" {
		t.Fatalf("read output = %q", got)
	}
	run("code", "replace", "--file", "replace.js", "--pattern", "function target() {}", "--content", "function target() { return 2; }")
	if got, _ := os.ReadFile(replace); string(got) != "function target() { return 2; }\n" {
		t.Fatalf("replace result = %q", got)
	}
	insert := write("insert.js", "function target() {}\n")
	run("code", "insert-before", "--file", "insert.js", "--pattern", "function target() {}", "--content", "function before() {}")
	run("code", "insert-after", "--file", "insert.js", "--pattern", "function target() {}", "--content", "function after() {}")
	first, _ := os.ReadFile(insert)
	run("code", "insert-after", "--file", "insert.js", "--pattern", "function target() {}", "--content", "function after() {}")
	second, _ := os.ReadFile(insert)
	if string(first) != string(second) || !strings.Contains(string(second), "function before() {}") {
		t.Fatalf("insert result is not preserved/idempotent: %q", second)
	}
	remove := write("remove.js", "function deleteMe() {}\n")
	run("code", "remove", "--file", "remove.js", "--pattern", "function deleteMe() {}")
	if got, _ := os.ReadFile(remove); string(got) != "\n" {
		t.Fatalf("remove result = %q", got)
	}
}

func TestInstallerInstallsAstGrepInSandbox(t *testing.T) {
	root := testutil.ModuleRoot(t)
	home := t.TempDir()
	fakeBin := t.TempDir()
	goTool := filepath.Join(fakeBin, "go")
	npmTool := filepath.Join(fakeBin, "npm")
	goScript := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then shift; cp /usr/bin/true "$1"; chmod 755 "$1"; exit 0; fi
  shift
done
exit 2
`
	npmScript := `#!/bin/sh
prefix=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--prefix" ]; then shift; prefix="$1"; fi
  shift
done
mkdir -p "$prefix/bin"
cp /usr/bin/true "$prefix/bin/ast-grep"
chmod 755 "$prefix/bin/ast-grep"
`
	for path, content := range map[string]string{goTool: goScript, npmTool: npmScript} {
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", "install.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home, "CODEX_HOME="+filepath.Join(home, "codex"), "PATH="+fakeBin+string(os.PathListSeparator)+"/usr/bin:/bin")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sandbox installer failed: %v\n%s", err, output)
	}
	info, err := os.Stat(filepath.Join(home, ".local", "bin", "ast-grep"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("installed ast-grep is not executable: %o", info.Mode().Perm())
	}
}

func TestRealAstGrepCLIEndToEndInSandbox(t *testing.T) {
	if _, err := exec.LookPath("ast-grep"); err != nil {
		t.Skipf("real ast-grep unavailable: %v", err)
	}
	root := t.TempDir()
	testutil.InitRepo(t, root)
	krn := filepath.Join(t.TempDir(), "krn")
	build := exec.Command("go", "build", "-o", krn, ".")
	build.Dir = mustGetwd(t)
	build.Env = append(os.Environ(), "GOCACHE="+t.TempDir())
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}
	run := func(args ...string) {
		cmd := exec.Command(krn, args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("krn %v failed: %v\n%s", args, err, output)
		}
	}
	check := func(name, want string) {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}

	write := func(name, source string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("sample.js", "function target() { return 1; }\n")
	run("code", "replace", "--file", "sample.js", "--pattern", "function target() { return 1; }", "--content", "function target() { return 2; }")
	check("sample.js", "function target() { return 2; }\n")

	write("sample.ts", "function target() { return 1; }\n")
	run("code", "replace", "--file", "sample.ts", "--pattern", "function target() { return 1; }", "--content", "function target() { return 2; }")
	check("sample.ts", "function target() { return 2; }\n")

	write("sample.sh", "function deploy() { echo old; }\n")
	run("code", "replace", "--file", "sample.sh", "--pattern", "function deploy() { echo old; }", "--content", "function deploy() { echo new; }")
	check("sample.sh", "function deploy() { echo new; }\n")

	write(".gitlab-ci.yml", "build: npm run build\n")
	run("code", "replace", "--file", ".gitlab-ci.yml", "--lang", "yaml", "--pattern", "build: npm run build", "--content", "build: npm run test")
	check(".gitlab-ci.yml", "build: npm run test\n")

	write("sample.go", "package main\n\nfunc target() { return 1 }\n")
	run("code", "replace", "--file", "sample.go", "--pattern", "func target() { return 1 }", "--content", "func target() { return 2 }")
	check("sample.go", "package main\n\nfunc target() { return 2 }\n")

	write("sample.css", "body { color: red; }\n")
	run("code", "replace", "--file", "sample.css", "--lang", "css", "--pattern", "body { color: red; }", "--content", "body { color: blue; }")
	check("sample.css", "body { color: blue; }\n")

	write("sample.scss", "$color: red;\n")
	run("code", "replace", "--file", "sample.scss", "--lang", "css", "--pattern", "$color: red;", "--content", "$color: blue;")
	check("sample.scss", "$color: blue;\n")

	reject := func(name string, args ...string) {
		before, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(krn, append([]string{"code", "replace", "--file", name}, args...)...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("krn accepted invalid edit to %s:\n%s", name, output)
		}
		check(name, string(before))
	}
	write("broken.ts", "const keep = 1;\nfunction target() { return 1; }\nfunction other() { return 3; }\n")
	reject("broken.ts", "--pattern", "function target() { return 1; }", "--content", "function target() { return (((; ")
	write("broken.go", "package main\n\nfunc A() {}\n")
	reject("broken.go", "--pattern", "func A() {}", "--content", "func A() { if {")
	reject("broken.go", "--pattern", "func A() {}", "--content", "func A() {")
	write("broken.yml", "a: 1\nb: 2\n")
	reject("broken.yml", "--lang", "yaml", "--pattern", "b: 2", "--content", "b: [")
	// Pre-existing errors do not block unrelated edits.
	write("preexisting.ts", "function target() { return 1; }\n}\n")
	run("code", "replace", "--file", "preexisting.ts", "--pattern", "function target() { return 1; }", "--content", "function target() { return 2; }")
	check("preexisting.ts", "function target() { return 2; }\n}\n")
}

func TestCodexAutomaticallyRoutesStructuralEditThroughKRn(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skipf("codex unavailable: %v", err)
	}
	root := t.TempDir()
	testutil.InitRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "target.js"), []byte("function target() { return 1; }\n// preserve this comment\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(integrate.ManagedBlock()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	krn := filepath.Join(t.TempDir(), "krn")
	build := exec.Command("go", "build", "-o", krn, ".")
	build.Dir = mustGetwd(t)
	build.Env = append(os.Environ(), "GOCACHE="+t.TempDir())
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}
	pathEnv := filepath.Dir(krn) + string(os.PathListSeparator) + os.Getenv("PATH")
	prompt := "Make the requested mechanical edit in this repository: change target.js so function target returns 2 instead of 1. Preserve the unrelated comment and whitespace. Do not ask questions; complete and verify the edit."
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "exec", "--json", "--ephemeral", "--model", "gpt-5.6-luna", "--approve-for-me", "--skip-git-repo-check", "-C", root)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(prompt + "\n")
	cmd.Env = append(os.Environ(), "PATH="+pathEnv)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("codex routing run failed: %v\n%s", err, output.String())
	}
	got, err := os.ReadFile(filepath.Join(root, "target.js"))
	if err != nil {
		t.Fatal(err)
	}
	want := "function target() { return 2; }\n// preserve this comment\n"
	if string(got) != want {
		t.Fatalf("target.js = %q, want %q\nCodex output:\n%s", got, want, output.String())
	}
	if !strings.Contains(output.String(), "krn code") {
		t.Fatalf("Codex changed the file without showing a krn code invocation\n%s", output.String())
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
