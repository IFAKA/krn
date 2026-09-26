package repomap

import (
	"path/filepath"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspy "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Tag is a definition (Def) or reference of an identifier in one file.
type Tag struct {
	Name string `json:"n"`
	Line int    `json:"l"`
	Def  bool   `json:"d,omitempty"`
}

type grammar struct {
	lang *sitter.Language
	// defs maps node kinds that define a symbol to the child field holding its name.
	defs map[string]string
	// refs maps node kinds that reference a symbol to the child field holding the
	// referenced expression ("" means the node text itself).
	refs map[string]string
}

var (
	jsDefs = map[string]string{"function_declaration": "name", "generator_function_declaration": "name", "class_declaration": "name", "method_definition": "name", "abstract_class_declaration": "name", "interface_declaration": "name", "type_alias_declaration": "name", "enum_declaration": "name"}
	jsRefs = map[string]string{"call_expression": "function", "new_expression": "constructor", "import_specifier": "name", "type_identifier": "", "jsx_opening_element": "name", "jsx_self_closing_element": "name"}
)

var grammars = map[string]*grammar{}

func init() {
	js := &grammar{lang: lang(tsjs.Language()), defs: jsDefs, refs: jsRefs}
	ts := &grammar{lang: lang(tsts.LanguageTypescript()), defs: jsDefs, refs: jsRefs}
	tsx := &grammar{lang: lang(tsts.LanguageTSX()), defs: jsDefs, refs: jsRefs}
	py := &grammar{lang: lang(tspy.Language()),
		defs: map[string]string{"function_definition": "name", "class_definition": "name"},
		refs: map[string]string{"call": "function", "aliased_import": "name", "import_from_statement": "name"}}
	golang := &grammar{lang: lang(tsgo.Language()),
		defs: map[string]string{"function_declaration": "name", "method_declaration": "name", "type_spec": "name", "const_spec": "name"},
		refs: map[string]string{"call_expression": "function", "type_identifier": "", "qualified_type": "name"}}
	for _, e := range []string{".js", ".jsx", ".mjs", ".cjs"} {
		grammars[e] = js
	}
	grammars[".ts"], grammars[".mts"], grammars[".cts"] = ts, ts, ts
	grammars[".tsx"] = tsx
	grammars[".py"] = py
	grammars[".go"] = golang
}

func lang(p unsafe.Pointer) *sitter.Language { return sitter.NewLanguage(p) }

func supported(path string) bool { return grammars[strings.ToLower(filepath.Ext(path))] != nil }

// extract parses src and returns its definition and reference tags.
func extract(path string, src []byte) []Tag {
	g := grammars[strings.ToLower(filepath.Ext(path))]
	if g == nil {
		return nil
	}
	p := sitter.NewParser()
	defer p.Close()
	if p.SetLanguage(g.lang) != nil {
		return nil
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()
	var tags []Tag
	c := tree.Walk()
	defer c.Close()
	for {
		n := c.Node()
		kind := n.Kind()
		if field, ok := g.defs[kind]; ok {
			if name := n.ChildByFieldName(field); name != nil {
				tags = append(tags, Tag{Name: name.Utf8Text(src), Line: int(n.StartPosition().Row) + 1, Def: true})
			}
		} else if kind == "variable_declarator" || kind == "assignment" {
			// const handler = () => {...}; top-level constants and Python module assignments.
			if name := declaredName(n, src); name != "" {
				tags = append(tags, Tag{Name: name, Line: int(n.StartPosition().Row) + 1, Def: true})
			}
		}
		if field, ok := g.refs[kind]; ok {
			target := n
			if field != "" {
				target = n.ChildByFieldName(field)
			}
			if name := refName(target, src); name != "" {
				tags = append(tags, Tag{Name: name, Line: int(n.StartPosition().Row) + 1})
			}
		}
		if c.GotoFirstChild() {
			continue
		}
		for !c.GotoNextSibling() {
			if !c.GotoParent() {
				return tags
			}
		}
	}
}

// declaredName names a variable declarator only when it is a callable/class value
// or a top-level declaration, so locals do not flood the map.
func declaredName(n *sitter.Node, src []byte) string {
	name := n.ChildByFieldName("name")
	if name == nil {
		name = n.ChildByFieldName("left")
	}
	if name == nil || name.Kind() != "identifier" {
		return ""
	}
	// Only top-level declarations: program > lexical_declaration > declarator, with an
	// optional export_statement in between; Python: module > expression_statement > assignment.
	p := n.Parent()
	for i := 0; p != nil && i < 3; i++ {
		switch p.Kind() {
		case "program", "module":
			return name.Utf8Text(src)
		case "statement_block", "block", "function_body", "class_body":
			return ""
		}
		p = p.Parent()
	}
	return ""
}

// refName reduces a referenced expression to the identifier it resolves by name:
// foo() -> foo, obj.foo() -> foo, pkg.Type -> Type.
func refName(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "identifier", "type_identifier", "property_identifier", "field_identifier":
		return n.Utf8Text(src)
	case "member_expression", "attribute", "selector_expression":
		for _, f := range []string{"property", "attribute", "field"} {
			if c := n.ChildByFieldName(f); c != nil {
				return c.Utf8Text(src)
			}
		}
	case "dotted_name", "nested_identifier", "member_expression_jsx":
		if k := n.NamedChildCount(); k > 0 {
			return n.NamedChild(k - 1).Utf8Text(src)
		}
	}
	return ""
}
