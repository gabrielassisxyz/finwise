import { describe, expect, it } from 'vitest';
import type { SubscriptionCycle } from '../types';
import { createFakeApi } from './index';
import type { FakeSubscription } from './model';
import { prototypeState } from './prototype-data';
import { addMonths } from './subscription-rules';

function subscription(id: string, details: Partial<FakeSubscription> = {}): FakeSubscription {
  return {
    id,
    name: id,
    amount: 120,
    cycle: 'Mensal',
    nextCharge: '2027-06-10',
    line: 'Assinaturas',
    status: 'Manter',
    reviewOn: '2027-09-01',
    note: '',
    flags: [],
    foreign: null,
    previousAmount: null,
    alternatives: [],
    ...details,
  };
}

/** The prototype's dataset, with the subscriptions replaced by the ones a test names. */
function apiWith(subscriptions: FakeSubscription[]) {
  const state = prototypeState();
  state.subscriptions = subscriptions;
  return createFakeApi({ state });
}

const list = async (api = createFakeApi(), grouped = false) => (await api.subscriptions.list({ grouped })).groups.flatMap((g) => g.subscriptions);
const rowOf = async (id: string, api = createFakeApi()) => (await list(api)).find((row) => row.id === id)!;

describe('the monthly equivalent', () => {
  it.each<[SubscriptionCycle, number]>([
    ['Mensal', 120],
    ['Trimestral', 40],
    ['Semestral', 20],
    ['Anual', 10],
    ['Bienal', 5],
  ])('spreads a %s charge over the months of its cycle', async (cycle, expected) => {
    const api = apiWith([subscription('x', { cycle })]);
    expect((await rowOf('x', api)).monthlyEquivalent).toBe(expected);
    expect((await api.subscriptions.detail('x'))?.annualEquivalent).toBe(expected * 12);
  });

  it('rounds to the cent', async () => {
    expect((await rowOf('do')).monthlyEquivalent).toBe(6.67);
  });
});

describe('the totals', () => {
  it('adds the equivalents as shown, and the year is twelve times the month', async () => {
    const result = await createFakeApi().subscriptions.list({ grouped: false });
    expect(result.monthlyTotal).toBe(202.37);
    expect(result.annualTotal).toBe(2428.44);
  });

  it('follows a new subscription', async () => {
    const api = createFakeApi();
    await api.subscriptions.create('t5');
    expect((await api.subscriptions.list({ grouped: false })).monthlyTotal).toBe(266.37);
  });
});

describe('the review date', () => {
  it('is overdue on the day it arrives and after, not before', async () => {
    const api = apiWith([
      subscription('before', { reviewOn: '2026-11-27' }),
      subscription('today', { reviewOn: '2026-11-28' }),
      subscription('after', { reviewOn: '2026-11-29' }),
    ]);
    const flags = (await list(api)).map((row) => row.reviewOverdue);
    expect(flags).toEqual([true, true, false]);
  });
});

describe('the marks', () => {
  it('flags a price that rose and an overlap, amber, in that order', async () => {
    expect((await rowOf('sp')).marks).toEqual([{ text: 'Preço subiu', tone: 'warning' }]);
    expect((await rowOf('ai')).marks).toEqual([{ text: 'Sobreposta', tone: 'warning' }]);
  });

  it('does not flag a price that fell', async () => {
    const api = apiWith([subscription('x', { previousAmount: 150 })]);
    expect((await rowOf('x', api)).marks).toEqual([]);
  });

  it('warns of a yearly renewal within thirty days, counting the days', async () => {
    expect((await rowOf('ap')).marks).toEqual([{ text: 'Renova em 9 dias', tone: 'warning' }]);
  });

  it.each([
    ['2026-11-28', 'Renova hoje'],
    ['2026-11-29', 'Renova em 1 dia'],
    ['2026-12-28', 'Renova em 30 dias'],
  ])('writes a renewal on %s as "%s"', async (nextCharge, text) => {
    const api = apiWith([subscription('x', { cycle: 'Anual', nextCharge })]);
    expect((await rowOf('x', api)).marks).toEqual([{ text, tone: 'warning' }]);
  });

  it.each([
    ['a renewal beyond thirty days', { cycle: 'Anual', nextCharge: '2026-12-29' }],
    ['a renewal already past', { cycle: 'Anual', nextCharge: '2026-11-27' }],
    ['a cycle shorter than a year', { cycle: 'Semestral', nextCharge: '2026-12-01' }],
  ] as const)('does not warn for %s', async (_label, details) => {
    const api = apiWith([subscription('x', details)]);
    expect((await rowOf('x', api)).marks).toEqual([]);
  });

  it('marks a charge after cancelling in red', async () => {
    const api = apiWith([subscription('x', { flags: ['charged-after-cancel'] })]);
    expect((await rowOf('x', api)).marks).toEqual([{ text: 'Cobrou após cancelar', tone: 'danger' }]);
  });
});

