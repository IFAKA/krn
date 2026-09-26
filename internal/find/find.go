package find

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"example.com/krn/internal/workspace"
)

// Output is shaped for agents: ranked files, the enclosing definition, and the
// best hit lines with a little context, bounded by a byte budget (~4 bytes per
// token). Location-only results cost agents follow-up reads; returning source
// lines is what lets an agent act on a result directly.
const (
	defaultMaxFiles = 5
	defaultBudget   = 2400
	hitsPerFile     = 3
	contextLines    = 2
	maxLineChars    = 160
	maxFileBytes    = 1 << 20
)

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("find requires a query")
	}
	f := flag.NewFlagSet("find", flag.ContinueOnError)
	js := f.Bool("json", false, "json")
	max := f.Int("max-files", defaultMaxFiles, "maximum files")
	budget := f.Int("budget", defaultBudget, "maximum output bytes for text output")
	regex := f.Bool("regex", false, "treat the query as one ripgrep regex")
	args = normalizeFindArgs(args)
	if err := f.Parse(args); err != nil {
		return err
	}
	if *max < 0 {
		return errors.New("max-files must be non-negative")
	}
	if f.NArg() == 0 {
		return errors.New("find requires a query")
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	q := strings.Join(f.Args(), " ")
	workspace.Ensure(r)
	start := time.Now()
	res, raw, searchErr := Search(r.Root, q, *regex, *max)
	log := workspace.SaveRun(r, "find", raw)
	status := "ok"
	if searchErr != nil {
		status = "unavailable"
	}
	if *js {
		return workspace.PrintJSON(map[string]any{"query": q, "terms": res.Terms, "matches": res.Matches, "files": res.Files, "full_log": log, "status": status, "duration_ms": time.Since(start).Milliseconds()})
	}
	fmt.Print(Render(res, *budget))
	if log != "" && res.Matches > 0 {
		fmt.Printf("full_log: %s\n", log)
	}
	if searchErr != nil {
		fmt.Printf("status: unavailable (%v)\n", searchErr)
	}
	return nil
}

type Hit struct {
	Line    int      `json:"line"`
	Text    string   `json:"text"`
	Symbol  string   `json:"symbol,omitempty"`
	Before  []string `json:"before,omitempty"`
	After   []string `json:"after,omitempty"`
	score   float64
	isDef   bool
	matched []string
}

type File struct {
	Path  string   `json:"file"`
	Score float64  `json:"score"`
	Terms []string `json:"terms"`
	Hits  []Hit    `json:"hits"`
}

type Result struct {
	Query   string   `json:"query"`
	Terms   []string `json:"terms"`
	Matches int      `json:"matches"`
	Files   []File   `json:"files"`
}

