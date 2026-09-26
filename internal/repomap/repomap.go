// Package repomap builds an Aider-style repository map: tree-sitter definitions
// and references form a file graph, personalized PageRank ranks definitions by
// relevance to a focus text, and the top signatures are rendered within a token
// budget. Output is deterministic for a given tree and focus.
package repomap

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"example.com/krn/internal/find"
	"example.com/krn/internal/workspace"
)

const (
	defaultTokens = 800
	bytesPerToken = 4
	maxSourceSize = 256 << 10
	cacheSchema   = "repomap.tags.v2"
)

func Run(args []string) error {
	f := flag.NewFlagSet("map", flag.ContinueOnError)
	tokens := f.Int("tokens", defaultTokens, "token budget (about 4 bytes per token)")
	focus := f.String("focus", "", "task text whose identifiers and words bias the ranking")
	if err := f.Parse(args); err != nil {
		return err
	}
	r, err := workspace.Discover()
	if err != nil {
		return err
	}
	out, err := Build(r, *focus, *tokens)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

type fileTags struct {
	Path string
	Tags []Tag
}

// Build returns the rendered map for the repository at r.
func Build(r workspace.Repo, focus string, tokens int) (string, error) {
	if tokens <= 0 {
		return "", errors.New("tokens must be positive")
	}
	paths, blobs, dirty, err := listFiles(r.Root)
	if err != nil {
		return "", err
	}
	files := loadTags(r, paths, blobs)
	budget := tokens * bytesPerToken
	header := renderHeader(r.Root, paths)
	header = clipHeader(header, budget/3)
	defs := rank(files, find.Terms(focus), dirty)
	return header + fit(defs, r.Root, budget-len(header)), nil
}

// listFiles returns tracked and untracked (not ignored) source files, their Git
// blob ids for clean tracked files, and the set of dirty paths.
func listFiles(root string) ([]string, map[string]string, map[string]bool, error) {
	staged, err := workspace.Git(root, "ls-files", "-s", "-z")
	if err != nil {
		return nil, nil, nil, err
	}
	blobs := map[string]string{}
	for _, rec := range strings.Split(staged, "\x00") {
		// "<mode> <blob> <stage>\t<path>"
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		if parts := strings.Fields(meta); len(parts) == 3 {
			blobs[path] = parts[1]
		}
	}
	dirty := map[string]bool{}
	status, _ := workspace.Git(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	for _, rec := range strings.Split(status, "\x00") {
		if len(rec) > 3 {
			dirty[rec[3:]] = true
		}
	}
	all, err := workspace.Git(root, "ls-files", "-z", "-c", "-o", "--exclude-standard")
	if err != nil {
		return nil, nil, nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, p := range strings.Split(all, "\x00") {
		if p == "" || seen[p] || excluded(p) {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, blobs, dirty, nil
}

func excluded(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "node_modules", "dist", "build", "vendor", "coverage", ".next", "__pycache__", ".venv", "venv":
			return true
		}
	}
	return strings.Contains(p, ".min.")
}

// loadTags parses supported files, reusing cached tags for clean tracked files
// whose Git blob id is unchanged. Dirty and untracked files are always reparsed.
func loadTags(r workspace.Repo, paths []string, blobs map[string]string) []fileTags {
	dir := filepath.Join(r.Private, "cache", "map")
	cachePath := filepath.Join(dir, "tags.json")
	var cache struct {
		Schema string           `json:"schema"`
		Tags   map[string][]Tag `json:"tags"`
	}
	if workspace.ReadJSON(cachePath, &cache) != nil || cache.Schema != cacheSchema {
		cache.Tags = map[string][]Tag{}
	}
	cache.Schema = cacheSchema
	next := map[string][]Tag{}
	changed := false
	var out []fileTags
	for _, p := range paths {
		if !supported(p) {
			continue
		}
		key := ""
		if b := blobs[p]; b != "" && !isDirty(r.Root, p) {
			key = p + "@" + b
		}
		if key != "" {
			if t, ok := cache.Tags[key]; ok {
				next[key] = t
				out = append(out, fileTags{p, t})
				continue
			}
		}
		src, err := os.ReadFile(filepath.Join(r.Root, p))
		if err != nil || len(src) > maxSourceSize || minified(src) {
			continue
		}
		t := extract(p, src)
		if key != "" {
			next[key] = t
			changed = true
		}
		out = append(out, fileTags{p, t})
	}
	if changed || len(next) != len(cache.Tags) {
		cache.Tags = next
		if os.MkdirAll(dir, 0700) == nil {
			_ = workspace.WriteJSON(cachePath, cache)
		}
	}
	return out
}

var dirtyCache map[string]bool

func isDirty(root, p string) bool {
	if dirtyCache == nil {
		dirtyCache = map[string]bool{}
		out, _ := workspace.Git(root, "diff", "--name-only", "-z", "HEAD")
		for _, d := range strings.Split(out, "\x00") {
			dirtyCache[d] = true
		}
	}
	return dirtyCache[p]
}

func minified(src []byte) bool {
	lines := bytes.Count(src, []byte("\n")) + 1
	return len(src)/lines > 300
}

type def struct {
	File, Name string
	Line       int
	Rank       float64
}

// rank builds the reference graph and returns definitions ordered by relevance.
func rank(files []fileTags, focusTerms []string, dirty map[string]bool) []def {
	definers := map[string]map[string]bool{}
	refs := map[string]map[string]int{} // ident -> referencing file -> count
	defLines := map[string]map[string]int{}
	for _, f := range files {
		for _, t := range f.Tags {
			if t.Def {
				if definers[t.Name] == nil {
					definers[t.Name] = map[string]bool{}
				}
				definers[t.Name][f.Path] = true
				if defLines[f.Path] == nil {
					defLines[f.Path] = map[string]int{}
				}
				if _, ok := defLines[f.Path][t.Name]; !ok {
					defLines[f.Path][t.Name] = t.Line
				}
			} else {
				if refs[t.Name] == nil {
					refs[t.Name] = map[string]int{}
				}
				refs[t.Name][f.Path]++
			}
		}
	}
	focus := map[string]bool{}
	for _, t := range focusTerms {
		focus[strings.ToLower(t)] = true
	}
	matchesFocus := func(s string) bool {
		if len(focus) == 0 {
			return false
		}
		if focus[strings.ToLower(s)] {
			return true
		}
		for _, part := range find.Terms(s) {
			if focus[strings.ToLower(part)] {
				return true
			}
		}
		return false
	}

	index := map[string]int{}
	var nodes []string
	for _, f := range files {
		index[f.Path] = len(nodes)
		nodes = append(nodes, f.Path)
	}
	n := len(nodes)
	if n == 0 {
		return nil
	}
	type edge struct {
		to    int
		w     float64
		ident string
	}
	out := make([][]edge, n)
	outW := make([]float64, n)
	for ident, users := range refs {
		ds := definers[ident]
		if len(ds) == 0 {
			continue
		}
		mul := 1.0
		if matchesFocus(ident) {
			mul *= 10
		}
		if len(ident) >= 8 && isCompound(ident) {
			mul *= 10
		}
		if strings.HasPrefix(ident, "_") {
			mul *= 0.1
		}
		if len(ds) > 5 {
			mul *= 0.1
		}
		for user, count := range users {
			for d := range ds {
				if d == user {
					continue
				}
				w := mul * math.Sqrt(float64(count))
				out[index[user]] = append(out[index[user]], edge{index[d], w, ident})
				outW[index[user]] += w
			}
		}
	}
	pers := make([]float64, n)
	total := 0.0
	for i, p := range nodes {
		pers[i] = 1
		if dirty[p] {
			pers[i] += 20
		}
		for _, part := range find.Terms(strings.NewReplacer("/", " ", ".", " ", "-", " ").Replace(p)) {
			if focus[strings.ToLower(part)] {
				pers[i] += 20
			}
		}
		for name := range defLines[p] {
			if matchesFocus(name) {
				pers[i] += 5
			}
		}
		total += pers[i]
	}
	for i := range pers {
		pers[i] /= total
	}
	pr := append([]float64(nil), pers...)
	for iter := 0; iter < 40; iter++ {
		next := make([]float64, n)
		dangling := 0.0
		for i := range nodes {
			if outW[i] == 0 {
				dangling += pr[i]
				continue
			}
			for _, e := range out[i] {
				next[e.to] += 0.85 * pr[i] * e.w / outW[i]
			}
		}
		for i := range next {
			next[i] += (0.15 + 0.85*dangling) * pers[i]
		}
		pr = next
	}
	// Spread each file's rank over the definitions its references point at.
	scores := map[[2]string]float64{}
	for i := range nodes {
		for _, e := range out[i] {
			if outW[i] > 0 {
				scores[[2]string{nodes[e.to], e.ident}] += pr[i] * e.w / outW[i]
			}
		}
	}
	wantsTests := false
	for t := range focus {
		wantsTests = wantsTests || strings.HasPrefix(t, "test")
	}
	var defs []def
	for file, names := range defLines {
		if !wantsTests && isTestPath(file) {
			continue
		}
		for name, line := range names {
			s := scores[[2]string{file, name}] + 0.02*pr[index[file]]
			if matchesFocus(name) {
				s += 0.05 * pr[index[file]]
			}
			defs = append(defs, def{file, name, line, s})
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Rank != defs[j].Rank {
			return defs[i].Rank > defs[j].Rank
		}
		if defs[i].File != defs[j].File {
			return defs[i].File < defs[j].File
		}
		return defs[i].Line < defs[j].Line
	})
	return defs
}

func isTestPath(p string) bool {
	l := strings.ToLower(p)
	return strings.HasPrefix(l, "test") || strings.Contains(l, "/test") || strings.Contains(l, "_test.") || strings.Contains(l, ".test.") || strings.Contains(l, ".spec.") || strings.Contains(l, "__tests__")
}

func isCompound(s string) bool {
	if strings.Contains(s, "_") {
		return true
	}
	lower, upper := false, false
	for _, r := range s {
		lower = lower || unicode.IsLower(r)
		upper = upper || unicode.IsUpper(r)
	}
	return lower && upper
}

// fit renders the largest prefix of ranked definitions that fits in budget bytes.
func fit(defs []def, root string, budget int) string {
	lines := map[string][]string{}
	lo, hi, best := 0, len(defs), ""
	for lo <= hi {
		mid := (lo + hi) / 2
		s := render(defs[:mid], root, lines)
		if len(s) <= budget {
			best, lo = s, mid+1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func render(defs []def, root string, cache map[string][]string) string {
	byFile := map[string][]def{}
	var order []string
	for _, d := range defs {
		if byFile[d.File] == nil {
			order = append(order, d.File)
		}
		byFile[d.File] = append(byFile[d.File], d)
	}
	var b strings.Builder
	for _, f := range order {
		src, ok := cache[f]
		if !ok {
			data, _ := os.ReadFile(filepath.Join(root, f))
			src = strings.Split(string(data), "\n")
			cache[f] = src
		}
		ds := byFile[f]
		sort.Slice(ds, func(i, j int) bool { return ds[i].Line < ds[j].Line })
		fmt.Fprintf(&b, "%s:\n", f)
		last := -1
		for _, d := range ds {
			if d.Line == last || d.Line < 1 || d.Line > len(src) {
				continue
			}
			last = d.Line
			fmt.Fprintf(&b, "  %d│ %s\n", d.Line, signature(src[d.Line-1]))
		}
	}
	return b.String()
}

func signature(line string) string {
	s := strings.TrimSpace(line)
	s = strings.TrimSuffix(s, "{")
	s = strings.TrimSpace(s)
	if len(s) > 110 {
		s = s[:110] + "…"
	}
	return s
}

// renderHeader answers "what is this project" deterministically: the README's
// first paragraph or the manifest description, then the top-level layout.
func renderHeader(root string, paths []string) string {
	var b strings.Builder
	if about := projectAbout(root); about != "" {
		fmt.Fprintf(&b, "project: %s\n", about)
	}
	counts := map[string]int{}
	for _, p := range paths {
		top, rest, nested := strings.Cut(p, "/")
		if nested {
			if sub, _, deeper := strings.Cut(rest, "/"); deeper {
				top += "/" + sub
			}
			counts[top+"/"]++
		} else {
			counts[top]++
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		di, dj := strings.HasSuffix(keys[i], "/"), strings.HasSuffix(keys[j], "/")
		if di != dj {
			return di
		}
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 14 {
		keys = keys[:14]
	}
	var parts []string
	for _, k := range keys {
		if strings.HasSuffix(k, "/") {
			parts = append(parts, fmt.Sprintf("%s(%d)", k, counts[k]))
		} else {
			parts = append(parts, k)
		}
	}
	fmt.Fprintf(&b, "layout: %s\n\n", strings.Join(parts, " "))
	return b.String()
}

// clipHeader shortens header to at most max bytes at a word boundary, keeping the
// blank line that separates it from the definitions. A line left with only its
// label ("layout:") is dropped.
func clipHeader(header string, max int) string {
	if len(header) <= max {
		return header
	}
	const sep = "\n\n"
	if max <= len(sep) {
		return ""
	}
	cut := header[:max-len(sep)]
	i := strings.LastIndexAny(cut, " \n")
	if i <= 0 {
		return ""
	}
	cut = strings.TrimRight(cut[:i], " \n")
	start := strings.LastIndex(cut, "\n") + 1
	if !strings.Contains(cut[start:], " ") {
		cut = strings.TrimRight(cut[:start], "\n")
	}
	if cut == "" {
		return ""
	}
	return cut + sep
}

func projectAbout(root string) string {
	var pkg struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Scripts     map[string]string `json:"scripts"`
	}
	about := ""
	if workspace.ReadJSON(filepath.Join(root, "package.json"), &pkg) == nil {
		about = strings.TrimSpace(pkg.Name + " — " + pkg.Description)
		about = strings.TrimSuffix(about, " —")
		if len(pkg.Scripts) > 0 {
			names := make([]string, 0, len(pkg.Scripts))
			for k := range pkg.Scripts {
				names = append(names, k)
			}
			sort.Strings(names)
			if len(names) > 8 {
				names = names[:8]
			}
			about += " (npm scripts: " + strings.Join(names, ", ") + ")"
		}
	}
	if para := readmeParagraph(root); para != "" && (pkg.Description == "") {
		if about != "" {
			about += "; "
		}
		about += para
	}
	if len(about) > 320 {
		about = about[:320] + "…"
	}
	return about
}

func readmeParagraph(root string) string {
	for _, name := range []string{"README.md", "readme.md", "README", "README.rst"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		var para []string
		for _, line := range strings.Split(string(data), "\n") {
			t := strings.TrimSpace(line)
			skip := strings.HasPrefix(t, "#") || strings.HasPrefix(t, "![") || strings.HasPrefix(t, "[![") || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "```")
			if t == "" || skip {
				if len(para) > 0 {
					break
				}
				continue
			}
			para = append(para, t)
		}
		return strings.Trim(strings.Join(para, " "), "*_ ")
	}
	return ""
}
