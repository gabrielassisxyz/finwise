package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The collector's yield statement costs several seconds whatever the window, so
// it stays in the reader as the reference the rules are tested against and no
// request may reach it. The scan covers every package a handler could call
// through, and every way of naming the statement.

// referenceNames are what a caller of the collector's statement has to write.
func isReferenceName(name string) bool {
	return name == "PotYields" || strings.HasPrefix(name, "queryPotYield")
}

// referencesTheCollectorsStatement lists the files, under the module's
// packages, that name the statement outside the reader's own file for it.
func referencesTheCollectorsStatement(t *testing.T, roots ...string) []string {
	t.Helper()
	var offenders []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			if strings.Contains(filepath.ToSlash(path), "internal/ledger/pot_yield.go") || strings.HasSuffix(path, "internal/ledger/queries.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			named := false
			ast.Inspect(parsed, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				named = named || (ok && isReferenceName(identifier.Name))
				return true
			})
			if named {
				offenders = append(offenders, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return offenders
}

func TestNoReadHandlerReachesTheCollectorsYieldStatement(t *testing.T) {
	for _, offender := range referencesTheCollectorsStatement(t, "../../internal", "../../cmd") {
		t.Errorf("%s names the collector's yield statement, which takes seconds a call", offender)
	}
}

// The scan above is only worth anything if it sees what it is meant to see.
func TestTheScanForTheCollectorsYieldStatementSeesAReaderOfIt(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "handler.go"), "package x\nfunc f(r interface{ PotYields() }) { r.PotYields() }\n")
	writeGoFile(t, filepath.Join(dir, "other.go"), "package x\nvar q = queryPotYieldReserves\n")
	writeGoFile(t, filepath.Join(dir, "clean.go"), "package x\nfunc g() {}\n")
	if offenders := referencesTheCollectorsStatement(t, dir); len(offenders) != 2 {
		t.Errorf("the scan found %v, want the two files that name the statement", offenders)
	}
}

func writeGoFile(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
