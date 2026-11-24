import { ApiFailure } from '../http/failure';
import type { ReviewApi } from '../types';
import { assertOpen, budgetFor, saveFrom } from './budget-versions';
import type { ChangeLog } from './changes';
import { createClosing } from './closing';
import { createFakeHome } from './home';
import { createHistoryRecorder } from './history';
import type { FakeState, FakeTransaction } from './model';
import { createPayday } from './payday';
import type { ProposalEffect } from './review-model';
import { awaitingApprovals, isMonthClosed, proposalRecords, reviewQueue, visibleWarnings } from './review-rules';
import { createFakeSubscriptions } from './subscriptions';
import { suggestedPurchase } from './subscription-rules';
import { nameOf } from './transaction-detail';
import { shiftMonth } from '@/lib/format';

const cents = (amount: number): number => Math.round(amount * 100) / 100;

export function createFakeReview(state: FakeState, original: FakeTransaction[], log: ChangeLog): ReviewApi {
  const history = createHistoryRecorder(state);
  const home = createFakeHome(state, original);
  const subscriptions = createFakeSubscriptions(state, log);
  const findTransaction = (id: string) => state.transactions.find((transaction) => transaction.id === id);

  /** A cap approved once the month is closed holds from the next month that is still open. */
  function firstOpenMonth(): string {
    let month = state.currentMonth;
    while (isMonthClosed(state, month)) month = shiftMonth(month, 1);
    return month;
  }

  function effectAlreadyHolds(effect: ProposalEffect): boolean {
    if (effect.kind === 'cap') {
      const line = budgetFor(state, firstOpenMonth())
        .flatMap((group) => group.lines)
        .find((candidate) => candidate.name === effect.line);
      return !line || line.amount === effect.amount;
    }
    if (effect.kind === 'status') {
      const subscription = state.subscriptions.find((candidate) => candidate.id === effect.subscriptionId);
      return !subscription || subscription.status === effect.status;
    }
    const transaction = findTransaction(effect.transactionId);
    return !transaction || transaction.excludedReason === effect.reason;
  }

  function answeredInstant(): string {
    return `${state.shell.today}T12:00:00.000Z`;
  }

  function applyEffect(effect: ProposalEffect): void {
    if (effect.kind === 'cap') {
      const month = firstOpenMonth();
      assertOpen(state, month);
      const draft = budgetFor(state, month).map((group) => ({
        ...group,
        lines: group.lines.map((line) => (line.name === effect.line ? { ...line, amount: effect.amount } : line)),
      }));
      saveFrom(state, month, draft);
      return;
    }
    if (effect.kind === 'status') {
      const subscription = state.subscriptions.find((candidate) => candidate.id === effect.subscriptionId);
      if (subscription) subscription.status = effect.status;
      return;
    }
    const transaction = findTransaction(effect.transactionId);
    if (!transaction) return;
    history.record(transaction, 'IA marcou como não conta (você aprovou)', ['excludedReason'], () => {
      transaction.excludedReason = effect.reason;
    });
  }

  function answerPending(transaction: FakeTransaction, line: string, reason: string): void {
    const changed = line !== transaction.pending?.suggestion;
    history.record(
      transaction,
      `${changed ? 'Você alterou para' : 'Você aceitou'} ${line}`,
      ['line', 'by', 'confidence', 'pending'],
      () => Object.assign(transaction, { line, by: 'Você', confidence: null, pending: null }),
    );
    if (!reason) return;
    transaction.reasonText = reason;
    const latest = transaction.history.at(-1);
    if (latest) latest.reason = reason;
  }

  return {
    ...createClosing(state, original, log),
    ...createPayday(state, log),

    async weekly(month) {
      const { limit, weeks } = await home.summary(month);
      const week = weeks.find((candidate) => candidate.current);
      const remaining = cents(limit.cap - limit.spent);
      const fillOf = (part: number, whole: number) => Math.min(100, whole ? (part / whole) * 100 : 0);
      return {
        limit: { cap: limit.cap, spent: limit.spent, remaining, over: remaining < 0, fill: fillOf(limit.spent, limit.cap) },
        week: week ? { guide: week.guide, spent: week.spent ?? 0, fill: fillOf(week.spent ?? 0, week.guide) } : null,
        queue: { items: reviewQueue(state), resolved: state.review.resolved },
        approvals: awaitingApprovals(state),
        warnings: visibleWarnings(state),
      };
    },

    async acceptPending(transactionId, answer) {
      const transaction = findTransaction(transactionId);
      if (!transaction?.pending) return null;
      const changeId = log.begin();
      const nickname = transaction.person ? answer.nickname.trim() : '';
      const establishmentName = nameOf(state, transaction);
      answerPending(transaction, answer.line, answer.reason.trim());
      const always = answer.always && !transaction.person;
      const rule = { ...(always ? { line: answer.line } : {}), ...(nickname ? { nickname } : {}) };
      if (always || nickname) {
        state.establishments[transaction.establishmentKey] = { ...state.establishments[transaction.establishmentKey], ...rule };
      }
      if (always) history.note(transaction, `Regra criada: ${establishmentName} sempre em ${answer.line}`);
      state.review.resolved += 1;
      return { changeId, name: nickname || establishmentName };
    },

    async approve(id) {
      const proposal = state.review.proposals.find((candidate) => candidate.id === id);
      if (!proposal || !state.review.awaiting.includes(id)) return null;
      if (effectAlreadyHolds(proposal.effect)) {
        state.review.awaiting = state.review.awaiting.filter((awaiting) => awaiting !== id);
        proposal.outcome = 'stale';
        proposal.answeredAt = answeredInstant();
        proposal.staleReason = 'proposal_stale';
        proposal.refusal = null;
        proposal.changeId = null;
        throw new ApiFailure('refused', { methodClass: 'write', reason: 'proposal_stale' });
      }
      const changeId = log.beginWithBudget();
      applyEffect(proposal.effect);
      state.review.awaiting = state.review.awaiting.filter((awaiting) => awaiting !== id);
      proposal.outcome = 'approved';
      proposal.answeredAt = answeredInstant();
      proposal.changeId = changeId;
      proposal.refusal = null;
      proposal.staleReason = null;
      return { changeId, title: proposal.title };
    },

    async refuse(id, reason = '') {
      const proposal = state.review.proposals.find((candidate) => candidate.id === id);
      if (!proposal || !state.review.awaiting.includes(id)) return null;
      state.review.awaiting = state.review.awaiting.filter((awaiting) => awaiting !== id);
      const trimmed = (reason ?? '').trim();
      proposal.outcome = 'refused';
      proposal.answeredAt = answeredInstant();
      proposal.refusal = trimmed || null;
      proposal.staleReason = null;
      proposal.changeId = null;
      return { title: proposal.title };
    },

    async propose(id) {
      const known = state.review.proposals.some((proposal) => proposal.id === id);
      if (!known || state.review.awaiting.includes(id)) return null;
      const changeId = log.begin();
      state.review.awaiting = [...state.review.awaiting, id];
      return { changeId };
    },

    async submitProposal() {
      return null;
    },

    async proposals() {
      return proposalRecords(state);
    },

    async dismissWarning(id) {
      if (!state.review.warnings.some((warning) => warning.id === id)) return null;
      const changeId = log.begin();
      state.review.warnings = state.review.warnings.filter((warning) => warning.id !== id);
      return { changeId };
    },

    async runWarningAction(id, action) {
      const warning = state.review.warnings.find((candidate) => candidate.id === id);
      if (!warning?.actions.some((offered) => offered.id === action)) return null;
      if (action === 'accept-suggestion') {
        const created = await subscriptions.acceptSuggestion();
        return created && { changeId: created.changeId, outcome: 'subscription-created', name: created.name };
      }
      if (action === 'reject-suggestion') {
        const purchase = suggestedPurchase(state);
        const rejected = purchase ? await subscriptions.rejectSuggestion() : null;
        return rejected && purchase && { changeId: rejected.changeId, outcome: 'suggestion-rejected', name: nameOf(state, purchase) };
      }
      const changeId = log.begin();
      const subscription = state.subscriptions.find((candidate) => candidate.id === warning.subscriptionId);
      if (subscription) subscription.flags = subscription.flags.filter((flag) => flag !== 'overlapping');
      state.review.warnings = state.review.warnings.filter((candidate) => candidate.id !== id);
      return { changeId, outcome: 'pair-silenced', name: subscription?.name ?? '' };
    },
  };
}
