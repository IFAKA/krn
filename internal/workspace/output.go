package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func SaveRun(r Repo, name string, b []byte) string {
	if err := Ensure(r); err != nil {
		return ""
	}
	p := filepath.Join(r.Private, "runs", time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+name+".log")
	if len(b) > 0 {
		_ = os.WriteFile(p, b, 0600)
	}
	return p
}

func Tail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

func Projection(b []byte, log string) string {
	const limit = 12000
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + fmt.Sprintf("\n... output bounded; full_log: %s\n", log)
}

// PrintJSON writes compact JSON: --json output is read by agents, so
// indentation and HTML escaping only add context bytes.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
