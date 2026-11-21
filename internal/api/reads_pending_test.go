package api

import (
	"sort"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// pendingReads are the `read` methods of the contract that still have no
// handler. The work that registers one removes its name here; the test below
// fails on a name that is both, on one that is neither, and on one that is not
// a read method at all.
var pendingReads = []string{
	"budget.document",
	"budget.establishments",
	"budget.scenarios",
	"budget.view",
	"card.overview",
	"home.summary",
	"netWorth.overview",
	"review.closing",
	"review.payday",
	"review.weekly",
	"savingsPots.detail",
	"savingsPots.list",
	"settings.overview",
	"subscriptions.candidates",
	"subscriptions.detail",
	"subscriptions.list",
	"subscriptions.recurring",
}

// readCoverageProblems names every disagreement between the manifest's reads
// and the registered handlers plus the pending list.
func readCoverageProblems(manifestReads, registered, pending []string) []string {
	covered := map[string]int{}
	for _, name := range registered {
		covered[name]++
	}
	for _, name := range pending {
		covered[name]++
	}
	inManifest := map[string]bool{}
	var problems []string
	for _, name := range manifestReads {
		inManifest[name] = true
		switch covered[name] {
		case 0:
			problems = append(problems, name+": a read method with neither a handler nor a place in the pending list")
		case 1:
		default:
			problems = append(problems, name+": both registered and pending")
		}
	}
	for name := range covered {
		if !inManifest[name] {
			problems = append(problems, name+": listed or registered but not a read method of the manifest")
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

func TestEveryReadMethodHasAHandlerOrIsPending(t *testing.T) {
	for _, problem := range readCoverageProblems(manifestReadNames(t), NewReadRegistry().Names(), pendingReads) {
		t.Error(problem)
	}
}

func TestTheCoverageCheckFailsWhenAPendingNameIsRemovedOrInvented(t *testing.T) {
	reads, handled := manifestReadNames(t), NewReadRegistry().Names()
	if len(readCoverageProblems(reads, handled, pendingReads[1:])) == 0 {
		t.Error("dropping a name from the pending list went unnoticed")
	}
	if len(readCoverageProblems(reads, handled, append([]string{"made.up"}, pendingReads...))) == 0 {
		t.Error("an invented pending name went unnoticed")
	}
	if len(readCoverageProblems(reads, append([]string{pendingReads[0]}, handled...), pendingReads)) == 0 {
		t.Error("a name both registered and pending went unnoticed")
	}
	if len(readCoverageProblems(reads, append([]string{pendingReads[0]}, handled...), pendingReads[1:])) != 0 {
		t.Error("moving a name from pending to registered was reported as a problem")
	}
}
