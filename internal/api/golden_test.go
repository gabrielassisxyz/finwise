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

func compareWithGolden(t *testing.T, path string, answer []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, answer, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: %v (run the test with -update to record it)", path, err)
		return
	}
	if !bytes.Equal(want, answer) {
		t.Errorf("%s differs from the answer; run with -update after checking the change is intended", path)
	}
}

func TestEveryGoldenFileHasACase(t *testing.T) {
	expected := map[string]bool{}
	for _, ledgerName := range []string{"full", "empty"} {
		for _, c := range readCases {
			path, err := goldenPath(ledgerName, c.method, c.arguments)
			if err != nil {
				t.Fatal(err)
			}
			expected[path] = true
		}
	}
	found, err := filepath.Glob(filepath.Join("testdata", "golden", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range found {
		if !expected[path] {
			t.Errorf("%s belongs to no case; delete it", path)
		}
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
