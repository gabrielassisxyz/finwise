import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

const NOVEMBER = { month: '2026-11', period: 'month' } as const;

async function setup() {
  const api = createFakeApi();
  const byId = async (id: string) => (await api.transactions.list({ month: '2026-11', period: 'last-3-months' })).find((t) => t.id === id)!;
  return { api, byId };
}

describe('changing a line', () => {
  it('classifies the transaction as the user, and clears its confidence and review', async () => {
    const { api, byId } = await setup();
    const result = await api.transactions.setLine('t5', 'Delivery');

    expect(result).toMatchObject({ establishment: { key: 'Bar do Zé', name: 'Bar do Zé' } });
    expect(await byId('t5')).toMatchObject({ line: 'Delivery', classifiedBy: 'Você', confidence: null, pending: null });

    await api.transactions.setLine('t4', 'Casa');
    expect(await byId('t4')).toMatchObject({ line: 'Casa', classifiedBy: 'Você', confidence: null, belowThreshold: false });
  });

  it('does nothing when the line is the same and no review was waiting', async () => {
    const { api } = await setup();
    expect(await api.transactions.setLine('t2', 'Transporte')).toBeNull();
    expect(await api.transactions.setLine('missing', 'Transporte')).toBeNull();
  });

  it('still settles a pending review when the line is the one already set', async () => {
    const state = prototypeState();
    state.transactions.find((transaction) => transaction.id === 't5')!.line = 'Restaurantes e bares';
    const api = createFakeApi({ state });

    expect(await api.transactions.setLine('t5', 'Restaurantes e bares')).not.toBeNull();
    expect((await api.shell.overview()).pendingCount).toBe(6);
    expect(await api.transactions.setLine('t5', 'Restaurantes e bares')).toBeNull();
  });

  it('does not offer the establishment for a person or a movement', async () => {
    const { api } = await setup();
    expect(await api.transactions.setLine('t34', 'Casa')).toMatchObject({ establishment: null });
    expect(await api.transactions.setLine('t52', 'Casa')).toMatchObject({ establishment: null });
  });

  it('lowers the pending count and moves the month limit', async () => {
    const { api } = await setup();
    const before = await api.home.summary('2026-11');
    expect((await api.shell.overview()).pendingCount).toBe(7);

    await api.transactions.setLine('t5', 'Restaurantes e bares');
    const after = await api.home.summary('2026-11');
    expect((await api.shell.overview()).pendingCount).toBe(6);
    expect(after.pendingCount).toBe(6);
    expect(after.limit.spent).toBe(before.limit.spent + 64);
  });
});

describe('applying a line to the whole establishment', () => {
  it('counts nothing when every other purchase is already on the line', async () => {
    const { api, byId } = await setup();
    const change = await api.transactions.setLine('t15', 'Delivery');
    const outcome = await api.transactions.applyLineToEstablishment(change!.changeId);

    expect(outcome.changed).toBe(0);
    expect(await byId('t15')).toMatchObject({ classifiedBy: 'Você' });
  });

  it('moves the others as the rule, clearing their review, and leaves those already there as they were', async () => {
    const state = prototypeState();
    state.transactions.find((transaction) => transaction.id === 't13')!.pending = state.transactions.find((transaction) => transaction.id === 't5')!.pending;
    const api = createFakeApi({ state });
    const byId = async (id: string) => (await api.transactions.list({ month: '2026-11', period: 'last-3-months' })).find((t) => t.id === id)!;

    await api.transactions.setLine('t22', 'Casa');
    const change = await api.transactions.setLine('t9', 'Casa');
    const outcome = await api.transactions.applyLineToEstablishment(change!.changeId);

    expect(outcome.changed).toBe(5);
    expect(await byId('t22')).toMatchObject({ line: 'Casa', classifiedBy: 'Você' });
    expect(await byId('t13')).toMatchObject({ line: 'Casa', classifiedBy: 'Regra', confidence: null, pending: null });
    expect(await byId('t4')).toMatchObject({ line: 'Casa', classifiedBy: 'Regra', confidence: null, belowThreshold: false });
    expect(await byId('t9')).toMatchObject({ classifiedBy: 'Você' });
    // Seven were waiting, t13 made eight, and applying the line settled it.
    expect((await api.shell.overview()).pendingCount).toBe(7);
  });

  it('refuses a change that was not a line change', async () => {
    const { api } = await setup();
    const bulk = await api.transactions.setLineForMany(['t3'], 'Lazer');
    await expect(api.transactions.applyLineToEstablishment(bulk.changeId)).rejects.toThrow();
  });
});

