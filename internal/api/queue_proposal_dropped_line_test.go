package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gabrielassisxyz/finwise/internal/authored/authoredtest"
	"github.com/gabrielassisxyz/finwise/internal/brday"
)

// documentLineSent and documentGroupSent are the budget document as a client
// posts it, which leaves out what a line does not have.
type documentLineSent struct {
	ID     *string `json:"id,omitempty"`
	Name   string  `json:"name"`
	Amount Reais   `json:"amount"`
	Flag   string  `json:"flag,omitempty"`
}

type documentGroupSent struct {
	Key   string             `json:"key"`
	Name  string             `json:"name"`
	Lines []documentLineSent `json:"lines"`
}

// saveBudgetWithout saves the budget of month as the person would after taking
// one line out of it: every other line keeps its id, so the transactions on
// them stay where they are.
func (env *writeEnv) saveBudgetWithout(month brday.Month, dropped string) {
	env.t.Helper()
	view := env.scratch.Database.View()
	budget, err := view.BudgetOf(env.t.Context(), month)
	view.Close()
	if err != nil {
		env.t.Fatal(err)
	}
	groups := []documentGroupSent{}
	for _, group := range authoredtest.DocumentOf(budget) {
		wire := documentGroupSent{Key: group.Key, Name: group.Key, Lines: []documentLineSent{}}
		for _, line := range group.Lines {
			if line.Name == dropped {
				continue
			}
			wire.Lines = append(wire.Lines, documentLineSent{ID: line.ID, Name: line.Name, Amount: Reais(line.Cents), Flag: line.Flag})
		}
		groups = append(groups, wire)
	}
	body, err := json.Marshal([]any{string(month), groups})
	if err != nil {
		env.t.Fatal(err)
	}
	if response := env.post("budget.save", string(body)); response.Code != http.StatusOK {
		env.t.Fatalf("saving the budget without %s answered %d %s", dropped, response.Code, response.Body.String())
	}
}

// A line an agent proposed counts only while the transaction's month still has
// it. The proposal itself keeps waiting, but what the queue, the list and the
// panel show, and what confirming puts the transaction on, is what they would
// be without it: the model's line, else the rule's, else none.
func TestALineAnAgentProposedThatTheMonthNoLongerHasIsNeitherShownNorConfirmed(t *testing.T) {
	const modelJustification = "o estabelecimento vende mantimentos"
	for _, c := range []struct {
		name, id, reason string
		model            string
		shown            string
		confirms         string
	}{
		{"the rule's line", reviewP1, "Baixa confiança", "", pendingOfP1, "Restaurantes"},
		{"the model's line", reviewP1, "Baixa confiança", "Mercado",
			`{"reason":"Baixa confiança","suggestion":"Mercado","confidence":90,"justification":"` + modelJustification + `"}`, "Mercado"},
		{"no line", reviewP3, "Estabelecimento novo", "",
			`{"reason":"Estabelecimento novo","suggestion":null,"confidence":null,"justification":null}`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, _ := reviewExamples(t)
			if c.model != "" {
				env.theModelSays(c.id, c.model, 90, modelJustification)
			}
			ids := linesOfMonthByName(t, env.scratch.Database, "2026-03")
			proposal := env.proposed(c.id, "Contas da casa", agentReason)
			env.expectShown(c.id, `{"reason":"`+c.reason+`","suggestion":"Contas da casa","confidence":null,"justification":"`+agentReason+`"}`)

			env.saveBudgetWithout("2026-03", "Contas da casa")

			if record := env.proposalRecord(proposal); record.Outcome != "waiting" {
				t.Fatalf("the proposal reads %+v after its line left the budget, want it waiting", record)
			}
			env.expectShown(c.id, c.shown)
			if _, found := linesOfMonthByName(t, env.scratch.Database, "2026-03")["Contas da casa"]; found {
				t.Fatal("the budget of March still has the line the agent proposed")
			}

			if response := env.post("transactions.confirmLine", `["`+c.id+`"]`); response.Code != http.StatusOK {
				t.Fatalf("confirming answered %d %s", response.Code, response.Body.String())
			}

			line := env.storedAbout(c.id).State.Line
			if c.confirms == "" {
				if line != nil {
					t.Errorf("confirming put the transaction on line %d, want it left without one", *line)
				}
				return
			}
			if line == nil || *line != ids[c.confirms] {
				t.Errorf("confirming put the transaction on %v, want %s (line %d)", line, c.confirms, ids[c.confirms])
			}
			if line != nil && *line == ids["Contas da casa"] {
				t.Errorf("confirming put the transaction on the line the month no longer has")
			}
		})
	}
}
