package api

import (
	"context"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// listedRow is a listed transaction with the ledger row it came from and what
// interpreting it needed from the rest of the ledger.
type listedRow struct {
	listed txrules.Transaction
	source ledger.Transaction
	around ledgerContext
}

// findListed resolves a provider id through the read the list answers from:
// the row's own Brazil month, widened by the pairing window and cut back. An
// id that read does not list (an unknown one, a tax row folded into its
// purchase, a pot movement) is not found.
func findListed(ctx context.Context, snapshot *ledger.View, id string) (listedRow, bool, error) {
	all, err := snapshot.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return listedRow{}, false, err
	}
	source, found := rowWithID(all, id)
	if !found {
		return listedRow{}, false, nil
	}
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return listedRow{}, false, err
	}
	view, err := interpretMonthOf(all, source, around)
	if err != nil {
		return listedRow{}, false, err
	}
	for _, listed := range view.listed {
		if listed.ID == id {
			return listedRow{listed: listed, source: source, around: around}, true, nil
		}
	}
	return listedRow{}, false, nil
}

// transactionTarget is the transaction a write names, with the ledger's facts
// about it now and the key everything said about it is kept under.
type transactionTarget struct {
	listedRow
	key authored.TxKey
}

// lookupTarget is how every write of a transaction finds the one it names: the
// listed transaction the detail would answer for id, indexed inside the
// write's change, which mints its key at first sight. found is false, and
// nothing is written, for an id the list does not show; what that answers is
// the caller's to decide.
func lookupTarget(ctx context.Context, writes Writes, id string) (transactionTarget, bool, error) {
	row, found, err := findListed(ctx, writes.Ledger, id)
	if err != nil || !found {
		return transactionTarget{}, found, err
	}
	key, err := indexListed(ctx, writes.Recorder, row.listed)
	if err != nil {
		return transactionTarget{}, false, err
	}
	return transactionTarget{listedRow: row, key: key}, true, nil
}

// listedLedger interprets every row of the ledger in one pass, as the card
// does, for a write that spans months; it indexes what it touches with
// indexListed.
func listedLedger(ctx context.Context, snapshot *ledger.View) (interpreted, ledgerContext, error) {
	around, err := loadContext(ctx, snapshot)
	if err != nil {
		return interpreted{}, ledgerContext{}, err
	}
	rows, err := snapshot.CleanTransactions(ctx, wholeLedger)
	if err != nil {
		return interpreted{}, ledgerContext{}, err
	}
	view, err := interpretRows(rows, around, wholeLedger)
	return view, around, err
}

// indexListed answers the key of a listed transaction inside the recorder's
// change. The amount it keeps is the row's own, signed as the list signs it
// and without the tax rows folded under it, which a bank adds after the
// purchase: a pending row and the posted one that replaces it compare equal
// only on what both of them carry.
func indexListed(ctx context.Context, recorder *authored.Recorder, listed txrules.Transaction) (authored.TxKey, error) {
	return recorder.IndexTransaction(ctx, authored.SeenTransaction{
		ProviderID: listed.ID, AccountID: listed.AccountID, Day: listed.Day, Cents: listed.Amount - listed.Tax,
		EstablishmentKey: listed.EstablishmentKey, BankPending: !listed.Posted,
	})
}

// shownName is the name to show for a row of an establishment or a person: the
// nickname kept for its key when there is one, otherwise the ledger's name.
func shownName(establishments map[string]authored.Establishment, key, ledgerName string) string {
	if nickname := establishments[key].Nickname; nickname != "" {
		return nickname
	}
	return ledgerName
}

// establishmentsOf is what is kept about the establishments and people of
// listed, read once for all of them.
func establishmentsOf(ctx context.Context, view *authored.View, listed []txrules.Transaction) (map[string]authored.Establishment, error) {
	seen := map[string]bool{}
	keys := make([]string, 0, len(listed))
	for _, transaction := range listed {
		if !seen[transaction.EstablishmentKey] {
			seen[transaction.EstablishmentKey] = true
			keys = append(keys, transaction.EstablishmentKey)
		}
	}
	return view.Establishments(ctx, keys)
}
