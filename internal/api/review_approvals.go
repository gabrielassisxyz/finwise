package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

const (
	maxProposerLength      = 40
	maxProposalReasonRunes = 500
	maxRefusalReasonRunes  = 500
)

// errAlreadyAnswered rolls back an approval transaction when another request
// answered the waiting proposal before this one claimed it.
var errAlreadyAnswered = errors.New("api: proposal is no longer waiting")

type approvalRequest struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Reason   string  `json:"reason"`
	Proposer *string `json:"proposer"`
}

type proposedChangeWire struct {
	Method    string            `json:"method"`
	Arguments []json.RawMessage `json:"arguments"`
}

type proposalDraftWire struct {
	Change   proposedChangeWire `json:"change"`
	Reason   string             `json:"reason"`
	Proposer string             `json:"proposer"`
}

type submittedProposal struct {
	ProposalID string `json:"proposalId"`
	Title      string `json:"title"`
}

type approvedProposal struct {
	ChangeID string `json:"changeId"`
	Title    string `json:"title"`
}

type refusedProposal struct {
	Title string `json:"title"`
}

type proposalRecordWire struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	Change      *proposedChangeWire `json:"change"`
	Reason      string              `json:"reason"`
	Proposer    *string             `json:"proposer"`
	SubmittedAt string              `json:"submittedAt"`
	Outcome     string              `json:"outcome"`
	AnsweredAt  *string             `json:"answeredAt"`
	Refusal     *string             `json:"refusal"`
	StaleReason *string             `json:"staleReason"`
	ChangeID    *string             `json:"changeId"`
	Undone      bool                `json:"undone"`
}

func registerReviewApprovals(registry *Registry) {
	registry.Register("review.proposals", readProposals)
	registry.RegisterJournal("review.submitProposal", submitProposalWith(registry))
	registry.RegisterJournal("review.approve", approveProposalWith(registry))
	registry.RegisterJournal("review.refuse", refuseProposal)
	registry.RegisterWrite("review.propose", answerNothingProposed)
}

func answerNothingProposed(context.Context, []json.RawMessage, Writes) (any, error) {
	return nil, nil
}

// waitingApprovals is the weekly review's approvals card, oldest first.
func waitingApprovals(ctx context.Context, view *authored.View) ([]approvalRequest, error) {
	stored, err := view.WaitingProposals(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]approvalRequest, 0, len(stored))
	for _, proposal := range stored {
		proposer := proposal.Proposer
		out = append(out, approvalRequest{
			ID:       proposal.ID.String(),
			Title:    proposal.Title,
			Reason:   proposal.Reason,
			Proposer: &proposer,
		})
	}
	return out, nil
}

// readProposals lists every stored proposal newest first, with its outcome. It
// needs no ledger snapshot and answers [] without the app's database.
func readProposals(ctx context.Context, args []json.RawMessage, views Views) (any, error) {
	if len(args) != 0 {
		return nil, &badArgumentsError{}
	}
	stored, err := views.Authored.Proposals(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]proposalRecordWire, 0, len(stored))
	for _, proposal := range stored {
		var parsedArgs []json.RawMessage
		if err := json.Unmarshal([]byte(proposal.Arguments), &parsedArgs); err != nil {
			return nil, err
		}
		proposer := proposal.Proposer
		record := proposalRecordWire{
			ID:          proposal.ID.String(),
			Title:       proposal.Title,
			Change:      &proposedChangeWire{Method: proposal.Method, Arguments: parsedArgs},
			Reason:      proposal.Reason,
			Proposer:    &proposer,
			SubmittedAt: proposal.SubmittedAt,
			Outcome:     string(proposal.Outcome),
			AnsweredAt:  proposal.AnsweredAt,
			Refusal:     proposal.Refusal,
			StaleReason: proposal.StaleReason,
			Undone:      proposal.Undone,
		}
		if proposal.ChangeID != nil {
			changeID := proposal.ChangeID.String()
			record.ChangeID = &changeID
		}
		records = append(records, record)
	}
	return records, nil
}

