package api

import (
	"sort"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// readCoverageProblems names every disagreement between the manifest's reads
// and the registered handlers: a read with no handler, and a handler for a
// method that is not a read of the manifest.
func readCoverageProblems(manifestReads, registered []string) []string {
	handled := map[string]bool{}
	for _, name := range registered {
		handled[name] = true
	}
	inManifest := map[string]bool{}
	var problems []string
	for _, name := range manifestReads {
		inManifest[name] = true
		if !handled[name] {
			problems = append(problems, name+": a read method with no handler")
		}
	}
	for name := range handled {
		if !inManifest[name] {
			problems = append(problems, name+": registered but not a read method of the manifest")
		}
	}
	sort.Strings(problems)
	return problems
}

func manifestReadNames(t *testing.T) []string {
	t.Helper()
	methods, err := contract.Load()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, method := range methods {
		if method.Class == contract.ClassRead {
			names = append(names, method.Qualified())
		}
	}
	return names
}

// registeredReadNames is the registry's handlers of the manifest's reads: the
// registry also holds a write's, which has no business in a read's coverage.
func registeredReadNames(t *testing.T) []string {
	t.Helper()
	isRead := map[string]bool{}
	for _, name := range manifestReadNames(t) {
		isRead[name] = true
	}
	var names []string
	for _, name := range NewReadRegistry().Names() {
		if isRead[name] {
			names = append(names, name)
		}
	}
	return names
}

func TestEveryReadMethodHasAHandler(t *testing.T) {
	for _, problem := range readCoverageProblems(manifestReadNames(t), registeredReadNames(t)) {
		t.Error(problem)
	}
}

func TestTheCoverageCheckFailsWhenAHandlerIsMissingOrInvented(t *testing.T) {
	reads, handled := manifestReadNames(t), registeredReadNames(t)
	if len(readCoverageProblems(reads, handled[1:])) == 0 {
		t.Error("a read without a handler went unnoticed")
	}
	if len(readCoverageProblems(reads, append([]string{"made.up"}, handled...))) == 0 {
		t.Error("a handler for a method that is not a read went unnoticed")
	}
	if len(readCoverageProblems(reads, handled)) != 0 {
		t.Error("the registry as it is was reported as a problem")
	}
}
