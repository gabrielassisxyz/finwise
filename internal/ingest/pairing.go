package ingest

import (
	"cmp"
	"maps"
	"slices"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/classrules"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// indexState is the index as the pass read it before writing to it. keyOf
// gains the posted ids a pairing maps.
type indexState struct {
	rows  map[authored.TxKey]authored.IndexedTransaction
	keys  []authored.TxKey
	keyOf map[string]authored.TxKey
}

func indexOf(rows []authored.IndexedTransaction) indexState {
	index := indexState{rows: map[authored.TxKey]authored.IndexedTransaction{}, keyOf: map[string]authored.TxKey{}}
	for _, row := range rows {
		index.rows[row.Key] = row
		index.keys = append(index.keys, row.Key)
		for _, providerID := range row.ProviderIDs {
			index.keyOf[providerID] = row.Key
		}
	}
	return index
}

// refreshAndPair is step 3. It answers the listed transaction each indexed key
// stands for now, the paired ones included. A key whose day moves to another
// month, by a pairing or because the bank changed the date, has its references
// to lines that month's budget lacks cleared in this change, as a save of the
// budget clears them, or every later undo would be refused.
func (step *pass) refreshAndPair(listed []txrules.Transaction, index indexState) (map[authored.TxKey]txrules.Transaction, error) {
	shown := standingFor(listed, index.keyOf)
	var moved []authored.TxKey
	for _, key := range slices.Sorted(maps.Keys(shown)) {
		if err := step.refresh(key, shown[key], 0); err != nil {
			return nil, err
		}
		if monthOf(index.rows[key].Day) != monthOf(shown[key].Day) {
			moved = append(moved, key)
		}
	}
	paired, err := step.pair(listed, index, shown)
	if err != nil {
		return nil, err
	}
	moved = append(moved, paired...)
	if len(moved) == 0 {
		return shown, nil
	}
	return shown, step.recorder.ClearLinesTheirMonthLacks(step.ctx, authored.OnlyTransactions(moved...))
}

// standingFor picks, for each indexed key the ledger lists, the one listed
// transaction it stands for: a key listed under its pending and its posted id
// at once takes the posted row's facts, so the index is not moved back to the
// pending one.
func standingFor(listed []txrules.Transaction, keyOf map[string]authored.TxKey) map[authored.TxKey]txrules.Transaction {
	shown := map[authored.TxKey]txrules.Transaction{}
	for _, transaction := range listed {
		key, indexed := keyOf[transaction.ID]
		if !indexed {
			continue
		}
		if current, taken := shown[key]; !taken || standsBefore(transaction, current) {
			shown[key] = transaction
		}
	}
	return shown
}

func standsBefore(candidate, current txrules.Transaction) bool {
	if candidate.Posted != current.Posted {
		return candidate.Posted
	}
	return cmp.Or(cmp.Compare(current.Day, candidate.Day), cmp.Compare(candidate.ID, current.ID)) < 0
}

// refresh gives the key's index row the listed transaction's facts and this run
// as the last that listed it. replaces is the key a posted row continues.
func (step *pass) refresh(key authored.TxKey, transaction txrules.Transaction, replaces authored.TxKey) error {
	if _, err := step.recorder.IndexTransaction(step.ctx, seenOf(transaction, replaces)); err != nil {
		return err
	}
	return step.recorder.MarkListed(step.ctx, key, step.run)
}

// pair matches the bank-pending index rows the ledger no longer lists, however
// long ago they vanished, with the listed posted transactions the index has
// never met. Each pairing maps the posted id to the pending row's key, so what
// was said about the pending row holds for the posted one and no authored row
// is rewritten. It answers the paired keys whose month the posted row moved.
func (step *pass) pair(listed []txrules.Transaction, index indexState, shown map[authored.TxKey]txrules.Transaction) ([]authored.TxKey, error) {
	vanished := vanishedPending(index, shown)
	fresh, posted := newPosted(listed, index.keyOf)
	pairs, err := classrules.Pair(vanished, fresh)
	if err != nil {
		return nil, err
	}
	paired := map[authored.TxKey]bool{}
	var moved []authored.TxKey
	for _, pairing := range pairs {
		key, transaction := authored.TxKey(pairing.PendingKey), posted[pairing.PostedProviderID]
		if err := step.refresh(key, transaction, key); err != nil {
			return nil, err
		}
		shown[key], index.keyOf[transaction.ID], paired[key] = transaction, key, true
		step.counts.Paired++
		if monthOf(index.rows[key].Day) != monthOf(transaction.Day) {
			moved = append(moved, key)
		}
	}
	for _, pending := range vanished {
		if key := authored.TxKey(pending.Key); !paired[key] {
			if err := step.countUnpaired(index.rows[key]); err != nil {
				return nil, err
			}
		}
	}
	return moved, nil
}

func vanishedPending(index indexState, shown map[authored.TxKey]txrules.Transaction) []classrules.VanishedPending {
	var vanished []classrules.VanishedPending
	for _, key := range index.keys {
		row := index.rows[key]
		if _, listed := shown[key]; listed || !row.BankPending {
			continue
		}
		vanished = append(vanished, classrules.VanishedPending{
			Key: classrules.TxKey(key), AccountID: row.AccountID, Day: row.Day, Cents: row.Cents, EstablishmentKey: row.EstablishmentKey,
		})
	}
	return vanished
}

func newPosted(listed []txrules.Transaction, keyOf map[string]authored.TxKey) ([]classrules.NewPosted, map[string]txrules.Transaction) {
	var fresh []classrules.NewPosted
	posted := map[string]txrules.Transaction{}
	for _, transaction := range listed {
		if _, met := keyOf[transaction.ID]; met || !transaction.Posted {
			continue
		}
		fresh = append(fresh, classrules.NewPosted{
			ProviderID: transaction.ID, AccountID: transaction.AccountID, Day: transaction.Day,
			Cents: ownCents(transaction), EstablishmentKey: transaction.EstablishmentKey,
		})
		posted[transaction.ID] = transaction
	}
	return fresh, posted
}

// countUnpaired counts a vanished row in the run that stopped listing it and in
// no other: the first run taken in after the last one that listed it or, for a
// row no snapshot taken in ever listed, the first run that meets it, which it
// then marks.
func (step *pass) countUnpaired(row authored.IndexedTransaction) error {
	switch {
	case row.LastRun != nil:
		if *row.LastRun == step.previous {
			step.counts.Unpaired++
		}
	case row.FirstRun == nil:
		step.counts.Unpaired++
		return step.recorder.MarkMetUnlisted(step.ctx, row.Key, step.run)
	}
	return nil
}

// seenOf is a listed transaction as the index takes it in. replaces is zero or
// the key the transaction continues.
func seenOf(transaction txrules.Transaction, replaces authored.TxKey) authored.SeenTransaction {
	return authored.SeenTransaction{
		ProviderID: transaction.ID, AccountID: transaction.AccountID, Day: transaction.Day,
		Cents: ownCents(transaction), EstablishmentKey: transaction.EstablishmentKey,
		BankPending: !transaction.Posted, Replaces: replaces,
	}
}

// ownCents is the row's own amount, without the tax rows a bank folds under a
// purchase after it posts, as a write of the API indexes a listed row: a
// pending row and the posted one that replaces it compare equal only on what
// both of them carry.
func ownCents(transaction txrules.Transaction) money.Cents {
	return transaction.Amount - transaction.Tax
}

func monthOf(day brday.Day) brday.Month {
	return brday.Month(day[:min(len(day), len("2006-01"))])
}