describe('changing many at once', () => {
  it('sets the line on each as the user and clears what was pending', async () => {
    const { api, byId } = await setup();
    await api.transactions.setLineForMany(['t5', 't4'], 'Lazer');
    expect(await byId('t5')).toMatchObject({ line: 'Lazer', classifiedBy: 'Você', pending: null });
    expect(await byId('t4')).toMatchObject({ line: 'Lazer', classifiedBy: 'Você', confidence: null });
    expect(await byId('t3')).toMatchObject({ line: 'Delivery', classifiedBy: 'Regra' });
  });

  it('adds a tag unless it is already there', async () => {
    const { api, byId } = await setup();
    await api.transactions.addTagToMany(['t16', 't3'], '#viagem');
    expect((await byId('t16')).tags).toEqual(['#viagem']);
    expect((await byId('t3')).tags).toEqual(['#viagem']);
  });

  it('marks as not counting, and takes them out of the month limit', async () => {
    const { api, byId } = await setup();
    const before = await api.home.summary('2026-11');
    await api.transactions.excludeMany(['t3'], 'Marcada em lote');
    expect(await byId('t3')).toMatchObject({ excluded: true, countsInTotals: false });
    expect((await api.home.summary('2026-11')).limit.spent).toBe(before.limit.spent - 54.9);
  });
});

describe('undo', () => {
  it('restores a single change, in the list and in the totals', async () => {
    const { api, byId } = await setup();
    const before = await api.home.summary('2026-11');
    const change = await api.transactions.setLine('t5', 'Restaurantes e bares');
    await api.changes.undo(change!.changeId);

    expect(await byId('t5')).toMatchObject({ line: null, classifiedBy: 'Regra' });
    expect((await byId('t5')).pending).not.toBeNull();
    expect(await api.home.summary('2026-11')).toEqual(before);
    expect((await api.shell.overview()).pendingCount).toBe(7);
  });

  it('takes back the establishment-wide change together with the one it extended', async () => {
    const { api, byId } = await setup();
    const untouched = await api.transactions.list(NOVEMBER);
    const change = await api.transactions.setLine('t3', 'Lazer');
    const outcome = await api.transactions.applyLineToEstablishment(change!.changeId);
    expect(outcome.changed).toBe(2);
    await api.changes.undo(change!.changeId);

    expect(await api.transactions.list(NOVEMBER)).toEqual(untouched);
    expect((await byId('t15')).line).toBe('Restaurantes e bares');
  });

  it('takes back a bulk change as one', async () => {
    const { api } = await setup();
    const untouched = await api.transactions.list(NOVEMBER);
    const change = await api.transactions.excludeMany(['t3', 't4', 't5'], 'Marcada em lote');
    await api.changes.undo(change.changeId);
    expect(await api.transactions.list(NOVEMBER)).toEqual(untouched);
  });

  it('refuses a change it does not know, or one already undone', async () => {
    const { api } = await setup();
    const change = await api.transactions.addTagToMany(['t3'], '#x');
    await api.changes.undo(change.changeId);
    await expect(api.changes.undo(change.changeId)).rejects.toThrow();
    await expect(api.changes.undo('nope')).rejects.toThrow();
  });
});
