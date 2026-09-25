package code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"example.com/krn/internal/workspace"
)

type codeMatch struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Range    *struct {
		ByteOffset *struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"byteOffset"`
	} `json:"range"`
}

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("code read|replace|insert-before|insert-after|remove")
	}
	op := args[0]
	if op != "read" && op != "replace" && op != "insert-before" && op != "insert-after" && op != "remove" {
		return errors.New("unsupported code operation")
	}
	f := workspace.FlagSet("code")
	file := f.String("file", "", "source file")
	pattern := f.String("pattern", "", "ast-grep structural pattern")
	lang := f.String("lang", "", "ast-grep language")
	content := f.String("content", "", "replacement or inserted content")
	contentFile := f.String("content-file", "", "file containing replacement or inserted content")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *file == "" || *pattern == "" {
		return errors.New("code operation requires --file PATH --pattern PATTERN")
	}
	if op == "read" || op == "remove" {
		if *content != "" || *contentFile != "" {
			return errors.New("content is only valid for replace or insert operations")
		}
	} else if (*content == "") == (*contentFile == "") {
		return errors.New("replace or insert requires exactly one of --content or --content-file")
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	path, err := codePath(r.Root, *file)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if op == "read" {
		match, err := resolveCodeMatch(path, source, *pattern, *lang)
		if err != nil {
			return err
		}
		fmt.Print(match.Text)
		return nil
	}
	var replacement string
	if *contentFile != "" {
		b, err := os.ReadFile(*contentFile)
		if err != nil {
			return err
		}
		replacement = string(b)
	} else {
		replacement = *content
	}
	diff, changed, err := editCodeFile(path, source, *pattern, *lang, op, replacement)
	if err != nil {
		return err
	}
	if changed {
		fmt.Print(diff)
	}
	return nil
}

func codePath(root, file string) (string, error) {
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("code file must be inside the repository")
	}
	return path, nil
}

func astGrepPath() (string, error) {
	path, err := exec.LookPath("ast-grep")
	if err != nil {
		return "", errors.New("ast-grep is required for krn code; install it separately and ensure ast-grep is on PATH")
	}
	return path, nil
}

func runAstGrep(path, pattern, lang string) ([]codeMatch, error) {
	args := []string{"run", "--pattern", pattern}
	if lang != "" {
		args = append(args, "--lang", lang)
	}
	return execAstGrep(nil, append(args, "--json=compact", path)...)
}

// execAstGrep runs ast-grep with JSON output and decodes its matches. stdin,
// when non-nil, is passed to ast-grep for --stdin invocations.
func execAstGrep(stdin []byte, args ...string) ([]codeMatch, error) {
	tool, err := astGrepPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(tool, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr != nil {
		// ast-grep uses exit status 1 for a successful search with no
		// matches.
		if exit, ok := runErr.(*exec.ExitError); ok && exit.ExitCode() == 1 && strings.TrimSpace(stderr.String()) == "" {
			var noMatch []codeMatch
			if err := json.Unmarshal(stdout.Bytes(), &noMatch); err == nil {
				return noMatch, nil
			}
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("ast-grep failed: %w: %s", runErr, detail)
		}
		return nil, fmt.Errorf("ast-grep failed: %w", runErr)
	}
	var matches []codeMatch
	if err := json.Unmarshal(stdout.Bytes(), &matches); err != nil {
		return nil, fmt.Errorf("ast-grep returned malformed JSON: %w", err)
	}
	return matches, nil
}

// syntaxErrors counts tree-sitter ERROR nodes in source. Tree-sitter recovers
// from malformed input instead of failing, so parse errors only surface as
// ERROR nodes. Nodes the parser silently inserts (for example a missing
// closing brace) are not reported.
func syntaxErrors(source []byte, lang string) (int, error) {
	rule := "id: krn-syntax-error\nlanguage: " + lang + "\nrule:\n  kind: ERROR\n"
	matches, err := execAstGrep(source, "scan", "--stdin", "--inline-rules", rule, "--json=compact")
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}

// validateCodeEdit rejects an edit that introduces syntax errors. Existing
// errors are tolerated so already-imperfect files remain editable.
func validateCodeEdit(path string, oldSource, newSource []byte, lang string) error {
	if lang == "" {
		return errors.New("cannot validate edit: ast-grep did not report a language; pass --lang")
	}
	after, err := syntaxErrors(newSource, lang)
	if err != nil {
		return err
	}
	// The original only needs scanning when the candidate has errors.
	if after > 0 {
		before, err := syntaxErrors(oldSource, lang)
		if err != nil {
			return err
		}
		if after > before {
			return fmt.Errorf("edit introduces syntax errors (%d before, %d after)", before, after)
		}
	}
	if filepath.Ext(path) == ".go" {
		if _, err := parser.ParseFile(token.NewFileSet(), path, newSource, parser.SkipObjectResolution); err != nil {
			if _, oldErr := parser.ParseFile(token.NewFileSet(), path, oldSource, parser.SkipObjectResolution); oldErr == nil {
				return err
			}
		}
	}
	return nil
}

func resolveCodeMatch(path string, source []byte, pattern, lang string) (codeMatch, error) {
	matches, err := runAstGrep(path, pattern, lang)
	if err != nil {
		return codeMatch{}, err
	}
	if len(matches) == 0 {
		return codeMatch{}, fmt.Errorf("pattern %q matched no nodes", pattern)
	}
	if len(matches) != 1 {
		return codeMatch{}, fmt.Errorf("pattern %q is ambiguous (%d matches)", pattern, len(matches))
	}
	match := matches[0]
	if match.Range == nil || match.Range.ByteOffset == nil {
		return codeMatch{}, errors.New("ast-grep returned a match without a UTF-8 byte range")
	}
	start, end := match.Range.ByteOffset.Start, match.Range.ByteOffset.End
	if start < 0 || end < start || end > len(source) {
		return codeMatch{}, fmt.Errorf("ast-grep returned out-of-bounds byte range [%d,%d) for %s", start, end, path)
	}
	if string(source[start:end]) != match.Text {
		return codeMatch{}, fmt.Errorf("ast-grep returned a stale byte range for %s", path)
	}
	return match, nil
}

func editCodeFile(path string, source []byte, pattern, lang, op, replacement string) (string, bool, error) {
	target, err := resolveCodeMatch(path, source, pattern, lang)
	if err != nil {
		return "", false, err
	}
	start, end := target.Range.ByteOffset.Start, target.Range.ByteOffset.End
	var newSource []byte
	switch op {
	case "remove":
		newSource = spliceCode(source, start, end, "")
	case "replace":
		newSource = spliceCode(source, start, end, replacement)
	case "insert-before":
		inserted := replacement + "\n"
		if start >= len(inserted) && string(source[start-len(inserted):start]) == inserted {
			return "", false, nil
		}
		newSource = spliceCode(source, start, start, inserted)
	case "insert-after":
		inserted := "\n" + replacement
		if end+len(inserted) <= len(source) && string(source[end:end+len(inserted)]) == inserted {
			return "", false, nil
		}
		newSource = spliceCode(source, end, end, inserted)
	default:
		return "", false, errors.New("unsupported code operation")
	}
	if lang == "" {
		lang = target.Language
	}
	if err := validateCodeEdit(path, source, newSource, lang); err != nil {
		return "", false, fmt.Errorf("resulting source is invalid: %w", err)
	}
	if bytes.Equal(newSource, source) {
		return "", false, nil
	}
	diff, err := codeDiff(path, source, newSource)
	if err != nil {
		return "", false, err
	}
	if err := atomicCodeWrite(path, newSource); err != nil {
		return "", false, err
	}
	return diff, true, nil
}

func spliceCode(source []byte, start, end int, replacement string) []byte {
	out := make([]byte, 0, len(source)-(end-start)+len(replacement))
	out = append(out, source[:start]...)
	out = append(out, replacement...)
	out = append(out, source[end:]...)
	return out
}

func codeDiff(path string, oldSource, newSource []byte) (string, error) {
	dir, err := os.MkdirTemp("", "krn-code-diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	oldPath, newPath := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, oldSource, 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(newPath, newSource, 0600); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "diff", "--no-index", "--unified=3", oldPath, newPath)
	b, err := cmd.CombinedOutput()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return "", fmt.Errorf("generate diff: %w: %s", err, strings.TrimSpace(string(b)))
		}
	}
	lines := strings.Split(string(b), "\n")
	display := filepath.Base(path)
	if len(lines) >= 4 {
		lines[0] = "diff --git a/" + display + " b/" + display
		lines[2] = "--- a/" + display
		lines[3] = "+++ b/" + display
	}
	return strings.Join(lines, "\n"), nil
}

func atomicCodeWrite(path string, source []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".krn-code-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(source); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
