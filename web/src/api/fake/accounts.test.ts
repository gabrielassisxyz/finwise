import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

describe('accounts list', () => {
  it('lists every account with what its row shows: the balance, or the open bill of the card', async () => {
    const rows = await createFakeApi().accounts.list();
    expect(rows.map(({ id, kind, amount }) => [id, kind, amount])).toEqual([
      ['itau', 'Conta', 7020],
      ['mp', 'Conta', 312.4],
      ['c6', 'Conta', 1180.25],
      ['nu', 'Conta', 96.1],
      ['card', 'Cartão', 2310],
    ]);
  });

  it('marks only the account the banks have sent nothing for as idle', async () => {
    const rows = await createFakeApi().accounts.list();
    expect(rows.filter((row) => row.idle).map((row) => row.id)).toEqual(['nu']);
  });

  it('turns an account idle once its last movement is more than thirty days old, and not before', async () => {
    const state = prototypeState();
    const api = createFakeApi({ state });
    // The C6 account's last movement is on 2026-11-16.
    state.shell.today = '2026-12-16';
    expect((await api.accounts.list()).find((row) => row.id === 'c6')!.idle).toBe(false);
    state.shell.today = '2026-12-17';
    expect((await api.accounts.list()).find((row) => row.id === 'c6')!.idle).toBe(true);
  });

  it('wakes an idle account when a movement arrives for it', async () => {
    const state = prototypeState();
    const api = createFakeApi({ state });
    const moved = { ...state.transactions.find((transaction) => transaction.account === 'C6')!, account: 'Nubank' };
    state.transactions.push(moved);
    expect((await api.accounts.list()).find((row) => row.id === 'nu')!.idle).toBe(false);
  });
});

describe('account detail', () => {
  it('names the ledger account the movements are filed under and the bucket it counts in', async () => {
    const detail = await createFakeApi().accounts.detail('itau');
    expect(detail).toEqual({
      id: 'itau',
      bank: 'Itaú',
      name: 'Conta corrente',
      ledgerAccount: 'Itaú',
      bucket: 'Disponível',
      balance: 7020,
    });
  });

  it('has nothing for the card, which has a page of its own, or for an unknown id', async () => {
    const api = createFakeApi();
    expect(await api.accounts.detail('card')).toBeNull();
    expect(await api.accounts.detail('nope')).toBeNull();
  });
});
