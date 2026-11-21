import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';
import type { SavingsPotDetail, SavingsPotRow } from '../types';

const api = () => createFakeApi();

async function rowOf(id: string, includeClosed = true): Promise<SavingsPotRow> {
  const groups = await api().savingsPots.list({ includeClosed });
  return groups.flatMap((group) => group.pots).find((pot) => pot.id === id)!;
}

async function detailOf(id: string): Promise<SavingsPotDetail> {
  return (await api().savingsPots.detail(id))!;
}

describe('savings pots list', () => {
  it('groups by bucket in the design order and drops an empty group', async () => {
    const groups = await api().savingsPots.list({ includeClosed: false });
    expect(groups.map((group) => group.bucket)).toEqual(['Reservado', 'Investido', null]);
  });

  it('adds each balance into its group subtotal', async () => {
    const groups = await api().savingsPots.list({ includeClosed: false });
    expect(groups.map((group) => group.subtotal)).toEqual([34150, 37800, 500]);
  });

  it('hides a closed pot unless asked, and shows it among the others when asked', async () => {
    const hidden = await api().savingsPots.list({ includeClosed: false });
    const shown = await api().savingsPots.list({ includeClosed: true });
    const ids = (groups: typeof shown) => groups.flatMap((group) => group.pots.map((pot) => pot.id));
    expect(ids(hidden)).not.toContain('i0');
    expect(ids(shown)).toContain('i0');
    expect(shown[1]?.pots.map((pot) => pot.id)).toEqual(['i1', 'i2', 'i0']);
  });

  it('gives an unnamed pot a null name instead of inventing one', async () => {
    expect((await rowOf('sn')).name).toBeNull();
    expect((await rowOf('rfs')).name).toBe('RF Saúde');
  });

  it('earns the month yield from the balance and the share of the CDI', async () => {
    expect((await rowOf('rfs')).monthlyYield).toBe(25.2);
    expect((await rowOf('i1')).monthlyYield).toBe(301.88);
    expect((await rowOf('emg')).monthlyYield).toBe(189);
  });

  it('earns nothing in a closed pot, whatever its balance and rate', async () => {
    const state = prototypeState();
    const closed = state.savingsPots.find((pot) => pot.id === 'i0')!;
    closed.balance = 5000;
    const groups = await createFakeApi({ state }).savingsPots.list({ includeClosed: true });
    const row = groups.flatMap((group) => group.pots).find((pot) => pot.id === 'i0')!;
    expect(row.monthlyYield).toBe(0);
    expect(row.closed).toBe(true);
  });

  it('measures progress to the target and caps it once reached', async () => {
    expect((await rowOf('rfp')).target).toEqual({ amount: 5000, progress: 84 });
    expect((await rowOf('gas')).target).toBeNull();
  });

  it('caps the progress at 100 for a pot past its target', async () => {
    const state = prototypeState();
    state.savingsPots.find((pot) => pot.id === 'rfs')!.balance = 2500;
    const groups = await createFakeApi({ state }).savingsPots.list({ includeClosed: false });
    expect(groups[0]?.pots[0]?.target?.progress).toBe(100);
  });
});

describe('savings pot detail', () => {
  it('is null for an id that no pot has', async () => {
    expect(await api().savingsPots.detail('nope')).toBeNull();
  });

  it('warns about the shortfall of a pot under its target, and only then', async () => {
    expect((await detailOf('rfp')).belowTarget).toEqual({ target: 5000, shortfall: 800 });
    expect((await detailOf('rfs')).belowTarget).toBeNull();
    expect((await detailOf('gas')).belowTarget).toBeNull();
  });

  it.each([
    ['i0', { state: 'closed' }],
    ['rfs', { state: 'switched-off' }],
    ['gas', { state: 'done', doneOn: '2026-11-05' }],
    ['rfp', { state: 'pending' }],
    ['com', { state: 'pending' }],
    ['i1', { state: 'pending' }],
    ['emg', { state: 'none-planned' }],
    ['sn', { state: 'none-planned' }],
  ])('puts the contribution of %s in the state %j', async (id, expected) => {
    expect((await detailOf(id)).contribution).toEqual(expected);
  });

  it('lets a pot at its target switch off even with a deposit still planned', async () => {
    const state = prototypeState();
    state.plannedContributions.push({ name: 'RF Saúde', amount: 100, state: 'pending', doneOn: null });
    const detail = await createFakeApi({ state }).savingsPots.detail('rfs');
    expect(detail?.contribution).toEqual({ state: 'switched-off' });
  });

  it('shares an investment deposit between every investment', async () => {
    const state = prototypeState();
    const planned = state.plannedContributions.find((item) => item.name === 'Investimento')!;
    planned.state = 'done';
    planned.doneOn = '2026-11-20';
    const fake = createFakeApi({ state });
    expect((await fake.savingsPots.detail('i2'))?.contribution).toEqual({ state: 'done', doneOn: '2026-11-20' });
  });

  it('lists the lines a pot feeds, with the shares of a shared pot', async () => {
    expect((await detailOf('gas')).linkedLines).toEqual([
      { name: 'Gás', share: 240 },
      { name: 'IPTU', share: 360 },
    ]);
    expect((await detailOf('rem')).linkedLines).toEqual([{ name: 'Remédios', share: null }]);
    expect((await detailOf('emg')).linkedLines).toEqual([]);
  });

  it('works out the investment figures and leaves a plain pot without them', async () => {
    expect((await detailOf('i1')).investment).toEqual({
      applied: 22400,
      gross: 25000,
      tax: 455,
      net: 24545,
      redeemableOn: '2028-03-15',
    });
    expect((await detailOf('rfs')).investment).toBeNull();
  });

  it('opens the movements with the month deposit once it is made, then what the ledger holds', async () => {
    expect((await detailOf('gas')).movements).toEqual([
      { day: '2026-11-05', description: 'Depósito · aporte do mês', amount: 120 },
    ]);
    expect((await detailOf('rfp')).movements).toEqual([
      { day: '2026-10-14', description: 'Retirada · veterinário', amount: -800 },
    ]);
  });

  it('has no movements for a pot nothing was deposited in or taken from', async () => {
    expect((await detailOf('com')).movements).toEqual([]);
  });

  it('spreads the month yield over its days so they add up to the total', async () => {
    for (const id of ['rfs', 'rfp', 'i1', 'i2', 'sn', 'emg']) {
      const { yield: earned } = await detailOf(id);
      expect(earned.days).toHaveLength(18);
      expect(Math.round(earned.days.reduce((total, day) => total + day.amount, 0) * 100) / 100).toBe(earned.total);
      expect(earned.days.every((day) => day.day.startsWith('2026-11-'))).toBe(true);
    }
  });

  it('shows no yield days for a closed pot', async () => {
    const { yield: earned } = await detailOf('i0');
    expect(earned.total).toBe(0);
    expect(earned.days).toEqual([]);
  });
});

