package budgetrules

import (
	"sort"
	"strconv"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

// The payday transfers are derived from the month's budget at every read and
// never stored; only a mark, by month and key, is kept of one. A key is never a
// name: a line keeps its id through a rename, a pot's id is the bank's, and two
// pots of one name are two transfers.

// InvestmentTransferKey is the key of the transfer of what the budget leaves to
// invest.
const InvestmentTransferKey = "investment"

const (
	potTransferPrefix  = "pot:"
	lineTransferPrefix = "line:"
)

// PotTransferKey is the key of the transfer into a pot: the whole pot id.
func PotTransferKey(potID string) string { return potTransferPrefix + potID }

// LineTransferKey is the key of the transfer that carries a line no pot counts
// for.
func LineTransferKey(line LineID) string {
	return lineTransferPrefix + strconv.FormatInt(int64(line), 10)
}

// TransferPot is a pot the ledger lists now. Name is nil for a pot the bank
// gives no name.
type TransferPot struct {
	ID     string
	Name   *string
	Closed bool
}

// PaydayInputs is what a month's transfers are derived from.
type PaydayInputs struct {
	// Plan is the budget in force for the month.
	Plan Plan
	// Links is, by line id, the pot the person has linked the line to now.
	Links map[LineID]string
	Pots  []TransferPot
	// UnnamedPot is what the transfer of a pot with no name shows, and the
	// name it is ordered by.
	UnnamedPot string
	// InvestmentName is what the investment transfer shows.
	InvestmentName string
}

// Transfer is one transfer of payday. Lines are the provision and reserve lines
// it carries, in the budget's order; the investment carries none.
type Transfer struct {
	Key    string
	Name   string
	Amount money.Cents
	Lines  []LineID
}

// Transfers is a month's transfers: one per open, listed pot that a provision
// or reserve line is linked to, adding up its lines, ordered by the name shown
// and then by pot id in Go's string order; then one per provision or reserve
// line linked to no pot that counts, in the budget's order; then the residual,
// when it is above zero. A transfer of zero is not listed. A line without an
// id, which only a draft has, cannot be linked or marked and takes no part.
func Transfers(inputs PaydayInputs) []Transfer {
	shown := inputs.openPotNames()
	var pots, lines []Transfer
	potAt := map[string]int{}
	for _, line := range inputs.carriedLines() {
		id := line.ID.ID
		potID, linked := inputs.Links[id]
		name, counted := shown[potID]
		if !linked || !counted {
			lines = append(lines, Transfer{Key: LineTransferKey(id), Name: line.Name, Amount: line.Amount, Lines: []LineID{id}})
			continue
		}
		at, seen := potAt[potID]
		if !seen {
			at, potAt[potID] = len(pots), len(pots)
			pots = append(pots, Transfer{Key: PotTransferKey(potID), Name: name})
		}
		pots[at].Amount += line.Amount
		pots[at].Lines = append(pots[at].Lines, id)
	}
	// Every pot key has the same prefix, so ordering keys orders pot ids.
	sort.SliceStable(pots, func(a, b int) bool {
		if pots[a].Name != pots[b].Name {
			return pots[a].Name < pots[b].Name
		}
		return pots[a].Key < pots[b].Key
	})
	transfers := append(withoutZero(pots), withoutZero(lines)...)
	if investment, planned := InvestmentTransferOf(inputs.Plan, inputs.InvestmentName); planned {
		transfers = append(transfers, investment)
	}
	return transfers
}

// InvestmentTransferOf is the transfer of the plan's residual, and false when
// the residual is zero or below: an outgrown income plans no investment.
func InvestmentTransferOf(plan Plan, name string) (Transfer, bool) {
	residual := Investment(plan)
	if residual <= 0 {
		return Transfer{}, false
	}
	return Transfer{Key: InvestmentTransferKey, Name: name, Amount: residual, Lines: []LineID{}}, true
}

// openPotNames is the name each counted pot shows, by id: a link counts only
// while the ledger lists its pot and the pot is open.
func (inputs PaydayInputs) openPotNames() map[string]string {
	shown := map[string]string{}
	for _, pot := range inputs.Pots {
		if pot.Closed {
			continue
		}
		shown[pot.ID] = inputs.UnnamedPot
		if pot.Name != nil {
			shown[pot.ID] = *pot.Name
		}
	}
	return shown
}

// carriedLines is the provisions then the reserves, each at its position.
func (inputs PaydayInputs) carriedLines() []PlannedLine {
	var carried []PlannedLine
	for _, line := range append(inputs.Plan.linesOf(GroupProvisions), inputs.Plan.linesOf(GroupReserves)...) {
		if line.ID.Set {
			carried = append(carried, line)
		}
	}
	return carried
}

func withoutZero(transfers []Transfer) []Transfer {
	kept := make([]Transfer, 0, len(transfers))
	for _, transfer := range transfers {
		if transfer.Amount != 0 {
			kept = append(kept, transfer)
		}
	}
	return kept
}

// TransferMarks is a month's marks: the day each key was marked done.
type TransferMarks map[string]brday.Day

// MarkedTransfer is a transfer with its mark. DoneOn is empty while it is
// pending.
type MarkedTransfer struct {
	Transfer
	DoneOn brday.Day
}

func (transfer MarkedTransfer) Done() bool { return transfer.DoneOn != "" }

// Marked joins a month's marks to its transfers by key. A mark whose key the
// month's list no longer has shows nowhere and counts for nothing.
func Marked(transfers []Transfer, marks TransferMarks) []MarkedTransfer {
	marked := make([]MarkedTransfer, 0, len(transfers))
	for _, transfer := range transfers {
		marked = append(marked, MarkedTransfer{Transfer: transfer, DoneOn: marks[transfer.Key]})
	}
	return marked
}

// TransferByKey is the listed transfer under key.
func TransferByKey(transfers []MarkedTransfer, key string) (MarkedTransfer, bool) {
	for _, transfer := range transfers {
		if transfer.Key == key {
			return transfer, true
		}
	}
	return MarkedTransfer{}, false
}

// TransferOfPot is the transfer a pot's page follows: the pot's own, else, for
// an investment, the one every investment shares.
func TransferOfPot(transfers []MarkedTransfer, potID string, investment bool) (MarkedTransfer, bool) {
	if own, found := TransferByKey(transfers, PotTransferKey(potID)); found || !investment {
		return own, found
	}
	return TransferByKey(transfers, InvestmentTransferKey)
}

// Contribution is how much of a month's plan of transfers is done. Progress is
// the done share as a whole percent, rounded half up and held to 100, and 0
// when nothing is planned.
type Contribution struct {
	Planned  money.Cents
	Done     money.Cents
	Progress int64
}

func ContributionOf(transfers []MarkedTransfer) Contribution {
	var contribution Contribution
	for _, transfer := range transfers {
		contribution.Planned += transfer.Amount
		if transfer.Done() {
			contribution.Done += transfer.Amount
		}
	}
	contribution.Progress = Share(contribution.Done, contribution.Planned)
	return contribution
}

// InvestmentRates are the plan's rate as the budget answers it, and the rate
// actually invested: the same once the investment transfer is marked, zero
// before, and nil both when the plan has no income.
func InvestmentRates(plan Plan, transfers []MarkedTransfer) (planned, actual *int64) {
	planned = InvestmentRate(plan)
	if planned == nil {
		return nil, nil
	}
	var invested int64
	if investment, listed := TransferByKey(transfers, InvestmentTransferKey); listed && investment.Done() {
		invested = *planned
	}
	return planned, &invested
}

// MadeInvestment is what the investment line shows as already invested: the
// whole residual in a closed month, whatever its sign, since a closed month is
// taken as wholly made; the investment transfer once the plan lists it and it
// is marked; and nothing otherwise.
func MadeInvestment(plan Plan, marks TransferMarks, closed bool) money.Cents {
	if closed {
		return Investment(plan)
	}
	transfer, listed := InvestmentTransferOf(plan, "")
	if listed && marks[transfer.Key] != "" {
		return transfer.Amount
	}
	return 0
}
