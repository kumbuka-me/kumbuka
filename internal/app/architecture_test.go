package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/kumbuka-me/kumbuka/"

func TestArchitectureDependencyDirection(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	assertPackageDoesNotImport(t, root, "internal/webview", []string{
		modulePath + "internal/application",
		modulePath + "internal/http",
		modulePath + "internal/postgres",
	})
	assertPackageDoesNotImport(t, root, "internal/application", []string{
		modulePath + "internal/http",
		modulePath + "internal/webview",
		modulePath + "internal/postgres",
	})
}

func TestApplicationHasNoTransportOrTemplateImports(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/application"), func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}

		markdownAliases := map[string]bool{}
		for _, imported := range file.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", rel, err)
			}
			if name == "net/http" || name == "html/template" || name == modulePath+"pkg/icons" {
				t.Errorf("%s imports forbidden application dependency %s", filepath.ToSlash(rel), name)
			}
			if name == modulePath+"pkg/markdown" {
				alias := "markdown"
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				markdownAliases[alias] = true
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Renderer" {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && markdownAliases[identifier.Name] {
				t.Errorf("%s depends on concrete markdown.Renderer", filepath.ToSlash(rel))
			}
			return true
		})
	})
}

func TestPostgresOwnsPGXImports(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, relative := range []string{"cmd", "internal", "pkg"} {
		walkGoFiles(t, filepath.Join(root, relative), func(path string, file *ast.File) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(filepath.ToSlash(rel), "internal/postgres/") {
				return
			}
			for _, imported := range file.Imports {
				name, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", rel, err)
				}
				if strings.HasPrefix(name, "github.com/jackc/pgx") {
					t.Errorf("%s imports %s; pgx belongs in internal/postgres", filepath.ToSlash(rel), name)
				}
			}
		})
	}
}

func TestWebviewHasNoUniversalDataOrLoaderType(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	walkGoFiles(t, filepath.Join(root, "internal/webview"), func(path string, file *ast.File) {
		rel, _ := filepath.Rel(root, path)
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if typeSpec.Name.Name == "Data" || typeSpec.Name.Name == "Loader" {
					t.Errorf("%s declares obsolete webview.%s", filepath.ToSlash(rel), typeSpec.Name.Name)
				}
			}
		}
	})
}

func assertPackageDoesNotImport(t *testing.T, root, relative string, forbidden []string) {
	t.Helper()
	walkGoFiles(t, filepath.Join(root, relative), func(path string, file *ast.File) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", rel, err)
			}
			for _, prefix := range forbidden {
				if name == prefix || strings.HasPrefix(name, prefix+"/") {
					t.Errorf("%s imports forbidden dependency %s", filepath.ToSlash(rel), name)
				}
			}
		}
	})
}

func walkGoFiles(t *testing.T, root string, visit func(string, *ast.File)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		visit(path, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}
