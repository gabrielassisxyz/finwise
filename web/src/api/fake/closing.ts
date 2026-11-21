import type { CardBill, ClosingApi, ClosingView, YearMonth } from '../types';
import type { ChangeLog } from './changes';
import type { FakeState, FakeTransaction } from './model';
import {
  closingContributions,
  closingSteps,
  isMonthClosed,
  leftover,
  MARKABLE_STEPS,
  monthEnd,
  monthReport,
  plannedActual,
  subscriptionsToReview,
} from './review-rules';
import { pendingCount } from './rules';
import { addMonths } from './subscription-rules';

/** A kept subscription is looked at again this many months after the end of the month it was reviewed in. */
const KEEP_REVIEW_MONTHS = 6;

function cardBill(state: FakeState): CardBill | null {
  if (!state.payAhead) return null;
  const purchases = state.review.cardBillPurchases.flatMap((id) => {
    const purchase = state.transactions.find((candidate) => candidate.id === id);
    return purchase ? [{ name: purchase.name, day: purchase.day, amount: Math.abs(purchase.amount) }] : [];
  });
  return { amount: state.payAhead.amount, cardLabel: state.payAhead.cardLabel, purchases };
}

export function createClosing(state: FakeState, original: FakeTransaction[], log: ChangeLog): ClosingApi {
  const find = (id: string) => state.subscriptions.find((subscription) => subscription.id === id);

  return {
    async closing(month: YearMonth): Promise<ClosingView> {
      const closedOn = state.shell.closedMonths.find((closed) => closed.month === month)?.closedOn ?? null;
      return {
        month,
        closed: closedOn !== null,
        closedOn,
        steps: closingSteps(state, month),
        pendingCount: pendingCount(state),
        plannedActual: plannedActual(state, original, month),
        leftover: leftover(state, original, month),
        contributions: closingContributions(state, month),
        cardBill: cardBill(state),
        subscriptionsToReview: subscriptionsToReview(state, month),
        report: monthReport(state, original, month),
        closedMonths: structuredClone(state.shell.closedMonths).sort((a, b) => b.month.localeCompare(a.month)),
      };
    },

    async markStepDone(month, step) {
      if (isMonthClosed(state, month) || !MARKABLE_STEPS.includes(step)) return null;
      const changeId = log.begin();
      const marked = state.review.stepsDone[month] ?? [];
      state.review.stepsDone[month] = marked.includes(step) ? marked : [...marked, step];
      return { changeId };
    },

    async keepSubscription(month, id) {
      const subscription = find(id);
      if (!subscription) return null;
      const changeId = log.begin();
      const nextReview = addMonths(monthEnd(month), KEEP_REVIEW_MONTHS);
      Object.assign(subscription, { status: 'Manter', reviewOn: nextReview });
      return { changeId, name: subscription.name, nextReview };
    },

    async cancelSubscription(id) {
      const subscription = find(id);
      if (!subscription) return null;
      const changeId = log.begin();
      subscription.status = 'Cancelar';
      return { changeId, name: subscription.name };
    },

    async closeMonth(month) {
      if (isMonthClosed(state, month)) throw new Error(`${month} is already closed`);
      if (pendingCount(state) > 0) throw new Error('A month with transactions waiting for review does not close');
      const changeId = log.begin();
      state.shell.closedMonths = [...state.shell.closedMonths, { month, closedOn: state.shell.today }];
      return { changeId };
    },
  };
}
