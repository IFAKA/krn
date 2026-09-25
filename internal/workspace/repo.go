package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Repo struct{ Root, GitDir, Private string }

type Metric struct {
	Operation       string   `json:"operation"`
	Parameters      []string `json:"parameters,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Result          string   `json:"result"`
	Verified        bool     `json:"verified"`
	Cached          bool     `json:"cached"`
	DurationMS      int64    `json:"duration_ms"`
	ProjectionBytes int      `json:"projection_bytes,omitempty"`
	Tokens          string   `json:"codex_tokens,omitempty"`
	Timestamp       string   `json:"timestamp"`
	Version         string   `json:"version"`
}

func Discover() (Repo, error) {
	p, err := os.Getwd()
	if err != nil {
		return Repo{}, err
	}
	root, err := Git(p, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, errors.New("not inside a git repository")
	}
	root = strings.TrimSpace(root)
	gd, err := Git(p, "rev-parse", "--git-dir")
	if err != nil {
		return Repo{}, err
	}
	gd = strings.TrimSpace(gd)
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(root, gd)
	}
	r := Repo{Root: root, GitDir: filepath.Clean(gd)}
	r.Private = filepath.Join(r.GitDir, "krn")
	return r, nil
}

func Git(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, err := c.Output()
	return string(b), err
}

func Ensure(r Repo) error { return os.MkdirAll(filepath.Join(r.Private, "runs"), 0700) }

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func RecordMetric(r Repo, m Metric) {
	_ = Ensure(r)
	if m.Tokens == "" {
		m.Tokens = "unavailable"
	}
	m.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	m.Version = Version
	b, _ := json.Marshal(m)
	f, e := os.OpenFile(filepath.Join(r.Private, "metrics.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e == nil {
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
}
