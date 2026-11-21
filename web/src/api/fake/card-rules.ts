import type { CardBill, CardOverview, CardPurchase, IsoDay, YearMonth } from '../types';
import type { FakeCardBill, FakeState, FakeTransaction } from './model';
import { PROTOTYPE_CARD_ACCOUNT, PROTOTYPE_CARD_FULL_LABEL } from './prototype-data';

// The card's rules: when the bill closes, what falls after it, what is left
// of the limit. The server will own them; the stand-in applies the same ones.

/** Purchases this close to the closing may be posted to either bill. */
const UNCERTAIN_DAYS_BEFORE_CLOSING = 2;

const cents = (amount: number): number => Math.round(amount * 100) / 100;

const pad = (value: number): string => String(value).padStart(2, '0');

const dayOf = (month: YearMonth, day: number): IsoDay => `${month}-${pad(day)}`;

function nextMonth(month: YearMonth): YearMonth {
  const [year, number] = month.split('-').map(Number) as [number, number];
  return number === 12 ? `${year + 1}-01` : `${year}-${pad(number + 1)}`;
}

function shiftDays(day: IsoDay, delta: number): IsoDay {
  const shifted = new Date(Date.parse(`${day}T00:00:00Z`) + delta * 86_400_000);
  return shifted.toISOString().slice(0, 10);
}

/** The day of the month the bill closes on: the user's when set to manual, the bank's otherwise. */
export function closingDayOf(state: FakeState): number {
  return state.settings.closingMode === 'manual' ? state.settings.manualClosingDay : state.card.bankClosingDay;
}

/** The next closing on or after today. */
export function nextClosing(state: FakeState): IsoDay {
  const today = state.shell.today;
  const thisMonth = dayOf(today.slice(0, 7), closingDayOf(state));
  return thisMonth >= today ? thisMonth : dayOf(nextMonth(today.slice(0, 7)), closingDayOf(state));
}

/** The bill closing on `closing` falls due on the bank's due day of the following month. */
export function dueDateFor(state: FakeState, closing: IsoDay): IsoDay {
  return dayOf(nextMonth(closing.slice(0, 7)), state.card.dueDay);
}

function cardPurchases(state: FakeState): FakeTransaction[] {
  return state.transactions
    .filter((transaction) => transaction.account === PROTOTYPE_CARD_ACCOUNT && transaction.kind === 'cartao' && transaction.amount < 0)
    .sort((a, b) => b.day.localeCompare(a.day) || a.sequence - b.sequence);
}

function toPurchase(state: FakeState, transaction: FakeTransaction): CardPurchase {
  return {
    transactionId: transaction.id,
    name: state.establishments[transaction.establishmentKey]?.nickname ?? transaction.name,
    day: transaction.day,
    amount: transaction.amount,
  };
}

/**
 * What was bought on the card since this month's closing: the bank bills it
 * with the next bill, but the month's spending already includes it. Before
 * the closing day there is nothing yet.
 */
export function payAheadOf(state: FakeState): { amount: number; purchases: CardPurchase[] } {
  const closing = dayOf(state.currentMonth, closingDayOf(state));
  const purchases =
    state.shell.today < closing
      ? []
      : cardPurchases(state)
          .filter((transaction) => transaction.day.startsWith(state.currentMonth) && transaction.day >= closing)
          .map((transaction) => toPurchase(state, transaction));
  // `0 - sum` rather than `-sum`: an empty list must give 0, not negative zero.
  return { amount: cents(0 - purchases.reduce((total, purchase) => total + purchase.amount, 0)), purchases };
}

function mayFallOnNextBill(state: FakeState, closing: IsoDay): CardPurchase[] {
  const from = shiftDays(closing, -UNCERTAIN_DAYS_BEFORE_CLOSING);
  return cardPurchases(state)
    .filter((transaction) => transaction.day >= from && transaction.day < closing)
    .map((transaction) => toPurchase(state, transaction));
}

function toBill(bill: FakeCardBill): CardBill {
  return {
    month: bill.month,
    closedOn: bill.closedOn,
    dueOn: bill.dueOn,
    total: cents(bill.items.reduce((total, item) => total + item.amount, 0)),
    items: structuredClone(bill.items),
  };
}

export function cardOverview(state: FakeState): CardOverview {
  const account = state.accounts.find((candidate) => candidate.kind === 'Cartão');
  const openBill = account?.balance ?? 0;
  const closesOn = nextClosing(state);
  const payAhead = payAheadOf(state);
  return {
    title: PROTOTYPE_CARD_FULL_LABEL,
    openBill,
    closesOn,
    dueOn: dueDateFor(state, closesOn),
    bestPurchaseDay: closesOn,
    limit: state.card.limit,
    // Purchases already made still use the limit, whichever bill they land on.
    availableLimit: cents(state.card.limit - openBill - payAhead.amount),
    payAhead,
    mayFallOnNextBill: mayFallOnNextBill(state, closesOn),
    bills: state.card.bills.map(toBill),
  };
}
