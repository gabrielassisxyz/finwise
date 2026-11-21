import type { NetWorthSummary, Transaction, YearMonth } from '../types';
import type { BudgetGroupKey, FakeBudgetGroup, FakeState, FakeTransaction } from './model';
import { PROTOTYPE_CARD_LABEL } from './prototype-data';

// The money rules. The server will own these; the stand-in applies the same
// ones so a screen shows the totals it will show against real data.

const cents = (amount: number): number => Math.round(amount * 100) / 100;

const sum = (amounts: number[]): number => amounts.reduce((total, amount) => total + amount, 0);

export function isExcluded(transaction: FakeTransaction): boolean {
  return !!transaction.excludedReason;
}

/** What the transaction counts for once its linked reimbursement is taken off. */
export function effectiveAmount(transaction: FakeTransaction): number {
  return transaction.amount + (transaction.reimbursement?.link?.amount ?? 0);
}

/**
 * The budget lines a transaction adds to, and by how much. Nothing for one
 * still waiting for review, a movement between own accounts or one marked as
 * not counting. A credit only counts when it is a refund pointed at a line,
 * which it then reduces.
 */
export function contributions(transaction: FakeTransaction): [line: string, amount: number][] {
  if (transaction.pending || transaction.movement || isExcluded(transaction)) return [];
  if (transaction.amount > 0) {
    const offsets = transaction.refund ? transaction.refundOffsetsLine : null;
    return offsets ? [[offsets, -transaction.amount]] : [];
  }
  if (transaction.split && transaction.split.length > 1) {
    return transaction.split.flatMap((part) => (part.line ? [[part.line, part.amount] as [string, number]] : []));
  }
  return transaction.line ? [[transaction.line, -effectiveAmount(transaction)]] : [];
}

/**
 * Spending per budget line in a month: the baseline, moved by whatever
 * changed since it was taken. Only the current month can move.
 */
export function spentByLine(state: FakeState, original: FakeTransaction[], month: YearMonth): Record<string, number> {
  const spent = { ...(state.spentBaseline[month] ?? {}) };
  if (month === state.currentMonth) {
    const inMonth = (transaction: FakeTransaction) => transaction.day.startsWith(month);
    const add = (transactions: FakeTransaction[], sign: 1 | -1) => {
      for (const transaction of transactions.filter(inMonth)) {
        for (const [line, amount] of contributions(transaction)) spent[line] = (spent[line] ?? 0) + sign * amount;
      }
    };
    add(original, -1);
    add(state.transactions, 1);
  }
  for (const line of Object.keys(spent)) spent[line] = cents(spent[line] ?? 0);
  return spent;
}

export function groupTotalOf(groups: FakeBudgetGroup[], key: BudgetGroupKey): number {
  const group = groups.find((candidate) => candidate.key === key);
  return sum((group?.lines ?? []).map((line) => line.amount));
}

export function groupTotal(state: FakeState, key: BudgetGroupKey): number {
  return groupTotalOf(state.budget, key);
}

/** Income minus every planned outgoing: what a budget sets aside to invest. */
export function plannedInvestmentOf(groups: FakeBudgetGroup[]): number {
  const out = (['fix', 'var', 'prov', 'res'] as const).reduce((total, key) => total + groupTotalOf(groups, key), 0);
  return groupTotalOf(groups, 'rec') - out;
}

export function percentOfIncome(income: number, amount: number): number {
  return income ? Math.round((amount / income) * 100) : 0;
}

export function netWorth(state: FakeState): NetWorthSummary {
  const inBucket = (bucket: string) =>
    sum(state.savingsPots.filter((pot) => pot.bucket === bucket).map((pot) => pot.balance));
  const inAccounts = sum(state.accounts.filter((account) => account.kind === 'Conta').map((account) => account.balance));
  return {
    available: cents(inAccounts + inBucket('Disponível')),
    reserved: cents(inBucket('Reservado')),
    invested: cents(inBucket('Investido')),
  };
}

export function pendingCount(state: FakeState): number {
  return state.transactions.filter((transaction) => transaction.pending).length;
}

function allTags(state: FakeState, transaction: FakeTransaction): string[] {
  const fixed = state.establishments[transaction.establishmentKey]?.tags ?? [];
  return [...new Set([...fixed, ...transaction.tags])];
}

/** The transaction as the contract describes it, with everything derived filled in. */
export function toContract(state: FakeState, transaction: FakeTransaction): Transaction {
  const isCard = transaction.kind === 'cartao';
  return {
    id: transaction.id,
    day: transaction.day,
    name: state.establishments[transaction.establishmentKey]?.nickname ?? transaction.name,
    originalName: transaction.name,
    rawDescription: transaction.raw,
    establishmentKey: transaction.establishmentKey,
    kind: transaction.kind,
    subtype: transaction.subtype,
    account: transaction.account,
    accountLabel: isCard ? PROTOTYPE_CARD_LABEL : transaction.account,
    amount: transaction.amount,
    effectiveAmount: cents(effectiveAmount(transaction)),
    countsInTotals: !transaction.movement && !isExcluded(transaction),
    line: transaction.line,
    classifiedBy: transaction.by,
    confidence: transaction.confidence,
    belowThreshold:
      transaction.by === 'IA' && transaction.confidence !== null && transaction.confidence < state.confidenceThreshold,
    pending: transaction.pending,
    posted: transaction.posted,
    tags: allTags(state, transaction),
    movement: transaction.movement,
    person: transaction.person,
    refund: transaction.refund,
    reimbursementIncome: transaction.reimbursementIncome,
    reimbursed: !!transaction.reimbursement?.link,
    excluded: isExcluded(transaction),
    split: !!transaction.split && transaction.split.length > 1,
    installment: transaction.installment,
    bill: transaction.bill,
    originalAmount: transaction.originalAmount,
    iof: transaction.iof,
  };
}
