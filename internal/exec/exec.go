package exec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"example.com/krn/internal/workspace"
)

const cacheSchema = "2"

type cacheRecord struct {
	Key, Primitive, PrimitiveVersion string
	Parameters                       []string
	Dependencies                     []string
	DependencyFingerprints           map[string]string
	Result, ResultFingerprint, Log   string
	ExitCode                         int
	Verified                         bool
	Created                          string
}

func fileFingerprint(path string) (string, error) {
	h := sha256.New()
	info, e := os.Stat(path)
	if e != nil {
		return "", e
	}
	if !info.IsDir() {
		b, e := os.ReadFile(path)
		if e != nil {
			return "", e
		}
		h.Write(b)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	var files []string
	if e := filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if i.IsDir() {
			if p != path && (i.Name() == ".git" || i.Name() == "node_modules" || i.Name() == "dist" || i.Name() == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if i.Mode().IsRegular() {
			files = append(files, p)
		}
		return nil
	}); e != nil {
		return "", e
	}
	sort.Strings(files)
	for _, p := range files {
		rel, _ := filepath.Rel(path, p)
		h.Write([]byte(rel))
		b, e := os.ReadFile(p)
		if e != nil {
			return "", e
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func cacheKeyAtRoot(root string, command []string, inputs []string) (string, map[string]string, error) {
	h := sha256.New()
	h.Write([]byte("exec/" + cacheSchema + "\x00"))
	h.Write([]byte(filepath.Clean(root) + "\x00"))
	for _, a := range command {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	fps := map[string]string{}
	canonical := canonicalInputs(root, inputs)
	for _, abs := range canonical {
		fp, e := fileFingerprint(abs)
		if e != nil {
			return "", nil, e
		}
		fps[abs] = fp
		h.Write([]byte(abs + "\x00" + fp))
	}
	return hex.EncodeToString(h.Sum(nil)), fps, nil
}

func canonicalInputs(root string, inputs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range inputs {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, p)
		}
		abs = filepath.Clean(abs)
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	sort.Strings(out)
	return out
}

func Run(args []string) error {
	f := flag.NewFlagSet("exec", flag.ContinueOnError)
	cache := f.Bool("cache", false, "cache with explicit inputs")
	verified := f.Bool("verified", false, "record that an external check verified this result")
	var inputs multiFlag
	f.Var(&inputs, "input", "declared dependency")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() == 0 {
		return errors.New("exec [--cache --input PATH ...] -- command [args]")
	}
	command := f.Args()
	r, e := workspace.Discover()
	if e != nil {
		return e
	}
	workspace.Ensure(r)
	if *cache && len(inputs) == 0 {
		return errors.New("cache requires at least one --input")
	}
	var canonical []string
	var key string
	var fps map[string]string
	var cp string
	if *cache {
		canonical = canonicalInputs(r.Root, inputs)
		key, fps, e = cacheKeyAtRoot(r.Root, command, canonical)
		if e != nil {
			return e
		}
		cp = filepath.Join(r.Private, "cache", key+".json")
		var rec cacheRecord
		if workspace.ReadJSON(cp, &rec) == nil && validCacheRecord(rec, key, command, canonical, fps) {
			fmt.Print(workspace.Projection([]byte(rec.Result), rec.Log))
			workspace.RecordMetric(r, workspace.Metric{Operation: "exec", Parameters: command, Dependencies: canonical, Result: "success", Verified: rec.Verified, Cached: true})
			return nil
		}
	}
	start := time.Now()
	c := exec.Command(command[0], command[1:]...)
	c.Dir = r.Root
	b, e := c.CombinedOutput()
	log := workspace.SaveRun(r, "exec", b)
	code := 0
	if e != nil {
		code = 1
		if x, ok := e.(*exec.ExitError); ok {
			code = x.ExitCode()
		}
	}
	fmt.Print(workspace.Projection(b, log))
	workspace.RecordMetric(r, workspace.Metric{Operation: "exec", Parameters: command, Dependencies: canonical, Result: map[bool]string{true: "success", false: "failure"}[e == nil], Verified: *verified && e == nil, DurationMS: time.Since(start).Milliseconds(), ProjectionBytes: len(workspace.Projection(b, log))})
	if *cache && e == nil {
		_ = os.MkdirAll(filepath.Dir(cp), 0700)
		result := string(b)
		_ = workspace.WriteJSON(cp, cacheRecord{Key: key, Primitive: "exec", PrimitiveVersion: cacheSchema, Parameters: command, Dependencies: canonical, DependencyFingerprints: fps, Result: result, ResultFingerprint: digest([]byte(result)), Log: log, ExitCode: code, Verified: *verified, Created: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	return e
}

func validCacheRecord(rec cacheRecord, key string, command, inputs []string, fps map[string]string) bool {
	if rec.Key != key || rec.Primitive != "exec" || rec.PrimitiveVersion != cacheSchema || rec.ExitCode != 0 || rec.Created == "" || rec.Log == "" {
		return false
	}
	if rec.ResultFingerprint == "" || rec.ResultFingerprint != digest([]byte(rec.Result)) {
		return false
	}
	if !sameStrings(rec.Parameters, command) || !sameStrings(rec.Dependencies, inputs) {
		return false
	}
	if len(rec.DependencyFingerprints) != len(fps) {
		return false
	}
	for p, fp := range fps {
		if rec.DependencyFingerprints[p] != fp {
			return false
		}
	}
	return true
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }
