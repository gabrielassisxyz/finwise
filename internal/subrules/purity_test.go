package subrules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/gabrielassisxyz/finwise/"

var allowedModuleImports = map[string]bool{
	module + "internal/money": true,
	module + "internal/brday": true,
}

// The package decides money and days from plain values: no SQL, no I/O, no
// package of the module beyond money and days, no third-party module, no
// float and no clock of its own.
func TestPackageSourceIsPureIntegerArithmetic(t *testing.T) {
	findings, checked, err := impuritiesIn(".")
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no source file was checked")
	}
	for _, finding := range findings {
		t.Error(finding)
	}
}

func TestThePurityGuardSeesEveryPlant(t *testing.T) {
	plants := map[string]string{
		"database/sql":         "package p\n\nimport _ \"database/sql\"\n",
		"os":                   "package p\n\nimport _ \"os\"\n",
		"net/http":             "package p\n\nimport _ \"net/http\"\n",
		"another module part":  "package p\n\nimport _ \"" + module + "internal/txrules\"\n",
		"a third-party module": "package p\n\nimport _ \"modernc.org/sqlite\"\n",
		"float64":              "package p\n\nvar x float64\n",
		"float32":              "package p\n\nvar x float32\n",
		"a float literal":      "package p\n\nvar x = 1.5\n",
		"time.Now":             "package p\n\nimport \"time\"\n\nvar x = time.Now()\n",
		"time.Now renamed":     "package p\n\nimport clock \"time\"\n\nvar x = clock.Now\n",
	}
	for name, source := range plants {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "plant.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		findings, checked, err := impuritiesIn(dir)
		if err != nil || checked != 1 || len(findings) == 0 {
			t.Errorf("%s planted: findings %v, checked %d, err %v", name, findings, checked, err)
		}
	}
}

// impuritiesIn reads the non-test Go files of dir; test files are left out
// because a test may draw random numbers and time its own run.
func impuritiesIn(dir string) (findings []string, checked int, err error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, 0, err
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return nil, 0, err
		}
		checked++
		timeName := ""
		for _, imported := range parsed.Imports {
			path, _ := strconv.Unquote(imported.Path.Value)
			if finding := forbiddenImport(path); finding != "" {
				findings = append(findings, file+" "+finding)
			}
			if path == "time" {
				timeName = "time"
				if imported.Name != nil {
					timeName = imported.Name.Name
				}
			}
		}
		findings = append(findings, floatsAndClocksIn(file, parsed, timeName)...)
	}
	return findings, checked, nil
}

// forbiddenImports are the standard library's doors to the world outside.
var forbiddenImports = map[string]bool{"database/sql": true, "os": true, "net/http": true, "io/fs": true}

func forbiddenImport(path string) string {
	switch {
	case forbiddenImports[path]:
		return "imports " + path
	case allowedModuleImports[path]:
		return ""
	case strings.Contains(strings.Split(path, "/")[0], "."):
		return "imports " + path + ", which is not the standard library, money or brday"
	}
	return ""
}

func floatsAndClocksIn(file string, parsed *ast.File, timeName string) []string {
	var findings []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch found := node.(type) {
		case *ast.Ident:
			if found.Name == "float64" || found.Name == "float32" {
				findings = append(findings, file+" uses "+found.Name)
			}
		case *ast.BasicLit:
			if found.Kind == token.FLOAT {
				findings = append(findings, file+" has the float literal "+found.Value)
			}
		case *ast.SelectorExpr:
			if receiver, ok := found.X.(*ast.Ident); ok && timeName != "" && receiver.Name == timeName && found.Sel.Name == "Now" {
				findings = append(findings, file+" reads the clock with time.Now")
			}
		}
		return true
	})
	return findings
}
