package classrules

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The rules take what the caller read and touch nothing themselves, and they
// keep an input of their own rather than the interpreted row.
var forbiddenImports = []string{
	"database/sql",
	"os",
	"net/http",
	"github.com/gabrielassisxyz/finwise/internal/txrules",
}

func TestPackageSourceDoesNoIOAndKnowsNoInterpretedRow(t *testing.T) {
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
			path := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range forbiddenImports {
				if path == forbidden {
					t.Errorf("%s imports %s", file, path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file was checked")
	}
}