describe('grouping by status', () => {
  it('lists one group per status that has members, in the design order, each with its total', async () => {
    const { groups } = await createFakeApi().subscriptions.list({ grouped: true });
    expect(groups.map((g) => [g.status, g.subscriptions.length, g.monthlyTotal])).toEqual([
      ['Manter', 4, 158.57],
      ['Analisar', 2, 43.8],
    ]);
  });

  it('is one group without a status when not grouped, in the order added', async () => {
    const { groups } = await createFakeApi().subscriptions.list({ grouped: false });
    expect(groups).toHaveLength(1);
    expect(groups[0]?.status).toBeNull();
    expect(groups[0]?.subscriptions.map((row) => row.id)).toEqual(['ai', 'sp', 'ap', 'ho', 'rv', 'do']);
  });
});

describe('the charge history', () => {
  it('goes back one cycle at a time, newest first', async () => {
    const detail = (await createFakeApi().subscriptions.detail('ho'))!;
    expect(detail.charges.map((charge) => charge.day)).toEqual(['2026-10-10', '2026-07-10', '2026-04-10']);
  });

  it('charges the price before a rise for the older ones', async () => {
    const detail = (await createFakeApi().subscriptions.detail('sp'))!;
    expect(detail.charges.map((charge) => charge.amount)).toEqual([21.9, 19.9, 19.9]);
  });

  it('carries the foreign currency and the IOF on every charge of a foreign subscription only', async () => {
    const foreign = (await createFakeApi().subscriptions.detail('ho'))!;
    expect(foreign.charges.every((charge) => charge.foreign?.originalAmount === 'US$ 10,50' && charge.foreign.iof === 2.03)).toBe(true);
    const domestic = (await createFakeApi().subscriptions.detail('sp'))!;
    expect(domestic.charges.every((charge) => charge.foreign === null)).toBe(true);
  });

  it('keeps the day inside a shorter month', () => {
    expect(addMonths('2027-03-31', -1)).toBe('2027-02-28');
    expect(addMonths('2027-12-31', 2)).toBe('2028-02-29');
  });

  it('gives no alternatives for a subscription the model has not looked at, and null for an unknown id', async () => {
    expect((await createFakeApi().subscriptions.detail('do'))?.alternatives).toEqual([]);
    expect((await createFakeApi().subscriptions.detail('sp'))?.alternatives).toHaveLength(2);
    expect(await createFakeApi().subscriptions.detail('missing')).toBeNull();
  });
});

