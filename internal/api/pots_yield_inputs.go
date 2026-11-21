package api

import (
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/potrules"
)

// yieldRulesOf hands the reader's rows to the rules, for the accounts asked
// about: the rules take plain values and the reader's types are not theirs.
func yieldRulesOf(history ledger.YieldHistory, accountIDs []string) map[string]potrules.AccountYield {
	rules := map[string]potrules.AccountYield{}
	for _, accountID := range accountIDs {
		rules[accountID] = potrules.AccountYield{AccountID: accountID}
	}
	addSightings(rules, history.Sightings)
	for _, walk := range history.Walks {
		if account, asked := rules[walk.AccountID]; asked {
			account.Walks = append(account.Walks, potrules.CoverageWalk{RunID: walk.RunID, Since: walk.Since})
			rules[walk.AccountID] = account
		}
	}
	addTransfers(rules, history)
	return rules
}

func addSightings(rules map[string]potrules.AccountYield, sightings []ledger.YieldSighting) {
	for _, sighting := range sightings {
		account, asked := rules[sighting.AccountID]
		if !asked {
			continue
		}
		entries := make([]potrules.YieldEntry, 0, len(sighting.Entries))
		for _, entry := range sighting.Entries {
			entries = append(entries, potrules.YieldEntry{
				ReserveID: entry.ReserveID, Name: entry.Name, Stable: entry.Stable,
				BRL: entry.BRL, AmountOK: entry.AmountOK, Cents: entry.Cents,
			})
		}
		account.Sightings = append(account.Sightings, potrules.YieldSighting{
			Day: sighting.Day, RunID: sighting.RunID, UpdatedAt: sighting.UpdatedAt, Entries: entries,
		})
		rules[sighting.AccountID] = account
	}
}

// addTransfers gives an account every document one of whose versions it held:
// the version that counts for a window is the document's latest by then, wherever
// that put it.
func addTransfers(rules map[string]potrules.AccountYield, history ledger.YieldHistory) {
	documents := map[int64]*potrules.TransferDocument{}
	var order []int64
	for _, transfer := range history.Transfers {
		document := documents[transfer.DocumentID]
		if document == nil {
			document = &potrules.TransferDocument{ID: transfer.DocumentID}
			documents[transfer.DocumentID] = document
			order = append(order, transfer.DocumentID)
		}
		document.Versions = append(document.Versions, potrules.TransferVersion{
			ObservationID: transfer.ObservationID, RunID: transfer.RunID, Parent: transfer.Parent, Day: transfer.Day,
			IsReserve: transfer.IsReserve, Date: transfer.Date, Instant: transfer.Instant, NotPosted: transfer.NotPosted,
			Unsettled: history.Unresolved[transfer.ProviderID], Name: transfer.Name, Fault: transfer.Fault, Cents: transfer.Cents,
		})
	}
	for _, removal := range history.Removals {
		if document := documents[removal.DocumentID]; document != nil {
			document.Removals = append(document.Removals, potrules.Removal{RunID: removal.RunID, Day: removal.Day})
		}
	}
	for _, id := range order {
		for _, version := range documents[id].Versions {
			if account, asked := rules[version.Parent]; asked {
				account.Transfers = appendOnce(account.Transfers, *documents[id])
				rules[version.Parent] = account
			}
		}
	}
}

func appendOnce(held []potrules.TransferDocument, document potrules.TransferDocument) []potrules.TransferDocument {
	if len(held) > 0 && held[len(held)-1].ID == document.ID {
		return held
	}
	return append(held, document)
}
