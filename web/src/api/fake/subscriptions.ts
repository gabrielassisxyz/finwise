import type {
  ChangeResult,
  CreatedSubscription,
  SubscriptionCandidate,
  SubscriptionDetail,
  SubscriptionList,
  SubscriptionsApi,
  SubscriptionSuggestion,
} from '../types';
import type { ChangeLog } from './changes';
import type { FakeState, FakeSubscription, FakeTransaction } from './model';
import {
  annualEquivalent,
  annualTotal,
  candidatePurchases,
  chargesOf,
  groupRows,
  marksOf,
  monthlyEquivalent,
  monthlyTotal,
  subscriptionFrom,
  suggestedPurchase,
  toRecurring,
} from './subscription-rules';
import { nameOf } from './transaction-detail';

function toDetail(subscription: FakeSubscription, today: string): SubscriptionDetail {
  return {
    id: subscription.id,
    name: subscription.name,
    cycle: subscription.cycle,
    amount: subscription.amount,
    marks: marksOf(subscription, today),
    nextCharge: subscription.nextCharge,
    monthlyEquivalent: monthlyEquivalent(subscription),
    annualEquivalent: annualEquivalent(subscription),
    line: subscription.line,
    status: subscription.status,
    reviewOn: subscription.reviewOn,
    note: subscription.note,
    charges: chargesOf(subscription),
    alternatives: [...subscription.alternatives],
  };
}

export function createFakeSubscriptions(state: FakeState, log: ChangeLog): SubscriptionsApi {
  const today = () => state.shell.today;

  function add(purchase: FakeTransaction, reviewFrom: string): CreatedSubscription {
    const changeId = log.begin();
    const created = subscriptionFrom(state, purchase, reviewFrom);
    state.subscriptions = [...state.subscriptions, created];
    return { changeId, name: created.name };
  }

  function change(id: string, apply: (subscription: FakeSubscription) => void): ChangeResult {
    const changeId = log.begin();
    const subscription = state.subscriptions.find((candidate) => candidate.id === id);
    if (subscription) apply(subscription);
    return { changeId };
  }

  return {
    async list({ grouped }): Promise<SubscriptionList> {
      const purchase = suggestedPurchase(state);
      const suggestion: SubscriptionSuggestion | null = purchase ? { name: nameOf(state, purchase) } : null;
      return {
        monthlyTotal: monthlyTotal(state.subscriptions),
        annualTotal: annualTotal(state.subscriptions),
        groups: groupRows(state.subscriptions, today(), grouped),
        suggestion,
      };
    },
    async detail(id) {
      const subscription = state.subscriptions.find((candidate) => candidate.id === id);
      return subscription ? toDetail(subscription, today()) : null;
    },
    async candidates(): Promise<SubscriptionCandidate[]> {
      return candidatePurchases(state).map((purchase) => ({
        transactionId: purchase.id,
        name: nameOf(state, purchase),
        amount: Math.abs(purchase.amount),
      }));
    },
    async recurring() {
      return state.recurringPurchases.map((purchase) => toRecurring(purchase, today()));
    },
    async create(transactionId) {
      const purchase = candidatePurchases(state).find((candidate) => candidate.id === transactionId);
      return purchase ? add(purchase, today()) : null;
    },
    async acceptSuggestion() {
      const purchase = suggestedPurchase(state);
      return purchase ? add(purchase, purchase.day) : null;
    },
    async rejectSuggestion() {
      if (!suggestedPurchase(state)) return null;
      const changeId = log.begin();
      if (state.subscriptionSuggestion) state.subscriptionSuggestion.dismissed = true;
      return { changeId };
    },
    async setStatus(id, status) {
      return change(id, (subscription) => {
        subscription.status = status;
      });
    },
    async setLine(id, line) {
      return change(id, (subscription) => {
        subscription.line = line;
      });
    },
    async setReviewDate(id, day) {
      return change(id, (subscription) => {
        subscription.reviewOn = day;
      });
    },
    async setNote(id, note) {
      return change(id, (subscription) => {
        subscription.note = note;
      });
    },
  };
}
