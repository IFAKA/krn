package workspace

import (
	"strings"
	"testing"
)

func TestProjectionIsBounded(t *testing.T) {
	got := Projection([]byte(strings.Repeat("x", 20000)), "/tmp/full.log")
	if len(got) > 12100 {
		t.Fatalf("projection too large: %d", len(got))
	}
	if !strings.Contains(got, "full.log") {
		t.Fatal("projection lost recoverable log reference")
	}
}
