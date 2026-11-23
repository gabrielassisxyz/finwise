package ingest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A run reads a person's whole financial history, so nothing in the package
// writes anywhere but its return values: it imports no logger and prints
// nothing. Only the caller logs, and only the counts and the kind of a failure.

// writesOutputIn lists, for each non-test Go file of directory, the logging
// imports and the calls that print.
func writesOutputIn(t *testing.T, directory string) (findings []string, scanned int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		name := filepath.Base(file)
		for _, spec := range parsed.Imports {
			if path := strings.Trim(spec.Path.Value, `"`); path == "log" || path == "log/slog" {
				findings = append(findings, name+" imports "+path)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok && printsSomething(call) {
				findings = append(findings, name+" writes to the process's output")
			}
			return true
		})
	}
	return findings, scanned
}

func printsSomething(call *ast.CallExpr) bool {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return function.Name == "print" || function.Name == "println"
	case *ast.SelectorExpr:
		pkg, ok := function.X.(*ast.Ident)
		return ok && pkg.Name == "fmt" && (strings.HasPrefix(function.Sel.Name, "Print") || strings.HasPrefix(function.Sel.Name, "Fprint"))
	}
	return false
}

func TestThePackageImportsNoLoggerAndPrintsNothing(t *testing.T) {
	findings, scanned := writesOutputIn(t, ".")
	if scanned < 5 {
		t.Fatalf("only %d files were scanned; the scan has stopped seeing the package", scanned)
	}
	for _, finding := range findings {
		t.Error(finding)
	}
}

func TestTheSilenceScanSeesEveryWayOfWriting(t *testing.T) {
	directory := t.TempDir()
	plants := map[string]string{
		"slog.go":       "package p\nimport \"log/slog\"\nfunc f() { slog.Info(\"x\") }\n",
		"log.go":        "package p\nimport \"log\"\nfunc f() { log.Print(\"x\") }\n",
		"printf.go":     "package p\nimport \"fmt\"\nfunc f() { fmt.Printf(\"x\") }\n",
		"fprint.go":     "package p\nimport (\"fmt\"; \"os\")\nfunc f() { fmt.Fprintln(os.Stderr, \"x\") }\n",
		"builtin.go":    "package p\nfunc f() { println(\"x\") }\n",
		"clean.go":      "package p\nimport \"fmt\"\nfunc f() string { return fmt.Sprint(1) }\n",
		"quiet_test.go": "package p\nimport \"log\"\nfunc g() { log.Print(\"x\") }\n",
	}
	for name, source := range plants {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	findings, scanned := writesOutputIn(t, directory)
	if scanned != 6 {
		t.Errorf("the scan read %d files, want the six that are not tests", scanned)
	}
	for _, plant := range []string{"slog.go", "log.go", "printf.go", "fprint.go", "builtin.go"} {
		found := false
		for _, finding := range findings {
			found = found || strings.HasPrefix(finding, plant+" ")
		}
		if !found {
			t.Errorf("%s went unnoticed: %v", plant, findings)
		}
	}
	for _, finding := range findings {
		if strings.HasPrefix(finding, "clean.go ") || strings.HasPrefix(finding, "quiet_test.go ") {
			t.Errorf("a file that writes nothing was accused: %s", finding)
		}
	}
}
