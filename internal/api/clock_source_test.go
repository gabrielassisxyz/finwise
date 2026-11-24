package api

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// hostClockFree are the directories, from the repository's root, whose code
// takes "now" from the clock it is given. Some are created by later work: a
// directory that is not there yet has nothing to scan, and is scanned from its
// first file without this list changing.
var hostClockFree = []string{
	"internal/api",
	"internal/ingest",
	"internal/budgetrules",
	"internal/classrules",
	"internal/subrules",
	"internal/txrules",
	"internal/cardrules",
	"internal/potrules",
	"internal/suggest",
}

// hostClockReaders are the functions of package time that read the host's clock.
var hostClockReaders = map[string]bool{"Now": true, "Since": true, "Until": true}

const repositoryRoot = "../.."

func TestNoCodeThatTakesAClockReadsTheHostsClock(t *testing.T) {
	if _, err := os.Stat(filepath.Join(repositoryRoot, "go.mod")); err != nil {
		t.Fatalf("the repository's root is not at %s: %v", repositoryRoot, err)
	}
	for _, directory := range hostClockFree {
		findings, scanned, err := hostClockReads(filepath.Join(repositoryRoot, directory))
		if err != nil {
			t.Fatalf("%s: %v", directory, err)
		}
		if directory == "internal/api" && scanned == 0 {
			t.Fatal("no file of internal/api was scanned; the scan has stopped seeing files")
		}
		for _, finding := range findings {
			t.Errorf("%s/%s", directory, finding)
		}
	}
}

// hostClockReads lists every read of the host's clock in the non-test files of
// directory, and how many files it scanned. A directory that does not exist
// has none.
func hostClockReads(directory string) (findings []string, scanned int, err error) {
	info, err := os.Stat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if !info.IsDir() {
		return nil, 0, fmt.Errorf("%s is not a directory", directory)
	}
	paths, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		return nil, 0, err
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, 0, err
		}
		scanned++
		for _, name := range clockReadsIn(parsed) {
			findings = append(findings, filepath.Base(path)+" names time."+name)
		}
	}
	return findings, scanned, nil
}

// clockReadsIn follows the name the file gives package time, so an alias does
// not hide a read; a dot import is refused outright, since its reads have no
// package name to find.
func clockReadsIn(file *ast.File) []string {
	local := ""
	for _, spec := range file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path != "time" {
			continue
		}
		local = "time"
		if spec.Name != nil {
			local = spec.Name.Name
		}
	}
	if local == "" {
		return nil
	}
	if local == "." {
		return []string{"* through a dot import"}
	}
	var names []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if owner, ok := selector.X.(*ast.Ident); ok && owner.Name == local && hostClockReaders[selector.Sel.Name] {
			names = append(names, selector.Sel.Name)
		}
		return true
	})
	return names
}

func TestTheClockScanFindsEveryReadAndOnlyThoseOfNonTestFiles(t *testing.T) {
	directory := t.TempDir()
	files := map[string]string{
		"plain.go":      "package p\nimport \"time\"\nfunc f() time.Time { return time.Now() }\n",
		"value.go":      "package p\nimport \"time\"\nvar clock = time.Now\n",
		"aliased.go":    "package p\nimport clock \"time\"\nfunc g() clock.Duration { return clock.Since(clock.Time{}) }\n",
		"dotted.go":     "package p\nimport . \"time\"\nvar zero Time\n",
		"injected.go":   "package p\nimport \"time\"\ntype c struct{ Now func() time.Time }\nfunc h(x c) time.Time { return x.Now() }\n",
		"other.go":      "package p\ntype q struct{}\nfunc (q) Now() int { return 0 }\nvar time q\nvar n = time.Now()\n",
		"plain_test.go": "package p\nimport \"time\"\nvar t0 = time.Now()\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	findings, scanned, err := hostClockReads(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"aliased.go names time.Since", "dotted.go names time.* through a dot import",
		"plain.go names time.Now", "value.go names time.Now",
	}
	if scanned != 6 || strings.Join(findings, "|") != strings.Join(want, "|") {
		t.Errorf("scanned %d files and found %q, want 6 files and %q", scanned, findings, want)
	}

	missing, scanned, err := hostClockReads(filepath.Join(directory, "not-yet"))
	if err != nil || scanned != 0 || len(missing) != 0 {
		t.Errorf("a missing directory: %q, %d files, %v", missing, scanned, err)
	}
}
