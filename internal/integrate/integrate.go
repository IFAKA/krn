package integrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const markerStart = "<!-- KRN:BEGIN -->"

const markerEnd = "<!-- KRN:END -->"

const policyTemplate = `KRn is available globally as a deterministic repository substrate. Decide routing before the first shell or file-reading tool call.

Routing decision:
- If the task requires learning the repository before answering or editing, the first operation MUST be ` + "`krn context --json`" + `. Repository orientation includes questions about what the project is, how it is structured, where behavior is implemented, and code changes whose target is not already known.
- After context, if more repository evidence is needed, prefer ` + "`krn find QUERY --json --max-files N`" + ` before broad manual traversal, repeated search, or reading many files. Use the bounded projection and follow its recoverable log path when necessary.
- For mechanical code or configuration edits that can be expressed as a supported ast-grep structural pattern, prefer ` + "`krn code read|replace|insert-before|insert-after|remove`" + ` as ` + "`krn code replace --file PATH --pattern PATTERN --content TEXT [--lang LANG]`" + ` (the pattern must match exactly once; content is inserted verbatim, so indent it as it should appear) over sed, perl, Python, or other ad-hoc text-editing commands. Use KRn after deciding the pattern and replacement, and do not use it for semantic decisions.
- For verification, prefer ` + "`krn verify --level fast|full --json`" + ` when its discovered checks fit the task; otherwise run the project-native focused command directly.
- Use ` + "`krn exec --cache --input PATH ... -- COMMAND ...`" + ` only for deterministic repeated commands with explicit input dependencies (files or directories, one ` + "`--input`" + ` per path); the command after ` + "`--`" + ` runs without a shell, so wrap pipelines or ` + "`&&`" + ` as ` + "`sh -c '...'`" + `; add ` + "`--verified`" + ` only after an external check verified the result.
- Use ` + "`krn state`" + ` only for irreducible durable facts: objective, constraints, proven facts, open questions, or negative results that are not cheaply reconstructable from Git/files/tests.

Bypass the orientation rule only when repository inspection is unnecessary or KRn cannot provide the evidence: a trivial answer, an exact known file/content named by the user, an explicit user-requested shell command, one clearly sufficient cheap direct operation, or unavailable KRn. Do not dump large KRn logs into context. Do not invoke KRn for every task; preserve these bypasses.

The model remains responsible for semantic decisions and edits. KRn does not replace AGENT, add a hook or daemon, or mutate AGENT configuration.`

// CodexText and ClaudeText are the same routing policy addressed to each agent.
var (
	CodexText  = strings.ReplaceAll(policyTemplate, "AGENT", "Codex")
	ClaudeText = strings.ReplaceAll(policyTemplate, "AGENT", "Claude Code")
)

type target struct {
	name, text, envVar, homeDir, file string
}

var targets = map[string]target{
	"codex":  {"Codex", CodexText, "CODEX_HOME", ".codex", "AGENTS.md"},
	"claude": {"Claude Code", ClaudeText, "CLAUDE_CONFIG_DIR", ".claude", "CLAUDE.md"},
}

func Run(args []string) error {
	if len(args) != 1 {
		return errors.New("integrate requires codex|remove-codex|claude|remove-claude")
	}
	name, remove := strings.CutPrefix(args[0], "remove-")
	t, ok := targets[name]
	if !ok {
		return errors.New("integrate requires codex|remove-codex|claude|remove-claude")
	}
	p, err := t.path()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	s := string(b)
	if remove {
		var found bool
		s, found = rewriteManagedBlocks(s, "")
		if found {
			return os.WriteFile(p, []byte(s), 0600)
		}
		return nil
	}
	block := managedBlock(t.text)
	if rewritten, found := rewriteManagedBlocks(s, block); found {
		s = rewritten
	} else {
		if len(s) > 0 && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += block + "\n"
	}
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		return e
	}
	fmt.Println(t.name + " integration installed")
	return nil
}

func (t target) path() (string, error) {
	home := os.Getenv(t.envVar)
	if home == "" {
		home = os.Getenv("HOME")
		if home == "" {
			return "", errors.New("HOME unavailable")
		}
		home = filepath.Join(home, t.homeDir)
	}
	return filepath.Join(home, t.file), nil
}

func ManagedBlock() string {
	return managedBlock(CodexText)
}

func managedBlock(text string) string {
	return markerStart + "\n" + text + "\n" + markerEnd
}

// rewriteManagedBlocks replaces the first well-formed KRn block and removes
// later KRn blocks. Malformed marker pairs are left untouched as user content.
func rewriteManagedBlocks(s, replacement string) (string, bool) {
	var out strings.Builder
	cursor := 0
	found := false
	for cursor < len(s) {
		relStart := strings.Index(s[cursor:], markerStart)
		if relStart < 0 {
			break
		}
		start := cursor + relStart
		relEnd := strings.Index(s[start+len(markerStart):], markerEnd)
		if relEnd < 0 {
			break
		}
		end := start + len(markerStart) + relEnd + len(markerEnd)
		out.WriteString(s[cursor:start])
		replaced := false
		if !found {
			if replacement != "" {
				out.WriteString(replacement)
				replaced = true
			}
			found = true
		}
		// A newline directly owned by the managed block keeps removal from
		// leaving an extra blank line and keeps replacement byte-stable.
		if end < len(s) && s[end] == '\n' {
			end++
			if replaced {
				out.WriteByte('\n')
			}
		}
		cursor = end
	}
	out.WriteString(s[cursor:])
	return out.String(), found
}

func Uninstall() error {
	for _, name := range []string{"remove-codex", "remove-claude"} {
		if err := Run([]string{name}); err != nil {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	exe, _ = filepath.EvalSymlinks(exe)
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	target := filepath.Join(home, ".local", "bin", "krn")
	if filepath.Clean(exe) == filepath.Clean(target) {
		_ = os.Remove(target)
		fmt.Println("krn uninstalled")
	}
	return nil
}
