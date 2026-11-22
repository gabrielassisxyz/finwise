package api

import (
	"context"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// The establishments and the people of the settings: everyone the bank's records
// name as the other side of a transaction the list shows, once, with what the
// person set for each. Nothing in them is a guess: what the person did not set
// is the contract's empty value.

// maxEstablishmentsListed is how many establishments the settings show; people
// are all shown, since they are few and each one is somebody the person knows.
const maxEstablishmentsListed = 10

type settingsEstablishmentWire struct {
	Key         string   `json:"key"`
	RawText     string   `json:"rawText"`
	Nickname    *string  `json:"nickname"`
	DefaultLine *string  `json:"defaultLine"`
	FixedTags   []string `json:"fixedTags"`
}

type settingsPersonWire struct {
	Key      string  `json:"key"`
	FullName string  `json:"fullName"`
	Nickname *string `json:"nickname"`
}

// counterpart is an establishment or a person the ledger lists. first is the
// key's first transaction in the order the list shows them, the newest, which is
// where the ledger's wording of it is read.
type counterpart struct {
	first  txrules.Transaction
	person bool
}

func (known counterpart) key() string { return known.first.EstablishmentKey }

// counterpartsOf walks the listed transactions in the order given and keeps each
// key once, at its first transaction that is not a movement. A movement is
// neither an establishment nor a person, so it is skipped without marking its
// key: an older transaction of the same key that is not one still lists it.
func counterpartsOf(listed []txrules.Transaction) []counterpart {
	seen := map[string]bool{}
	var found []counterpart
	for _, transaction := range listed {
		sections := sectionsOf(transaction)
		if !sections.Establishment && !sections.Person || seen[transaction.EstablishmentKey] {
			continue
		}
		seen[transaction.EstablishmentKey] = true
		found = append(found, counterpart{first: transaction, person: sections.Person})
	}
	return found
}

// listedCounterparts reads the whole ledger in one pass, as the card does, so a
// key the bank sent long ago is listed as surely as one from this week.
func listedCounterparts(ctx context.Context, snapshot *ledger.View) ([]counterpart, error) {
	view, _, err := listedLedger(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	return counterpartsOf(view.listed), nil
}

// counterpartNamedBy is the establishment or the person a write names by its
// key. A key no listed transaction has, and one only movements carry, is no
// establishment and no person: the write is refused and records nothing.
func counterpartNamedBy(ctx context.Context, writes Writes, key string) (counterpart, error) {
	all, err := listedCounterparts(ctx, writes.Ledger)
	if err != nil {
		return counterpart{}, err
	}
	for _, known := range all {
		if known.key() == key {
			return known, nil
		}
	}
	return counterpart{}, refuse(reasonNotFound)
}

// settingsCounterparts are the two lists of the overview.
type settingsCounterparts struct {
	establishments []settingsEstablishmentWire
	people         []settingsPersonWire
}

func settingsCounterpartsOf(ctx context.Context, views Views) (settingsCounterparts, error) {
	all, err := listedCounterparts(ctx, views.Ledger)
	if err != nil {
		return settingsCounterparts{}, err
	}
	var establishments, people []counterpart
	for _, known := range all {
		switch {
		case known.person:
			people = append(people, known)
		case len(establishments) < maxEstablishmentsListed:
			establishments = append(establishments, known)
		}
	}
	return settingsCounterpartsWith(ctx, views, establishments, people)
}

// settingsCounterpartsWith joins what the person kept for each key, read once
// for all of them. A default line is named by the current month's budget, as the
// transaction panel names it, so the two never disagree.
func settingsCounterpartsWith(ctx context.Context, views Views, establishments, people []counterpart) (settingsCounterparts, error) {
	keys := make([]string, 0, len(establishments)+len(people))
	for _, known := range append(append([]counterpart(nil), establishments...), people...) {
		keys = append(keys, known.key())
	}
	kept, err := views.Authored.Establishments(ctx, keys)
	if err != nil {
		return settingsCounterparts{}, err
	}
	current, err := views.Authored.BudgetOf(ctx, brday.MonthOf(nowOf(ctx)))
	if err != nil {
		return settingsCounterparts{}, err
	}
	lines := transactionLinesOf(current)
	answer := settingsCounterparts{
		establishments: make([]settingsEstablishmentWire, 0, len(establishments)),
		people:         make([]settingsPersonWire, 0, len(people)),
	}
	for _, known := range establishments {
		own := kept[known.key()]
		answer.establishments = append(answer.establishments, settingsEstablishmentWire{
			Key: known.key(), RawText: withoutPixPrefix(known.first.RawDescription), Nickname: textOrNil(own.Nickname),
			DefaultLine: lines.nameOf(own.DefaultLine), FixedTags: tagsOf(own.FixedTags, nil),
		})
	}
	for _, known := range people {
		answer.people = append(answer.people, settingsPersonWire{
			Key: known.key(), FullName: known.first.Name, Nickname: textOrNil(kept[known.key()].Nickname),
		})
	}
	return answer, nil
}

// withoutPixPrefix is the bank's wording of an establishment without the words
// the bank puts in front of a PIX, which say which way the money went and not
// who got it.
func withoutPixPrefix(rawText string) string {
	for _, prefix := range []string{"PIX ENVIADO ", "PIX RECEBIDO "} {
		if rest, found := strings.CutPrefix(rawText, prefix); found {
			return rest
		}
	}
	return rawText
}