// Search runs one ripgrep pass for all query terms and ranks files by
// IDF-weighted distinct-term coverage, definition lines, and path matches.
// raw is the full ripgrep output, returned so callers can save it for recovery.
func Search(root, query string, regex bool, maxFiles int) (Result, []byte, error) {
	res := Result{Query: query}
	terms := []string{query}
	if !regex {
		terms = Terms(query)
	}
	res.Terms = terms
	if len(terms) == 0 {
		return res, nil, nil
	}
	wantsTests := strings.Contains(strings.ToLower(query), "test")
	args := []string{"-n", "--no-heading", "--hidden", "--max-columns", "400", "--max-count", "200", "-i"}
	for _, g := range excludedGlobs {
		args = append(args, "--glob", "!"+g)
	}
	if !regex {
		args = append(args, "-F")
	}
	for _, t := range terms {
		args = append(args, "-e", t)
	}
	args = append(args, ".")
	c := exec.Command("rg", args...)
	c.Dir = root
	raw, err := c.Output()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil // exit 1 means no matches
	}
	lines := map[string][]Hit{}
	termFiles := map[string]map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		n, e := strconv.Atoi(parts[1])
		if e != nil {
			continue
		}
		file := strings.TrimPrefix(parts[0], "./")
		text := parts[2]
		var matched []string
		for _, t := range terms {
			if regex || containsTerm(text, t) {
				matched = append(matched, t)
				if termFiles[t] == nil {
					termFiles[t] = map[string]bool{}
				}
				termFiles[t][file] = true
			}
		}
		if len(matched) == 0 {
			continue
		}
		res.Matches++
		lines[file] = append(lines[file], Hit{Line: n, Text: text, isDef: isDefinition(text), matched: matched})
	}
	nFiles := float64(len(lines))
	idf := map[string]float64{}
	for _, t := range terms {
		idf[t] = math.Log(1 + nFiles/float64(1+len(termFiles[t])))
		if len(splitIdent(t)) > 1 {
			idf[t] *= 2 // a whole compound identifier is far more specific than its parts
		}
	}
	var files []File
	for path, hits := range lines {
		covered := map[string]bool{}
		for i := range hits {
			h := &hits[i]
			for _, t := range h.matched {
				covered[t] = true
				h.score += idf[t]
			}
			// Terms co-occurring on one line are the strongest relevance signal.
			h.score *= 1 + 0.5*float64(len(h.matched)-1)
			if h.isDef {
				h.score *= 1.5
			}
			if importRe.MatchString(h.Text) {
				h.score *= 0.3 // imports name things; they rarely implement them
			}
		}
		// Score files by their best few lines, not by total volume, so large
		// files that mention every common word do not win by size alone.
		best := make([]float64, 0, len(hits))
		for _, h := range hits {
			best = append(best, h.score)
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(best)))
		score := 0.0
		for i, w := range []float64{1, 0.5, 0.25} {
			if i < len(best) {
				score += w * best[i]
			}
		}
		var ft []string
		lowPath := strings.ToLower(path)
		for _, t := range terms {
			if covered[t] {
				score += 0.3 * idf[t]
				ft = append(ft, t)
			}
			if strings.Contains(lowPath, strings.ToLower(t)) {
				score += 0.5 * idf[t]
			}
		}
		switch {
		case isNoise(path):
			score *= 0.5
		case !wantsTests && isTest(path):
			score *= 0.5
		}
		files = append(files, File{Path: path, Score: math.Round(score*100) / 100, Terms: ft, Hits: hits})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Score != files[j].Score {
			return files[i].Score > files[j].Score
		}
		return files[i].Path < files[j].Path
	})
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	for i := range files {
		files[i] = selectHits(root, files[i])
	}
	res.Files = files
	return res, raw, err
}

// selectHits keeps the best-scoring, non-overlapping lines, adds context, and
// names the enclosing definition of the best hit.
func selectHits(root string, f File) File {
	hits := f.Hits
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].Line < hits[j].Line
	})
	var picked []Hit
	for _, h := range hits {
		near := false
		for _, p := range picked {
			if abs(p.Line-h.Line) <= contextLines*2 {
				near = true
				break
			}
		}
		if !near {
			picked = append(picked, h)
		}
		if len(picked) == hitsPerFile {
			break
		}
	}
	src := readLines(filepath.Join(root, f.Path))
	for i := range picked {
		h := &picked[i]
		h.Text = clip(h.Text)
		if src == nil {
			continue
		}
		h.Symbol = enclosing(src, h.Line)
		for l := h.Line - contextLines; l < h.Line; l++ {
			if l >= 1 && l <= len(src) {
				h.Before = append(h.Before, clip(src[l-1]))
			}
		}
		for l := h.Line + 1; l <= h.Line+contextLines; l++ {
			if l >= 1 && l <= len(src) {
				h.After = append(h.After, clip(src[l-1]))
			}
		}
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].Line < picked[j].Line })
	f.Hits = picked
	return f
}

