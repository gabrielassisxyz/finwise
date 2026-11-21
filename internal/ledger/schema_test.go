package ledger_test

import (
	"database/sql"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	_ "modernc.org/sqlite"
)

func TestSchemaFileHoldsDefinitionsAndNoData(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "ledger-schema-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if insert := regexp.MustCompile(`(?i)\binsert\b`); insert.Match(content) {
		t.Fatal("the schema file contains an INSERT: it must hold definitions only")
	}
	if !strings.Contains(string(content), "CREATE VIEW pending_posted_candidates") {
		t.Fatal("the schema file lacks the collector's views")
	}
	if string(content) != ledger.SchemaV1 {
		t.Fatal("the embedded schema differs from the file")
	}
}

func TestEveryQueryPreparesAgainstTheCollectorsSchema(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if _, err := database.Exec(ledger.SchemaV1); err != nil {
		t.Fatalf("the schema file does not load: %v", err)
	}

	queries := ledger.AllQueries()
	if len(queries) == 0 {
		t.Fatal("no queries registered")
	}
	for name, query := range queries {
		statement, err := database.PrepareContext(t.Context(), query)
		if err != nil {
			t.Errorf("%s does not prepare: %v", name, err)
			continue
		}
		statement.Close()
	}
}

func TestEveryQueryConstantIsRegistered(t *testing.T) {
	declared := declaredQueryConstants(t)
	registered := ledger.AllQueries()

	for _, name := range declared {
		if _, ok := registered[name]; !ok {
			t.Errorf("%s is declared in queries.go but missing from the registry, so no test prepares it", name)
		}
	}
	if len(declared) != len(registered) {
		t.Errorf("queries.go declares %d query constants, the registry holds %d", len(declared), len(registered))
	}
}

// Every statement the package runs must be one of the registered constants, or
// the preparation test above could not have seen it.
func TestNoStatementEscapesTheRegistry(t *testing.T) {
	registered := ledger.AllQueries()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !isStatementMethod(selector.Sel.Name) {
				return true
			}
			checked++
			if !isRegisteredOrPragma(call.Args[1], registered) {
				t.Errorf("%s: %s runs a statement that is not a registered query constant", file, selector.Sel.Name)
			}
			return true
		})
	}
	if checked < 8 {
		t.Fatalf("only %d statement calls were inspected; the scan has stopped seeing them", checked)
	}
}

func isStatementMethod(name string) bool {
	switch name {
	case "QueryContext", "QueryRowContext", "ExecContext", "PrepareContext":
		return true
	}
	return false
}

func isRegisteredOrPragma(argument ast.Expr, registered map[string]string) bool {
	switch value := argument.(type) {
	case *ast.Ident:
		_, ok := registered[value.Name]
		return ok
	case *ast.BasicLit:
		text, err := strconv.Unquote(value.Value)
		return err == nil && text == "PRAGMA user_version"
	}
	return false
}

func declaredQueryConstants(t *testing.T) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "queries.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				if strings.HasPrefix(name.Name, "query") {
					names = append(names, name.Name)
				}
			}
		}
	}
	sort.Strings(names)
	return names
}
