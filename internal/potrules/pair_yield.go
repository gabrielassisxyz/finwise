package potrules

import (
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// What the collector's yield statement answers for a window, worked out from
// the sightings and the transfers the statement reads, so that every pair of
// consecutive sightings of an account is answered from one read of the ledger
// instead of one statement each. The statement stays in the ledger reader as the
// reference these rules are tested against, row for row.
//
// The yield of a reserve over a window is the change in its balance plus the
// transfers into and out of it inside the window: a deposit is negative on the
// checking account and raises the balance, so adding it back leaves the yield.

// The reasons the statement gives for a figure it cannot stand behind.
const (
	ReasonCoverageGap       = "coverage gap"
	ReasonClosed            = "closed or missing at D2"
	ReasonUnstable          = "unstable"
	ReasonCurrency          = "currency"
	ReasonAmount            = "amount"
	ReasonTransferRemoved   = "transfer removed"
	ReasonTransferPending   = "transfer pending"
	ReasonTransferUnsettled = "transfer unresolved"
	ReasonTransferSign      = "transfer sign"
	ReasonUnmatchedTransfer = "unmatched transfer"
	ReasonNameReused        = "name reused"
	ReasonNameChanged       = "name changed"

	// ReasonNoWindow is not the statement's: it answers no row at all when the
	// window has no start the ledger can place, and a day of a month cannot be left
	// out of its sum in silence.
	ReasonNoWindow = "no window"
)

// YieldEntry is a reserve entry of a sighting with the checks on its amount
// already made by the reader.
type YieldEntry struct {
	ReserveID *string
	Name      *string
	// Stable says the id names this entry and no other in its sighting.
	Stable bool
	BRL    bool
	// AmountOK says the amount is readable, finite and has at most two decimals.
	AmountOK bool
	// Cents is nil only when the entry has no amount.
	Cents *money.Cents
}

// YieldSighting is an unambiguous sighting that reports its reserves as a list.
// UpdatedAt is the provider's time in a form that compares as text, empty when
// the provider gave none that can be compared.
type YieldSighting struct {
	Day       brday.Day
	RunID     int64
	UpdatedAt string
	Entries   []YieldEntry
}

// TransferVersion is one unambiguous version of a document that was a reserve
// transfer. Fault is the reason the version alone gives for not counting it.
type TransferVersion struct {
	ObservationID int64
	RunID         int64
	Parent        string
	Day           brday.Day
	IsReserve     bool
	// Date is the provider's own text, Instant the same time in a form that
	// compares; Instant is empty when it cannot be read.
	Date      string
	Instant   string
	NotPosted bool
	// Unsettled marks a transfer the ledger holds both as pending and as posted.
	Unsettled bool
	// Name is the reserve the description names, nil when it names none.
	Name  *string
	Fault *string
	Cents *money.Cents
}

// Removal is an event that makes a document gone from its day on.
type Removal struct {
	RunID int64
	Day   brday.Day
}

// TransferDocument is a transfer with every version the ledger holds of it,
// oldest observation first.
type TransferDocument struct {
	ID       int64
	Versions []TransferVersion
	Removals []Removal
}

// CoverageWalk is a complete walk of the account's transactions, in a run, that
// reached back to Since.
type CoverageWalk struct {
	RunID int64
	Since string
}

// AccountYield is what the rules need to answer any window of one account.
type AccountYield struct {
	AccountID string
	// Sightings are oldest first, one per day.
	Sightings []YieldSighting
	Walks     []CoverageWalk
	Transfers []TransferDocument
}

// PairYield is one row of the statement's answer for a window. Yield is nil
// exactly when Reason is set.
type PairYield struct {
	ReserveID   *string
	ReserveName *string
	Previous    *money.Cents
	Current     *money.Cents
	Transfers   *money.Cents
	Yield       *money.Cents
	Reason      *string
}

// Between answers the window from one day to another as the statement does: one
// row per reserve of the sightings it starts and ends on, one per unstable entry
// and one per transfer that names no stable reserve. It answers no row when a
// day has no sighting to stand on, when both fall on the same sighting, or when
// the provider's times do not increase.
func (account AccountYield) Between(from, to brday.Day) []PairYield {
	window, ok := account.windowOf(from, to)
	if !ok {
		return nil
	}
	return window.rows()
}

// DailyYields answers the days of a run of sightings, for the pots the ledger
// can follow: one row per pair of consecutive sightings, which together cover
// the run without a gap or an overlap. Two cases make a row cover more than one
// pair, and neither loses a day. A sighting that falls on the same sighting of
// reserves as the one before adds nothing of its own. A sighting whose provider
// time does not move on from the start's leaves the window that would end on it
// unanswerable, so the row reaches over it to the next sighting that answers.
// What no sighting can answer, because the run starts before the first sighting
// of reserves or ends on a time that does not move on, is an unknown row for
// every pot it ends: a day cannot be left out of a month's sum in silence.
func (account AccountYield) DailyYields(days []brday.Day) []DayYield {
	var rows []DayYield
	if len(days) == 0 {
		return nil
	}
	start, unanswered := days[0], false
	for _, day := range days[1:] {
		ends := account.endsOf(start, day)
		switch {
		case !ends.hasEnd:
			start = day
		case ends.hasStart && ends.start == ends.end:
		case !ends.hasStart:
			rows = append(rows, account.unknownRows(start, day, ends)...)
			start = day
		default:
			answered, placed := account.answerOf(start, day)
			if !placed {
				unanswered = true
				continue
			}
			rows = append(rows, answered...)
			start, unanswered = day, false
		}
	}
	if unanswered {
		last := days[len(days)-1]
		rows = append(rows, account.unknownRows(start, last, account.endsOf(start, last))...)
	}
	return rows
}

// answerOf is the pots' rows of a window, and whether the window could be placed.
func (account AccountYield) answerOf(from, to brday.Day) ([]DayYield, bool) {
	window, placed := account.windowOf(from, to)
	if !placed {
		return nil, false
	}
	var rows []DayYield
	for _, row := range window.rows() {
		if row.ReserveID != nil {
			rows = append(rows, DayYield{AccountID: account.AccountID, ReserveID: *row.ReserveID, From: from, To: to, Yield: row.Yield})
		}
	}
	return rows, true
}

// unknownRows gives every pot with a stable id at either end a row with no yield.
func (account AccountYield) unknownRows(from, to brday.Day, ends windowEnds) []DayYield {
	sightings := []YieldSighting{account.Sightings[ends.end]}
	if ends.hasStart {
		sightings = append(sightings, account.Sightings[ends.start])
	}
	seen := map[string]bool{}
	var ids []string
	for _, sighting := range sightings {
		for _, entry := range sighting.Entries {
			if entry.Stable && entry.ReserveID != nil && !seen[*entry.ReserveID] {
				seen[*entry.ReserveID] = true
				ids = append(ids, *entry.ReserveID)
			}
		}
	}
	sort.Strings(ids)
	rows := make([]DayYield, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, DayYield{AccountID: account.AccountID, ReserveID: id, From: from, To: to})
	}
	return rows
}

// windowEnds are the sightings a window stands on: the latest on or before each
// day.
type windowEnds struct {
	start, end       int
	hasStart, hasEnd bool
}

func (account AccountYield) endsOf(from, to brday.Day) windowEnds {
	var ends windowEnds
	ends.start, ends.hasStart = account.sightingOn(from)
	ends.end, ends.hasEnd = account.sightingOn(to)
	return ends
}

func (account AccountYield) sightingOn(day brday.Day) (int, bool) {
	index := sort.Search(len(account.Sightings), func(at int) bool { return account.Sightings[at].Day > day })
	return index - 1, index > 0
}