// Render prints the grep-shaped projection within budget bytes. Files that no
// longer fit degrade to location-only lines before anything is dropped.
func Render(res Result, budget int) string {
	var b strings.Builder
	if len(res.Files) == 0 {
		fmt.Fprintf(&b, "no matches for terms: %s\n", strings.Join(res.Terms, ", "))
		return b.String()
	}
	fmt.Fprintf(&b, "terms: %s (%d matching lines)\n", strings.Join(res.Terms, ", "), res.Matches)
	for _, f := range res.Files {
		sym := ""
		var s strings.Builder
		fmt.Fprintf(&s, "\n%s%s\n", f.Path, sym)
		for _, h := range f.Hits {
			for i, t := range h.Before {
				fmt.Fprintf(&s, "  %d  %s\n", h.Line-len(h.Before)+i, t)
			}
			owner := ""
			if h.Symbol != "" && !h.isDef {
				owner = " (" + h.Symbol + ")"
			}
			fmt.Fprintf(&s, "> %d%s  %s\n", h.Line, owner, h.Text)
			for i, t := range h.After {
				fmt.Fprintf(&s, "  %d  %s\n", h.Line+1+i, t)
			}
		}
		if budget <= 0 || b.Len()+s.Len() <= budget {
			b.WriteString(s.String())
			continue
		}
		var short strings.Builder
		fmt.Fprintf(&short, "\n%s%s: lines", f.Path, sym)
		for _, h := range f.Hits {
			fmt.Fprintf(&short, " %d", h.Line)
		}
		short.WriteString("\n")
		if b.Len()+short.Len() > budget {
			b.WriteString("\n(output bounded)\n")
			break
		}
		b.WriteString(short.String())
	}
	return b.String()
}

