package api

import "fmt"

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

	potDepositDescription    = "Depósito"
	potWithdrawalDescription = "Retirada"
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
