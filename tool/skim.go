package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skim: a file's outline, no agents. An agent touring a package greps '^func|^type|^const' home
// over and over; skim answers that in one step with an AST pass — declaration order kept, doc
// first-lines carried, line numbers anchored. Packages expand to one outline per file; non-Go
// files answer with a size hint (an outline needs a grammar and none is wired for them yet).
func SkimTool() *Tool {
	return &Tool{
		Name: "skim",
		Description: "Outline a Go file or package: every import line, every type/const/var/func declaration with its line anchor and leading doc's first line — no agents. " +
			"Cheaper and surer than a grep for the map-before-the-read.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		Class:  Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Path string }
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
			}
			return Classification{Class: Read, Why: "skims " + env.Rel(abs), Paths: []string{abs}}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Path string }
			if err := decode(in, &a); err != nil || a.Path == "" {
				return fail("skim: path is required")
			}
			abs := env.Resolve(a.Path)
			st, err := os.Stat(abs)
			if err != nil {
				return fail("skim: %v", err)
			}
			var files []string
			if st.IsDir() {
				ents, err := os.ReadDir(abs)
				if err != nil {
					return fail("skim: %v", err)
				}
				for _, e := range ents {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
						files = append(files, filepath.Join(abs, e.Name()))
					}
				}
				sort.Strings(files)
				if len(files) == 0 {
					return fail("skim: %s holds no Go files", env.Rel(abs))
				}
			} else {
				if !strings.HasSuffix(abs, ".go") {
					return fail("skim: %s is not Go (%d bytes) — read it with a window instead", env.Rel(abs), st.Size())
				}
				files = []string{abs}
			}
			var b strings.Builder
			for i, f := range files {
				if st.IsDir() { // a package read heads every file, even a lone one
					if i > 0 {
						b.WriteString("\n")
					}
					fmt.Fprintf(&b, "== %s ==\n", env.Rel(f))
				}
				b.WriteString(skimFile(f))
			}
			return Result{Output: clip(strings.TrimRight(b.String(), "\n"))}
		},
	}
}

// skimFile outlines one Go file: imports verbatim, then every declaration — grouped consts and
// vars named one per line, types and funcs with their signature heads and doc first-lines.
func skimFile(abs string) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
	if err != nil {
		return fmt.Sprintf("! %v\n", err)
	}
	var b strings.Builder
	for _, im := range f.Imports {
		fmt.Fprintf(&b, "%6d\timport %s\n", fset.Position(im.Pos()).Line, im.Path.Value)
	}
	docLine := func(d *ast.CommentGroup) string {
		if d == nil {
			return ""
		}
		s := strings.TrimSpace(strings.SplitN(d.Text(), "\n", 2)[0])
		if s == "" {
			return ""
		}
		return " — " + s
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			head := "func " + d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				head = "func (" + exprText(fset, d.Recv.List[0].Type) + ") " + d.Name.Name
			}
			head += signatureText(fset, d.Type)
			fmt.Fprintf(&b, "%6d\t%s%s\n", fset.Position(d.Pos()).Line, head, docLine(d.Doc))
		case *ast.GenDecl:
			for _, sp := range d.Specs {
				switch sp := sp.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch sp.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					doc := sp.Doc
					if doc == nil {
						doc = d.Doc
					}
					fmt.Fprintf(&b, "%6d\t%s %s%s\n", fset.Position(sp.Pos()).Line, kind, sp.Name.Name, docLine(doc))
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						fmt.Fprintf(&b, "%6d\t%s %s%s\n", fset.Position(sp.Pos()).Line, d.Tok.String(), n.Name, docLine(sp.Doc))
					}
				}
			}
		}
	}
	return b.String()
}

// exprText renders a type expression without layout (receiver heads, field types): the printer
// writes plain text, so printing the node with the fileset is the whole type on one line.
func exprText(fset *token.FileSet, e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, e); err != nil {
		return "…"
	}
	return b.String()
}

// signatureText renders a func type as "(params) results", agents never.
func signatureText(fset *token.FileSet, t *ast.FuncType) string {
	var b strings.Builder
	field := func(fl *ast.FieldList, sepOut bool) {
		b.WriteString("(")
		if fl != nil {
			for i, f := range fl.List {
				if i > 0 {
					b.WriteString(", ")
				}
				names := ""
				for _, n := range f.Names {
					if names != "" {
						names += ", "
					}
					names += n.Name
				}
				if names != "" {
					b.WriteString(names + " ")
				}
				b.WriteString(exprText(fset, f.Type))
			}
		}
		b.WriteString(")")
		if sepOut {
			b.WriteString(" ")
		}
	}
	field(t.Params, t.Results != nil && len(t.Results.List) > 0)
	if t.Results != nil && len(t.Results.List) > 0 {
		if len(t.Results.List) == 1 && len(t.Results.List[0].Names) == 0 {
			b.WriteString(exprText(fset, t.Results.List[0].Type))
		} else {
			field(t.Results, false)
		}
	}
	return b.String()
}
