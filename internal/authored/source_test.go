package authored_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const modulePath = "github.com/gabrielassisxyz/finwise/"

// sourceFiles parses every non-test file of the package, including files later
// work adds: statements live beside the tables they touch, not in one file.
func sourceFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = parsed
	}
	if len(files) == 0 {
		t.Fatal("no source file was found")
	}
	return files
}

func TestEveryStatementPreparesAgainstTheMigratedSchema(t *testing.T) {
	database := migratedPool(t)
	statements := authored.AllStatements()
	if len(statements) == 0 {
		t.Fatal("no statement is registered")
	}
	for name, statement := range statements {
		prepared, err := database.PrepareContext(t.Context(), statement)
		if err != nil {
			t.Errorf("%s does not prepare: %v", name, err)
			continue
		}
		prepared.Close()
	}
}

func TestEveryStatementConstantIsRegistered(t *testing.T) {
	registered := authored.AllStatements()
	var declared []string
	for _, file := range sourceFiles(t) {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if strings.HasPrefix(name.Name, "statement") {
						declared = append(declared, name.Name)
					}
				}
			}
		}
	}
	sort.Strings(declared)
	for _, name := range declared {
		if _, ok := registered[name]; !ok {
			t.Errorf("%s is declared but missing from allStatements, so no test prepares it", name)
		}
	}
	if len(declared) != len(registered) {
		t.Errorf("the package declares %d statement constants, the registry holds %d", len(declared), len(registered))
	}
}

// statementArgument is where each method of database/sql takes its SQL text.
var statementArgument = map[string]int{
	"Exec": 0, "Query": 0, "QueryRow": 0, "Prepare": 0,
	"ExecContext": 1, "QueryContext": 1, "QueryRowContext": 1, "PrepareContext": 1,
}

// namedExceptions are the only statements that are not a registered constant.
// A migration file's body is checked by its checksum instead, and is the one.
var namedExceptions = map[string]string{
	"migration.body": "a migration file, vouched for by the checksum recorded in schema_migrations",
	// A journaled table's statements are named by its registry entry, and a
	// production entry names registered constants: see
	// TestEveryJournaledTableStatementIsRegistered.
	"statements.SelectRow":   journaledStatement,
	"statements.SelectAll":   journaledStatement,
	"statements.SelectOwned": journaledStatement,
	"statements.Insert":      journaledStatement,
	"statements.Update":      journaledStatement,
	"statements.Delete":      journaledStatement,
}

const journaledStatement = "a journaled table's statement, executed through its registry entry"

// Every statement the package executes must be a registered constant, or the
// preparation test could not have seen it.
func TestNoStatementEscapesTheRegistry(t *testing.T) {
	registered := authored.AllStatements()
	inspected := 0
	for path, file := range sourceFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			position, ok := statementArgument[selector.Sel.Name]
			if !ok || len(call.Args) <= position {
				return true
			}
			inspected++
			if !isRegisteredOrNamed(call.Args[position], registered) {
				t.Errorf("%s: %s executes a statement that is neither a registered constant nor a named exception", path, selector.Sel.Name)
			}
			return true
		})
	}
	if inspected < 8 {
		t.Fatalf("only %d statement calls were inspected; the scan has stopped seeing them", inspected)
	}
}

func isRegisteredOrNamed(argument ast.Expr, registered map[string]string) bool {
	switch value := argument.(type) {
	case *ast.Ident:
		_, ok := registered[value.Name]
		return ok
	case *ast.SelectorExpr:
		owner, ok := value.X.(*ast.Ident)
		if !ok {
			return false
		}
		_, named := namedExceptions[owner.Name+"."+value.Sel.Name]
		return named
	}
	return false
}

func TestNoSourceFileOfThePackageTouchesAFloat(t *testing.T) {
	for path, file := range sourceFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			switch found := node.(type) {
			case *ast.Ident:
				if found.Name == "float64" || found.Name == "float32" {
					t.Errorf("%s uses %s", path, found.Name)
				}
			case *ast.SelectorExpr:
				if found.Sel.Name == "ParseFloat" || found.Sel.Name == "FromFloat" {
					t.Errorf("%s calls %s", path, found.Sel.Name)
				}
			case *ast.BasicLit:
				if found.Kind == token.FLOAT {
					t.Errorf("%s has the float literal %s", path, found.Value)
				}
			}
			return true
		})
	}
}

// The clock comes from the caller; the host's is read nowhere in the package.
func TestNoSourceFileReadsTheHostsClock(t *testing.T) {
	for path, file := range sourceFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			owner, ok := selector.X.(*ast.Ident)
			if ok && owner.Name == "time" && (selector.Sel.Name == "Now" || selector.Sel.Name == "Since" || selector.Sel.Name == "Until") {
				t.Errorf("%s names time.%s", path, selector.Sel.Name)
			}
			return true
		})
	}
}

// The package stands on money and brday only: never on the ledger, whose
// snapshot it must not open, nor on the HTTP layer.
func TestThePackageImportsNothingElseOfTheRepository(t *testing.T) {
	allowed := map[string]bool{modulePath + "internal/money": true, modulePath + "internal/brday": true}
	for path, file := range sourceFiles(t) {
		for _, imported := range file.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(name, modulePath) && !allowed[name] {
				t.Errorf("%s imports %s", path, name)
			}
		}
	}
}
