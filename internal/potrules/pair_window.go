package potrules

import (
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// yieldWindow is one window with everything the statement derives from it.
type yieldWindow struct {
	prev, curr YieldSighting
	// names and namesOf are the statement's "name_ids" and "id_names", over every
	// sighting from prev through curr.
	names     map[string]*nameUse
	namesOf   map[string]map[string]bool
	gap       bool
	transfers []resolvedTransfer
}

type nameUse struct {
	ids      map[string]bool
	oneID    string
	unstable bool
}

type resolvedTransfer struct {
	name      *string
	reserveID *string
	reason    *string
	cents     *money.Cents
	date      string
}

func (account AccountYield) windowOf(from, to brday.Day) (yieldWindow, bool) {
	ends := account.endsOf(from, to)
	if !ends.hasStart || !ends.hasEnd {
		return yieldWindow{}, false
	}
	window := yieldWindow{prev: account.Sightings[ends.start], curr: account.Sightings[ends.end]}
	if window.prev.UpdatedAt == "" || window.curr.UpdatedAt == "" || window.prev.UpdatedAt >= window.curr.UpdatedAt {
		return yieldWindow{}, false
	}
	window.names, window.namesOf = namesAcross(account.Sightings[ends.start : ends.end+1])
	window.gap = account.hasGap(window.prev, window.curr)
	window.transfers = account.transfersIn(window, to)
	return window, true
}

func namesAcross(span []YieldSighting) (map[string]*nameUse, map[string]map[string]bool) {
	names := map[string]*nameUse{}
	namesOf := map[string]map[string]bool{}
	for _, sighting := range span {
		for _, entry := range sighting.Entries {
			if entry.Name == nil {
				continue
			}
			use := names[*entry.Name]
			if use == nil {
				use = &nameUse{ids: map[string]bool{}}
				names[*entry.Name] = use
			}
			if entry.ReserveID != nil {
				if len(use.ids) == 0 || *entry.ReserveID < use.oneID {
					use.oneID = *entry.ReserveID
				}
				use.ids[*entry.ReserveID] = true
			}
			use.unstable = use.unstable || !entry.Stable
			if entry.Stable && entry.ReserveID != nil {
				if namesOf[*entry.ReserveID] == nil {
					namesOf[*entry.ReserveID] = map[string]bool{}
				}
				namesOf[*entry.ReserveID][*entry.Name] = true
			}
		}
	}
	return names, namesOf
}

// hasGap is the statement's "account_reason": the transactions walk of the run
// that ended the window must have gone back to where it started.
func (account AccountYield) hasGap(prev, curr YieldSighting) bool {
	for _, walk := range account.Walks {
		if walk.RunID == curr.RunID && walk.Since <= prev.UpdatedAt {
			return false
		}
	}
	return true
}

// transfersIn judges every transfer the window counts: the version of each
// document the ledger knew by the last day, if it is the account's and falls
// between the two times.
func (account AccountYield) transfersIn(window yieldWindow, last brday.Day) []resolvedTransfer {
	var resolved []resolvedTransfer
	for _, document := range account.Transfers {
		version, known := latestVersionBy(document, last)
		if !known || version.Parent != account.AccountID || !version.IsReserve {
			continue
		}
		if version.Instant == "" || version.Instant <= window.prev.UpdatedAt || version.Instant > window.curr.UpdatedAt {
			continue
		}
		resolved = append(resolved, window.resolve(version, goneBy(document, version, last)))
	}
	return resolved
}

func latestVersionBy(document TransferDocument, last brday.Day) (TransferVersion, bool) {
	var latest TransferVersion
	known := false
	for _, version := range document.Versions {
		if version.Day <= last && (!known || version.ObservationID > latest.ObservationID) {
			latest, known = version, true
		}
	}
	return latest, known
}

func goneBy(document TransferDocument, version TransferVersion, last brday.Day) bool {
	for _, removal := range document.Removals {
		if removal.RunID > version.RunID && removal.Day <= last {
			return true
		}
	}
	return false
}

// resolve is the statement's "judged" and "resolved": why the version cannot be
// counted, and which reserve its name picks out.
func (window yieldWindow) resolve(version TransferVersion, gone bool) resolvedTransfer {
	transfer := resolvedTransfer{name: version.Name, cents: version.Cents, date: version.Date}
	reason := judgedReason(version, gone)
	var use *nameUse
	if version.Name != nil {
		use = window.names[*version.Name]
	}
	if use != nil && len(use.ids) == 1 && !use.unstable {
		transfer.reserveID = &use.oneID
	}
	if reason == nil {
		reason = window.nameReason(use)
	}
	transfer.reason = reason
	return transfer
}

func judgedReason(version TransferVersion, gone bool) *string {
	switch {
	case gone:
		return reasonText(ReasonTransferRemoved)
	case version.NotPosted:
		return reasonText(ReasonTransferPending)
	case version.Unsettled:
		return reasonText(ReasonTransferUnsettled)
	}
	return version.Fault
}

func (window yieldWindow) nameReason(use *nameUse) *string {
	switch {
	case use == nil:
		return reasonText(ReasonUnmatchedTransfer)
	case len(use.ids) > 1 || use.unstable:
		return reasonText(ReasonNameReused)
	case len(window.namesOf[use.oneID]) > 1:
		return reasonText(ReasonNameChanged)
	}
	return nil
}

func reasonText(reason string) *string { return &reason }

// rows are the statement's three SELECTs: the stable reserves, the entries with
// no stable identity, and the transfers that name no stable reserve.
func (window yieldWindow) rows() []PairYield {
	rows := window.reserveRows()
	rows = append(rows, unstableRows(window.prev.Entries, true)...)
	rows = append(rows, unstableRows(window.curr.Entries, false)...)
	for _, transfer := range window.transfers {
		if transfer.reserveID == nil {
			rows = append(rows, PairYield{ReserveName: transfer.name, Transfers: transfer.cents, Reason: transfer.reason})
		}
	}
	return rows
}

func stableByID(entries []YieldEntry) map[string]YieldEntry {
	byID := map[string]YieldEntry{}
	for _, entry := range entries {
		if entry.Stable && entry.ReserveID != nil {
			byID[*entry.ReserveID] = entry
		}
	}
	return byID
}

func unstableIDs(sightings ...YieldSighting) map[string]bool {
	ids := map[string]bool{}
	for _, sighting := range sightings {
		for _, entry := range sighting.Entries {
			if !entry.Stable && entry.ReserveID != nil {
				ids[*entry.ReserveID] = true
			}
		}
	}
	return ids
}

func (window yieldWindow) reserveRows() []PairYield {
	before, after := stableByID(window.prev.Entries), stableByID(window.curr.Entries)
	unstable := unstableIDs(window.prev, window.curr)
	ids := make([]string, 0, len(before)+len(after))
	for id := range before {
		ids = append(ids, id)
	}
	for id := range after {
		if _, twice := before[id]; !twice {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	rows := make([]PairYield, 0, len(ids))
	for _, id := range ids {
		prev, inPrev := before[id]
		curr, inCurr := after[id]
		row := PairYield{ReserveID: &id, ReserveName: curr.Name}
		if row.ReserveName == nil {
			row.ReserveName = prev.Name
		}
		if inPrev {
			row.Previous = prev.Cents
		}
		if inCurr {
			row.Current = curr.Cents
		}
		sum, transferReason := window.transfersOf(id)
		row.Transfers = sum
		row.Reason = window.reasonOf(unstable[id], inPrev, prev, inCurr, curr, transferReason)
		if row.Reason == nil {
			yield := cents(row.Current) - cents(row.Previous) + cents(row.Transfers)
			row.Yield = &yield
		}
		rows = append(rows, row)
	}
	return rows
}

func cents(amount *money.Cents) money.Cents {
	if amount == nil {
		return 0
	}
	return *amount
}

// transfersOf adds up what moved into and out of the reserve and names the
// reason of the earliest transfer that cannot be counted.
func (window yieldWindow) transfersOf(id string) (sum *money.Cents, reason *string) {
	var earliest *resolvedTransfer
	for index := range window.transfers {
		transfer := &window.transfers[index]
		if transfer.reserveID == nil || *transfer.reserveID != id {
			continue
		}
		if transfer.reason == nil {
			total := cents(sum) + cents(transfer.cents)
			sum = &total
		} else if earliest == nil || transfer.date < earliest.date {
			earliest = transfer
		}
	}
	if earliest != nil {
		reason = earliest.reason
	}
	return sum, reason
}

// reasonOf is the statement's "decided", in its order: the first thing that
// breaks the arithmetic wins.
func (window yieldWindow) reasonOf(unstable, inPrev bool, prev YieldEntry, inCurr bool, curr YieldEntry, transfer *string) *string {
	switch {
	case window.gap:
		return reasonText(ReasonCoverageGap)
	case unstable:
		return reasonText(ReasonUnstable)
	case !inCurr:
		return reasonText(ReasonClosed)
	case (inPrev && !prev.BRL) || !curr.BRL:
		return reasonText(ReasonCurrency)
	case (inPrev && !prev.AmountOK) || !curr.AmountOK:
		return reasonText(ReasonAmount)
	}
	return transfer
}

func unstableRows(entries []YieldEntry, previous bool) []PairYield {
	var rows []PairYield
	for _, entry := range entries {
		if entry.Stable {
			continue
		}
		row := PairYield{ReserveID: entry.ReserveID, ReserveName: entry.Name, Reason: reasonText(ReasonUnstable)}
		if previous {
			row.Previous = entry.Cents
		} else {
			row.Current = entry.Cents
		}
		rows = append(rows, row)
	}
	return rows
}
