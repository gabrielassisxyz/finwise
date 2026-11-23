package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// What the person keeps as subscriptions over the card establishments of the
// full ledger, beside the one the invented authored state holds on a key no
// listed transaction has. Every text is invented.
const (
	noteOfTheCloud = "Migrar os arquivos antes de cancelar"
	// cancelledTheCloud is before the cloud's charge of 2026-03-10, which is
	// then a charge after cancelling.
	cancelledTheCloud brday.Day = "2026-03-09"
)

var subscriptionsMade = utc(2026, time.March, 9, 15, 0)

func init() {
	authoredReadCases = append(authoredReadCases,
		authoredRead("subscriptions.list", `[{"grouped":true}]`),
		authoredRead("subscriptions.list", `[{"grouped":false}]`),
		authoredRead("subscriptions.detail", `["1"]`),
		authoredRead("subscriptions.detail", `["2"]`),
		authoredRead("subscriptions.detail", `["3"]`),
		authoredRead("subscriptions.detail", `["4"]`),
		authoredRead("subscriptions.detail", `["sub-1"]`),
		authoredRead("subscriptions.candidates", `[]`),
	)
}

// inventedSubscriptions makes three subscriptions in one change, after the
// invented one: the foreign cloud service marked to cancel the day before it
// charged again, the bookshop kept on a line of the current month's budget,
// and the market under review with no line, whose one charge is of the
// previous month and has its tax folded in. Each purchase is found with the
// lookup every write uses.
func inventedSubscriptions(t testing.TB, scratch *authoredtest.Scratch, fixture *ledgertest.Fixture) {
	t.Helper()
	lines := lineIDsByName(t, scratch.Database)
	snapshot := viewOf(t, fixture.Path)
	authorOver(t, scratch, snapshot, lines, subscriptionsMade, "scratch.seedSubscriptions", func(a authoring) error {
		return each(
			func() error {
				return a.subscription(txForeign, authored.StatusCancel, nil, "2026-03-20", noteOfTheCloud)
			},
			func() error {
				return a.subscription(txInstalment, authored.StatusKeep, a.line("Viagens"), "2026-06-06", "")
			},
			func() error { return a.subscription(txMonthEnd, authored.StatusReview, nil, "2026-03-01", "") },
		)
	})
}

func (a authoring) subscription(id string, status authored.SubscriptionStatus, line *authored.LineID, reviewOn brday.Day, note string) error {
	target, found, err := lookupTarget(a.ctx, a.writes, id)
	if err == nil && !found {
		err = fmt.Errorf("%s is not listed", id)
	}
	if err != nil {
		return err
	}
	_, err = a.writes.Recorder.CreateSubscription(a.ctx, authored.Subscription{
		EstablishmentKey: target.listed.EstablishmentKey, CreatedFrom: target.key, CreatedDay: target.listed.Day,
		CreatedCents: -target.listed.Amount, Name: target.listed.Name, Line: line, Status: status,
		StatusDay: cancelledTheCloud, ReviewOn: reviewOn, Note: note,
	})
	return err
}
