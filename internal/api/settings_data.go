package api

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// dataSettingsOf is the connections as the bank reports them and the last
// complete synchronisation. This backend makes no backup, so there is none to
// show.
func dataSettingsOf(state ledger.SyncState, now time.Time, itemBanks map[string]string) dataSettingsWire {
	data := dataSettingsWire{Connections: connectionsOf(state.Connections, now, itemBanks)}
	if state.LastCompleteSync != nil {
		finished := *state.LastCompleteSync
		data.LastSync = &momentWire{Day: string(brday.DayOf(finished)), Time: clockTimeOf(finished)}
	}
	return data
}

// connectionsOf has one entry per bank link. The contract knows two states, and
// a link that is only late or failing is not one of them: that is the header's
// warning to give, so here it stays "ok" until the consent itself has lapsed.
func connectionsOf(connections []ledger.Connection, now time.Time, itemBanks map[string]string) []connectionWire {
	listed := make([]connectionWire, 0, len(connections))
	for _, connection := range connections {
		bank, known := itemBanks[connection.ItemID]
		if !known {
			bank = neutralBankLabel
		}
		status := "ok"
		if expires := connection.ConsentExpiresAt; expires != nil && !expires.After(now) {
			status = "consent-expired"
		}
		listed = append(listed, connectionWire{Bank: bank, Status: status})
	}
	return listed
}
