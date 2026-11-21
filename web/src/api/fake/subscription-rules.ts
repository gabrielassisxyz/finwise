import type {
  IsoDay,
  RecurringPurchase,
  SubscriptionCharge,
  SubscriptionCycle,
  SubscriptionGroup,
  SubscriptionMark,
  SubscriptionRow,
  SubscriptionStatus,
} from '../types';
import type { FakeRecurringPurchase, FakeState, FakeSubscription, FakeTransaction } from './model';
import { nameOf } from './transaction-detail';

// The money and date rules of the subscription screens. The browser shows
// what these return and computes none of it.

export const STATUS_ORDER: SubscriptionStatus[] = ['Manter', 'Analisar', 'Backup e cancelar', 'Cancelar'];

const CYCLE_MONTHS: Record<SubscriptionCycle, number> = { Mensal: 1, Trimestral: 3, Semestral: 6, Anual: 12, Bienal: 24 };

/** How far ahead a renewal of a yearly cycle or longer is worth a warning. */
const RENEWAL_NOTICE_DAYS = 30;
const CHARGES_SHOWN = 3;
const CANDIDATES_SHOWN = 12;
/** A subscription made from a purchase is reviewed this many months after it. */
const REVIEW_AFTER_MONTHS = 3;
const MS_PER_DAY = 86_400_000;

const cents = (amount: number): number => Math.round(amount * 100) / 100;

function parts(day: IsoDay): [number, number, number] {
  return [Number(day.slice(0, 4)), Number(day.slice(5, 7)), Number(day.slice(8, 10))];
}

const format = (date: Date): IsoDay => date.toISOString().slice(0, 10);

/** Moves a day by whole months; a day the target month lacks becomes that month's last. */
export function addMonths(day: IsoDay, months: number): IsoDay {
  const [year, month, dayOfMonth] = parts(day);
  const first = new Date(Date.UTC(year, month - 1 + months, 1));
  const lastDay = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate();
  return format(new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), Math.min(dayOfMonth, lastDay))));
}

export function addDays(day: IsoDay, days: number): IsoDay {
  const [year, month, dayOfMonth] = parts(day);
  return format(new Date(Date.UTC(year, month - 1, dayOfMonth + days)));
}

/** Whole days from `from` to `to`; negative when `to` is earlier. */
export function daysBetween(from: IsoDay, to: IsoDay): number {
  const [fromYear, fromMonth, fromDay] = parts(from);
  const [toYear, toMonth, toDay] = parts(to);
  return Math.round((Date.UTC(toYear, toMonth - 1, toDay) - Date.UTC(fromYear, fromMonth - 1, fromDay)) / MS_PER_DAY);
}

export function monthlyEquivalent(subscription: FakeSubscription): number {
  return cents(subscription.amount / CYCLE_MONTHS[subscription.cycle]);
}

export function annualEquivalent(subscription: FakeSubscription): number {
  return cents((subscription.amount * 12) / CYCLE_MONTHS[subscription.cycle]);
}

/** The sum of the equivalents as they are shown, so the total adds up to what the rows say. */
export function monthlyTotal(subscriptions: FakeSubscription[]): number {
  return cents(subscriptions.reduce((total, subscription) => total + monthlyEquivalent(subscription), 0));
}

/** Twelve times the monthly total that is shown, so the two lines of the header agree. */
export function annualTotal(subscriptions: FakeSubscription[]): number {
  return cents(monthlyTotal(subscriptions) * 12);
}

const renewalText = (days: number): string =>
  days === 0 ? 'Renova hoje' : `Renova em ${days} ${days === 1 ? 'dia' : 'dias'}`;

export function marksOf(subscription: FakeSubscription, today: IsoDay): SubscriptionMark[] {
  const marks: SubscriptionMark[] = [];
  const priceRose = subscription.previousAmount !== null && subscription.previousAmount < subscription.amount;
  if (priceRose) marks.push({ text: 'Preço subiu', tone: 'warning' });
  if (subscription.flags.includes('overlapping')) marks.push({ text: 'Sobreposta', tone: 'warning' });
  const daysToRenewal = daysBetween(today, subscription.nextCharge);
  const yearly = CYCLE_MONTHS[subscription.cycle] >= 12;
  if (yearly && daysToRenewal >= 0 && daysToRenewal <= RENEWAL_NOTICE_DAYS) {
    marks.push({ text: renewalText(daysToRenewal), tone: 'warning' });
  }
  if (subscription.flags.includes('charged-after-cancel')) marks.push({ text: 'Cobrou após cancelar', tone: 'bad' });
  return marks;
}

