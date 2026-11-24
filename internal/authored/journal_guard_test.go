package authored_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func TestEveryTableOfTheSchemaIsJournaledOrDeliberatelyNot(t *testing.T) {
	excluded := authored.NotJournaledTables()
	journaled := map[string]bool{}
	for _, table := range authored.ProductionTables() {
		journaled[table.Name] = true
	}
	for name := range excluded {
		if journaled[name] {
			t.Errorf("%s is both journaled and deliberately not", name)
		}
	}
	schema := tableNames(t, migratedPool(t))
	if len(schema) < 4 {
		t.Fatalf("the schema lists only %v; the instrument is not seeing it", schema)
	}
	for _, name := range schema {
		if strings.HasPrefix(name, "sqlite_") {
			continue
		}
		_, notJournaled := excluded[name]
		if journaled[name] == notJournaled {
			t.Errorf("%s is in neither or both of the registry and the tables deliberately not journaled", name)
		}
	}
}

func TestTheTablesDeliberatelyNotJournaledAreTheNamedOnes(t *testing.T) {
	var got []string
	for name := range authored.NotJournaledTables() {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"change_effects", "changes", "lines", "meta", "model_suggestions", "pin", "schema_migrations", "tx_ids", "tx_index",
		"proposals", "queue_proposals"}
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("not journaled: %v, want %v", got, want)
	}
}

func TestARegistryRefusesATableThatIsDeliberatelyNotJournaled(t *testing.T) {
	tables := append(scratchTables(), authored.Table{
		Name: "meta", Shape: authored.ShapeSet, Columns: columnsOf("key", "value"), Key: []string{"key"},
		Statements: scratchStatements("meta", []string{"key", "value"}, []string{"key"}, nil),
	})
	if _, err := authored.NewRegistry(tables); err == nil {
		t.Error("a registry accepted meta")
	}
}

// A production entry's statements are registered constants, so the test that
// prepares every registered statement prepares them too.
func TestEveryJournaledTableStatementIsRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, statement := range authored.AllStatements() {
		registered[statement] = true
	}
	for _, table := range authored.ProductionTables() {
		s := table.Statements
		for _, statement := range []string{s.SelectRow, s.SelectAll, s.Insert, s.Update, s.Delete, s.SelectOwned} {
			if statement != "" && !registered[statement] {
				t.Errorf("%s names a statement that is not a registered constant: %s", table.Name, statement)
			}
		}
	}
}

func TestAnEntryWhoseSelectReturnsOtherColumnsIsRefusedAtOpen(t *testing.T) {
	tables := scratchTables()
	tables[1].Statements.SelectAll = "SELECT id, amount, name, note FROM things"
	registry, err := authored.NewRegistry(tables)
	if err != nil {
		t.Fatal(err)
	}
	_, err = authored.Open(t.Context(), databasePath(t), authored.Options{
		Now: clockAt(t, creationInstant), Migrations: withMigrations(t, map[string]string{"9000_scratch": scratchSchema}), Journal: registry,
	})
	if err == nil {
		t.Fatal("a registry whose statement swaps two columns was accepted")
	}
}

func TestNoWriteToAJournaledTableEscapesTheRecorder(t *testing.T) {
	findings := writesOutsideTheRecorder(sourceFiles(t), authored.ProductionTables(), authored.AllStatements())
	for _, finding := range findings {
		t.Error(finding)
	}
}

// The scan above finds nothing while no table is registered in production, so
// it is proved here on source that does what it must catch.
func TestTheRecorderGuardCatchesAWriteOutsideTheRecorder(t *testing.T) {
	tables := scratchTables()
	facts := tables[0].Statements
	source := `package fixture

const statementStray = "UPDATE facts SET refund = 1 WHERE id = ?"

func stray() {
	tx.ExecContext(ctx, statementStray)
	tx.ExecContext(ctx, statementEntryUpdate)
	tx.ExecContext(ctx, statementUnrelated)
}
`
	parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	// statementStray is declared in the source and registered nowhere: the scan
	// must read it from the file.
	statements := map[string]string{
		"statementEntryUpdate": facts.Update,
		"statementUnrelated":   "UPDATE meta SET value = ? WHERE key = ?",
	}
	findings := writesOutsideTheRecorder(map[string]*ast.File{"fixture.go": parsed}, tables, statements)
	want := []string{
		"statementStray writes facts and is not a statement of its registry entry",
		"fixture.go: statementEntryUpdate, which writes facts, is executed outside the recorder",
		"fixture.go: statementStray, which writes facts, is executed outside the recorder",
	}
	sort.Strings(findings)
	sort.Strings(want)
	if strings.Join(findings, "\n") != strings.Join(want, "\n") {
		t.Errorf("findings:\n%s\nwant:\n%s", strings.Join(findings, "\n"), strings.Join(want, "\n"))
	}
}

var writeTarget = regexp.MustCompile("(?is)\\b(?:INSERT(?:\\s+OR\\s+\\w+)?\\s+INTO|REPLACE\\s+INTO|UPDATE(?:\\s+OR\\s+\\w+)?|DELETE\\s+FROM)\\s+[\"`\\[]?(\\w+)")

// writesOutsideTheRecorder finds a registered statement that writes a
// journaled table without being one its entry names, and any statement
// writing a journaled table that a call executes by its constant's name: the
// recorder reaches a table's statements only through its registry entry.
func writesOutsideTheRecorder(files map[string]*ast.File, tables []authored.Table, registered map[string]string) []string {
	statements := declaredStatements(files)
	for name, statement := range registered {
		statements[name] = statement
	}
	byName := map[string]authored.Table{}
	for _, table := range tables {
		byName[table.Name] = table
	}
	writes := map[string]string{}
	var findings []string
	for name, statement := range statements {
		for _, match := range writeTarget.FindAllStringSubmatch(statement, -1) {
			table, journaled := byName[match[1]]
			if !journaled {
				continue
			}
			writes[name] = table.Name
			s := table.Statements
			if statement != s.Insert && statement != s.Update && statement != s.Delete {
				findings = append(findings, name+" writes "+table.Name+" and is not a statement of its registry entry")
			}
		}
	}
	for path, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			position, isStatementCall := statementArgument[selectorName(selector, ok)]
			if !isStatementCall || len(call.Args) <= position {
				return true
			}
			if ident, ok := call.Args[position].(*ast.Ident); ok {
				if table, writesJournaled := writes[ident.Name]; writesJournaled {
					findings = append(findings, path+": "+ident.Name+", which writes "+table+", is executed outside the recorder")
				}
			}
			return true
		})
	}
	return findings
}

// declaredStatements is every string constant of the files, registered or not.
func declaredStatements(files map[string]*ast.File) map[string]string {
	declared := map[string]string{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if literal, ok := value.Values[i].(*ast.BasicLit); ok && literal.Kind == token.STRING {
						if text, err := strconv.Unquote(literal.Value); err == nil {
							declared[name.Name] = text
						}
					}
				}
			}
		}
	}
	return declared
}

func selectorName(selector *ast.SelectorExpr, ok bool) string {
	if !ok {
		return ""
	}
	return selector.Sel.Name
}
