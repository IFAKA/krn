package eval

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"example.com/krn/internal/testutil"
)

func TestParseClaudeEventsCountsToolsAndKrnCalls(t *testing.T) {
	stream := `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"krn find rest --json"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"git log | head; krn map"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Grep","input":{"pattern":"krn "}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"grep -rn krnx ."}}]}}
{"type":"result","result":"timers.js:21","num_turns":4,"total_cost_usd":0.03,"usage":{"input_tokens":10,"cache_creation_input_tokens":90,"cache_read_input_tokens":500,"output_tokens":20}}
`
	res := piResult{ToolCalls: map[string]int{}}
	parseClaudeEvents(stream, &res)
	if res.Final != "timers.js:21" || res.Turns != 4 || res.CostUSD != 0.03 {
		t.Fatalf("result fields: %+v", res)
	}
	if res.UncachedIn != 100 || res.CachedIn != 500 || res.Output != 20 {
		t.Fatalf("usage: %+v", res)
	}
	if res.ToolCalls["Bash"] != 3 || res.ToolCalls["Grep"] != 1 || res.KrnCalls != 2 {
		t.Fatalf("tool counts: %+v krn=%d", res.ToolCalls, res.KrnCalls)
	}
}

func TestFileListFitsBudgetAndCountsTheRest(t *testing.T) {
	dir := t.TempDir()
	testutil.InitRepo(t, dir)
	for _, f := range []string{"a.go", "b.go", "c/d.go"} {
		p := filepath.Join(dir, f)
		_ = os.MkdirAll(filepath.Dir(p), 0700)
		if err := os.WriteFile(p, []byte("package x\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := exec.Command("git", "-C", dir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, b)
	}
	if got := fileList(dir, 100); got != "a.go\nb.go\nc/d.go\n" {
		t.Fatalf("full list: %q", got)
	}
	if got := fileList(dir, 10); got != "a.go\nb.go\n… 1 more files\n" || !strings.HasPrefix(got, "a.go") {
		t.Fatalf("clipped list: %q", got)
	}
}