// submitProposalWith validates a proposal against the current state via a dry
// run that always rolls back, words its title from what the dry run saw, and
// inserts it as waiting when the dry run recorded at least one effect.
func submitProposalWith(registry *Registry) JournalHandler {
	return func(ctx context.Context, args []json.RawMessage, journal Journal, ledgerView *ledger.View) (any, error) {
		draft, err := decodeProposalDraft(args)
		if err != nil {
			return nil, err
		}
		registered, ok := registry.lookup(draft.Change.Method)
		if !ok || registered.write == nil {
			return nil, errors.New("api: proposed method has no registered write handler")
		}
		instant := nowOf(ctx)
		facts := authored.ChangeFacts{Author: authored.AuthorModel, Method: draft.Change.Method, At: instant}
		var title string
		effects, basedOn, err := journal.DryRun(ctx, facts, func(recorder *authored.Recorder) error {
			writes := Writes{Ledger: ledgerView, Authored: recorder.View(), Recorder: recorder}
			if _, err := callWrite(ctx, registered.write, draft.Change.Arguments, writes); err != nil {
				return err
			}
			if recorder.EffectCount() == 0 {
				return nil
			}
			title, err = wordProposalTitle(ctx, ledgerView, recorder.View(), draft.Change.Method, draft.Change.Arguments)
			return err
		})
		if err != nil {
			return nil, err
		}
		if effects == 0 {
			return nil, nil
		}
		encodedArgs, err := json.Marshal(draft.Change.Arguments)
		if err != nil {
			return nil, err
		}
		id, err := journal.InsertProposal(ctx, authored.ProposalDraft{
			SubmittedAt: instant,
			Proposer:    draft.Proposer,
			Method:      draft.Change.Method,
			Arguments:   string(encodedArgs),
			Reason:      draft.Reason,
			Title:       title,
			BasedOn:     basedOn,
		})
		if err != nil {
			return nil, err
		}
		return submittedProposal{ProposalID: id.String(), Title: title}, nil
	}
}

func decodeProposalDraft(args []json.RawMessage) (proposalDraftWire, error) {
	if len(args) != 1 {
		return proposalDraftWire{}, &badArgumentsError{}
	}
	var draft *proposalDraftWire
	if err := decodeArgument(args[0], &draft); err != nil || draft == nil {
		return proposalDraftWire{}, &badArgumentsError{}
	}
	proposer := strings.TrimSpace(draft.Proposer)
	reason := strings.TrimSpace(draft.Reason)
	if n := utf8.RuneCountInString(proposer); n < 1 || n > maxProposerLength {
		return proposalDraftWire{}, &badArgumentsError{}
	}
	if n := utf8.RuneCountInString(reason); n < 1 || n > maxProposalReasonRunes {
		return proposalDraftWire{}, &badArgumentsError{}
	}
	if !isProposableMethod(draft.Change.Method) || draft.Change.Arguments == nil {
		return proposalDraftWire{}, &badArgumentsError{}
	}
	if draft.Change.Method == "transactions.setExcluded" {
		if _, on, err := idAndFlag(draft.Change.Arguments); err != nil || on {
			return proposalDraftWire{}, &badArgumentsError{}
		}
	}
	draft.Proposer = proposer
	draft.Reason = reason
	return *draft, nil
}

