// Package ingest brings the app's database up to date with the ledger's
// current snapshot: it refreshes the index of the transactions the database
// has met, carries what was said about a bank-pending charge to the posted row
// that replaced it, and classifies each new transaction by rule once. It
// returns counts and logs nothing: what it reads is a person's financial
// history.
package ingest

import (
	"context"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// Trigger is why a run was called.
type Trigger int

const (
	// TriggerTick is the timer. It is the zero value: a call that does not say
	// why does the least.
	TriggerTick Trigger = iota
	// TriggerStart is the server starting.
	TriggerStart
	// TriggerBudgetChange follows a save of the budget, which can give a month
	// its first line and so transactions to evaluate in a run already taken in.
	TriggerBudgetChange
	// TriggerCommand is the person asking for a run.
	TriggerCommand
)

// stopsAtTheRunTakenIn says whether a call for this reason does nothing when
// the snapshot's run is the one taken in last.
func (trigger Trigger) stopsAtTheRunTakenIn() bool {
	return trigger == TriggerTick || trigger == TriggerStart
}

// Method is the method recorded on the change a run writes. It is no method of
// the contract, so nothing that counts changes by method counts a run's.
const Method = "ingest.run"

// Sources are the two files a run brings together.
type Sources struct {
	Ledger   *ledger.Reader
	Database *authored.Database
}

// Run takes the snapshot in. The ledger is read first, in one view held until
// the run ends; then one immediate transaction of the app's database, by the
// rule, holds every write, so a failure changes nothing. What it authors is one
// change by the rule, written only when something authored changed, which an
// undo takes back whole. now is the instant of that change.
func Run(ctx context.Context, sources Sources, trigger Trigger, now time.Time) (Counts, error) {
	return run(ctx, sources, trigger, now, nil)
}

// run is Run with a seam between reading the ledger and taking the lock, where
// a test makes a write of the person arrive.
func run(ctx context.Context, sources Sources, trigger Trigger, now time.Time, afterRead func()) (Counts, error) {
	view := sources.Ledger.View()
	defer view.Close()
	taking, err := view.ProjectedRun(ctx)
	if err != nil {
		return Counts{}, ledgerFailure(err)
	}
	stop, err := alreadyTakenIn(ctx, sources.Database, taking, trigger)
	if err != nil || stop {
		return Counts{Stopped: stop}, err
	}
	listed, err := listedTransactions(ctx, view)
	if err != nil {
		return Counts{}, err
	}
	if afterRead != nil {
		afterRead()
	}
	return takeIn(ctx, sources.Database, snapshot{run: taking, listed: listed}, trigger, now)
}

// snapshot is what one view of the ledger gave the run.
type snapshot struct {
	run    int64
	listed []txrules.Transaction
}

// alreadyTakenIn is the stop of step 1, read before the lock so a tick on a
// run already taken in never waits for it. The run decides again under the
// lock, from what it reads there.
func alreadyTakenIn(ctx context.Context, database *authored.Database, run int64, trigger Trigger) (bool, error) {
	view := database.View()
	defer view.Close()
	taken, found, err := view.LastIngestedRun(ctx)
	if err != nil {
		return false, databaseFailure(err)
	}
	return stopsAt(taken, found, run, trigger), nil
}

// stopsAt says whether a run of the snapshot of run does nothing. A snapshot
// older than the run taken in last is never taken in, whoever calls: a caller
// that read it before another caller took a newer one in would move the index
// back. Run ids only grow, so older is lower.
func stopsAt(taken int64, found bool, run int64, trigger Trigger) bool {
	if !found {
		return false
	}
	return taken > run || taken == run && trigger.stopsAtTheRunTakenIn()
}

func takeIn(ctx context.Context, database *authored.Database, taking snapshot, trigger Trigger, now time.Time) (Counts, error) {
	var counts Counts
	facts := authored.ChangeFacts{Author: authored.AuthorRule, Method: Method, At: now}
	err := database.Change(ctx, facts, func(recorder *authored.Recorder) error {
		view := recorder.View()
		defer view.Close()
		taken, found, err := view.LastIngestedRun(ctx)
		if err != nil {
			return err
		}
		if stopsAt(taken, found, taking.run, trigger) {
			counts.Stopped = true
			return nil
		}
		step := &pass{ctx: ctx, recorder: recorder, view: view, run: taking.run, previous: taken, counts: &counts}
		if err := step.takeIn(taking.listed); err != nil {
			return err
		}
		return recorder.RecordIngestedRun(ctx, taking.run)
	})
	if err != nil {
		return Counts{}, databaseFailure(err)
	}
	return counts, nil
}

// pass is one run's work inside its transaction. view reads on that
// transaction, so every decision is taken from what it sees under the lock.
type pass struct {
	ctx      context.Context
	recorder *authored.Recorder
	view     *authored.View
	run      int64
	// previous is the run taken in before this one, zero for none.
	previous int64
	counts   *Counts
}

func (step *pass) takeIn(listed []txrules.Transaction) error {
	rows, err := step.view.IndexedTransactions(step.ctx)
	if err != nil {
		return err
	}
	index := indexOf(rows)
	shown, err := step.refreshAndPair(listed, index)
	if err != nil {
		return err
	}
	return step.evaluate(listed, index, shown)
}
