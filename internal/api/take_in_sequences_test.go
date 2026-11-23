package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ingest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

// The two ways a write that reads a snapshot the ingest has not taken in yet
// went wrong, each driven through the router with the real ingest as its
// take-in, and the bulk write and "all purchases here" over the first.

const (
	bakeryPending = "tx-padaria-pendente"
	bakeryPosted  = "tx-padaria-lancada"
	bakeryEarlier = "tx-padaria-anterior"
	newsstand     = "tx-banca"
	hotel         = "tx-hotel"
	bookshop      = "tx-livraria"
)

// pendingPurchaseSaidAbout is the first step of the pending purchase: a
// snapshot lists a pending card purchase, the ingest takes it in, and the
// person puts it on a line, gives it a tag and a note.
func pendingPurchaseSaidAbout(t *testing.T, kind takeIn, options ...envOption) *nightsWorld {
	t.Helper()
	w := newNightsWorld(t, kind, options...)
	w.saveBudget("2026-03", nightsBudget())
	w.night(nightsPendingPurchase(bakeryPending, onDay(time.March, 10), "PADARIA EXEMPLO", 45))
	w.ingest(ingest.TriggerStart)
	w.write("transactions.setLine", `["`+bakeryPending+`","Mercado"]`)
	w.write("transactions.addTag", `["`+bakeryPending+`","feira"]`)
	w.write("transactions.setNote", `["`+bakeryPending+`","pão de queijo"]`)
	return w
}

// bakeryPostedNight is the next snapshot: the pending row is gone, a posted one
// with another id, the same account, amount and establishment is listed, and so
// is a purchase the rules have never met, which the ingest queues.
func (w *nightsWorld) bakeryPostedNight(more ...ledgertest.Transaction) {
	w.t.Helper()
	w.night(append(more, nightsPurchase(bakeryPosted, onDay(time.March, 11), "PADARIA EXEMPLO", 45),
		nightsPurchase(newsstand, onDay(time.March, 12), "BANCA EXEMPLO", 12))...)
}

func tagNames(tags []authored.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names
}

// requireThePostedRowKeepsWhatWasSaid is step 5's expectation: the posted row
// under the pending row's key, on the line, with the tags and the note, and no
// key of its own in the index.
func (w *nightsWorld) requireThePostedRowKeepsWhatWasSaid(pendingKey authored.TxKey, keys int, tags ...string) {
	w.t.Helper()
	if key, found := w.key(bakeryPosted); !found || key != pendingKey {
		w.t.Errorf("the posted row is under key %d (indexed %t), want the pending row's %d", key, found, pendingKey)
	}
	kept, market := w.kept(bakeryPosted), w.line("2026-03", "Mercado")
	if kept.State.Line == nil || *kept.State.Line != market || kept.State.Note != "pão de queijo" || !slices.Equal(tagNames(kept.Tags), tags) {
		t := kept.State
		w.t.Errorf("the posted row shows line %v, tags %v and note %q, want Mercado (%d), %v and the pending row's note", t.Line, tagNames(kept.Tags), t.Note, market, tags)
	}
	if index := w.index(); len(index) != keys {
		w.t.Errorf("the index holds %d keys, want %d", len(index), keys)
	}
}

// requireTheUndoReturnsTheStateTheTakeInLeft undoes the person's change and
// compares what is journaled with the dump taken right after the take-in.
func (w *nightsWorld) requireTheUndoReturnsTheStateTheTakeInLeft(label, id string) {
	w.t.Helper()
	w.undoAccepted(label, id)
	if w.takeIns != 1 || w.scratch.Canonical(w.t) != w.afterTakeIn {
		w.t.Errorf("%s: after %d take-ins it did not return the state the take-in left", label, w.takeIns)
	}
}

func TestAPostedPurchaseKeepsWhatWasSaidAboutThePendingOneWhenAWriteComesBeforeTheIngest(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	pendingKey, _ := w.key(bakeryPending)
	w.bakeryPostedNight()
	changes := len(w.scratch.Changes(t))

	id := w.write("transactions.addTag", `["`+bakeryPosted+`","lanche"]`)
	w.ingest(ingest.TriggerTick)

	w.requireThePostedRowKeepsWhatWasSaid(pendingKey, 2, "#feira", "#lanche")
	w.requireTheRulesChangeBefore(changes, id)
	w.requireTheUndoReturnsTheStateTheTakeInLeft("undoing the new tag", id)
	if tags := tagNames(w.kept(bakeryPosted).Tags); !slices.Equal(tags, []string{"#feira"}) {
		t.Errorf("after the undo the posted row has tags %v, want the pending row's", tags)
	}
}

// movedMonthSaidAbout is the first step of the moved month: March's budget
// lacks Viagem and April's has it, a purchase of April is on Viagem, and a note
// is recorded on another transaction. It answers that unrelated change.
func movedMonthSaidAbout(t *testing.T) (*nightsWorld, string) {
	t.Helper()
	w := newNightsWorld(t, theRealIngest)
	w.saveBudget("2026-03", nightsBudget())
	w.saveBudget("2026-04", nightsBudget(authoredtest.Line("Viagem", 90_000)))
	w.night(nightsPurchase(hotel, onDay(time.April, 2), "HOTEL EXEMPLO", 300), nightsPurchase(bookshop, onDay(time.April, 5), "LIVRARIA EXEMPLO", 60))
	w.ingest(ingest.TriggerStart)
	w.write("transactions.setLine", `["`+hotel+`","Viagem"]`)
	return w, w.write("transactions.setNote", `["`+bookshop+`","presente"]`)
}

