package api

import (
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/money"
)

// Everything a person reads that the server writes lives in this file, in
// Brazilian Portuguese. The wording follows the stand-in's.

const (
	bankItau        = "Itaú"
	bankMercadoPago = "Mercado Pago"
	bankC6          = "C6"
	bankNubank      = "Nubank"

	// neutralBankLabel is for an account no signal places: better a label that
	// says so than a bank it might not belong to.
	neutralBankLabel = "Banco não identificado"

	unnamedAccount = "Conta"
	unnamedCard    = "Cartão"

	awaitingFirstSync = "Aguardando a primeira sincronização"

	// unnamedScenario is what a scenario saved with a blank name is called.
	unnamedScenario = "Cenário sem nome"

	potDepositDescription    = "Depósito"
	potWithdrawalDescription = "Retirada"

	// bankAccountItemDetail is the second line of a bank account among the
	// parts of the net worth; a pot's second line is its bank.
	bankAccountItemDetail = "conta"
)

func consentExpiredMessage(bank string) string {
	return bank + ": consentimento expirado"
}

func consentExpiringMessage(bank string) string {
	return bank + ": consentimento perto de expirar"
}

func connectionFailedMessage(bank string) string {
	return bank + ": problema na conexão"
}

func connectionStaleMessage(bank string) string {
	return bank + ": sem atualização há mais de 48 horas"
}

// moreConnectionsMessage follows the first problem when others share the warning.
func moreConnectionsMessage(first string, others int) string {
	return fmt.Sprintf("%s (e mais %d)", first, others)
}

func cardAccountDescription(name, finalDigits string) string {
	return name + " · final " + finalDigits
}

func bankAccountDescription(bank, name string) string {
	if name == "" || name == bank {
		return bank
	}
	return bank + " · " + name
}

var monthAbbreviations = [12]string{
	"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez",
}

// foreignSymbols are how an amount in another currency is introduced; a
// currency with no entry is introduced by its code.
var foreignSymbols = map[string]string{
	"USD": "US$",
	"EUR": "€",
	"GBP": "£",
}

// The groups of a budget, as the budget screen names them.
const (
	budgetGroupIncome     = "Receita"
	budgetGroupFixed      = "Fixos e essenciais"
	budgetGroupVariable   = "Variáveis"
	budgetGroupProvisions = "Provisões"
	budgetGroupWishes     = "Reservas de desejo"
	budgetGroupRainyFunds = "Rainy Funds"
	budgetGroupFees       = "Tarifas e juros"
	budgetGroupInvestment = "Investimento"
)

// The marks of the budget screen: a variable line past its cap, and a budget
// that plans more than its income.
const (
	budgetMarkOverspent  = "Estourou"
	budgetMarkOverIncome = "Budget maior que a receita"
)

// The steps a transaction's history records when the person changes what it
// counts for.
const (
	countedAgainStep   = "Você voltou a contar esta transação"
	refundMarkedStep   = "Você marcou como estorno"
	refundUnmarkedStep = "Você desmarcou o estorno"
)

func refundLineStep(line string) string {
	return "Você definiu que o estorno abate " + line
}

// The steps a line change leaves in a transaction's history.

func lineChangedStep(line string) string { return "Você alterou para " + line }

func lineConfirmedStep(line string) string { return "Você confirmou " + line }

func lineAppliedByRuleStep(line, establishment string) string {
	return "Regra aplicou " + line + " (você pediu para todas em " + establishment + ")"
}

// The step an answer of the weekly queue leaves when the person keeps the
// suggested line, and the note of the rule it creates for an establishment.

func lineAcceptedStep(line string) string { return "Você aceitou " + line }

func ruleCreatedNote(establishment, line string) string {
	return "Regra criada: " + establishment + " sempre em " + line
}

// The cycles of a subscription, the contract's SubscriptionCycle.
const (
	cycleMonthly    = "Mensal"
	cycleQuarterly  = "Trimestral"
	cycleSemiannual = "Semestral"
	cycleYearly     = "Anual"
	cycleBiennial   = "Bienal"
)

// The marks of a subscription.
const (
	markPriceRose          = "Preço subiu"
	markChargedAfterCancel = "Cobrou após cancelar"
)

func renewalMark(days int) string {
	switch days {
	case 0:
		return "Renova hoje"
	case 1:
		return "Renova em 1 dia"
	}
	return fmt.Sprintf("Renova em %d dias", days)
}

// The steps of a purchase's history when a received PIX is linked to it as its
// reimbursement and when the link is undone. The amount is written as the
// interface writes reais: a no-break space after R$.
const unlinkedReimbursementStep = "Você desfez a ligação com o PIX"

func linkedReimbursementStep(name string, cents money.Cents) string {
	return "Você ligou ao PIX de " + name + " · R$\u00a0" + brazilianDecimal(cents)
}
