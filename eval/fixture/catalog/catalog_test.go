package catalog

import "testing"

func TestDefaultCatalogTotal(t *testing.T) {
	if got := Total(Default()); got != 2198 {
		t.Fatalf("total = %d, want 2198", got)
	}
}
