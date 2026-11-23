import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

const MONTH = '2026-11';

describe('the payday view', () => {
  it('sets the cushion (fixed plus variable lines) against the balance of the salary account', async () => {
    const view = await createFakeApi().review.payday(MONTH);
    expect(view).toMatchObject({
      cushion: 6780,
      balance: 7020,
      balanceLabel: 'Itaú · Conta corrente',
      difference: 240,
      covered: true,
    });
  });

  it('says the cushion is not covered when the balance falls short, with the shortfall negative', async () => {
    const state = prototypeState();
    const itau = state.accounts.find((account) => account.id === 'itau')!;
    itau.balance = 6000;
    const view = await createFakeApi({ state }).review.payday(MONTH);
    expect(view.difference).toBe(-780);
    expect(view.covered).toBe(false);
  });

  it('counts a balance equal to the cushion as covered', async () => {
    const state = prototypeState();
    state.accounts.find((account) => account.id === 'itau')!.balance = 6780;
    const view = await createFakeApi({ state }).review.payday(MONTH);
    expect(view).toMatchObject({ difference: 0, covered: true });
  });

  it('follows the budget: raising a cap raises the cushion', async () => {
    const api = createFakeApi();
    await api.review.approve('teto');
    expect((await api.review.payday(MONTH)).cushion).toBe(6830);
  });

  it('lists the month transfers with their states', async () => {
    const { transfers } = await createFakeApi().review.payday(MONTH);
    expect(transfers.map((transfer) => [transfer.name, transfer.amount, transfer.state, transfer.doneOn])).toEqual([
      ['RF Casa', 0, 'on-target', null],
      ['Gás e IPTU', 120, 'done', '2026-11-05'],
      ['Assinaturas anuais', 50, 'done', '2026-11-05'],
      ['Remédios', 150, 'done', '2026-11-05'],
      ['Compras', 500, 'pending', null],
      ['Viagens', 600, 'pending', null],
      ['Investimento', 3800, 'pending', null],
    ]);
    expect(transfers.every((transfer) => !transfer.markedByHand)).toBe(true);
  });

  it('gives every transfer a key of its own that stays the same from one call to the next', async () => {
    const api = createFakeApi();
    const keys = (await api.review.payday(MONTH)).transfers.map((transfer) => transfer.key);
    expect(keys.every((key) => key !== '')).toBe(true);
    expect(new Set(keys).size).toBe(keys.length);
    expect((await api.review.payday('2026-10')).transfers.map((transfer) => transfer.key)).toEqual(keys);
  });
});

describe('marking a transfer as done', () => {
  it('dates it today and says it was marked by hand', async () => {
    const api = createFakeApi();
    await api.review.markTransferDone(MONTH, 'line:compras');
    const compras = (await api.review.payday(MONTH)).transfers.find((transfer) => transfer.name === 'Compras');
    expect(compras).toMatchObject({ state: 'done', doneOn: '2026-11-28', markedByHand: true });
  });

  it('shows on the savings pot the transfer belongs to', async () => {
    const api = createFakeApi();
    const contribution = async () => (await api.savingsPots.detail('com'))?.contribution;
    const before = await contribution();
    await api.review.markTransferDone(MONTH, 'line:compras');
    const after = await contribution();
    expect(before).toEqual({ state: 'pending' });
    expect(after).toEqual({ state: 'done', doneOn: '2026-11-28' });
  });

  it('is taken back by its change', async () => {
    const api = createFakeApi();
    const marked = await api.review.markTransferDone(MONTH, 'line:viagens');
    await api.changes.undo(marked!.changeId);
    const viagens = (await api.review.payday(MONTH)).transfers.find((transfer) => transfer.name === 'Viagens');
    expect(viagens).toMatchObject({ state: 'pending', doneOn: null, markedByHand: false });
  });

  it('does nothing for a transfer that is done, on target or unknown', async () => {
    const api = createFakeApi();
    expect(await api.review.markTransferDone(MONTH, 'line:gas-e-iptu')).toBeNull();
    expect(await api.review.markTransferDone(MONTH, 'pot:rf-casa')).toBeNull();
    expect(await api.review.markTransferDone(MONTH, 'nada')).toBeNull();
  });

  it('finds the transfer by its key, never by its name', async () => {
    const api = createFakeApi();
    expect(await api.review.markTransferDone(MONTH, 'Compras')).toBeNull();
    expect((await api.review.payday(MONTH)).transfers.find((transfer) => transfer.key === 'line:compras')?.state).toBe('pending');
  });
});