export function toRow(subscription: FakeSubscription, today: IsoDay): SubscriptionRow {
  return {
    id: subscription.id,
    name: subscription.name,
    amount: subscription.amount,
    cycle: subscription.cycle,
    nextCharge: subscription.nextCharge,
    monthlyEquivalent: monthlyEquivalent(subscription),
    line: subscription.line,
    status: subscription.status,
    reviewOn: subscription.reviewOn,
    reviewOverdue: subscription.reviewOn <= today,
    marks: marksOf(subscription, today),
  };
}

/** The subscriptions in the order they were added, or one group per status that has any. */
export function groupRows(subscriptions: FakeSubscription[], today: IsoDay, grouped: boolean): SubscriptionGroup[] {
  if (!grouped) {
    return [{ status: null, monthlyTotal: monthlyTotal(subscriptions), subscriptions: subscriptions.map((s) => toRow(s, today)) }];
  }
  return STATUS_ORDER.map((status) => ({ status, members: subscriptions.filter((candidate) => candidate.status === status) }))
    .filter(({ members }) => members.length > 0)
    .map(({ status, members }) => ({
      status,
      monthlyTotal: monthlyTotal(members),
      subscriptions: members.map((member) => toRow(member, today)),
    }));
}

/** The latest charges, newest first; those before a price change cost what the price was then. */
export function chargesOf(subscription: FakeSubscription): SubscriptionCharge[] {
  const step = CYCLE_MONTHS[subscription.cycle];
  return Array.from({ length: CHARGES_SHOWN }, (_, index) => ({
    day: addMonths(subscription.nextCharge, -step * (index + 1)),
    amount: index === 0 ? subscription.amount : (subscription.previousAmount ?? subscription.amount),
    foreign: subscription.foreign,
  }));
}

export function nextExpected(purchase: FakeRecurringPurchase): IsoDay {
  const { every, unit } = purchase.cadence;
  if (unit === 'months') return addMonths(purchase.lastPurchase, every);
  return addDays(purchase.lastPurchase, unit === 'weeks' ? every * 7 : every);
}

export function toRecurring(purchase: FakeRecurringPurchase, today: IsoDay): RecurringPurchase {
  const next = nextExpected(purchase);
  return {
    name: purchase.name,
    cadence: purchase.cadence,
    lastPurchase: purchase.lastPurchase,
    nextExpected: next,
    averageAmount: purchase.amount,
    due: next <= today,
  };
}

/** This month's card purchases that are not subscriptions yet, one per establishment. */
export function candidatePurchases(state: FakeState): FakeTransaction[] {
  const subscribed = new Set(state.subscriptions.map((subscription) => subscription.name));
  const seen = new Set<string>();
  const candidates: FakeTransaction[] = [];
  for (const purchase of state.transactions) {
    const isCandidate =
      purchase.kind === 'cartao' &&
      purchase.amount < 0 &&
      purchase.day.startsWith(state.currentMonth) &&
      !subscribed.has(nameOf(state, purchase)) &&
      !seen.has(purchase.establishmentKey);
    if (!isCandidate) continue;
    seen.add(purchase.establishmentKey);
    candidates.push(purchase);
  }
  return candidates.slice(0, CANDIDATES_SHOWN);
}

/** A new subscription from a card purchase: monthly, repeating on the purchase's day of the next month. */
export function subscriptionFrom(state: FakeState, purchase: FakeTransaction, reviewFrom: IsoDay): FakeSubscription {
  return {
    id: `sub-${purchase.id}`,
    name: nameOf(state, purchase),
    amount: Math.abs(purchase.amount),
    cycle: 'Mensal',
    nextCharge: addMonths(purchase.day, 1),
    line: purchase.line ?? 'Assinaturas',
    status: 'Analisar',
    reviewOn: addMonths(reviewFrom, REVIEW_AFTER_MONTHS),
    note: '',
    flags: [],
    foreign: purchase.originalAmount && purchase.iof !== null ? { originalAmount: purchase.originalAmount, iof: purchase.iof } : null,
    previousAmount: null,
    alternatives: [],
  };
}