describe('recurring purchases', () => {
  it('computes the next expected day from the last one and the cadence', async () => {
    const recurring = await createFakeApi().subscriptions.recurring();
    expect(recurring.map((purchase) => [purchase.name, purchase.nextExpected])).toEqual([
      ['Areia do gato', '2026-11-29'],
      ['Filtro de água', '2026-11-14'],
      ['Remédio de uso contínuo', '2026-11-26'],
      ['Ração', '2026-12-07'],
    ]);
  });

  it('is due once the next day has arrived, today included', async () => {
    const state = prototypeState();
    state.recurringPurchases = [
      { name: 'ontem', cadence: { every: 1, unit: 'days' }, lastPurchase: '2026-11-26', amount: 1 },
      { name: 'hoje', cadence: { every: 1, unit: 'days' }, lastPurchase: '2026-11-27', amount: 1 },
      { name: 'amanhã', cadence: { every: 2, unit: 'days' }, lastPurchase: '2026-11-27', amount: 1 },
    ];
    const recurring = await createFakeApi({ state }).subscriptions.recurring();
    expect(recurring.map((purchase) => purchase.due)).toEqual([true, true, false]);
  });
});

describe('purchases a subscription can be created from', () => {
  it('lists this month card purchases once per establishment, up to twelve', async () => {
    const candidates = await createFakeApi().subscriptions.candidates();
    expect(candidates).toHaveLength(12);
    const names = candidates.map((candidate) => candidate.name);
    expect(new Set(names).size).toBe(12);
    expect(names.slice(0, 4)).toEqual(['Cinema Belas Artes', 'Posto Shell', 'iFood', 'Pão de Açúcar']);
    expect(candidates[0]?.amount).toBe(65.1);
  });

  it('leaves out what is already a subscription, a credit, and anything that is not a card purchase', async () => {
    const names = (await createFakeApi().subscriptions.candidates()).map((candidate) => candidate.name);
    expect(names).not.toContain('Spotify');
    expect(names).not.toContain('Assistente de IA');
    expect(names).not.toContain('Estorno de anuidade');
    expect(names).not.toContain('Padaria Estrela');
  });

  it('leaves out last month', async () => {
    const state = prototypeState();
    state.transactions = state.transactions.filter((purchase) => purchase.kind === 'cartao');
    const api = createFakeApi({ state });
    const before = (await api.subscriptions.candidates()).length;
    state.transactions.forEach((purchase) => {
      purchase.day = purchase.day.replace('2026-11', '2026-10');
    });
    expect(before).toBeGreaterThan(0);
    expect(await api.subscriptions.candidates()).toEqual([]);
  });
});

describe('creating a subscription from a purchase', () => {
  it('makes it monthly, to analyse, from the purchase, due on its day of the next month', async () => {
    const api = createFakeApi();
    const created = await api.subscriptions.create('t5');
    expect(created).toMatchObject({ name: 'Bar do Zé' });
    expect(await rowOf('sub-t5', api)).toMatchObject({
      name: 'Bar do Zé',
      amount: 64,
      cycle: 'Mensal',
      nextCharge: '2026-12-27',
      status: 'Analisar',
      line: 'Assinaturas',
      marks: [],
    });
  });

  it('is reviewed three months from today', async () => {
    const api = createFakeApi();
    await api.subscriptions.create('t5');
    expect((await rowOf('sub-t5', api)).reviewOn).toBe('2027-02-28');
  });

  it('keeps the line and the foreign amount of the purchase', async () => {
    const api = createFakeApi();
    await api.subscriptions.create('t7');
    expect(await rowOf('sub-t7', api)).toMatchObject({ line: 'Ferramentas de trabalho' });
    expect((await api.subscriptions.detail('sub-t7'))?.charges[0]?.foreign).toEqual({ originalAmount: 'US$ 20,00', iof: 3.85 });
  });

  it('refuses a purchase that was not offered, and the same one twice', async () => {
    const api = createFakeApi();
    expect(await api.subscriptions.create('missing')).toBeNull();
    expect(await api.subscriptions.create('t8')).toBeNull();
    expect(await api.subscriptions.create('t5')).not.toBeNull();
    expect(await api.subscriptions.create('t5')).toBeNull();
    expect(await list(api)).toHaveLength(7);
  });

  it('takes the purchase out of the offered ones', async () => {
    const api = createFakeApi();
    await api.subscriptions.create('t5');
    expect((await api.subscriptions.candidates()).map((candidate) => candidate.name)).not.toContain('Bar do Zé');
  });
});

