package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// theIndexDoor is the one function of the package that may give a ledger row a
// key: the comparison of runs it makes first is what keeps a write from moving
// the index ahead of the ingest.
const (
	theIndexDoorFile     = "transaction_target.go"
	theIndexDoorFunction = "indexListed"
)

// indexCalls is every call of a method named IndexTransaction in the package's
// source files, as file and enclosing function. The tests seed the index
// through the store directly and are not read.
func indexCalls(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			enclosing := "outside any function"
			if function, ok := declaration.(*ast.FuncDecl); ok {
				enclosing = function.Name.Name
			}
			ast.Inspect(declaration, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "IndexTransaction" {
						calls = append(calls, file+" "+enclosing)
					}
				}
				return true
			})
		}
	}
	return calls
}

func TestOnlyTheIndexDoorGivesALedgerRowAKey(t *testing.T) {
	calls := indexCalls(t)
	if len(calls) != 1 || calls[0] != theIndexDoorFile+" "+theIndexDoorFunction {
		t.Errorf("IndexTransaction is called in %v, want once, in %s of %s", calls, theIndexDoorFunction, theIndexDoorFile)
	}
}
