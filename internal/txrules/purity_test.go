package txrules

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The package does no SQL and no I/O. Test files are left out: the opt-in
// snapshot test has to read an environment variable.
func TestPackageSourceImportsNeitherDatabaseSQLNorOS(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imported := range parsed.Imports {
			if path := strings.Trim(imported.Path.Value, `"`); path == "database/sql" || path == "os" {
				t.Errorf("%s imports %s", file, path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file was checked")
	}
}
