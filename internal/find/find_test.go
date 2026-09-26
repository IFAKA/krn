package find

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindAcceptsDocumentedFlagOrder(t *testing.T) {
	got := normalizeFindArgs([]string{"auth", "--json", "--max-files", "3"})
	want := []string{"--json", "--max-files", "3", "auth"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestTermsSplitsIdentifiersAndDropsStopwords(t *testing.T) {
	got := strings.Join(Terms("Where is restEndsAt computed and what rule decides the duration?"), ",")
	want := "restEndsAt,rest,Ends,comput,rule,duration"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestShortTermsMatchOnlyAtBoundaries(t *testing.T) {
	cases := map[string]bool{"a.restEndsAt = x": true, "end_time": true, "items.append(x)": false, "render()": false}
	for text, want := range cases {
		if got := containsTerm(text, "end"); got != want {
			t.Errorf("containsTerm(%q, end) = %v, want %v", text, got, want)
		}
	}
}

func TestSearchRanksCoOccurringDefinitionAboveMentions(t *testing.T) {
	dir := t.TempDir()
	write := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("src/timers.js", "import { REST_MS } from './c.js';\nexport function startRest(a) {\n  a.restEndsAt = Date.now() + REST_MS;\n}\n")
	write("src/view.js", "import { startRest } from './timers.js';\n// rest view\nconst rest = 1;\nconst end = 2;\nconst time = 3;\n")
	write("docs/notes.md", "rest end time rest end time rest end time\n")
	write("tests/timers.test.js", "test('rest end time', () => { startRest({}); });\n")
	res, _, err := Search(dir, "where is the rest end time computed", false, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 || res.Files[0].Path != "src/timers.js" {
		t.Fatalf("top file = %+v, want src/timers.js", res.Files)
	}
	out := Render(res, 2400)
	if !strings.Contains(out, "> 3 (startRest)") {
		t.Fatalf("missing hit with enclosing symbol:\n%s", out)
	}
}

func TestRenderStaysWithinBudget(t *testing.T) {
	var files []File
	for i := 0; i < 5; i++ {
		files = append(files, File{Path: fmt.Sprintf("f%d.go", i), Hits: []Hit{{Line: 1, Text: strings.Repeat("x", 150), Before: []string{strings.Repeat("y", 150)}, After: []string{strings.Repeat("z", 150)}}}})
	}
	out := Render(Result{Terms: []string{"x"}, Files: files}, 1000)
	if len(out) > 1100 {
		t.Fatalf("render exceeded budget: %d bytes", len(out))
	}
	if !strings.Contains(out, "f4.go") && !strings.Contains(out, "(output bounded)") {
		t.Fatalf("expected degraded or bounded tail:\n%s", out)
	}
}
