package doctor

import (
	"strings"
	"testing"

	"example.com/krn/internal/testutil"
)

func TestDoctorReportsAstGrepVersion(t *testing.T) {
	testutil.FakeAstGrep(t, "[]")
	status := astGrepStatus()
	if !strings.Contains(status, "ast-grep 0.1.0") {
		t.Fatalf("ast-grep status = %q", status)
	}
}
