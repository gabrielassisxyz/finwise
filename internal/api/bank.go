package api

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// knownBank is a bank the ledger can hold accounts of. The connector name is the
// same for every bank, so an account is told apart by the bank code that opens
// its transfer number or by words in its own name.
type knownBank struct {
	label    string
	code     int
	keywords []string
}

var knownBanks = []knownBank{
	{label: bankItau, code: 341, keywords: []string{"itau"}},
	{label: bankMercadoPago, code: 323, keywords: []string{"mercado pago", "mercadopago"}},
	{label: bankC6, code: 336, keywords: []string{"c6"}},
	{label: bankNubank, code: 260, keywords: []string{"nubank"}},
}

// bankLabels says which bank each account belongs to, by account id, and later
// handlers (the card's closing table, a pot's yield base) read it the same way.
//
// Two independent signals are weighed, the code in the transfer number and the
// account's name. They must agree when both speak; one that speaks alone decides;
// an account that neither places takes the bank of the one other account of its
// connection that is placed, and otherwise gets the neutral label. A guess is
// never made: a wrong bank would apply the wrong closing table to a card.
func bankLabels(accounts []ledger.Account) map[string]string {
	labels := make(map[string]string, len(accounts))
	byItem := make(map[string]map[string]bool)
	var unplaced []ledger.Account
	for _, account := range accounts {
		label, spoke := bankFromSignals(account)
		if !spoke {
			unplaced = append(unplaced, account)
			continue
		}
		labels[account.ID] = label
		if label != neutralBankLabel {
			remember(byItem, account.ItemID, label)
		}
	}
	for _, account := range unplaced {
		labels[account.ID] = neutralBankLabel
		if siblings := byItem[account.ItemID]; account.ItemID != "" && len(siblings) == 1 {
			for label := range siblings {
				labels[account.ID] = label
			}
		}
	}
	return labels
}

func remember(byItem map[string]map[string]bool, itemID, label string) {
	if byItem[itemID] == nil {
		byItem[itemID] = map[string]bool{}
	}
	byItem[itemID][label] = true
}

// bankFromSignals returns the neutral label with spoke true when the signals
// contradict each other, and spoke false when neither says anything.
func bankFromSignals(account ledger.Account) (label string, spoke bool) {
	found := map[string]bool{}
	if bank, ok := bankOfCode(account.TransferNumber); ok {
		found[bank] = true
	}
	found = unionOf(found, banksNamedIn(account.Name))
	switch len(found) {
	case 0:
		return "", false
	case 1:
		for only := range found {
			return only, true
		}
	}
	return neutralBankLabel, true
}

func unionOf(into, from map[string]bool) map[string]bool {
	for label := range from {
		into[label] = true
	}
	return into
}

// bankOfCode reads the first segment of a transfer number such as 341/0001/12345-6.
func bankOfCode(transferNumber *string) (string, bool) {
	if transferNumber == nil {
		return "", false
	}
	first, _, _ := strings.Cut(strings.TrimSpace(*transferNumber), "/")
	code, err := strconv.Atoi(first)
	if err != nil {
		return "", false
	}
	for _, bank := range knownBanks {
		if bank.code == code {
			return bank.label, true
		}
	}
	return "", false
}

func banksNamedIn(name string) map[string]bool {
	words := " " + strings.Join(strings.FieldsFunc(foldAccents(name), isSeparator), " ") + " "
	found := map[string]bool{}
	for _, bank := range knownBanks {
		for _, keyword := range bank.keywords {
			if strings.Contains(words, " "+keyword+" ") {
				found[bank.label] = true
			}
		}
	}
	return found
}

func isSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

var accentFolds = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i",
	"ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ç", "c",
)

func foldAccents(text string) string {
	return accentFolds.Replace(strings.ToLower(text))
}

// itemBankLabels gives each connection the bank of its accounts, for the message
// of a connection in trouble. A connection whose accounts disagree, or that has
// none, is not attributed to any bank.
func itemBankLabels(accounts []ledger.Account, labels map[string]string) map[string]string {
	found := map[string]map[string]bool{}
	for _, account := range accounts {
		remember(found, account.ItemID, labels[account.ID])
	}
	items := make(map[string]string, len(found))
	for itemID, set := range found {
		items[itemID] = neutralBankLabel
		if len(set) != 1 {
			continue
		}
		for label := range set {
			items[itemID] = label
		}
	}
	return items
}
