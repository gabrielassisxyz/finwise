package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const ingestUsage = "usage: finwise ingest --db <file> --ledger <snapshot>"

// ingestFunc is ingest.Run, which main hands the commands; a test hands one
// that counts or holds the runs.
type ingestFunc func(ctx context.Context, sources ingest.Sources, trigger ingest.Trigger, now time.Time) (ingest.Counts, error)

// errIngestPanicked stands for a run that panicked. What it panicked with is
// dropped, since it may quote a value of the ledger.
var errIngestPanicked = errors.New("finwise: the ingest panicked")

// ingester takes the ledger in for one caller. All that is ever written about
// a run is its counts or the kind of its failure: a run reads a person's whole
// financial history, and the text of an error of the ledger's reader can quote
// a row.
type ingester struct {
	sources ingest.Sources
	run     ingestFunc
}

// take runs the ingest once. Its context cannot end it midway: a run that has
// begun goes on to its commit or its rollback, so a shutdown or a client that
// hangs up never leaves work half done, and whoever waits for it knows the
// database is still when it returns.
func (in ingester) take(ctx context.Context, trigger ingest.Trigger, at time.Time) (counts ingest.Counts, err error) {
	defer func() {
		if recover() != nil {
			counts, err = ingest.Counts{}, errIngestPanicked
		}
	}()
	return in.run(context.WithoutCancel(ctx), in.sources, trigger, at)
}

// serverIngests are the runs of a server, each reported on its log: one line
// for a run that went past its first step or failed, none for a start or a
// tick that found the snapshot's run already taken in.
type serverIngests struct {
	in     ingester
	logger *slog.Logger
	now    func() time.Time
}

func (runs serverIngests) once(ctx context.Context, trigger ingest.Trigger, at time.Time) error {
	counts, err := runs.in.take(ctx, trigger, at)
	switch {
	case err != nil:
		runs.logger.Warn("ingest failed", "trigger", triggerName(trigger), "kind", failureKind(err))
	case !counts.Stopped:
		logCounts(runs.logger, trigger, counts)
	}
	return err
}

// afterBudgetSaved is the API's call once a budget save has committed. It
// goes past a run already taken in: the save may have given a month its first
// line, and so transactions to evaluate.
func (runs serverIngests) afterBudgetSaved(ctx context.Context, at time.Time) {
	runs.once(ctx, ingest.TriggerBudgetChange, at)
}

// beforeWrite is the API's call when a write found that the snapshot it reads
// has not been taken in. Its error goes back to the write, which then records
// nothing rather than index from that snapshot.
func (runs serverIngests) beforeWrite(ctx context.Context, at time.Time) error {
	return runs.once(ctx, ingest.TriggerWrite, at)
}

// every runs the ingest at once, as the start, and then on each tick of
// interval, one run at a time, until ctx ends. A failed run is only logged:
// the next tick tries again. stop ends the timer and returns once no run is in
// progress, so the database can be closed after it.
func (runs serverIngests) every(ctx context.Context, interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runs.once(ctx, ingest.TriggerStart, runs.now())
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				runs.once(ctx, ingest.TriggerTick, runs.now())
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// logCounts writes a run's line: why it ran and how many, never which.
func logCounts(logger *slog.Logger, trigger ingest.Trigger, counts ingest.Counts) {
	logger.Info("ingest", "trigger", triggerName(trigger), "stopped", counts.Stopped,
		"indexed", counts.Indexed, "paired", counts.Paired, "unpaired", counts.Unpaired, "applied", counts.Applied,
		"pending_low_confidence", counts.PendingLowConfidence, "pending_unnamed_person", counts.PendingUnnamedPerson,
		"pending_new_establishment", counts.PendingNewEstablishment, "left", counts.Left,
		"nothing_to_store", counts.NothingToStore, "already_decided", counts.AlreadyDecided)
}

func triggerName(trigger ingest.Trigger) string {
	switch trigger {
	case ingest.TriggerStart:
		return "start"
	case ingest.TriggerBudgetChange:
		return "budget-change"
	case ingest.TriggerCommand:
		return "command"
	case ingest.TriggerWrite:
		return "write"
	}
	return "tick"
}

// failureKind is the one word a failure is reported by.
func failureKind(err error) string {
	if errors.Is(err, errIngestPanicked) {
		return "panic"
	}
	if kind, classified := ingest.FailureOf(err); classified {
		return kind.String()
	}
	return "unknown"
}

// runIngest takes the snapshot in once, as the person asking: a run already
// taken in is gone through again, and finds nothing new. The two paths are read
// as serve reads them and the database is opened as serve opens it; no PIN is
// needed, since nothing is served.
func runIngest(ctx context.Context, args []string, env environment) int {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(env.stderr)
	databaseFlag := flags.String("db", "", "path of the app's own database, created if missing (FINWISE_DB)")
	ledgerFlag := flags.String("ledger", "", "path of the ledger snapshot (FINWISE_LEDGER)")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	databasePath := firstNonEmpty(*databaseFlag, env.getenv("FINWISE_DB"))
	ledgerPath := firstNonEmpty(*ledgerFlag, env.getenv("FINWISE_LEDGER"))
	if databasePath == "" || ledgerPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(env.stderr, ingestUsage)
		return exitUsage
	}
	database, err := authored.Open(ctx, databasePath, authored.Options{Now: env.now})
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: %v\n", err)
		return exitFailure
	}
	in := ingester{sources: ingest.Sources{Ledger: ledger.New(ledgerPath), Database: database}, run: env.runIngest}
	counts, err := in.take(ctx, ingest.TriggerCommand, env.now())
	if closeErr := database.Close(); closeErr != nil && err == nil {
		fmt.Fprintf(env.stderr, "finwise: close %s: %v\n", databasePath, closeErr)
		return exitFailure
	}
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: the ingest failed: %s; nothing was taken in\n", failureKind(err))
		return exitFailure
	}
	logCounts(slog.New(slog.NewTextHandler(env.stdout, nil)), ingest.TriggerCommand, counts)
	return 0
}