var excludedGlobs = []string{".git/**", "node_modules/**", "**/node_modules/**", "dist/**", "build/**", "vendor/**", "*.min.js", "*.min.css", "*.map", "*.lock", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.sum", "*.svg", "*.snap"}

// isNoise down-weights prose so code wins ties against docs mentioning it.
func isNoise(path string) bool {
	p := strings.ToLower(path)
	return strings.HasPrefix(p, "docs/") || strings.Contains(p, "/docs/") || strings.HasSuffix(p, ".md") || strings.Contains(p, "changelog")
}

func isTest(path string) bool {
	p := strings.ToLower(path)
	return strings.HasPrefix(p, "test") || strings.Contains(p, "/test") || strings.Contains(p, "_test.") || strings.Contains(p, ".test.") || strings.Contains(p, ".spec.") || strings.Contains(p, "__tests__")
}

// containsTerm matches case-insensitively; terms shorter than 5 characters must
// start at a word or camelCase boundary so "end" matches "restEndsAt" and
// "end_time" but not "append" or "render".
func containsTerm(text, term string) bool {
	low, lt := strings.ToLower(text), strings.ToLower(term)
	if len(lt) >= 5 {
		return strings.Contains(low, lt)
	}
	for from := 0; ; {
		i := strings.Index(low[from:], lt)
		if i < 0 {
			return false
		}
		i += from
		if i == 0 {
			return true
		}
		prev, cur := rune(text[i-1]), rune(text[i])
		if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) {
			return true
		}
		if unicode.IsLower(prev) && unicode.IsUpper(cur) {
			return true
		}
		from = i + 1
	}
}

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an and are as at be by can code does do each file files find for from function functions
		get has have how i if implemented implement in into is it its line lines me method of on or project repo repository set show
		that the their them then there these this to use used uses using was what when where which who why will with work works
		you your cite decide decides decided logic`) {
		stopwords[w] = true
	}
}

var identRe = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)

// Terms turns a natural-language or identifier query into literal search
// terms: compound identifiers are kept whole and split into camel/snake parts;
// plain words lose stopwords and common suffixes. Order is stable, deduplicated.
func Terms(q string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		k := strings.ToLower(t)
		if len(k) < 3 || stopwords[k] || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, t)
	}
	for _, tok := range identRe.FindAllString(q, -1) {
		if stopwords[strings.ToLower(tok)] {
			continue
		}
		parts := splitIdent(tok)
		if len(parts) > 1 {
			add(tok)
			for _, p := range parts {
				add(stem(p))
			}
			continue
		}
		add(stem(tok))
	}
	return out
}

func splitIdent(s string) []string {
	var parts []string
	var cur []rune
	rs := []rune(s)
	for i, r := range rs {
		if r == '_' || r == '$' {
			if len(cur) > 0 {
				parts = append(parts, string(cur))
				cur = nil
			}
			continue
		}
		if i > 0 && unicode.IsUpper(r) && len(cur) > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
			parts = append(parts, string(cur))
			cur = nil
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		parts = append(parts, string(cur))
	}
	return parts
}

func stem(w string) string {
	lw := strings.ToLower(w)
	for _, suf := range []string{"ation", "ing", "ed", "es", "s"} {
		if strings.HasSuffix(lw, suf) && len(lw)-len(suf) >= 4 {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

var (
	defRe = regexp.MustCompile(`^\s*(export\s+)?(default\s+)?(async\s+)?(pub(\([a-z]+\))?\s+)?(function\*?|class|def|func|interface|type|enum|struct|trait|impl|fn|const|let|var)\s+[A-Za-z_$(]`)
	// Object/class members and arrow functions: name(...) {, name = (...) =>, name: function
	memberRe = regexp.MustCompile(`^\s*(export\s+)?(const\s+|let\s+|var\s+|static\s+|async\s+|public\s+|private\s+|protected\s+|get\s+|set\s+)*([A-Za-z_$][\w$]*)\s*(\([^)]*\)\s*\{|[:=]\s*(async\s*)?(function\b|\([^)]*\)\s*=>|[A-Za-z_$][\w$]*\s*=>))`)
	// Owners for "in SYMBOL": callables and types, never plain local variables.
	ownerRe   = regexp.MustCompile(`^\s*(export\s+)?(default\s+)?(async\s+)?(pub(\([a-z]+\))?\s+)?(function\*?|class|def|func|interface|type|enum|struct|trait|impl|fn)\s+[A-Za-z_$(]`)
	importRe  = regexp.MustCompile(`^\s*(import\b|from\s+\S+\s+import\b|#include\b|use\s|require\()|^\s*(const|let|var)\s+.*=\s*require\(`)
	keywordRe = regexp.MustCompile(`^\s*(if|for|while|switch|catch|return|else|do|try|with)\b`)
	nameRe    = regexp.MustCompile(`(?:function\*?|class|def|func|interface|type|enum|struct|trait|impl|fn|const|let|var)\s+(?:\([^)]*\)\s*)?([A-Za-z_$][\w$]*)`)
)

func isDefinition(line string) bool {
	if keywordRe.MatchString(line) {
		return false
	}
	return defRe.MatchString(line) || memberRe.MatchString(line)
}

func definitionName(line string) string {
	if keywordRe.MatchString(line) {
		return ""
	}
	if m := nameRe.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	if m := memberRe.FindStringSubmatch(line); m != nil {
		return m[3]
	}
	return ""
}

func isOwner(line string) bool {
	return !keywordRe.MatchString(line) && (ownerRe.MatchString(line) || memberRe.MatchString(line))
}

// enclosing returns the nearest definition at or above line whose indentation
// does not exceed the line's own, so nested blocks resolve to their owner.
func enclosing(src []string, line int) string {
	if line < 1 || line > len(src) {
		return ""
	}
	limit := indentOf(src[line-1])
	for l := line; l >= 1 && line-l < 400; l-- {
		t := src[l-1]
		if strings.TrimSpace(t) == "" {
			continue
		}
		ind := indentOf(t)
		if ind <= limit && isOwner(t) {
			if n := definitionName(t); n != "" {
				return n
			}
		}
		if ind < limit {
			limit = ind
		}
	}
	return ""
}

func indentOf(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

func readLines(path string) []string {
	st, err := os.Stat(path)
	if err != nil || st.Size() > maxFileBytes {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(string(b), "\n")
}

func clip(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) > maxLineChars {
		return s[:maxLineChars] + "…"
	}
	return s
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func normalizeFindArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "--regex":
			flags = append(flags, a)
		case a == "--max-files" || a == "--budget":
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case strings.HasPrefix(a, "--max-files=") || strings.HasPrefix(a, "--budget="):
			flags = append(flags, a)
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}
