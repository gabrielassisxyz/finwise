package api

import (
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// Views is what one read is answered from: one state of the ledger snapshot and
// one of the app's database, opened for the request and closed when it ends, so
// that every query of one answer sees the same records even when the collector
// replaces the snapshot or another request commits meanwhile. It carries no
// HTTP type: a handler is a plain function of its arguments and its views.
type Views struct {
	Ledger   *ledger.View
	Authored *authored.View
}

// openViews touches neither file: each view opens at its first query, so a
// handler that asks a store nothing never fails because that store is missing.
// A router built without a ledger answers every ledger query as a missing
// snapshot; one built without the app's database hands out the empty view.
func openViews(reader *ledger.Reader, database *authored.Database) Views {
	if reader == nil {
		reader = ledger.New("")
	}
	views := Views{Ledger: reader.View(), Authored: authored.EmptyView()}
	if database != nil {
		views.Authored = database.View()
	}
	return views
}

// close ends both views. Their errors are dropped: the answer is already
// decided, and a read-only transaction that fails to end kept nothing.
func (views Views) close() {
	_ = views.Ledger.Close()
	_ = views.Authored.Close()
}
