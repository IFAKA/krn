package find

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/krn/internal/workspace"
)

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("find requires a query")
	}
	f := flag.NewFlagSet("find", flag.ContinueOnError)
	js := f.Bool("json", false, "json")
	max := f.Int("max-files", 12, "maximum files")
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
	q := f.Arg(0)
	workspace.Ensure(r)
	start := time.Now()
	c := exec.Command("rg", "-n", "--no-heading", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**", q, ".")
	c.Dir = r.Root
	raw, e := c.CombinedOutput()
	log := workspace.SaveRun(r, "find", raw)
	type hit struct {
		File string `json:"file"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	counts := map[string]int{}
	hits := map[string][]hit{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		n, _ := strconv.Atoi(parts[1])
		file := strings.TrimPrefix(parts[0], "./")
		counts[file]++
		if len(hits[file]) < 2 {
			hits[file] = append(hits[file], hit{file, n, parts[2]})
		}
	}
	files := make([]string, 0, len(counts))
	for p := range counts {
		files = append(files, p)
	}
	sort.Slice(files, func(i, j int) bool {
		if counts[files[i]] != counts[files[j]] {
			return counts[files[i]] > counts[files[j]]
		}
		return files[i] < files[j]
	})
	if len(files) > *max {
		files = files[:*max]
	}
	if *js {
		selected := map[string][]hit{}
		for _, p := range files {
			selected[p] = hits[p]
		}
		status := "ok"
		if e != nil {
			status = "unavailable"
		}
		return workspace.PrintJSON(map[string]any{"query": q, "matches": len(counts), "files": files, "hits": selected, "full_log": log, "status": status, "duration_ms": time.Since(start).Milliseconds()})
	}
	fmt.Printf("query: %s\nfiles: %d\nfull_log: %s\n", q, len(files), log)
	for _, p := range files {
		fmt.Printf("%d %s\n", counts[p], p)
		for _, h := range hits[p] {
			fmt.Printf("  %d: %s\n", h.Line, h.Text)
		}
	}
	if e != nil {
		fmt.Printf("status: unavailable (%v)\n", e)
	}
	return nil
}

func normalizeFindArgs(args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			flags = append(flags, a)
		case a == "--max-files":
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case strings.HasPrefix(a, "--max-files="):
			flags = append(flags, a)
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}
