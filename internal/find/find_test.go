package find

import (
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
