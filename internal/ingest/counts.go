package ingest

// Counts is everything a run reports: how many, never which. Every transaction
// a run marks evaluated is counted in exactly one of Applied, the three
// Pending counts, Left, NothingToStore and AlreadyDecided, so they add up to
// Indexed.
type Counts struct {
	// Stopped is set when the run took nothing in: a tick or the start found the
	// snapshot's run already taken in, or the snapshot is older than the run
	// taken in last, whoever called.
	Stopped bool
	// Indexed is the transactions this run marked evaluated.
	Indexed int
	// Paired is the vanished bank-pending transactions whose posted row this
	// run found, one to one.
	Paired int
	// Unpaired is the bank-pending transactions the ledger stopped listing in
	// this run with no posted row matching one to one.
	Unpaired int

	Applied                 int
	PendingLowConfidence    int
	PendingUnnamedPerson    int
	PendingNewEstablishment int
	Left                    int
	NothingToStore          int
	// AlreadyDecided is the transactions the person or a rule had put on a line
	// or in the queue, or the person had split or left out with a reason, before
	// the rules met them, which the rules leave as they are.
	AlreadyDecided int
}

// Outcomes is how many evaluated transactions the outcome counts hold.
func (counts Counts) Outcomes() int {
	return counts.Applied + counts.PendingLowConfidence + counts.PendingUnnamedPerson + counts.PendingNewEstablishment +
		counts.Left + counts.NothingToStore + counts.AlreadyDecided
}