// approveProposalWith runs the proposed method's own registered handler inside
// a change journaled under AuthorModel, verifying inside the change's write
// lock that the proposal still waits, recorded at least one effect, and does
// not overlap any change still in force since based_on.
func approveProposalWith(registry *Registry) JournalHandler {
	return func(ctx context.Context, args []json.RawMessage, journal Journal, ledgerView *ledger.View) (any, error) {
		text, err := stringArgument(args)
		if err != nil {
			return nil, err
		}
		id, ok := authored.ParseProposalID(text)
		if !ok {
			return nil, nil
		}
		proposal, waiting, err := loadWaitingProposal(ctx, journal, id)
		if err != nil || !waiting {
			return nil, err
		}
		registered, ok := registry.lookup(proposal.Method)
		if !ok || registered.write == nil {
			return nil, errors.New("api: proposed method has no registered write handler")
		}
		var proposedArgs []json.RawMessage
		if err := json.Unmarshal([]byte(proposal.Arguments), &proposedArgs); err != nil {
			return nil, err
		}
		instant := nowOf(ctx)
		facts := authored.ChangeFacts{Author: authored.AuthorModel, Method: proposal.Method, At: instant}
		stillWaiting := true
		var changeID authored.ChangeID
		err = journal.Change(ctx, facts, func(recorder *authored.Recorder) error {
			view := recorder.View()
			defer view.Close()
			current, found, err := view.Proposal(ctx, id)
			if err != nil {
				return err
			}
			if !found || current.Outcome != authored.ProposalWaiting {
				stillWaiting = false
				return nil
			}
			writes := Writes{Ledger: ledgerView, Authored: view, Recorder: recorder}
			if _, err := callWrite(ctx, registered.write, proposedArgs, writes); err != nil {
				return err
			}
			if recorder.EffectCount() == 0 {
				return refuse(reasonProposalStale)
			}
			overlapped, err := recorder.OverlapsSince(ctx, current.BasedOn)
			if err != nil {
				return err
			}
			if overlapped {
				return refuse(reasonProposalStale)
			}
			if err := addProposalHistoryNotes(ctx, recorder, view, current.Method, proposedArgs, current.Proposer); err != nil {
				return err
			}
			claimed, err := recorder.ApproveProposal(ctx, id, instant)
			if err != nil {
				return err
			}
			if !claimed {
				stillWaiting = false
				return errAlreadyAnswered
			}
			changeID, err = recorder.ID(ctx)
			return err
		})
		if err != nil {
			if errors.Is(err, errAlreadyAnswered) {
				return nil, nil
			}
			if kind, reason := classify(err); kind == kindRefused {
				if _, markErr := journal.MarkProposalStale(ctx, id, string(reason), instant); markErr != nil {
					return nil, markErr
				}
			}
			return nil, err
		}
		if !stillWaiting {
			return nil, nil
		}
		return approvedProposal{ChangeID: changeID.String(), Title: proposal.Title}, nil
	}
}

func loadWaitingProposal(ctx context.Context, journal Journal, id authored.ProposalID) (authored.Proposal, bool, error) {
	view := journal.View()
	defer view.Close()
	proposal, found, err := view.Proposal(ctx, id)
	if err != nil || !found || proposal.Outcome != authored.ProposalWaiting {
		return authored.Proposal{}, false, err
	}
	return proposal, true, nil
}

func addProposalHistoryNotes(ctx context.Context, recorder *authored.Recorder, view *authored.View, method string, args []json.RawMessage, proposer string) error {
	txIDs, err := proposalTransactionIDs(method, args)
	if err != nil || len(txIDs) == 0 {
		return err
	}
	keys, err := view.TransactionKeys(ctx, txIDs)
	if err != nil {
		return err
	}
	note := proposalApprovedNote(proposer)
	for _, id := range txIDs {
		key, indexed := keys[id]
		if !indexed {
			continue
		}
		if _, err := recorder.AddHistoryStep(ctx, key, authored.StepNote, note); err != nil {
			return err
		}
	}
	return nil
}

// refuseProposal closes a waiting proposal as refused with an optional reason
// (at most 500 characters), recording no change.
func refuseProposal(ctx context.Context, args []json.RawMessage, journal Journal, _ *ledger.View) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, &badArgumentsError{}
	}
	idText, err := textArgument(args[0])
	if err != nil {
		return nil, err
	}
	var reason string
	if len(args) == 2 {
		var optional *string
		if err := decodeArgument(args[1], &optional); err != nil {
			return nil, &badArgumentsError{}
		}
		if optional != nil {
			reason = strings.TrimSpace(*optional)
		}
	}
	if utf8.RuneCountInString(reason) > maxRefusalReasonRunes {
		return nil, &badArgumentsError{}
	}
	id, ok := authored.ParseProposalID(idText)
	if !ok {
		return nil, nil
	}
	refused, found, err := journal.RefuseProposal(ctx, id, reason, nowOf(ctx))
	if err != nil || !found {
		return nil, err
	}
	return refusedProposal{Title: refused.Title}, nil
}
