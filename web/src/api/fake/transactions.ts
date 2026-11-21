import type {
  BudgetLineOption,
  Transaction,
  TransactionDetail,
  TransactionFilterOptions,
  TransactionQuery,
  TransactionsApi,
  YearMonth,
} from '../types';
import { shiftMonth } from '@/lib/format';
import type { BudgetGroupKey, FakeState, FakeTransaction } from './model';
import { PROTOTYPE_CARD_ACCOUNT, PROTOTYPE_CARD_LABEL } from './prototype-data';
import { toContract } from './rules';
import type { ChangeLog } from './changes';
import { toDetail } from './transaction-detail';
import { createTransactionWrites } from './transaction-writes';

// A menu of budget lines lists where spending goes first and income last.
const LINE_MENU_ORDER: BudgetGroupKey[] = ['var', 'fix', 'res', 'prov', 'rec'];
const FEES_LINE = 'Tarifas e juros';

function inPeriod(query: TransactionQuery): (month: YearMonth) => boolean {
  if (query.period === 'year') return (month) => month.startsWith(query.month.slice(0, 4));
  if (query.period === 'last-3-months') {
    const from = shiftMonth(query.month, -2);
    return (month) => month >= from && month <= query.month;
  }
  return (month) => month === query.month;
}

export function newestFirst(a: FakeTransaction, b: FakeTransaction): number {
  return b.day.localeCompare(a.day) || a.sequence - b.sequence;
}

/** Fees have no group of their own in the budget, so the menu lists them after the provisions. */
export function lineOptions(state: FakeState): BudgetLineOption[] {
  return LINE_MENU_ORDER.flatMap((key) => {
    const group = state.budget.find((candidate) => candidate.key === key);
    const lines = (group?.lines ?? []).map((line) => ({ name: line.name, group: group?.name ?? '' }));
    return key === 'prov' ? [...lines, { name: FEES_LINE, group: FEES_LINE }] : lines;
  });
}

export function createFakeTransactions(state: FakeState, log: ChangeLog): TransactionsApi {
  return {
    ...createTransactionWrites(state, log),
    async list(query: TransactionQuery): Promise<Transaction[]> {
      const matches = inPeriod(query);
      return state.transactions
        .filter((transaction) => matches(transaction.day.slice(0, 7)))
        .sort(newestFirst)
        .map((transaction) => toContract(state, transaction));
    },
    async detail(id: string): Promise<TransactionDetail | null> {
      const found = state.transactions.find((transaction) => transaction.id === id);
      return found ? toDetail(state, found) : null;
    },
    async filterOptions(): Promise<TransactionFilterOptions> {
      const banks = state.accounts.filter((account) => account.kind === 'Conta').map((account) => account.bank);
      const card = state.accounts.some((account) => account.kind === 'Cartão')
        ? [{ value: PROTOTYPE_CARD_ACCOUNT, label: PROTOTYPE_CARD_LABEL }]
        : [];
      return {
        accounts: [...banks.map((bank) => ({ value: bank, label: bank })), ...card],
        tags: [...state.tags],
        lines: lineOptions(state),
      };
    },
  };
}
