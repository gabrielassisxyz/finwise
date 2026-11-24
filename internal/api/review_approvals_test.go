package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/ledger/ledgertest"
)

func init() {
	writeCases = append(writeCases, proposalWriteCases...)
	secrets = append(secrets,
		"agente-revisor", "Etiqueta da viagem", "Motivo da recusa",
		"transactions.addTag", "transactions.setNote", "Nota da proposta", "Nota da pessoa",
	)
}

var proposalWriteCases = []writeCase{
	{
		method:    "review.submitProposal",
		arguments: `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#viagem"]},"reason":"Etiqueta da viagem","proposer":"agente-revisor"}]`,
		expects:   expectsProposal,
		state:     startFromTheInventedState,
	},
	{
		method:    "review.submitProposal",
		arguments: `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#viagem"]},"reason":"Etiqueta da viagem","proposer":"agente-revisor"}]`,
		expects:   expectsNothing,
		state:     startFromTheInventedState,
		before:    []writeCall{{"transactions.addTag", `["tx-instalment","#viagem"]`}},
	},
	{
		method:    "review.approve",
		arguments: `["1"]`,
		expects:   expectsRecorded,
		state:     startFromTheInventedState,
		before: []writeCall{
			{"review.submitProposal", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#viagem"]},"reason":"Etiqueta da viagem","proposer":"agente-revisor"}]`},
		},
	},
	{
		method:    "review.approve",
		arguments: `["1"]`,
		expects:   expectsRefusal,
		reason:    reasonProposalStale,
		state:     startFromTheInventedState,
		before: []writeCall{
			{"review.submitProposal", `[{"change":{"method":"transactions.setNote","arguments":["tx-instalment","Nota da proposta"]},"reason":"Etiqueta da viagem","proposer":"agente-revisor"}]`},
			{"transactions.setNote", `["tx-instalment","Nota da pessoa"]`},
		},
	},
	{
		method:    "review.refuse",
		arguments: `["1","Motivo da recusa"]`,
		expects:   expectsProposal,
		state:     startFromTheInventedState,
		before: []writeCall{
			{"review.submitProposal", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#viagem"]},"reason":"Etiqueta da viagem","proposer":"agente-revisor"}]`},
		},
	},
}

func proposalsOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, full *ledgertest.Fixture) {
	t.Helper()
	ctx := t.Context()
	db := scratch.Database
	// 1. Waiting proposal
	_, err := db.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: fixtureNow.Add(-4 * time.Hour),
		Proposer:    "revisor-semanal",
		Method:      "transactions.addTag",
		Arguments:   `["tx-instalment","#viagem"]`,
		Reason:      "Compra feita na semana da viagem",
		Title:       "Adicionar #viagem em Livraria Exemplo R$ 207,00",
		BasedOn:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2. Approved (still in force)
	id2, err := db.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: fixtureNow.Add(-3 * time.Hour),
		Proposer:    "revisor-semanal",
		Method:      "transactions.setNote",
		Arguments:   `["tx-instalment","Presente de aniversário"]`,
		Reason:      "Recibo salvo no e-mail",
		Title:       `Anotar em Livraria Exemplo R$ 207,00: "Presente de aniversário"`,
		BasedOn:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Change(ctx, authored.ChangeFacts{Author: authored.AuthorModel, Method: "transactions.setNote", At: fixtureNow.Add(-2 * time.Hour)}, func(r *authored.Recorder) error {
		_, err := r.ApproveProposal(ctx, id2, fixtureNow.Add(-2*time.Hour))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// 3. Approved then undone
	id3, err := db.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: fixtureNow.Add(-2 * time.Hour),
		Proposer:    "agente-notas",
		Method:      "transactions.addTag",
		Arguments:   `["tx-foreign","#viagem"]`,
		Reason:      "Gasto fora do país",
		Title:       "Adicionar #viagem em Loja Exemplo R$ 110,00",
		BasedOn:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var undoneChange authored.ChangeID
	err = db.Change(ctx, authored.ChangeFacts{Author: authored.AuthorModel, Method: "transactions.addTag", At: fixtureNow.Add(-90 * time.Minute)}, func(r *authored.Recorder) error {
		if _, err := r.ApproveProposal(ctx, id3, fixtureNow.Add(-90*time.Minute)); err != nil {
			return err
		}
		var err error
		undoneChange, err = r.ID(ctx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Undo(ctx, undoneChange, fixtureNow.Add(-80*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// 4. Refused with reason
	id4, err := db.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: fixtureNow.Add(-time.Hour),
		Proposer:    "revisor-semanal",
		Method:      "transactions.setNote",
		Arguments:   `["tx-salary","Salário do mês"]`,
		Reason:      "Identificar entrada recorrente",
		Title:       `Anotar em Empresa Exemplo R$ 12.000,00: "Salário do mês"`,
		BasedOn:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.RefuseProposal(ctx, id4, "Não precisa dessa nota", fixtureNow.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// 5. Stale
	id5, err := db.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: fixtureNow.Add(-20 * time.Minute),
		Proposer:    "agente-notas",
		Method:      "transactions.setNote",
		Arguments:   `["tx-month-end","Fim do mês"]`,
		Reason:      "Marcar fechamento",
		Title:       `Anotar em Mercado Exemplo R$ 90,00: "Fim do mês"`,
		BasedOn:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.MarkProposalStale(ctx, id5, string(reasonProposalStale), fixtureNow.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	_ = full
}

func waitingProposalsOverTheThirdState(t *testing.T, scratch *authoredtest.Scratch, _ *ledgertest.Fixture) {
	t.Helper()
	ctx := t.Context()
	for _, draft := range []authored.ProposalDraft{
		{
			SubmittedAt: fixtureNow.Add(-2 * time.Hour),
			Proposer:    "revisor-semanal",
			Method:      "transactions.addTag",
			Arguments:   `["tx-instalment","#viagem"]`,
			Reason:      "Compra feita na viagem de março",
			Title:       "Adicionar #viagem em Livraria Exemplo R$ 207,00",
			BasedOn:     1,
		},
		{
			SubmittedAt: fixtureNow.Add(-time.Hour),
			Proposer:    "agente-assinaturas",
			Method:      "subscriptions.setNote",
			Arguments:   `["1","Plano anual"]`,
			Reason:      "Anotar tipo de renovação",
			Title:       `Anotar em Streaming Exemplo: "Plano anual"`,
			BasedOn:     1,
		},
	} {
		if _, err := scratch.Database.InsertProposal(ctx, draft); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProposableMethodsMatchTheManifestProposedChangeUnion(t *testing.T) {
	raw, err := os.ReadFile("../contract/contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Definitions map[string]struct {
			AnyOf []struct {
				Properties struct {
					Method struct {
						Const string `json:"const"`
					} `json:"method"`
				} `json:"properties"`
			} `json:"anyOf"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	def, ok := manifest.Definitions["ProposedChange"]
	if !ok {
		t.Fatal("ProposedChange definition missing from contract.json")
	}
	var fromManifest []string
	for _, branch := range def.AnyOf {
		if branch.Properties.Method.Const != "" {
			fromManifest = append(fromManifest, branch.Properties.Method.Const)
		}
	}
	got := ProposableMethods()
	if len(got) != 21 {
		t.Fatalf("ProposableMethods has %d entries, want 21", len(got))
	}
	if !slices.Equal(got, fromManifest) {
		t.Errorf("ProposableMethods = %v, manifest ProposedChange = %v", got, fromManifest)
	}
}

func newProposalBench(t *testing.T) (*writeEnv, *ledgertest.Fixture) {
	t.Helper()
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	full.AddTransaction(ledgertest.Transaction{
		ID: "tx-pix-in", AccountID: accountItau, Date: utc(2026, time.March, 15, 12, 0),
		Description: "PIX RECEBIDO JOAO EXEMPLO", Amount: 150, Currency: "BRL", Type: "CREDIT",
		Raw: map[string]any{"operationType": "PIX", "paymentData": payment("PIX", party("João Exemplo", "44455566677"), nil)},
	})
	inventedAuthoredState(t, scratch)
	inventedTransactionAuthorship(t, scratch, full)
	inventedSubscriptions(t, scratch, full)
	env := newTestEnv(t, withLedger(full.Path, fixtureNow), withDatabase(scratch.Database))
	return &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}, full
}

func postDraft(t *testing.T, env *writeEnv, method string, args any, reason, proposer string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal([]any{map[string]any{
		"change":   map[string]any{"method": method, "arguments": args},
		"reason":   reason,
		"proposer": proposer,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return env.post("review.submitProposal", string(body))
}

func TestEveryProposableMemberSubmitsWithExpectedTitleAndRollsBackDryRun(t *testing.T) {
	// Prepare a bench with every shape needed for the 20 members that are cards.
	env, _ := newProposalBench(t)
	// Ensure baseline states exist for the remove/clear/unlink/recount members.
	// Note from inventedTransactionAuthorship:
	// - tx-instalment: LIVRARIA EXEMPLO R$ 129,00 (already split, subscribed id 3)
	// - tx-foreign: CLOUDSVC SUBSCRIPTION R$ 110,00 (excluded "", subscribed id 2 with note)
	// - tx-month-end: MERCADO EXEMPLO R$ 207,00 (unsplit, not excluded, subscribed id 4)
	// - tx-pending: Padaria do Bairro R$ 18,50 (tags #manhã/#lanche, note "Café com a equipe", fixed tags #lanche/#Trabalho)
	// - tx-pix-out: Maria Exemplo R$ 80,00 (excluded with reason)
	// - tx-first-of-march: Cafeteria Exemplo R$ 10,00 (reimbursable, expected 5.00, linked to tx-stale)
	// - tx-stale: Amigo Exemplo R$ 50,00 (credit linked to tx-first-of-march)
	// - tx-salary: Empresa Exemplo Ltda R$ 5.000,00 (credit)
	setup := []writeCall{
		{"transactions.addTag", `["tx-instalment","#anterior"]`},
		{"transactions.setReimbursable", `["tx-last-of-february",true]`},
	}
	for _, call := range setup {
		if resp := env.post(call.method, call.arguments); resp.Code != http.StatusOK {
			t.Fatalf("setup %s %s answered %d %s", call.method, call.arguments, resp.Code, resp.Body.String())
		}
	}

	type memberCase struct {
		method string
		args   []any
		title  string
	}
	cases := []memberCase{
		{"transactions.addTag", []any{"tx-instalment", "#viagem"}, "Adicionar #viagem a LIVRARIA EXEMPLO R$ 129,00"},
		{"transactions.removeTag", []any{"tx-instalment", "#anterior"}, "Tirar #anterior de LIVRARIA EXEMPLO R$ 129,00"},
		{"transactions.addTagToMany", []any{[]string{"tx-instalment"}, "#lote"}, "Adicionar #lote a LIVRARIA EXEMPLO R$ 129,00"},
		{"transactions.addTagToMany", []any{[]string{"tx-instalment", "tx-foreign"}, "#lote"}, "Adicionar #lote a 2 transações"},
		{"transactions.addFixedTag", []any{"tx-instalment", "#livros"}, "Adicionar #livros a todas as compras em LIVRARIA EXEMPLO"},
		{"transactions.removeFixedTag", []any{"tx-pending", "#lanche"}, "Tirar #lanche de todas as compras em Padaria do Bairro"},
		{"transactions.setNote", []any{"tx-instalment", "Presente"}, `Anotar em LIVRARIA EXEMPLO R$ 129,00: "Presente"`},
		{"transactions.setNote", []any{"tx-pending", ""}, "Apagar a nota de Padaria do Bairro R$ 18,50"},
		{"transactions.setDefaultLine", []any{"tx-instalment", "Feira"}, "Pôr as próximas compras em LIVRARIA EXEMPLO em Feira"},
		{"transactions.setExcluded", []any{"tx-pix-out", false}, "Voltar a contar Maria Exemplo R$ 80,00"},
		{"transactions.setExclusionReason", []any{"tx-pix-out", "Viagem a trabalho"}, `Mudar o motivo de "não conta" de Maria Exemplo R$ 80,00 para "Viagem a trabalho"`},
		{"transactions.excludeMany", []any{[]string{"tx-instalment"}, "Empresa"}, `Marcar como "não conta": LIVRARIA EXEMPLO R$ 129,00`},
		{"transactions.excludeMany", []any{[]string{"tx-instalment", "tx-month-end"}, "Empresa"}, `Marcar 2 transações como "não conta"`},
		{"transactions.setSplit", []any{"tx-month-end", []map[string]any{{"line": "Mercado", "amount": 107}, {"line": "Delivery", "amount": 100}}}, "Dividir MERCADO EXEMPLO R$ 207,00: Mercado R$ 107,00, Delivery R$ 100,00"},
		{"transactions.setSplit", []any{"tx-instalment", []any{}}, "Desfazer a divisão de LIVRARIA EXEMPLO R$ 129,00"},
		{"transactions.setReimbursable", []any{"tx-month-end", true}, "Marcar MERCADO EXEMPLO R$ 207,00 como reembolsável"},
		{"transactions.setReimbursable", []any{"tx-first-of-march", false}, "Desmarcar o reembolso de Cafeteria Exemplo R$ 10,00"},
		{"transactions.setExpectedReimbursement", []any{"tx-first-of-march", 8}, "Esperar R$ 8,00 de reembolso por Cafeteria Exemplo R$ 10,00"},
		{"transactions.setExpectedReimbursement", []any{"tx-first-of-march", nil}, "Tirar o reembolso esperado de Cafeteria Exemplo R$ 10,00"},
		{"transactions.linkReimbursement", []any{"tx-last-of-february", "tx-pix-in"}, "Ligar o PIX de João Exemplo R$ 150,00 a Padaria do Bairro R$ 42,00"},
		{"transactions.unlinkReimbursement", []any{"tx-first-of-march"}, "Desligar o PIX de Cafeteria Exemplo R$ 10,00"},
		{"subscriptions.create", []any{"tx-pending"}, "Criar assinatura de Padaria do Bairro R$ 18,50"},
		{"subscriptions.setStatus", []any{"3", "Cancelar"}, "Mudar status de LIVRARIA EXEMPLO para Cancelar"},
		{"subscriptions.setLine", []any{"3", "Feira"}, "Pôr a assinatura LIVRARIA EXEMPLO em Feira"},
		{"subscriptions.setReviewDate", []any{"3", "2026-06-15"}, "Revisar LIVRARIA EXEMPLO em 15/06/2026"},
		{"subscriptions.setNote", []any{"3", "Rever antes de renovar"}, `Anotar em LIVRARIA EXEMPLO: "Rever antes de renovar"`},
		{"subscriptions.setNote", []any{"2", ""}, "Apagar a nota de CLOUDSVC SUBSCRIPTION"},
	}

	covered := map[string]bool{}
	canonicalBefore := env.scratch.Canonical(t)
	journalBefore := env.scratch.Journal(t)

	for _, tc := range cases {
		covered[tc.method] = true
		resp := postDraft(t, env, tc.method, tc.args, "Sugestão de teste", "revisor")
		if resp.Code != http.StatusOK {
			t.Fatalf("%s %v: answered %d %s, want 200", tc.method, tc.args, resp.Code, resp.Body.String())
		}
		if problem := conformanceProblem("review.submitProposal", resp.Body.Bytes()); problem != "" {
			t.Errorf("%s: does not conform: %s", tc.method, problem)
		}
		var got submittedProposal
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ProposalID == "" || got.Title != tc.title {
			t.Errorf("%s %v: title = %q (id %q), want %q", tc.method, tc.args, got.Title, got.ProposalID, tc.title)
		}
	}

	for _, method := range ProposableMethods() {
		// A line proposed for a transaction waiting for review is no card: the
		// queue's own tests exercise it.
		if method == proposedLineMethod {
			continue
		}
		if !covered[method] {
			t.Errorf("proposable method %s was not exercised", method)
		}
	}
	if after := env.scratch.Canonical(t); after != canonicalBefore {
		t.Errorf("dry runs changed journaled tables:\n%s", differences(canonicalBefore, after))
	}
	if after := env.scratch.Journal(t); after != journalBefore {
		t.Errorf("dry runs wrote to the journal:\n%s", differences(journalBefore, after))
	}

	// review.weekly lists waiting proposals oldest first with proposer.
	weeklyResp := env.post("review.weekly", `["2026-03"]`)
	if weeklyResp.Code != http.StatusOK {
		t.Fatalf("review.weekly answered %d %s", weeklyResp.Code, weeklyResp.Body.String())
	}
	var weekly struct {
		Approvals []approvalRequest `json:"approvals"`
	}
	if err := json.Unmarshal(weeklyResp.Body.Bytes(), &weekly); err != nil {
		t.Fatal(err)
	}
	if len(weekly.Approvals) != len(cases) {
		t.Fatalf("review.weekly listed %d approvals, want %d", len(weekly.Approvals), len(cases))
	}
	if weekly.Approvals[0].ID != "1" || weekly.Approvals[0].Proposer == nil || *weekly.Approvals[0].Proposer != "revisor" {
		t.Errorf("first approval = %+v, want id 1 with proposer revisor", weekly.Approvals[0])
	}
}

func TestSubmittingInvalidIdleOrRefusedProposalsStoresNothing(t *testing.T) {
	env, _ := newProposalBench(t)
	if resp := env.post("transactions.addTag", `["tx-instalment","#ja-tem"]`); resp.Code != http.StatusOK {
		t.Fatalf("setup addTag answered %d", resp.Code)
	}
	before := env.scratch.Everything(t)

	badRequests := []struct {
		label string
		body  string
	}{
		{"empty proposer", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#novo"]},"reason":"ok","proposer":"   "}]`},
		{"proposer over 40 runes", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#novo"]},"reason":"ok","proposer":"` + strings.Repeat("a", 41) + `"}]`},
		{"empty reason", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#novo"]},"reason":" ","proposer":"bot"}]`},
		{"reason over 500 runes", `[{"change":{"method":"transactions.addTag","arguments":["tx-instalment","#novo"]},"reason":"` + strings.Repeat("é", 501) + `","proposer":"bot"}]`},
		{"method outside union", `[{"change":{"method":"budget.save","arguments":["2026-03",[]]},"reason":"ok","proposer":"bot"}]`},
		{"review.approve not proposable", `[{"change":{"method":"review.approve","arguments":["1"]},"reason":"ok","proposer":"bot"}]`},
		{"setExcluded true rejected", `[{"change":{"method":"transactions.setExcluded","arguments":["tx-instalment",true]},"reason":"ok","proposer":"bot"}]`},
	}
	for _, tc := range badRequests {
		expectError(t, tc.label, env.post("review.submitProposal", tc.body), http.StatusBadRequest, "bad_request")
	}

	// Refusals at submission propagate and store nothing.
	expectRefused(t, "unknown tx", postDraft(t, env, "transactions.addTag", []any{"no-such-tx", "#x"}, "Motivo", "bot"), "not_found")
	expectRefused(t, "unknown line", postDraft(t, env, "transactions.setDefaultLine", []any{"tx-instalment", "Linha Inexistente"}, "Motivo", "bot"), "unknown_line")

	// Idle change (0 effects) answers 200 null and stores nothing.
	idle := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#ja-tem"}, "Já marcada", "bot")
	if idle.Code != http.StatusOK || idle.Body.String() != "null" {
		t.Errorf("idle proposal answered %d %s, want 200 null", idle.Code, idle.Body.String())
	}

	if after := env.scratch.Everything(t); after != before {
		t.Errorf("rejected/idle proposals changed the file:\n%s", differences(before, after))
	}
}

func TestApprovingAProposalRecordsUnderModelAddsHistoryNotesAndCanBeUndone(t *testing.T) {
	env, _ := newProposalBench(t)
	if resp := env.post("transactions.setReimbursable", `["tx-month-end",true]`); resp.Code != http.StatusOK {
		t.Fatalf("setReimbursable answered %d %s", resp.Code, resp.Body.String())
	}
	canonicalBefore := env.scratch.Canonical(t)

	// Submit linkReimbursement (names two transactions) and approve it.
	sub := postDraft(t, env, "transactions.linkReimbursement", []any{"tx-month-end", "tx-pix-in"}, "PIX cobriu a despesa", "revisor-semanal")
	if sub.Code != http.StatusOK {
		t.Fatalf("submit answered %d %s", sub.Code, sub.Body.String())
	}
	var submitted submittedProposal
	if err := json.Unmarshal(sub.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	if submitted.ProposalID == "" {
		t.Fatalf("submit answered %s, want a proposalId", sub.Body.String())
	}

	app := env.post("review.approve", `["`+submitted.ProposalID+`"]`)
	if app.Code != http.StatusOK {
		t.Fatalf("approve answered %d %s", app.Code, app.Body.String())
	}
	if problem := conformanceProblem("review.approve", app.Body.Bytes()); problem != "" {
		t.Errorf("approve does not conform: %s", problem)
	}
	var approved approvedProposal
	if err := json.Unmarshal(app.Body.Bytes(), &approved); err != nil {
		t.Fatal(err)
	}
	if approved.ChangeID == "" || approved.Title != submitted.Title {
		t.Fatalf("approved = %+v, want changeId and title %q", approved, submitted.Title)
	}

	// Check journal author == model and method == transactions.linkReimbursement.
	changes := env.scratch.Changes(t)
	last := changes[len(changes)-1]
	if last.Author != string(authored.AuthorModel) || last.Method != "transactions.linkReimbursement" {
		t.Errorf("last change = %+v, want author=model method=transactions.linkReimbursement", last)
	}

	// Check both named transactions received the history note.
	wantNote := "Proposto por revisor-semanal · você aprovou"
	for _, txID := range []string{"tx-month-end", "tx-pix-in"} {
		detailResp := env.post("transactions.detail", `["`+txID+`"]`)
		if detailResp.Code != http.StatusOK {
			t.Fatalf("detail %s answered %d", txID, detailResp.Code)
		}
		if !strings.Contains(detailResp.Body.String(), wantNote) {
			t.Errorf("transaction %s history missing %q: %s", txID, wantNote, detailResp.Body.String())
		}
	}

	// Approving again answers null.
	again := env.post("review.approve", `["`+submitted.ProposalID+`"]`)
	if again.Code != http.StatusOK || again.Body.String() != "null" {
		t.Errorf("second approve answered %d %s, want 200 null", again.Code, again.Body.String())
	}

	// Undo restores every journaled table, and review.proposals shows undone: true.
	if undo := env.undo(approved.ChangeID); undo.Code != http.StatusOK || undo.Body.String() != "null" {
		t.Fatalf("undo answered %d %s", undo.Code, undo.Body.String())
	}
	if after := env.scratch.Canonical(t); after != canonicalBefore {
		t.Errorf("after undo:\n%s", differences(canonicalBefore, after))
	}
	listResp := env.post("review.proposals", `[]`)
	var listed []proposalRecordWire
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Outcome != "approved" || !listed[0].Undone || listed[0].ChangeID == nil || *listed[0].ChangeID != approved.ChangeID {
		t.Errorf("after undo proposals = %+v", listed)
	}
}

func TestConcurrentApprovalsRecordExactlyOneChange(t *testing.T) {
	env, _ := newProposalBench(t)
	sub := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#concorrente"}, "Teste concorrente", "revisor")
	if sub.Code != http.StatusOK {
		t.Fatalf("submit answered %d %s", sub.Code, sub.Body.String())
	}
	var submitted submittedProposal
	if err := json.Unmarshal(sub.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	changesBefore := len(env.scratch.Changes(t))

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = env.post("review.approve", `["`+submitted.ProposalID+`"]`)
		}(i)
	}
	close(start)
	wg.Wait()

	var okChange, okNull int
	for _, resp := range results {
		if resp.Code != http.StatusOK {
			t.Fatalf("concurrent approve answered %d %s", resp.Code, resp.Body.String())
		}
		if resp.Body.String() == "null" {
			okNull++
		} else if _, hasID := changeIDIn(resp.Body.Bytes()); hasID {
			okChange++
		} else {
			t.Fatalf("unexpected body %s", resp.Body.String())
		}
	}
	if okChange != 1 || okNull != 1 {
		t.Errorf("concurrent approvals: %d change, %d null, want 1 and 1", okChange, okNull)
	}
	if got := len(env.scratch.Changes(t)) - changesBefore; got != 1 {
		t.Errorf("journal gained %d changes, want 1", got)
	}
}

func TestStalenessDetectsOverlapAndZeroEffectsAndClosesProposalAsStale(t *testing.T) {
	t.Run("overlapping note column", func(t *testing.T) {
		env, _ := newProposalBench(t)
		sub := postDraft(t, env, "transactions.setNote", []any{"tx-instalment", "Nota do agente"}, "Motivo", "bot")
		var submitted submittedProposal
		_ = json.Unmarshal(sub.Body.Bytes(), &submitted)

		edit := env.post("transactions.setNote", `["tx-instalment","Nota da pessoa"]`)
		if edit.Code != http.StatusOK {
			t.Fatalf("person edit answered %d", edit.Code)
		}
		journalBefore := env.scratch.Journal(t)
		canonicalBefore := env.scratch.Canonical(t)

		expectRefused(t, "stale note", env.post("review.approve", `["`+submitted.ProposalID+`"]`), "proposal_stale")
		if env.scratch.Journal(t) != journalBefore || env.scratch.Canonical(t) != canonicalBefore {
			t.Error("stale approval changed journal or journaled tables")
		}
		assertProposalOutcome(t, env, submitted.ProposalID, "stale", "proposal_stale")

		// After undoing the person's change, a new proposal of the same change approves cleanly.
		editChangeID, _ := changeIDIn(edit.Body.Bytes())
		if undo := env.undo(editChangeID); undo.Code != http.StatusOK {
			t.Fatalf("undo answered %d", undo.Code)
		}
		sub2 := postDraft(t, env, "transactions.setNote", []any{"tx-instalment", "Nota do agente"}, "Motivo", "bot")
		var submitted2 submittedProposal
		_ = json.Unmarshal(sub2.Body.Bytes(), &submitted2)
		if resp := env.post("review.approve", `["`+submitted2.ProposalID+`"]`); resp.Code != http.StatusOK {
			t.Errorf("fresh proposal after undo answered %d %s, want 200", resp.Code, resp.Body.String())
		}
	})

	t.Run("overlapping split rows", func(t *testing.T) {
		env, _ := newProposalBench(t)
		sub := postDraft(t, env, "transactions.setSplit", []any{"tx-month-end", []map[string]any{
			{"line": "Mercado", "amount": 107}, {"line": "Delivery", "amount": 100},
		}}, "Motivo", "bot")
		var submitted submittedProposal
		_ = json.Unmarshal(sub.Body.Bytes(), &submitted)

		if edit := env.post("transactions.setSplit", `["tx-month-end",[{"line":"Mercado","amount":200},{"line":"Delivery","amount":7}]]`); edit.Code != http.StatusOK {
			t.Fatalf("person split answered %d %s", edit.Code, edit.Body.String())
		}
		expectRefused(t, "stale split", env.post("review.approve", `["`+submitted.ProposalID+`"]`), "proposal_stale")
		assertProposalOutcome(t, env, submitted.ProposalID, "stale", "proposal_stale")
	})

	t.Run("zero effects because person already added the tag", func(t *testing.T) {
		env, _ := newProposalBench(t)
		sub := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#viagem"}, "Motivo", "bot")
		var submitted submittedProposal
		_ = json.Unmarshal(sub.Body.Bytes(), &submitted)

		if edit := env.post("transactions.addTag", `["tx-instalment","#viagem"]`); edit.Code != http.StatusOK {
			t.Fatalf("person addTag answered %d", edit.Code)
		}
		expectRefused(t, "zero-effect stale tag", env.post("review.approve", `["`+submitted.ProposalID+`"]`), "proposal_stale")
		assertProposalOutcome(t, env, submitted.ProposalID, "stale", "proposal_stale")
	})

	t.Run("unrelated change does not make proposal stale", func(t *testing.T) {
		env, _ := newProposalBench(t)
		sub := postDraft(t, env, "transactions.setNote", []any{"tx-instalment", "Nota do agente"}, "Motivo", "bot")
		var submitted submittedProposal
		_ = json.Unmarshal(sub.Body.Bytes(), &submitted)

		if edit := env.post("transactions.setNote", `["tx-foreign","Outra transação"]`); edit.Code != http.StatusOK {
			t.Fatalf("unrelated edit answered %d", edit.Code)
		}
		if resp := env.post("review.approve", `["`+submitted.ProposalID+`"]`); resp.Code != http.StatusOK {
			t.Errorf("unrelated change broke approval: %d %s", resp.Code, resp.Body.String())
		}
	})
}

func assertProposalOutcome(t *testing.T, env *writeEnv, id, wantOutcome, wantStaleReason string) {
	t.Helper()
	resp := env.post("review.proposals", `[]`)
	var list []proposalRecordWire
	if err := json.Unmarshal(resp.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID != id {
			continue
		}
		if p.Outcome != wantOutcome {
			t.Errorf("proposal %s outcome = %q, want %q", id, p.Outcome, wantOutcome)
		}
		gotReason := ""
		if p.StaleReason != nil {
			gotReason = *p.StaleReason
		}
		if gotReason != wantStaleReason {
			t.Errorf("proposal %s staleReason = %q, want %q", id, gotReason, wantStaleReason)
		}
		return
	}
	t.Errorf("proposal %s not found in review.proposals", id)
}

func TestHandlerRefusalClosesProposalAsStaleAndLedgerUnavailableLeavesItWaiting(t *testing.T) {
	env, _ := newProposalBench(t)
	sub := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#sumiu"}, "Motivo", "bot")
	var submitted submittedProposal
	_ = json.Unmarshal(sub.Body.Bytes(), &submitted)

	// Ledger unavailable leaves the proposal waiting.
	missingEnv := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "missing.db"), fixtureNow), withDatabase(env.scratch.Database))
	missingWrite := &writeEnv{testEnv: missingEnv, scratch: env.scratch, cookie: missingEnv.unlock()}
	expectError(t, "missing ledger", missingWrite.post("review.approve", `["`+submitted.ProposalID+`"]`), http.StatusServiceUnavailable, "ledger_unavailable")
	assertProposalOutcome(t, missingWrite, submitted.ProposalID, "waiting", "")

	// A ledger without the transaction makes the handler refuse with not_found -> proposal closed as stale with not_found.
	empty := newEmptyLedger(t)
	authoredtest.TakeRunIn(t, env.scratch.Database, 1)
	emptyEnv := newTestEnv(t, withLedger(empty.Path, fixtureNow), withDatabase(env.scratch.Database))
	emptyWrite := &writeEnv{testEnv: emptyEnv, scratch: env.scratch, cookie: emptyEnv.unlock()}
	expectRefused(t, "disappeared tx", emptyWrite.post("review.approve", `["`+submitted.ProposalID+`"]`), "not_found")
	assertProposalOutcome(t, emptyWrite, submitted.ProposalID, "stale", "not_found")
}

func TestRefusingAProposalRecordsNoChangeAndStoresOptionalReason(t *testing.T) {
	env, _ := newProposalBench(t)
	s1 := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#um"}, "Motivo 1", "bot")
	s2 := postDraft(t, env, "transactions.addTag", []any{"tx-instalment", "#dois"}, "Motivo 2", "bot")
	var p1, p2 submittedProposal
	_ = json.Unmarshal(s1.Body.Bytes(), &p1)
	_ = json.Unmarshal(s2.Body.Bytes(), &p2)

	journalBefore := env.scratch.Journal(t)
	canonicalBefore := env.scratch.Canonical(t)

	// Reason over 500 runes is bad_request and leaves waiting.
	expectError(t, "reason over 500", env.post("review.refuse", `["`+p1.ProposalID+`","`+strings.Repeat("x", 501)+`"]`), http.StatusBadRequest, "bad_request")

	// Refuse with reason.
	r1 := env.post("review.refuse", `["`+p1.ProposalID+`","  Não era viagem  "]`)
	if r1.Code != http.StatusOK || r1.Body.String() != `{"title":"`+p1.Title+`"}` {
		t.Fatalf("refuse 1 answered %d %s", r1.Code, r1.Body.String())
	}
	// Refuse without reason (omitted).
	r2 := env.post("review.refuse", `["`+p2.ProposalID+`"]`)
	if r2.Code != http.StatusOK || r2.Body.String() != `{"title":"`+p2.Title+`"}` {
		t.Fatalf("refuse 2 answered %d %s", r2.Code, r2.Body.String())
	}
	// Refusing again answers null.
	again := env.post("review.refuse", `["`+p1.ProposalID+`"]`)
	if again.Code != http.StatusOK || again.Body.String() != "null" {
		t.Errorf("second refuse answered %d %s, want 200 null", again.Code, again.Body.String())
	}
	if env.scratch.Journal(t) != journalBefore || env.scratch.Canonical(t) != canonicalBefore {
		t.Error("refusing changed journal or journaled tables")
	}

	listResp := env.post("review.proposals", `[]`)
	var listed []proposalRecordWire
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	// Newest first: p2 then p1.
	if len(listed) != 2 || listed[0].ID != p2.ProposalID || listed[0].Refusal != nil {
		t.Errorf("newest refused = %+v, want id %s with nil refusal", listed[0], p2.ProposalID)
	}
	if listed[1].ID != p1.ProposalID || listed[1].Refusal == nil || *listed[1].Refusal != "Não era viagem" {
		t.Errorf("older refused = %+v, want id %s with trimmed refusal", listed[1], p1.ProposalID)
	}
}

func TestProposalsReadWorksWithoutDatabaseOrSnapshot(t *testing.T) {
	// Without --db, answers [].
	noDB := newTestEnv(t, withLedger(newFullLedger(t).Path, fixtureNow))
	cookie := noDB.unlock()
	resp := noDB.call("/api/review/proposals", `[]`, cookie)
	if resp.Code != http.StatusOK || resp.Body.String() != "[]" {
		t.Errorf("without db answered %d %s, want 200 []", resp.Code, resp.Body.String())
	}

	// With --db and missing snapshot, still answers stored proposals.
	scratch := authoredtest.Open(t, fixtureClock)
	_, err := scratch.Database.InsertProposal(t.Context(), authored.ProposalDraft{
		SubmittedAt: fixtureNow, Proposer: "bot", Method: "transactions.addTag",
		Arguments: `["tx-1","#x"]`, Reason: "Motivo", Title: "Título", BasedOn: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	missing := newTestEnv(t, withLedger(filepath.Join(t.TempDir(), "missing.db"), fixtureNow), withDatabase(scratch.Database))
	resp = missing.call("/api/review/proposals", `[]`, missing.unlock())
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"proposer":"bot"`) {
		t.Errorf("with missing snapshot answered %d %s", resp.Code, resp.Body.String())
	}
}

func TestProposalSubmissionAndApprovalTakeAnUnseenSnapshotIn(t *testing.T) {
	scratch := authoredtest.Open(t, fixtureClock)
	full := newFullLedger(t)
	taken := 0
	env := newTestEnvNotTakenIn(t,
		withLedger(full.Path, fixtureNow),
		withDatabase(scratch.Database),
		func(config *Config) {
			config.TakeSnapshotIn = func(_ context.Context, _ time.Time) error {
				taken++
				authoredtest.TakeRunIn(t, scratch.Database, 1)
				return nil
			}
		},
	)
	wenv := &writeEnv{testEnv: env, scratch: scratch, cookie: env.unlock()}
	sub := postDraft(t, wenv, "transactions.addTag", []any{"tx-instalment", "#novo"}, "Motivo", "bot")
	if sub.Code != http.StatusOK || taken != 1 {
		t.Fatalf("submit over unseen snapshot: status %d body %s taken=%d", sub.Code, sub.Body.String(), taken)
	}
	var submitted submittedProposal
	_ = json.Unmarshal(sub.Body.Bytes(), &submitted)
	if app := wenv.post("review.approve", `["`+submitted.ProposalID+`"]`); app.Code != http.StatusOK {
		t.Fatalf("approve answered %d %s", app.Code, app.Body.String())
	}
}