describe('the suggestion', () => {
  it('names the purchase that looks like a subscription', async () => {
    expect((await createFakeApi().subscriptions.list({ grouped: false })).suggestion).toEqual({ name: 'Serviço de nuvem' });
  });

  it('accepting makes the subscription, reviewed three months after the purchase, and closes the suggestion', async () => {
    const api = createFakeApi();
    expect(await api.subscriptions.acceptSuggestion()).toMatchObject({ name: 'Serviço de nuvem' });
    expect(await rowOf('sub-t7', api)).toMatchObject({ amount: 113.85, nextCharge: '2026-12-24', reviewOn: '2027-02-24', status: 'Analisar' });
    expect((await api.subscriptions.list({ grouped: false })).suggestion).toBeNull();
    expect(await api.subscriptions.acceptSuggestion()).toBeNull();
  });

  it('rejecting closes it without making a subscription, and it cannot be accepted afterwards', async () => {
    const api = createFakeApi();
    expect(await api.subscriptions.rejectSuggestion()).not.toBeNull();
    expect((await api.subscriptions.list({ grouped: false })).suggestion).toBeNull();
    expect(await list(api)).toHaveLength(6);
    expect(await api.subscriptions.acceptSuggestion()).toBeNull();
    expect(await api.subscriptions.rejectSuggestion()).toBeNull();
  });

  it('is closed by creating the same purchase from the menu', async () => {
    const api = createFakeApi();
    await api.subscriptions.create('t7');
    expect((await api.subscriptions.list({ grouped: false })).suggestion).toBeNull();
  });
});

describe('editing a subscription', () => {
  it.each([
    ['status', (api: ReturnType<typeof createFakeApi>) => api.subscriptions.setStatus('sp', 'Cancelar'), 'status', 'Cancelar', 'Manter'],
    ['line', (api: ReturnType<typeof createFakeApi>) => api.subscriptions.setLine('sp', 'Lazer'), 'line', 'Lazer', 'Assinaturas'],
    ['review date', (api: ReturnType<typeof createFakeApi>) => api.subscriptions.setReviewDate('sp', '2026-11-01'), 'reviewOn', '2026-11-01', '2027-01-15'],
    ['note', (api: ReturnType<typeof createFakeApi>) => api.subscriptions.setNote('sp', 'Cancelar em janeiro'), 'note', 'Cancelar em janeiro', ''],
  ] as const)('changes the %s and takes it back', async (_label, write, field, after, before) => {
    const api = createFakeApi();
    const { changeId } = await write(api);
    expect((await api.subscriptions.detail('sp'))?.[field]).toBe(after);
    await api.changes.undo(changeId);
    expect((await api.subscriptions.detail('sp'))?.[field]).toBe(before);
  });

  it('turns an overdue review date into a pending one when moved forward', async () => {
    const api = createFakeApi();
    expect((await rowOf('rv', api)).reviewOverdue).toBe(true);
    await api.subscriptions.setReviewDate('rv', '2027-02-01');
    expect((await rowOf('rv', api)).reviewOverdue).toBe(false);
  });

  it('takes back a creation, an accepted suggestion and a rejected one', async () => {
    const api = createFakeApi();
    const created = (await api.subscriptions.create('t5'))!;
    await api.changes.undo(created.changeId);
    expect(await list(api)).toHaveLength(6);

    const accepted = (await api.subscriptions.acceptSuggestion())!;
    await api.changes.undo(accepted.changeId);
    expect(await list(api)).toHaveLength(6);
    expect((await api.subscriptions.list({ grouped: false })).suggestion).not.toBeNull();

    const rejected = (await api.subscriptions.rejectSuggestion())!;
    await api.changes.undo(rejected.changeId);
    expect((await api.subscriptions.list({ grouped: false })).suggestion).not.toBeNull();
  });
});
