package api

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/ledger"
	"github.com/gabrielassisxyz/finwise/internal/money"
	"github.com/gabrielassisxyz/finwise/internal/txrules"
)

// proposableMethods is the closed list of writes an external agent may propose,
// in the contract's order. A test pins this list to the manifest's ProposedChange
// union, and the CLI reads it from here.
var proposableMethods = []string{
	"transactions.addTag",
	"transactions.removeTag",
	"transactions.addTagToMany",
	"transactions.addFixedTag",
	"transactions.removeFixedTag",
	"transactions.setNote",
	"transactions.setDefaultLine",
	"transactions.setExcluded",
	"transactions.setExclusionReason",
	"transactions.excludeMany",
	"transactions.setSplit",
	"transactions.setReimbursable",
	"transactions.setExpectedReimbursement",
	"transactions.linkReimbursement",
	"transactions.unlinkReimbursement",
	"subscriptions.create",
	"subscriptions.setStatus",
	"subscriptions.setLine",
	"subscriptions.setReviewDate",
	"subscriptions.setNote",
}

// ProposableMethods returns a copy of the closed list of methods an agent may
// propose, in contract order.
func ProposableMethods() []string {
	return slices.Clone(proposableMethods)
}

func isProposableMethod(method string) bool {
	return slices.Contains(proposableMethods, method)
}

// proposalTransactionIDs returns the deduplicated transaction ids named by a
// proposed method's arguments, in order, so an approval can add its history
// note to each of them.
func proposalTransactionIDs(method string, args []json.RawMessage) ([]string, error) {
	switch method {
	case "transactions.addTag", "transactions.removeTag",
		"transactions.addFixedTag", "transactions.removeFixedTag",
		"transactions.setNote", "transactions.setDefaultLine",
		"transactions.setExcluded", "transactions.setExclusionReason",
		"transactions.setSplit", "transactions.setReimbursable",
		"transactions.setExpectedReimbursement", "transactions.unlinkReimbursement",
		"subscriptions.create":
		id, err := textArgument(args[0])
		if err != nil {
			return nil, err
		}
		return []string{id}, nil
	case "transactions.addTagToMany", "transactions.excludeMany":
		ids, _, err := idsAndText(args)
		if err != nil {
			return nil, err
		}
		return uniqueIDs(ids), nil
	case "transactions.linkReimbursement":
		id, creditID, err := idAndText(args)
		if err != nil {
			return nil, err
		}
		return uniqueIDs([]string{id, creditID}), nil
	case "subscriptions.setStatus", "subscriptions.setLine",
		"subscriptions.setReviewDate", "subscriptions.setNote":
		return nil, nil
	default:
		return nil, &badArgumentsError{}
	}
}

func uniqueIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// wordProposalTitle produces the app-authored Portuguese sentence describing
// what approving the proposed change will do.
func wordProposalTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, method string, args []json.RawMessage) (string, error) {
	switch method {
	case "transactions.addTag":
		id, tag, err := idAndText(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalAddTagTitle(authored.NormalizeTag(tag), label), nil
	case "transactions.removeTag":
		id, tag, err := idAndText(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalRemoveTagTitle(authored.NormalizeTag(tag), label), nil
	case "transactions.addTagToMany":
		return wordAddTagToManyTitle(ctx, snapshot, view, args)
	case "transactions.addFixedTag":
		id, tag, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := establishmentLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalAddFixedTagTitle(authored.NormalizeTag(tag), name), nil
	case "transactions.removeFixedTag":
		id, tag, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := establishmentLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalRemoveFixedTagTitle(authored.NormalizeTag(tag), name), nil
	case "transactions.setNote":
		id, note, err := idAndText(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return noteTitle(label, note), nil
	case "transactions.setDefaultLine":
		id, line, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := establishmentLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalSetDefaultLineTitle(name, line), nil
	case "transactions.setExcluded":
		id, on, err := idAndFlag(args)
		if err != nil || on {
			return "", &badArgumentsError{}
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalRecountTitle(label), nil
	case "transactions.setExclusionReason":
		id, reason, err := idAndText(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalSetExclusionReasonTitle(label, strings.TrimSpace(reason)), nil
	case "transactions.excludeMany":
		return wordExcludeManyTitle(ctx, snapshot, view, args)
	case "transactions.setSplit":
		return wordSetSplitTitle(ctx, snapshot, view, args)
	case "transactions.setReimbursable":
		id, on, err := idAndFlag(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		if on {
			return proposalMarkReimbursableTitle(label), nil
		}
		return proposalUnmarkReimbursableTitle(label), nil
	case "transactions.setExpectedReimbursement":
		return wordSetExpectedReimbursementTitle(ctx, snapshot, view, args)
	case "transactions.linkReimbursement":
		return wordLinkReimbursementTitle(ctx, snapshot, view, args)
	case "transactions.unlinkReimbursement":
		id, err := stringArgument(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalUnlinkReimbursementTitle(label), nil
	case "subscriptions.create":
		id, err := stringArgument(args)
		if err != nil {
			return "", err
		}
		label, err := transactionLabel(ctx, snapshot, view, id)
		if err != nil {
			return "", err
		}
		return proposalCreateSubscriptionTitle(label), nil
	case "subscriptions.setStatus":
		idText, status, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := subscriptionLabel(ctx, view, idText)
		if err != nil {
			return "", err
		}
		return proposalSetSubscriptionStatusTitle(name, status), nil
	case "subscriptions.setLine":
		idText, line, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := subscriptionLabel(ctx, view, idText)
		if err != nil {
			return "", err
		}
		return proposalSetSubscriptionLineTitle(name, line), nil
	case "subscriptions.setReviewDate":
		idText, dayText, err := idAndText(args)
		if err != nil {
			return "", err
		}
		day, err := brday.ParseDay(dayText)
		if err != nil {
			return "", &badArgumentsError{}
		}
		name, err := subscriptionLabel(ctx, view, idText)
		if err != nil {
			return "", err
		}
		return proposalSetSubscriptionReviewDateTitle(name, brazilianDay(day)), nil
	case "subscriptions.setNote":
		idText, note, err := idAndText(args)
		if err != nil {
			return "", err
		}
		name, err := subscriptionLabel(ctx, view, idText)
		if err != nil {
			return "", err
		}
		return noteTitle(name, note), nil
	default:
		return "", &badArgumentsError{}
	}
}

func wordAddTagToManyTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, args []json.RawMessage) (string, error) {
	ids, tag, err := idsAndText(args)
	if err != nil {
		return "", err
	}
	unique := uniqueIDs(ids)
	normalized := authored.NormalizeTag(tag)
	if len(unique) == 1 {
		label, err := transactionLabel(ctx, snapshot, view, unique[0])
		if err != nil {
			return "", err
		}
		return proposalAddTagTitle(normalized, label), nil
	}
	return proposalAddTagToManyTitle(normalized, len(unique)), nil
}

func wordExcludeManyTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, args []json.RawMessage) (string, error) {
	ids, _, err := idsAndText(args)
	if err != nil {
		return "", err
	}
	unique := uniqueIDs(ids)
	if len(unique) == 1 {
		label, err := transactionLabel(ctx, snapshot, view, unique[0])
		if err != nil {
			return "", err
		}
		return proposalExcludeOneTitle(label), nil
	}
	return proposalExcludeManyTitle(len(unique)), nil
}

func wordSetSplitTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, args []json.RawMessage) (string, error) {
	if len(args) != 2 {
		return "", &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return "", err
	}
	var parts []splitPartArgument
	if err := decodeArgument(args[1], &parts); err != nil {
		return "", err
	}
	label, err := transactionLabel(ctx, snapshot, view, id)
	if err != nil {
		return "", err
	}
	if len(parts) < 2 {
		return proposalClearSplitTitle(label), nil
	}
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Amount == nil {
			return "", &badArgumentsError{}
		}
		lineName := proposalSplitPartNoLine
		if part.Line != nil && *part.Line != "" {
			lineName = *part.Line
		}
		rendered = append(rendered, lineName+" "+reaisText(money.Cents(*part.Amount)))
	}
	return proposalSetSplitTitle(label, strings.Join(rendered, ", ")), nil
}

func wordSetExpectedReimbursementTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, args []json.RawMessage) (string, error) {
	if len(args) != 2 {
		return "", &badArgumentsError{}
	}
	id, err := textArgument(args[0])
	if err != nil {
		return "", err
	}
	var amount *Reais
	if err := decodeArgument(args[1], &amount); err != nil {
		return "", err
	}
	label, err := transactionLabel(ctx, snapshot, view, id)
	if err != nil {
		return "", err
	}
	if amount == nil {
		return proposalClearExpectedReimbursementTitle(label), nil
	}
	return proposalSetExpectedReimbursementTitle(reaisText(money.Cents(*amount)), label), nil
}

func wordLinkReimbursementTitle(ctx context.Context, snapshot *ledger.View, view *authored.View, args []json.RawMessage) (string, error) {
	id, creditID, err := idAndText(args)
	if err != nil {
		return "", err
	}
	purchase, err := listedTransaction(ctx, snapshot, id)
	if err != nil {
		return "", err
	}
	credit, err := listedTransaction(ctx, snapshot, creditID)
	if err != nil {
		return "", err
	}
	establishments, err := establishmentsOf(ctx, view, []txrules.Transaction{purchase, credit})
	if err != nil {
		return "", err
	}
	purchaseLabel := formatTransactionLabel(establishments, purchase)
	creditName := shownName(establishments, credit.EstablishmentKey, credit.Name)
	return proposalLinkReimbursementTitle(creditName, reaisText(absCents(credit.Amount)), purchaseLabel), nil
}

func noteTitle(target, note string) string {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return proposalClearNoteTitle(target)
	}
	return proposalSetNoteTitle(target, trimmed)
}

func listedTransaction(ctx context.Context, snapshot *ledger.View, id string) (txrules.Transaction, error) {
	row, found, err := findListed(ctx, snapshot, id)
	if err != nil {
		return txrules.Transaction{}, err
	}
	if !found {
		return txrules.Transaction{}, errors.New("api: proposed transaction disappeared during dry run")
	}
	return row.listed, nil
}

func transactionLabel(ctx context.Context, snapshot *ledger.View, view *authored.View, id string) (string, error) {
	listed, err := listedTransaction(ctx, snapshot, id)
	if err != nil {
		return "", err
	}
	establishments, err := establishmentsOf(ctx, view, []txrules.Transaction{listed})
	if err != nil {
		return "", err
	}
	return formatTransactionLabel(establishments, listed), nil
}

func formatTransactionLabel(establishments map[string]authored.Establishment, listed txrules.Transaction) string {
	name := shownName(establishments, listed.EstablishmentKey, listed.Name)
	return name + " " + reaisText(absCents(listed.Amount))
}

func establishmentLabel(ctx context.Context, snapshot *ledger.View, view *authored.View, id string) (string, error) {
	listed, err := listedTransaction(ctx, snapshot, id)
	if err != nil {
		return "", err
	}
	establishments, err := establishmentsOf(ctx, view, []txrules.Transaction{listed})
	if err != nil {
		return "", err
	}
	return shownName(establishments, listed.EstablishmentKey, listed.Name), nil
}

func subscriptionLabel(ctx context.Context, view *authored.View, idText string) (string, error) {
	id, ok := authored.ParseSubscriptionID(idText)
	if !ok {
		return "", errors.New("api: proposed subscription id does not parse")
	}
	subscription, found, err := view.Subscription(ctx, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("api: proposed subscription disappeared during dry run")
	}
	return subscription.Name, nil
}

func absCents(cents money.Cents) money.Cents {
	if cents < 0 {
		return -cents
	}
	return cents
}

func brazilianDay(day brday.Day) string {
	text := string(day)
	return text[8:10] + "/" + text[5:7] + "/" + text[0:4]
}
