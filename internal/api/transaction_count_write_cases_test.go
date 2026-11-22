package api

import (
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
)

// startFromTheInventedAuthorship is the state of a write case that changes
// what the person already said about the full ledger's transactions: the
// invented state, whose budget names line 6 Mercado until February and Feira
// from March, and the authorship on top of it.
func startFromTheInventedAuthorship(t *testing.T, scratch *authoredtest.Scratch) {
	t.Helper()
	inventedAuthoredState(t, scratch)
	inventedTransactionAuthorship(t, scratch, newFullLedger(t))
}

// The ids no transaction write may find: one the ledger never had, a tax row
// folded into its purchase and a pot movement.
var idsNoWriteFinds = []string{"no-such-transaction", txForeignTax, txPotTransfer}

// singleTargetArguments are, for each write that names one transaction, the
// arguments it is refused with when the id is not one the list shows.
var singleTargetArguments = map[string]string{
	"transactions.setSplit":           `null`,
	"transactions.setExcluded":        `true`,
	"transactions.setExclusionReason": `"Compra da empresa"`,
	"transactions.setRefund":          `true`,
	"transactions.setRefundLine":      `"Mercado"`,
}

func notFoundCountWriteCases() []writeCase {
	var cases []writeCase
	for _, method := range []string{
		"transactions.setSplit", "transactions.setExcluded", "transactions.setExclusionReason",
		"transactions.setRefund", "transactions.setRefundLine",
	} {
		for _, id := range idsNoWriteFinds {
			cases = append(cases, writeCase{method: method, arguments: `["` + id + `",` + singleTargetArguments[method] + `]`,
				expects: expectsRefusal, reason: reasonNotFound, state: budgetState()})
		}
	}
	return append(cases, writeCase{method: "transactions.excludeMany", arguments: `[["tx-pending","no-such-transaction"],"Marcada em lote"]`,
		expects: expectsRefusal, reason: reasonNotFound, state: budgetState()})
}

// transactionCountWriteCases are the six writes of what a transaction counts
// for, each recorded and undone over what the person already said, and their
// refusals. The split is created with the panel's first payload on February's
// purchase, whose line that month calls Mercado; replacing the parts of the
// instalment keeps the first one as it was.
var transactionCountWriteCases = append([]writeCase{
	{method: "transactions.setSplit", arguments: `["tx-month-end",[{"line":"Mercado","amount":207},{"line":null,"amount":0}]]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setSplit", arguments: `["tx-instalment",[{"line":"Viagens","amount":60},{"line":"Feira","amount":69}]]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setSplit", arguments: `["tx-instalment",null]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setExcluded", arguments: `["tx-pix-out",true]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setExcluded", arguments: `["tx-foreign",false]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setExclusionReason", arguments: `["tx-pending","  Compra da empresa "]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.excludeMany", arguments: `[["tx-pending","tx-boleto","tx-pending","tx-pix-out"],"Marcada em lote"]`,
		expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setRefund", arguments: `["tx-refund",true]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	{method: "transactions.setRefundLine", arguments: `["tx-salary","Feira"]`, expects: expectsRecorded, state: startFromTheInventedAuthorship},
	// February's and January's budget call line 6 Mercado; only March's calls it Feira.
	{method: "transactions.setSplit", arguments: `["tx-month-end",[{"line":"Feira","amount":200},{"line":null,"amount":7}]]`,
		expects: expectsRefusal, reason: reasonUnknownLine, state: startFromTheInventedAuthorship},
	{method: "transactions.setRefundLine", arguments: `["tx-stale","Feira"]`,
		expects: expectsRefusal, reason: reasonUnknownLine, state: startFromTheInventedAuthorship},
	// With no budget at all no month has a line, and so no fees line either.
	{method: "transactions.setSplit", arguments: `["tx-pending",[{"line":"Tarifas e juros","amount":18.5},{"line":null,"amount":0}]]`,
		expects: expectsRefusal, reason: reasonUnknownLine},
	{method: "transactions.setRefundLine", arguments: `["tx-refund","Tarifas e juros"]`, expects: expectsRefusal, reason: reasonUnknownLine},
}, notFoundCountWriteCases()...)
