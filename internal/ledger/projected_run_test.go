package ledger_test

import (
	"database/sql"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// projectedRun asks a view of its own for the projected run.
func projectedRun(t *testing.T, path string) (int64, error) {
	t.Helper()
	view := ledger.New(path).View()
	defer view.Close()
	return view.ProjectedRun(t.Context())
}

func wantProjectedRun(t *testing.T, f *ledgertest.Fixture, want int64) {
	t.Helper()
	got, err := projectedRun(t, f.Path)
	if err != nil || got != want {
		t.Errorf("projected run = %d, %v; want %d", got, err, want)
	}
}

func TestANewFixtureIsProjectedToItsFirstRun(t *testing.T) {
	wantProjectedRun(t, ledgertest.New(t), 1)
}

func TestTheProjectedRunIsTheOneProjectedNotTheLatestOrTheLatestComplete(t *testing.T) {
	f := ledgertest.New(t)
	partial := f.AddRun(ledgertest.Run{Outcome: "partial", FinishedAt: instant(t, "2026-03-16T03:00:00Z")})
	if partial != 2 {
		t.Fatalf("AddRun returned %d, want 2", partial)
	}
	f.ProjectTo(partial)
	wantProjectedRun(t, f, 2)

	complete := f.AddRun(ledgertest.Run{Outcome: "complete", FinishedAt: instant(t, "2026-03-17T03:00:00Z")})
	if complete != 3 {
		t.Fatalf("AddRun returned %d, want 3", complete)
	}
	wantProjectedRun(t, f, 2)

	f.ProjectTo(complete)
	wantProjectedRun(t, f, 3)
}

func TestAFixtureWhoseOnlyRunWasPartialIsProjectedToIt(t *testing.T) {
	wantProjectedRun(t, ledgertest.NewWithFirstRunOutcome(t, "partial"), 1)
}

func TestASnapshotWithNoProjectionAnswersTheTypedError(t *testing.T) {
	const sentinel = "sentinel-7f3a-account-name"
	f := ledgertest.New(t)
	f.AddAccount(ledgertest.Account{ID: "acc-1", ItemID: "item-1", Type: "BANK", Name: sentinel, Currency: "BRL"})
	database, err := sql.Open("sqlite", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("DELETE FROM projection_state"); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(f.Path); err != nil || !strings.Contains(string(content), sentinel) {
		t.Fatalf("the sentinel is not in the file (%v), so the check below proves nothing", err)
	}

	run, err := projectedRun(t, f.Path)
	if !ledger.IsNotProjected(err) {
		t.Fatalf("projected run = %d, %v (%T); want the not-projected error", run, err, err)
	}
	if run != 0 {
		t.Errorf("a value came back with the error: %d", run)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Errorf("the error carries a value from the file: %q", err)
	}
}

// fatalRecorder stands in for the test a fixture reports to, so that a fixture
// refusing a call can be observed instead of failing this test.
type fatalRecorder struct {
	testing.TB
	message string
}

func (recorder *fatalRecorder) Helper() {}

func (recorder *fatalRecorder) Fatalf(format string, args ...any) {
	recorder.message = format
	runtime.Goexit()
}

func TestProjectingARunThatDoesNotExistFailsTheTest(t *testing.T) {
	recorder := &fatalRecorder{TB: t}
	f := ledgertest.New(recorder)

	var done sync.WaitGroup
	done.Go(func() { f.ProjectTo(9) })
	done.Wait()

	if recorder.message == "" {
		t.Fatal("ProjectTo accepted a run the ledger does not have")
	}
	wantProjectedRun(t, f, 1)
}
