package api

import (
	"time"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// billLabelsOf names each bill by the month it falls due in, as the interface
// writes it: Abr/26.
func billLabelsOf(bills []ledger.Bill) map[string]string {
	labels := make(map[string]string, len(bills))
	for _, bill := range bills {
		due, err := time.Parse("2006-01-02", string(bill.DueDate))
		if err != nil {
			continue
		}
		labels[bill.ID] = monthAbbreviations[due.Month()-1] + "/" + due.Format("06")
	}
	return labels
}
