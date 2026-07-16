package quality

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var shellFunctionPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\(\)\s*\{`)

// TestNamedGoDeclarationsAreDocumented requires documentation for named functions, methods, and types.
func TestNamedGoDeclarationsAreDocumented(t *testing.T) {
	root := repositoryRoot(t)
	paths := repositoryFiles(t, func(path string, entry fs.DirEntry) bool {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return false
		}
		return filepath.Ext(path) == ".go" &&
			(strings.HasPrefix(relative, "cmd"+string(filepath.Separator)) ||
				strings.HasPrefix(relative, "internal"+string(filepath.Separator)))
	})

	fileset := token.NewFileSet()
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if bytes.Contains(source, []byte("Code generated")) {
			continue
		}
		file, err := parser.ParseFile(fileset, path, source, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				checkDeclarationComment(t, fileset, path, declaration.Name.Name, declaration.Doc, declaration.Pos())
			case *ast.GenDecl:
				if declaration.Tok != token.TYPE {
					continue
				}
				for _, specification := range declaration.Specs {
					typeSpecification := specification.(*ast.TypeSpec)
					documentation := typeSpecification.Doc
					if documentation == nil {
						documentation = declaration.Doc
					}
					checkDeclarationComment(t, fileset, path, typeSpecification.Name.Name, documentation, typeSpecification.Pos())
				}
			}
		}
	}
}

// TestNamedShellFunctionsAreDocumented requires a name-prefixed comment above each shell function.
func TestNamedShellFunctionsAreDocumented(t *testing.T) {
	paths := repositoryFiles(t, func(path string, _ fs.DirEntry) bool {
		return filepath.Ext(path) == ".sh"
	})
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Errorf("open %s: %v", path, err)
			continue
		}
		var previousNonEmpty string
		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			if match := shellFunctionPattern.FindStringSubmatch(line); match != nil {
				expectedPrefix := "# " + match[1]
				if !strings.HasPrefix(strings.TrimSpace(previousNonEmpty), expectedPrefix) {
					t.Errorf("%s:%d: function %s must have a directly preceding comment beginning with %q", path, lineNumber, match[1], expectedPrefix)
				}
			}
			if strings.TrimSpace(line) != "" {
				previousNonEmpty = line
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("scan %s: %v", path, err)
		}
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}
}

// checkDeclarationComment reports missing or incorrectly prefixed declaration documentation.
func checkDeclarationComment(t *testing.T, fileset *token.FileSet, path, name string, documentation *ast.CommentGroup, position token.Pos) {
	t.Helper()
	location := fileset.Position(position)
	if documentation == nil || strings.TrimSpace(documentation.Text()) == "" {
		t.Errorf("%s:%d: %s must have a documentation comment", path, location.Line, name)
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(documentation.Text()), name) {
		t.Errorf("%s:%d: documentation for %s must begin with its name", path, location.Line, name)
	}
}
