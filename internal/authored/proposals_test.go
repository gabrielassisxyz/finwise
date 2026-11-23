package authored_test

import (
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

func TestTheProposalsMigrationIsRecorded(t *testing.T) {
	path := databasePath(t)
	openAndClose(t, path, creationInstant, nil)
	want := embeddedMigrationNames(t)
	if !slices.Contains(want, "0010_proposals") {
		t.Fatalf("the embedded set lacks 0010_proposals: %v", want)
	}
	if got := namesOf(t, path); !equalStrings(got, want) {
		t.Errorf("recorded = %v, want %v", got, want)
	}
}

func TestTheProposalsTableRefusesInconsistentRows(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	pool := database.Pool()
	ctx := t.Context()

	// Seed one change so foreign-key checks on change_id can be exercised.
	now := clockAt(t, creationInstant)()
	var changeID authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorModel, Method: "transactions.setNote", At: now}
	if err := database.Change(ctx, facts, func(recorder *authored.Recorder) error {
		var err error
		changeID, err = recorder.ID(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	const ts = "2026-11-28T12:00:00.000Z"
	const ans = "2026-11-28T13:00:00.000Z"

	valid := map[string]struct {
		query string
		args  []any
	}{
		"waiting": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'agent', 'transactions.setNote', '["tx","x"]', 'why', 'title', 0)`,
			[]any{ts},
		},
		"approved": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, change_id) VALUES (?, 'agent', 'transactions.setNote', '["tx","x"]', 'why', 'title', 0, 'approved', ?, ?)`,
			[]any{ts, ans, int64(changeID)},
		},
		"refused with reason": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, refusal) VALUES (?, 'agent', 'transactions.setNote', '["tx","x"]', 'why', 'title', 0, 'refused', ?, 'no')`,
			[]any{ts, ans},
		},
		"refused without reason": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at) VALUES (?, 'agent', 'transactions.setNote', '["tx","x"]', 'why', 'title', 0, 'refused', ?)`,
			[]any{ts, ans},
		},
		"stale": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, stale_reason) VALUES (?, 'agent', 'transactions.setNote', '["tx","x"]', 'why', 'title', 0, 'stale', ?, 'proposal_stale')`,
			[]any{ts, ans},
		},
	}
	for label, c := range valid {
		if _, err := pool.ExecContext(ctx, c.query, c.args...); err != nil {
			t.Fatalf("%s was refused: %v", label, err)
		}
	}

	refused := map[string]struct {
		query string
		args  []any
	}{
		"bad submitted_at": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES ('2026-11-28', 'agent', 'm', '[]', 'r', 't', 0)`, nil,
		},
		"empty proposer": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, '', 'm', '[]', 'r', 't', 0)`, []any{ts},
		},
		"empty method": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', '', '[]', 'r', 't', 0)`, []any{ts},
		},
		"arguments not an array": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', 'm', '{}', 'r', 't', 0)`, []any{ts},
		},
		"arguments not json": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', 'm', 'nope', 'r', 't', 0)`, []any{ts},
		},
		"empty reason": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', 'm', '[]', '', 't', 0)`, []any{ts},
		},
		"empty title": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', 'm', '[]', 'r', '', 0)`, []any{ts},
		},
		"negative based_on": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on) VALUES (?, 'a', 'm', '[]', 'r', 't', -1)`, []any{ts},
		},
		"unknown outcome": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'done')`, []any{ts},
		},
		"waiting with answered_at": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'waiting', ?)`, []any{ts, ans},
		},
		"approved without change_id": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'approved', ?)`, []any{ts, ans},
		},
		"approved with refusal": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, change_id, refusal) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'approved', ?, ?, 'x')`, []any{ts, ans, int64(changeID)},
		},
		"approved with unknown change_id": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, change_id) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'approved', ?, 999)`, []any{ts, ans},
		},
		"refused with empty refusal string": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, refusal) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'refused', ?, '')`, []any{ts, ans},
		},
		"refused without answered_at": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, refusal) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'refused', 'no')`, []any{ts},
		},
		"refused with change_id": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, change_id) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'refused', ?, ?)`, []any{ts, ans, int64(changeID)},
		},
		"stale without stale_reason": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'stale', ?)`, []any{ts, ans},
		},
		"stale with empty stale_reason": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, stale_reason) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'stale', ?, '')`, []any{ts, ans},
		},
		"stale with change_id": {
			`INSERT INTO proposals (submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, stale_reason, change_id) VALUES (?, 'a', 'm', '[]', 'r', 't', 0, 'stale', ?, 'proposal_stale', ?)`, []any{ts, ans, int64(changeID)},
		},
	}
	for label, c := range refused {
		if _, err := pool.ExecContext(ctx, c.query, c.args...); err == nil {
			t.Errorf("the database accepted %s", label)
		}
	}
}

func TestDryRunRollsBackEverythingAndReportsEffectsAndNewestChange(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	ctx := t.Context()

	beforeDump, err := database.CanonicalDump(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := clockAt(t, creationInstant)()
	effects, basedOn, err := database.DryRun(ctx, authored.ChangeFacts{Author: authored.AuthorModel, Method: "settings.setCardLabel", At: now}, func(recorder *authored.Recorder) error {
		return recorder.SetCardLabel(ctx, "Cartão Teste")
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if effects != 1 || basedOn != 0 {
		t.Errorf("effects=%d basedOn=%d, want 1 and 0", effects, basedOn)
	}
	afterDump, err := database.CanonicalDump(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterDump != beforeDump {
		t.Errorf("dry run modified a journaled table")
	}
	view := database.View()
	defer view.Close()
	if newest, err := view.NewestChangeID(ctx); err != nil || newest != 0 {
		t.Errorf("newest change after dry run = %d (%v), want 0", newest, err)
	}
	if label, err := view.CardLabel(ctx); err != nil || label != authored.DefaultCardLabel {
		t.Errorf("card label after dry run = %q (%v), want default", label, err)
	}
}

func TestProposalLifecycleInsertApproveUndoRefuseStale(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	ctx := t.Context()

	t1 := time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)
	t4 := t3.Add(time.Minute)

	firstID, err := database.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: t1, Proposer: "assistente", Method: "settings.setCardLabel",
		Arguments: `["Cartão A"]`, Reason: "nome mais claro", Title: "Renomear cartão", BasedOn: 0,
	})
	if err != nil || firstID != 1 {
		t.Fatalf("insert 1: id=%d err=%v", firstID, err)
	}
	secondID, err := database.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: t2, Proposer: "assistente", Method: "settings.setCardLabel",
		Arguments: `["Cartão B"]`, Reason: "outra ideia", Title: "Outra proposta", BasedOn: 0,
	})
	if err != nil || secondID != 2 {
		t.Fatalf("insert 2: id=%d err=%v", secondID, err)
	}
	thirdID, err := database.InsertProposal(ctx, authored.ProposalDraft{
		SubmittedAt: t3, Proposer: "assistente", Method: "settings.setCardLabel",
		Arguments: `["Cartão C"]`, Reason: "terceira ideia", Title: "Terceira proposta", BasedOn: 0,
	})
	if err != nil || thirdID != 3 {
		t.Fatalf("insert 3: id=%d err=%v", thirdID, err)
	}

	view := database.View()
	waiting, err := view.WaitingProposals(ctx)
	view.Close()
	if err != nil || len(waiting) != 3 || waiting[0].ID != firstID || waiting[1].ID != secondID || waiting[2].ID != thirdID {
		t.Fatalf("waiting = %+v (%v), want 1, 2, 3 oldest first", waiting, err)
	}

	// Approve first inside a change under AuthorModel.
	var approvedChange authored.ChangeID
	facts := authored.ChangeFacts{Author: authored.AuthorModel, Method: "settings.setCardLabel", At: t4}
	if err := database.Change(ctx, facts, func(recorder *authored.Recorder) error {
		if err := recorder.SetCardLabel(ctx, "Cartão A"); err != nil {
			return err
		}
		claimed, err := recorder.ApproveProposal(ctx, firstID, t4)
		if err != nil || !claimed {
			t.Fatalf("approve claim: claimed=%v err=%v", claimed, err)
		}
		// Claiming a second time inside the same or another change answers false.
		again, err := recorder.ApproveProposal(ctx, firstID, t4)
		if err != nil || again {
			t.Fatalf("second claim: again=%v err=%v", again, err)
		}
		approvedChange, err = recorder.ID(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Refuse second with a reason; refusing again answers false.
	refused, ok, err := database.RefuseProposal(ctx, secondID, "prefiro como está", t4)
	if err != nil || !ok || refused.Outcome != authored.ProposalRefused || refused.Refusal == nil || *refused.Refusal != "prefiro como está" {
		t.Fatalf("refuse 2: %+v ok=%v err=%v", refused, ok, err)
	}
	if _, again, err := database.RefuseProposal(ctx, secondID, "de novo", t4); err != nil || again {
		t.Errorf("second refuse: again=%v err=%v", again, err)
	}

	// Mark third stale.
	closed, err := database.MarkProposalStale(ctx, thirdID, "proposal_stale", t4)
	if err != nil || !closed {
		t.Fatalf("mark stale 3: closed=%v err=%v", closed, err)
	}

	// Undo the approved change: the proposal stays approved, with Undone=true.
	if err := database.Undo(ctx, approvedChange, t4.Add(time.Minute)); err != nil {
		t.Fatalf("undo approved change: %v", err)
	}

	view = database.View()
	defer view.Close()
	all, err := view.Proposals(ctx)
	if err != nil || len(all) != 3 {
		t.Fatalf("proposals = %+v (%v)", all, err)
	}
	// Newest first: 3, 2, 1.
	if all[0].ID != thirdID || all[0].Outcome != authored.ProposalStale || all[0].StaleReason == nil || *all[0].StaleReason != "proposal_stale" {
		t.Errorf("proposal 3 = %+v", all[0])
	}
	if all[1].ID != secondID || all[1].Outcome != authored.ProposalRefused || all[1].Refusal == nil || *all[1].Refusal != "prefiro como está" {
		t.Errorf("proposal 2 = %+v", all[1])
	}
	if all[2].ID != firstID || all[2].Outcome != authored.ProposalApproved || all[2].ChangeID == nil || *all[2].ChangeID != approvedChange || !all[2].Undone {
		t.Errorf("proposal 1 after undo = %+v, want approved and undone=true", all[2])
	}
	if rest, err := view.WaitingProposals(ctx); err != nil || len(rest) != 0 {
		t.Errorf("waiting after answering all = %+v (%v)", rest, err)
	}
}

func TestOverlapsSinceFollowsTheUndoEnginesUnitRule(t *testing.T) {
	database := openDatabase(t, databasePath(t), creationInstant, nil)
	ctx := t.Context()

	now := clockAt(t, creationInstant)()
	// Seed a transaction in the index so we can test tx_state (optional facts) and tx_tags (set).
	var key authored.TxKey
	var baseChange authored.ChangeID
	if err := database.Change(ctx, authored.ChangeFacts{Author: authored.AuthorPerson, Method: "seed", At: now}, func(recorder *authored.Recorder) error {
		var err error
		key, err = recorder.IndexTransaction(ctx, authored.SeenTransaction{
			ProviderID: "tx-1", AccountID: "acc-1", Day: "2026-11-10", Cents: -5000, EstablishmentKey: "text:LOJA",
		})
		if err != nil {
			return err
		}
		if err := recorder.SetNote(ctx, key, "inicial"); err != nil {
			return err
		}
		baseChange, err = recorder.ID(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Later change modifies the note column on tx_state.
	var noteChange authored.ChangeID
	if err := database.Change(ctx, authored.ChangeFacts{Author: authored.AuthorPerson, Method: "transactions.setNote", At: now}, func(recorder *authored.Recorder) error {
		if err := recorder.SetNote(ctx, key, "editada pela pessoa"); err != nil {
			return err
		}
		var err error
		noteChange, err = recorder.ID(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	checkOverlap := func(basedOn authored.ChangeID, write func(*authored.Recorder) error) bool {
		t.Helper()
		var overlapped bool
		_, _, err := database.DryRun(ctx, authored.ChangeFacts{Author: authored.AuthorModel, Method: "check", At: now}, func(recorder *authored.Recorder) error {
			if err := write(recorder); err != nil {
				return err
			}
			var err error
			overlapped, err = recorder.OverlapsSince(ctx, basedOn)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return overlapped
	}

	// Another change on the same column (note) overlaps since baseChange, but not since noteChange.
	if !checkOverlap(baseChange, func(r *authored.Recorder) error { return r.SetNote(ctx, key, "proposta") }) {
		t.Error("a change to note after baseChange did not overlap with a setNote proposal")
	}
	if checkOverlap(noteChange, func(r *authored.Recorder) error { return r.SetNote(ctx, key, "proposta") }) {
		t.Error("a proposal based on noteChange overlapped with noteChange itself")
	}
	// A change to a different table (tx_tags) does not overlap with the note change.
	if checkOverlap(baseChange, func(r *authored.Recorder) error { return r.AddTag(ctx, key, "#viagem") }) {
		t.Error("adding a tag overlapped with a later change to note")
	}

	// Once noteChange is undone, it is no longer in force, so overlap is false.
	if err := database.Undo(ctx, noteChange, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if checkOverlap(baseChange, func(r *authored.Recorder) error { return r.SetNote(ctx, key, "proposta") }) {
		t.Error("an undone change still caused overlap")
	}
}

func TestEmptyViewProposalsAnswersEmpty(t *testing.T) {
	view := authored.EmptyView()
	ctx := t.Context()
	if list, err := view.Proposals(ctx); err != nil || len(list) != 0 {
		t.Errorf("Proposals on empty view = %v (%v)", list, err)
	}
	if list, err := view.WaitingProposals(ctx); err != nil || len(list) != 0 {
		t.Errorf("WaitingProposals on empty view = %v (%v)", list, err)
	}
	if _, found, err := view.Proposal(ctx, 1); err != nil || found {
		t.Errorf("Proposal on empty view = found:%v err:%v", found, err)
	}
	if newest, err := view.NewestChangeID(ctx); err != nil || newest != 0 {
		t.Errorf("NewestChangeID on empty view = %d (%v)", newest, err)
	}
}

func TestParseProposalIDAcceptsOnlyCanonicalPositiveDecimals(t *testing.T) {
	if id, ok := authored.ParseProposalID("42"); !ok || id != 42 {
		t.Errorf("42: got %d %v", id, ok)
	}
	for _, bad := range []string{"", "0", "-1", "01", "+1", "teto", "1.0"} {
		if _, ok := authored.ParseProposalID(bad); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}
