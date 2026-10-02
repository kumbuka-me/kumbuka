// Command check-go-docs verifies documentation on authored, non-test Go declarations.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// main checks the requested source trees, defaulting to the current directory.
func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	failed := false
	for _, root := range roots {
		problems, err := checkTree(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
		}
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, problem)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// checkTree scans Go source without requiring dependencies, compilation, or matching build tags.
func checkTree(root string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if !ast.IsGenerated(file) {
			problems = append(problems, checkFile(set, file)...)
		}
		return nil
	})
	return problems, err
}

// checkFile checks named functions, named structs, and fields including anonymous and embedded fields.
func checkFile(set *token.FileSet, file *ast.File) []string {
	var problems []string
	report := func(pos token.Pos, message string) {
		problems = append(problems, fmt.Sprintf("%s: %s", set.Position(pos), message))
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch declaration := node.(type) {
		case *ast.FuncDecl:
			if !described(declaration.Doc) {
				report(declaration.Pos(), "function "+declaration.Name.Name+" needs a GoDoc summary")
			} else if !singleLineDescription(declaration.Doc) {
				report(declaration.Pos(), "function "+declaration.Name.Name+" needs a one-line GoDoc summary")
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				named, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, ok := named.Type.(*ast.StructType); ok && !described(named.Doc) && !described(declaration.Doc) {
					report(named.Pos(), "struct "+named.Name.Name+" needs a description")
				}
			}
		case *ast.StructType:
			for _, field := range declaration.Fields.List {
				if !described(field.Doc) && !described(field.Comment) {
					report(field.Pos(), "struct field needs a description")
				}
			}
		}
		return true
	})
	return problems
}

// described reports whether a comment contains prose rather than only compiler directives.
func described(comment *ast.CommentGroup) bool {
	return comment != nil && strings.TrimSpace(comment.Text()) != ""
}

// singleLineDescription reports whether a declaration description contains exactly one prose line.
func singleLineDescription(comment *ast.CommentGroup) bool {
	return !strings.Contains(strings.TrimSpace(comment.Text()), "\n")
}
