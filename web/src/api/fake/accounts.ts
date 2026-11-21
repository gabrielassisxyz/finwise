import type { AccountDetail, AccountRow, AccountsApi, IsoDay } from '../types';
import type { FakeAccount, FakeState } from './model';
import { PROTOTYPE_CARD_ACCOUNT } from './prototype-data';

// An account with no movement for this long is shown faded, with a note.
const IDLE_AFTER_DAYS = 30;
const MS_PER_DAY = 86_400_000;

export function daysBetween(from: IsoDay, to: IsoDay): number {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / MS_PER_DAY);
}

/** What `Transaction.account` carries for the account's movements. */
export function ledgerAccountOf(account: FakeAccount): string {
  return account.kind === 'Cartão' ? PROTOTYPE_CARD_ACCOUNT : account.bank;
}

export function isIdle(state: FakeState, account: FakeAccount): boolean {
  const ledger = ledgerAccountOf(account);
  const latest = state.transactions
    .filter((transaction) => transaction.account === ledger)
    .reduce<IsoDay | null>((newest, transaction) => (newest && newest > transaction.day ? newest : transaction.day), null);
  return latest === null || daysBetween(latest, state.shell.today) > IDLE_AFTER_DAYS;
}

function toRow(state: FakeState, account: FakeAccount): AccountRow {
  return {
    id: account.id,
    bank: account.bank,
    name: account.name,
    kind: account.kind,
    amount: account.balance,
    idle: isIdle(state, account),
  };
}

export function createFakeAccounts(state: FakeState): AccountsApi {
  return {
    async list(): Promise<AccountRow[]> {
      return state.accounts.map((account) => toRow(state, account));
    },
    async detail(id: string): Promise<AccountDetail | null> {
      const account = state.accounts.find((candidate) => candidate.id === id && candidate.kind === 'Conta');
      if (!account) return null;
      return {
        id: account.id,
        bank: account.bank,
        name: account.name,
        ledgerAccount: ledgerAccountOf(account),
        bucket: 'Disponível',
        balance: account.balance,
      };
    },
  };
}
