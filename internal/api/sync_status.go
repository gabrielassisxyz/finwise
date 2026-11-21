package api

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// consentWarningWindow is how long before a consent lapses the header starts to
// say so: a week is enough to act, and the provider gives no earlier signal.
const consentWarningWindow = 7 * 24 * time.Hour

// syncStatus is the contract's union of a good state and a warning.
type syncStatus interface{ isSyncStatus() }

type syncOK struct {
	State        string `json:"state"`
	LastSyncDay  string `json:"lastSyncDay"`
	LastSyncTime string `json:"lastSyncTime"`
}

type syncWarning struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

func (syncOK) isSyncStatus()      {}
func (syncWarning) isSyncStatus() {}

// syncStatusOf is a warning when any connection is in trouble, which is more
// useful to the person than a sync time that is only partly true.
func syncStatusOf(state ledger.SyncState, now time.Time, itemBanks map[string]string) syncStatus {
	problems := connectionProblems(state.Connections, now, itemBanks)
	if len(problems) > 0 {
		return syncWarning{State: "warning", Message: joinProblems(problems)}
	}
	if state.LastCompleteSync == nil {
		return syncWarning{State: "warning", Message: awaitingFirstSync}
	}
	finished := *state.LastCompleteSync
	return syncOK{State: "ok", LastSyncDay: string(brday.DayOf(finished)), LastSyncTime: clockTimeOf(finished)}
}

func connectionProblems(connections []ledger.Connection, now time.Time, itemBanks map[string]string) []string {
	var problems []string
	for _, connection := range connections {
		bank, known := itemBanks[connection.ItemID]
		if !known {
			bank = neutralBankLabel
		}
		if message, trouble := problemOf(connection, now, bank); trouble {
			problems = append(problems, message)
		}
	}
	return problems
}

func problemOf(connection ledger.Connection, now time.Time, bank string) (string, bool) {
	if expires := connection.ConsentExpiresAt; expires != nil {
		if !expires.After(now) {
			return consentExpiredMessage(bank), true
		}
		if expires.Sub(now) <= consentWarningWindow {
			return consentExpiringMessage(bank), true
		}
	}
	if !connection.InTrouble(now) {
		return "", false
	}
	if connection.Status != "UPDATED" || connection.ExecutionStatus != "SUCCESS" {
		return connectionFailedMessage(bank), true
	}
	return connectionStaleMessage(bank), true
}

func joinProblems(problems []string) string {
	if len(problems) == 1 {
		return problems[0]
	}
	return moreConnectionsMessage(problems[0], len(problems)-1)
}
