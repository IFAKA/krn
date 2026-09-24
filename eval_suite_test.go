package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureMaterializationIsIndependentAndDeterministic(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	a, err := materializeFixture(filepath.Join("eval", "fixture"), first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := materializeFixture(filepath.Join("eval", "fixture"), second)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("fixture commits differ: %s != %s", a, b)
	}
	if err := os.WriteFile(filepath.Join(first, "config", "limits.json"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(second, "config", "limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "changed\n" {
		t.Fatal("materialized fixture repositories are not independent")
	}
}

func TestDetailedTelemetryCountsCompletedCommandsOnce(t *testing.T) {
	data := []byte(`{"type":"item.started","item":{"id":"x","type":"command_execution","command":"printf ok"}}
{"type":"item.completed","item":{"id":"x","type":"command_execution","command":"printf ok"}}
{"type":"item.completed","item":{"id":"y","type":"message"}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"output_tokens":4,"reasoning_output_tokens":2,"total_tokens":16}}`)
	got := parseDetailedTelemetry(data)
	if got.ToolExecutions != 1 || got.CommandExecutions != 1 || got.AgentMessages != 1 {
		t.Fatalf("completed events were not counted once: %+v", got)
	}
	if !got.InputKnown || got.Input != 10 || !got.CachedKnown || got.CachedInput != 3 || !got.OutputKnown || got.Output != 4 || !got.ReasoningKnown || got.Reasoning != 2 || !got.TotalKnown || got.Total != 16 {
		t.Fatalf("usage fields were not preserved: %+v", got)
	}
	if len(got.Unavailable()) == 0 || got.Unavailable()[0] != "uncached_input_tokens" {
		t.Fatalf("unavailable telemetry was not explicit: %#v", got.Unavailable())
	}
}

func TestOptionalMetricDoesNotInventTelemetry(t *testing.T) {
	if got := optionalInt(42, false); got != "unavailable" {
		t.Fatalf("invented unavailable metric: %v", got)
	}
	if got := optionalInt(42, true); got != int64(42) {
		t.Fatalf("known metric changed type: %v", got)
	}
}
