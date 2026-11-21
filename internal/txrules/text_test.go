package txrules

import "testing"

func TestEstablishmentTextSharesOneKeyAcrossCaseSpacingAndInstalmentMarker(t *testing.T) {
	same := []string{"ACME STORE 03/10", "acme  store", "ACME STORE", "  Acme Store 1/3 "}
	want := establishmentText(same[0])
	for _, description := range same {
		if got := establishmentText(description); got != want {
			t.Errorf("establishmentText(%q) = %q, want %q", description, got, want)
		}
	}
}

func TestEstablishmentTextKeepsWhatDistinguishesEstablishments(t *testing.T) {
	distinct := [][2]string{
		{"ACME STORE", "ACME STORES"},
		{"ACME STORE 1", "ACME STORE 2"},
		{"ACME-STORE", "ACME STORE"},
		{"ACME STORE 3/10", "ACME STORE 3/10 B"},
	}
	for _, pair := range distinct {
		if establishmentText(pair[0]) == establishmentText(pair[1]) {
			t.Errorf("%q and %q must not share a key", pair[0], pair[1])
		}
	}
}

func TestFoldTextRemovesAccentsAndCase(t *testing.T) {
	if got := foldText("  Pagamento  de Fatura é Crédito "); got != "PAGAMENTO DE FATURA E CREDITO" {
		t.Fatalf("foldText = %q", got)
	}
}

func TestPrefixAndWordMatchesRespectWordEdges(t *testing.T) {
	if !hasPrefixWord("IOF INTERNACIONAL - X", "IOF INTERNACIONAL") || !hasPrefixWord("IOF INTERNACIONAL", "IOF INTERNACIONAL") {
		t.Error("keyword at the start must match")
	}
	if hasPrefixWord("IOF INTERNACIONALX", "IOF INTERNACIONAL") || hasPrefixWord("X IOF INTERNACIONAL", "IOF INTERNACIONAL") {
		t.Error("keyword inside a longer word or not at the start must not match")
	}
	if !containsWord("ALGO PAGAMENTO X", "PAGAMENTO") || containsWord("PAGAMENTOS", "PAGAMENTO") {
		t.Error("containsWord must match whole words only")
	}
}

func TestDaysApartIsSymmetricAndCountsCalendarDays(t *testing.T) {
	if daysApart("2026-03-01", "2026-02-26") != 3 || daysApart("2026-02-26", "2026-03-01") != 3 {
		t.Error("expected 3 days across a month end in both directions")
	}
	if daysApart("2026-03-01", "not a day") < 1000 {
		t.Error("an unparseable day must never count as near")
	}
}