// hotelMovedNight is the next snapshot, with the purchase dated in March.
func (w *nightsWorld) hotelMovedNight() {
	w.t.Helper()
	w.night(nightsPurchase(hotel, onDay(time.March, 30), "HOTEL EXEMPLO", 300), nightsPurchase(bookshop, onDay(time.April, 5), "LIVRARIA EXEMPLO", 60))
}

func TestAPurchaseWhoseMonthMovedLeavesUndoWorkingWhenAWriteComesBeforeTheIngest(t *testing.T) {
	t.Run("the unrelated undo with the note in force", func(t *testing.T) {
		w, unrelated := movedMonthSaidAbout(t)
		w.hotelMovedNight()
		w.write("transactions.setNote", `["`+hotel+`","reserva"]`)
		w.ingest(ingest.TriggerTick)
		w.ingest(ingest.TriggerCommand)

		w.undoAccepted("the unrelated undo", unrelated)
		if line := w.kept(hotel).State.Line; line != nil {
			t.Errorf("the purchase dated in March is still on line %d, which March's budget lacks", *line)
		}
	})
	t.Run("the note's own undo", func(t *testing.T) {
		w, unrelated := movedMonthSaidAbout(t)
		w.hotelMovedNight()
		changes := len(w.scratch.Changes(t))
		id := w.write("transactions.setNote", `["`+hotel+`","reserva"]`)
		w.ingest(ingest.TriggerTick)

		w.requireTheRulesChangeBefore(changes, id)
		w.requireTheUndoReturnsTheStateTheTakeInLeft("undoing the note", id)
		w.undoAccepted("the unrelated undo after it", unrelated)
	})
}

func TestABulkWriteOverTheNextSnapshotFindsThePostedRowUnderThePendingRowsKey(t *testing.T) {
	w := pendingPurchaseSaidAbout(t, theRealIngest)
	pendingKey, _ := w.key(bakeryPending)
	w.bakeryPostedNight()
	changes := len(w.scratch.Changes(t))

	id := w.write("transactions.addTagToMany", `[["`+bakeryPosted+`","`+newsstand+`"],"lanche"]`)
	w.ingest(ingest.TriggerTick)

	w.requireThePostedRowKeepsWhatWasSaid(pendingKey, 2, "#feira", "#lanche")
	if tags := tagNames(w.kept(newsstand).Tags); !slices.Equal(tags, []string{"#lanche"}) {
		t.Errorf("the other purchase of the selection has tags %v, want the new one", tags)
	}
	w.requireTheRulesChangeBefore(changes, id)
	w.requireTheUndoReturnsTheStateTheTakeInLeft("undoing the bulk tag", id)
}

func earlierBakeryPurchase() ledgertest.Transaction {
	return nightsPurchase(bakeryEarlier, onDay(time.March, 3), "PADARIA EXEMPLO", 30)
}

func TestAllPurchasesHereOverTheNextSnapshotFindsThePostedRowUnderThePendingRowsKey(t *testing.T) {
	w := newNightsWorld(t, theRealIngest)
	w.saveBudget("2026-03", nightsBudget())
	w.night(nightsPendingPurchase(bakeryPending, onDay(time.March, 10), "PADARIA EXEMPLO", 45), earlierBakeryPurchase())
	w.ingest(ingest.TriggerStart)
	pendingKey, _ := w.key(bakeryPending)
	origin := w.write("transactions.setLine", `["`+bakeryEarlier+`","Mercado"]`)
	w.bakeryPostedNight(earlierBakeryPurchase())
	changes := len(w.scratch.Changes(t))

	response := w.post("transactions.applyLineToEstablishment", `["`+origin+`"]`)
	w.ingest(ingest.TriggerTick)

	var result establishmentLineResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); response.Code != http.StatusOK || err != nil || result.Changed != 1 {
		t.Fatalf("all purchases here answered %d %s, want the posted row moved", response.Code, response.Body)
	}
	if key, found := w.key(bakeryPosted); !found || key != pendingKey {
		t.Errorf("the posted row is under key %d (indexed %t), want the pending row's %d", key, found, pendingKey)
	}
	if line := w.kept(bakeryPosted).State.Line; line == nil || *line != w.line("2026-03", "Mercado") {
		t.Errorf("the posted row is on line %v, want Mercado", line)
	}
	since := w.changesSince(changes)
	if len(since) != 1 || since[0].Author != string(authored.AuthorRule) || since[0].Method != ingest.Method || w.takeIns != 1 {
		t.Errorf("after %d take-ins the changes since the night are %+v, want the ingest's alone", w.takeIns, since)
	}
	w.undoAccepted("undoing the line and all purchases here", origin)
}
