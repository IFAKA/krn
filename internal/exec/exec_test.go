package exec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileFingerprintChangesOnlyDeclaredInput(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a.txt")
	b := filepath.Join(d, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	one, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	two, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("unrelated file changed fingerprint: %s != %s", one, two)
	}
	if err := os.WriteFile(a, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	three, err := fileFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	if one == three {
		t.Fatal("declared input change did not change fingerprint")
	}
}

func TestCacheKeyCanonicalizesDependencyOrderAndDuplicates(t *testing.T) {
	d := t.TempDir()
	a := filepath.Join(d, "a.txt")
	b := filepath.Join(d, "b.txt")
	if err := os.WriteFile(a, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	one, fpsOne, err := cacheKeyAtRoot(d, []string{"printf", "ok"}, []string{"b.txt", "a.txt", "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	two, fpsTwo, err := cacheKeyAtRoot(d, []string{"printf", "ok"}, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if one != two || len(fpsOne) != 2 || len(fpsTwo) != 2 {
		t.Fatalf("dependency canonicalization failed: %q %q %#v %#v", one, two, fpsOne, fpsTwo)
	}
}

func TestMalformedOrMismatchedCacheRecordIsRejected(t *testing.T) {
	fps := map[string]string{"/repo/input.txt": "hash"}
	base := cacheRecord{
		Key: "key", Primitive: "exec", PrimitiveVersion: cacheSchema,
		Parameters: []string{"printf", "ok"}, Dependencies: []string{"/repo/input.txt"},
		DependencyFingerprints: fps, Result: "ok", ResultFingerprint: digest([]byte("ok")), Log: "/repo/run.log", ExitCode: 0, Created: "now",
	}
	if !validCacheRecord(base, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("valid cache record rejected")
	}
	bad := base
	bad.DependencyFingerprints = map[string]string{"/repo/input.txt": "wrong"}
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("stale dependency fingerprint accepted")
	}
	bad = base
	bad.Parameters = []string{"printf", "different"}
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("different command accepted")
	}
	bad = base
	bad.PrimitiveVersion = "1"
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("old cache schema accepted")
	}
	bad = base
	bad.Result = "tampered"
	if validCacheRecord(bad, "key", base.Parameters, base.Dependencies, fps) {
		t.Fatal("tampered cached result accepted")
	}
}
