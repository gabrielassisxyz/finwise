package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// -update rewrites the golden files. They are committed to a public repository,
// so it refuses to run while FINWISE_LEDGER_SNAPSHOT points at a real ledger.
var updateGolden = flag.Bool("update", false, "rewrite the golden files from the current answers")

const snapshotVariable = "FINWISE_LEDGER_SNAPSHOT"

// goldenUpdateRefusal is the error that keeps a real ledger's answers out of a file.
func goldenUpdateRefusal(update bool, snapshot string) error {
	if update && snapshot != "" {
		return errors.New("-update is refused while " + snapshotVariable + " is set: golden files are committed and a real ledger must not reach them")
	}
	return nil
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// goldenName is the file of a call: <domain>.<method>[.<slug of the arguments>].json.
// The slug is the argument array as compact JSON with object keys sorted,
// lowercased, every run of characters outside [a-z0-9] replaced by one "-" and
// the ends trimmed. A call with no arguments has no slug.
func goldenName(method, arguments string) (string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return "", err
	}
	var compact bytes.Buffer
	encoder := json.NewEncoder(&compact)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(decoded); err != nil {
		return "", err
	}
	slug := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(compact.String()), "-"), "-")
	if slug == "" {
		return method + ".json", nil
	}
	return method + "." + slug + ".json", nil
}

func goldenPath(ledgerName, method, arguments string) (string, error) {
	name, err := goldenName(method, arguments)
	if err != nil {
		return "", err
	}
	return filepath.Join("testdata", "golden", ledgerName, name), nil
}

func prettyJSON(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func TestEveryAnswerMatchesItsGoldenFile(t *testing.T) {
	if err := goldenUpdateRefusal(*updateGolden, os.Getenv(snapshotVariable)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"full", "empty"} {
		env := newReadEnv(t, ledgerNamed(t, name))
		for _, c := range readCases {
			status, body := env.answer(c.method, c.arguments)
			if status != http.StatusOK {
				t.Errorf("%s ledger, %s %s: status %d", name, c.method, c.arguments, status)
				continue
			}
			path, err := goldenPath(name, c.method, c.arguments)
			if err != nil {
				t.Fatal(err)
			}
			compareWithGolden(t, path, prettyJSON(t, body))
		}
	}
}

// writeGoldenFile records an answer as a golden file. Every directory of golden
// files is committed to a public repository, so every one of them is refused here
// while a real ledger is named.
func writeGoldenFile(path string, answer []byte, update bool, snapshot string) error {
	if err := goldenUpdateRefusal(update, snapshot); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, answer, 0o644)
}

// goldenProblem is empty when the file holds exactly the answer.
func goldenProblem(path string, answer []byte) string {
	want, err := os.ReadFile(path)
	if err != nil {
		return path + ": " + err.Error() + " (run the test with -update to record it)"
	}
	if !bytes.Equal(want, answer) {
		return path + " differs from the answer; run with -update after checking the change is intended"
	}
	return ""
}

func compareWithGolden(t *testing.T, path string, answer []byte) {
	t.Helper()
	if *updateGolden {
		if err := writeGoldenFile(path, answer, true, os.Getenv(snapshotVariable)); err != nil {
			t.Fatal(err)
		}
		return
	}
	if problem := goldenProblem(path, answer); problem != "" {
		t.Error(problem)
	}
}

// goldenDirectories are the four directories of golden files: the read answers
// over each of the two invented ledgers, the read answers over the full ledger
// with an invented authored state, and the effects of the recorded writes.
var goldenDirectories = []string{"full", "empty", "full-authored", "writes"}

// strayGoldenFiles names every .json file under root that is not in expected
// (paths of the form <directory>/<name>), whatever its directory is called.
func strayGoldenFiles(root string, expected map[string]bool) ([]string, error) {
	found, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var stray []string
	for _, path := range found {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		if !expected[filepath.ToSlash(relative)] {
			stray = append(stray, relative)
		}
	}
	return stray, nil
}

