package repomap

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/krn/internal/testutil"
	"example.com/krn/internal/workspace"
)

func fixture(t *testing.T) workspace.Repo {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"README.md":        "# Demo\n\nA tiny timer app for tests.\n",
		"src/timers.js":    "import { REST_MS } from './constants.js';\nexport function startRest(a) {\n  a.restEndsAt = Date.now() + REST_MS;\n}\nexport function formatDuration(ms) { return String(ms); }\n",
		"src/constants.js": "export const REST_MS = 90000;\n",
		"src/view.js":      "import { startRest, formatDuration } from './timers.js';\nexport function renderRest() { startRest({}); return formatDuration(1); }\n",
		"src/unrelated.js": "export function parseCsv(text) { return text.split(','); }\n",
		"tests/x.test.js":  "const now = 1;\n",
		"tools/report.py":  "def build_report(rows):\n    return len(rows)\n",
		"cmd/main.go":      "package main\n\ntype Server struct{}\n\nfunc main() { _ = Server{} }\n",
	}
	for p, s := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	testutil.InitRepo(t, dir)
	c := exec.Command("git", "-C", dir, "add", ".")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, b)
	}
	testutil.Chdir(t, dir)
	r, err := workspace.Discover()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMapRanksFocusAndFitsBudget(t *testing.T) {
	r := fixture(t)
	out, err := Build(r, "where is the rest end time computed", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 200*bytesPerToken {
		t.Fatalf("map exceeds budget: %d bytes", len(out))
	}
	if !strings.Contains(out, "A tiny timer app") {
		t.Fatalf("missing README blurb:\n%s", out)
	}
	ti, ui := strings.Index(out, "src/timers.js:"), strings.Index(out, "src/unrelated.js:")
	if ti < 0 || (ui >= 0 && ui < ti) {
		t.Fatalf("timers.js should rank above unrelated.js:\n%s", out)
	}
	if !strings.Contains(out, "export function startRest(a)") {
		t.Fatalf("missing signature:\n%s", out)
	}
	if strings.Contains(out, "const now") {
		t.Fatalf("test-file definitions should be skipped:\n%s", out)
	}
	for _, want := range []string{"def build_report(rows):", "type Server struct"} {
		if !strings.Contains(out+mustBuild(t, r, "", 2000), want) {
			t.Fatalf("missing %q from Python/Go extraction", want)
		}
	}
}

func TestMapIsDeterministicAndCached(t *testing.T) {
	r := fixture(t)
	a := mustBuild(t, r, "rest timer", 300)
	if _, err := os.Stat(filepath.Join(r.Private, "cache", "map", "tags.json")); err != nil {
		t.Fatalf("tag cache not written: %v", err)
	}
	b := mustBuild(t, r, "rest timer", 300)
	if a != b {
		t.Fatalf("map not deterministic:\n%s\n---\n%s", a, b)
	}
}

func mustBuild(t *testing.T, r workspace.Repo, focus string, tokens int) string {
	t.Helper()
	out, err := Build(r, focus, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