describe('net worth overview', () => {
  it('adds the three buckets and leaves out a pot that belongs to none', async () => {
    const overview = await api().netWorth.overview();
    expect(overview.buckets.map((bucket) => [bucket.bucket, bucket.total])).toEqual([
      ['Disponível', 8608.75],
      ['Reservado', 34150],
      ['Investido', 37800],
    ]);
    expect(overview.total).toBe(80558.75);
  });

  it('builds the available bucket from bank accounts only, never the card', async () => {
    const { buckets } = await api().netWorth.overview();
    expect(buckets[0]?.items.map((item) => item.name)).toEqual([
      'Itaú · Conta corrente',
      'Mercado Pago · Conta',
      'C6 · Conta corrente',
      'Nubank · Conta',
    ]);
  });

  it('builds a pot bucket from its open pots', async () => {
    const { buckets } = await api().netWorth.overview();
    expect(buckets[2]?.items).toEqual([
      { name: 'Investimento 1', detail: 'Nubank', amount: 25000 },
      { name: 'Investimento 2', detail: 'Nubank', amount: 12800 },
    ]);
  });

  it('records one month on a chart that has room for six', async () => {
    const { chart, historyStart } = await api().netWorth.overview();
    expect(chart.months).toEqual(['2026-10', '2026-11', '2026-12', '2027-01', '2027-02', '2027-03']);
    expect(chart.points).toEqual([
      { month: '2026-11', available: 8608.75, reserved: 34150, invested: 37800, total: 80558.75 },
    ]);
    expect(chart.max).toBe(100000);
    expect(historyStart).toBe('2026-11');
  });

  it('plans the contribution from the provisions, the reserves and what is left to invest', async () => {
    const { contribution } = await api().netWorth.overview();
    expect(contribution.planned).toBe(5220);
    expect(contribution.done).toBe(320);
    expect(contribution.progress).toBeCloseTo(6.13, 2);
  });

  it('caps the contribution progress at the whole plan', async () => {
    const state = prototypeState();
    state.plannedContributions[0]!.state = 'done';
    state.plannedContributions[0]!.amount = 99999;
    expect((await createFakeApi({ state }).netWorth.overview()).contribution.progress).toBe(100);
  });

  it('plans the investment rate from the budget and counts the real one only once invested', async () => {
    expect((await api().netWorth.overview()).investmentRate).toEqual({ month: '2026-11', planned: 32, actual: 0 });

    const state = prototypeState();
    const planned = state.plannedContributions.find((item) => item.name === 'Investimento')!;
    planned.state = 'done';
    planned.doneOn = '2026-11-20';
    expect((await createFakeApi({ state }).netWorth.overview()).investmentRate.actual).toBe(32);
  });

  it('has no rate, rather than a zero, when the budget plans no income', async () => {
    const state = prototypeState();
    state.budget.find((group) => group.key === 'rec')!.lines = [];
    expect((await createFakeApi({ state }).netWorth.overview()).investmentRate).toMatchObject({ planned: null, actual: null });
  });

  it('lists the yield of every open pot and totals it', async () => {
    const { yields } = await api().netWorth.overview();
    expect(yields.pots).toHaveLength(13);
    expect(yields.pots.find((pot) => pot.name === null)).toEqual({ name: null, cdiPercent: 100, amount: 5.25 });
    expect(yields.total).toBe(854.18);
  });
});