// expectedGoldenFiles is every file a case owns, as <directory>/<name>.
func expectedGoldenFiles(t *testing.T) map[string]bool {
	t.Helper()
	expected := map[string]bool{}
	add := func(directory, method, arguments string) {
		name, err := goldenName(method, arguments)
		if err != nil {
			t.Fatal(err)
		}
		expected[directory+"/"+name] = true
	}
	for _, ledgerName := range []string{"full", "empty"} {
		for _, c := range readCases {
			add(ledgerName, c.method, c.arguments)
		}
	}
	for _, c := range authoredReadCases {
		add("full-authored", c.method, c.arguments)
	}
	for _, c := range writeCases {
		if c.expects == expectsRecorded {
			add("writes", c.method, c.arguments)
		}
	}
	return expected
}

func TestEveryGoldenFileHasACase(t *testing.T) {
	stray, err := strayGoldenFiles(filepath.Join("testdata", "golden"), expectedGoldenFiles(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range stray {
		t.Errorf("%s belongs to no case; delete it", path)
	}
}

func TestAStrayGoldenFileIsFoundInEveryDirectoryAndInOneOfAnyOtherName(t *testing.T) {
	root := t.TempDir()
	expected := map[string]bool{}
	for _, directory := range append(append([]string(nil), goldenDirectories...), "another") {
		for _, name := range []string{"owned.json", "stray.json"} {
			path := filepath.Join(root, directory, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if directory != "another" {
			expected[directory+"/owned.json"] = true
		}
	}
	stray, err := strayGoldenFiles(root, expected)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(stray)
	want := []string{"another/owned.json", "another/stray.json", "empty/stray.json", "full-authored/stray.json", "full/stray.json", "writes/stray.json"}
	if !slices.Equal(stray, want) {
		t.Errorf("found %v, want %v", stray, want)
	}
}

func TestGoldenNamesCanBeDerivedInAnotherLanguage(t *testing.T) {
	cases := []struct{ method, arguments, want string }{
		{"shell.overview", `[]`, "shell.overview.json"},
		{"transactions.detail", `["tx-1"]`, "transactions.detail.tx-1.json"},
		{"transactions.list", `[{"period":"month","month":"2026-02"}]`, "transactions.list.month-2026-02-period-month.json"},
		{"transactions.list", `[ { "month" : "2026-03", "period" : "last-3-months" } ]`, "transactions.list.month-2026-03-period-last-3-months.json"},
	}
	for _, c := range cases {
		if got, err := goldenName(c.method, c.arguments); err != nil || got != c.want {
			t.Errorf("%s %s named %q (%v), want %q", c.method, c.arguments, got, err, c.want)
		}
	}
}

func TestTheGoldenUpdateRefusesARealLedger(t *testing.T) {
	if goldenUpdateRefusal(true, "/some/real.db") == nil {
		t.Error("-update with a snapshot set was allowed")
	}
	if goldenUpdateRefusal(false, "/some/real.db") != nil {
		t.Error("a plain run was refused because a snapshot is set")
	}
	if goldenUpdateRefusal(true, "") != nil {
		t.Error("-update was refused with no snapshot set")
	}
}

// Every directory is written through writeGoldenFile, so the refusal that keeps
// a real ledger's answers out of the files covers the two directories of the
// write side as it covers the two of the reads.
func TestNoDirectoryOfGoldenFilesIsWrittenWhileARealLedgerIsNamed(t *testing.T) {
	root := t.TempDir()
	for _, directory := range goldenDirectories {
		path := filepath.Join(root, directory, "x.json")
		if err := writeGoldenFile(path, []byte("null\n"), true, "/some/real.db"); err == nil {
			t.Errorf("%s: the update was allowed", directory)
		}
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s: a file was written", directory)
		}
		if err := writeGoldenFile(path, []byte("null\n"), true, ""); err != nil {
			t.Errorf("%s: an update with no snapshot named was refused: %v", directory, err)
		}
	}
}

func TestAGoldenFileThatIsMissingOrDiffersIsAProblem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "writes", "x.json")
	if goldenProblem(path, []byte("1\n")) == "" {
		t.Error("a missing file was not a problem")
	}
	if err := writeGoldenFile(path, []byte("1\n"), true, ""); err != nil {
		t.Fatal(err)
	}
	if problem := goldenProblem(path, []byte("1\n")); problem != "" {
		t.Errorf("an equal answer was a problem: %s", problem)
	}
	if goldenProblem(path, []byte("2\n")) == "" {
		t.Error("a different answer was not a problem")
	}
}
