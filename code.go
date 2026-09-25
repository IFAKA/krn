package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type codeEntity struct {
	Name       string
	Kind       string
	Start, End int
}

func codeCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("code read|replace|insert-before|insert-after|remove")
	}
	op := args[0]
	if op != "read" && op != "replace" && op != "insert-before" && op != "insert-after" && op != "remove" {
		return errors.New("unsupported code operation")
	}
	f := flagSet("code")
	file := f.String("file", "", "Go source file")
	entity := f.String("entity", "", "top-level declaration name")
	content := f.String("content", "", "replacement or inserted declaration")
	contentFile := f.String("content-file", "", "file containing replacement or inserted declaration")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *file == "" || *entity == "" {
		return errors.New("code operation requires --file PATH --entity NAME")
	}
	if op == "read" || op == "remove" {
		if *content != "" || *contentFile != "" {
			return errors.New("content is only valid for replace or insert operations")
		}
	} else if (*content == "") == (*contentFile == "") {
		return errors.New("replace or insert requires exactly one of --content or --content-file")
	}
	r, err := discover()
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
		entity, err := resolveCodeEntity(path, source, *entity)
		if err != nil {
			return err
		}
		fmt.Print(string(source[entity.Start:entity.End]))
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
	diff, changed, err := editCodeFile(path, source, *entity, op, replacement)
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
	if filepath.Ext(path) != ".go" {
		return "", errors.New("code supports .go files only")
	}
	return path, nil
}

func parseCode(path string, source []byte) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return fset, file, nil
}

func codeEntities(path string, source []byte) ([]codeEntity, error) {
	fset, file, err := parseCode(path, source)
	if err != nil {
		return nil, err
	}
	var entities []codeEntity
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			entities = append(entities, codeEntity{Name: d.Name.Name, Kind: "func", Start: fset.Position(d.Pos()).Offset, End: fset.Position(d.End()).Offset})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				var name, kind string
				switch s := spec.(type) {
				case *ast.TypeSpec:
					name, kind = s.Name.Name, "type"
				case *ast.ValueSpec:
					if len(s.Names) != 1 {
						continue
					}
					name, kind = s.Names[0].Name, strings.ToLower(d.Tok.String())
				default:
					continue
				}
				entities = append(entities, codeEntity{Name: name, Kind: kind, Start: fset.Position(spec.Pos()).Offset, End: fset.Position(spec.End()).Offset})
			}
		}
	}
	return entities, nil
}

func resolveCodeEntity(path string, source []byte, name string) (codeEntity, error) {
	entities, err := codeEntities(path, source)
	if err != nil {
		return codeEntity{}, err
	}
	var matches []codeEntity
	for _, entity := range entities {
		if entity.Name == name {
			matches = append(matches, entity)
		}
	}
	if len(matches) == 0 {
		return codeEntity{}, fmt.Errorf("entity %q not found", name)
	}
	if len(matches) != 1 {
		return codeEntity{}, fmt.Errorf("entity %q is ambiguous (%d matches)", name, len(matches))
	}
	return matches[0], nil
}

func parseReplacement(path, source string) (codeEntity, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return codeEntity{}, errors.New("replacement is empty")
	}
	prefix := "package krnreplacement\n"
	entities, err := codeEntities(path, []byte(prefix+trimmed+"\n"))
	if err != nil {
		return codeEntity{}, fmt.Errorf("parse replacement: %w", err)
	}
	if len(entities) != 1 {
		return codeEntity{}, errors.New("replacement must contain exactly one supported top-level declaration")
	}
	entities[0].Start -= len(prefix)
	entities[0].End -= len(prefix)
	return entities[0], nil
}

func editCodeFile(path string, source []byte, name, op, replacement string) (string, bool, error) {
	target, err := resolveCodeEntity(path, source, name)
	if err != nil {
		return "", false, err
	}
	newSource := append([]byte(nil), source...)
	switch op {
	case "remove":
		newSource = append(newSource[:target.Start:target.Start], newSource[target.End:]...)
	case "replace", "insert-before", "insert-after":
		candidate, err := parseReplacement(path, replacement)
		if err != nil {
			return "", false, err
		}
		if op == "replace" && (candidate.Name != target.Name || candidate.Kind != target.Kind) {
			return "", false, fmt.Errorf("replacement must remain a %s named %q", target.Kind, target.Name)
		}
		text := strings.TrimSpace(replacement)
		if op != "replace" {
			entities, err := codeEntities(path, source)
			if err != nil {
				return "", false, err
			}
			for _, existing := range entities {
				if existing.Name == candidate.Name {
					if string(source[existing.Start:existing.End]) == text {
						return "", false, nil
					}
					return "", false, fmt.Errorf("inserted entity %q already exists", candidate.Name)
				}
			}
		}
		if op == "replace" {
			newSource = spliceCode(newSource, target.Start, target.End, text)
		} else if op == "insert-before" {
			newSource = spliceCode(newSource, target.Start, target.Start, text+"\n")
		} else {
			newSource = spliceCode(newSource, target.End, target.End, "\n"+text)
		}
	default:
		return "", false, errors.New("unsupported code operation")
	}
	if _, _, err := parseCode(path, newSource); err != nil {
		return "", false, fmt.Errorf("resulting source is invalid: %w", err)
	}
	if string(newSource) == string(source) {
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
