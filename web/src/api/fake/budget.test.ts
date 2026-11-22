import { describe, expect, it } from 'vitest';
import { createFakeApi } from '.';
import type { BudgetDoc, BudgetGroupView, BudgetView, FinwiseApi } from '../types';
import { prototypeState } from './prototype-data';

const NOV = '2026-11';
const OCT = '2026-10';
const DEC = '2026-12';

const group = (view: BudgetView, key: string): BudgetGroupView => view.groups.find((g) => g.key === key)!;
const line = (view: BudgetView, key: string, name: string) => group(view, key).lines.find((l) => l.name === name)!;

async function withAmount(api: FinwiseApi, groupKey: string, name: string, amount: number): Promise<BudgetDoc> {
  const doc = await api.budget.document(NOV);
  doc.find((g) => g.key === groupKey)!.lines.find((l) => l.name === name)!.amount = amount;
  return doc;
}

describe('budget figures', () => {
  it('weighs the income against everything planned, and rates the investment on the income', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(view.income).toBe(12000);
    expect(view.allocated).toBe(12000);
    expect(view.investmentRate).toBe(32);
    expect(group(view, 'inv').subtotal).toBe(3800);
    expect(line(view, 'inv', 'Investimento').cells[2]).toEqual({ type: 'percent', value: 32 });
  });

  it('orders the groups as the screen lists them', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(view.groups.map((g) => g.key)).toEqual(['rec', 'fix', 'var', 'prov', 'res', 'rf', 'tar', 'inv']);
  });

  it('gives a fixed line what is left against what it was meant to cost, and the group its spending beside the plan', async () => {
    const view = await createFakeApi().budget.view(NOV);
    const house = line(view, 'fix', 'Contas da casa');
    expect(house.cells).toEqual([
      { type: 'money', amount: 2000 },
      { type: 'money', amount: 2059.9 },
      { type: 'difference', amount: -59.9 },
    ]);
    expect(house.over).toBe(true);
    expect(house.bar).toBeNull();
    expect(line(view, 'fix', 'Saúde').over).toBe(false);
    // Spending exactly what was planned is not an overspend.
    expect(line(view, 'fix', 'Transporte para o trabalho')).toMatchObject({ over: false });
    expect(line(view, 'fix', 'Transporte para o trabalho').cells[2]).toEqual({ type: 'difference', amount: 0 });
    expect(group(view, 'fix')).toMatchObject({ subtotal: 3900, subtotalSpent: 3835.9, over: false });
  });

  it('gives a variable line a bar, capped at the full width, and marks the one that went over', async () => {
    const view = await createFakeApi().budget.view(NOV);
    const delivery = line(view, 'var', 'Delivery');
    expect(delivery).toMatchObject({ over: true, bar: 100, marks: [{ text: 'Estourou', tone: 'bad' }] });
    expect(delivery.cells[2]).toEqual({ type: 'difference', amount: -6 });
    expect(line(view, 'var', 'Mercado')).toMatchObject({ over: false, bar: 77, marks: [] });
    expect(line(view, 'var', 'Educação').bar).toBe(0);
  });

  it('flags a variable group only when its lines together spent more than their caps', async () => {
    const api = createFakeApi();
    expect(group(await api.budget.view(NOV), 'var')).toMatchObject({ subtotal: 2880, subtotalSpent: 1460, over: false });
    const draft = await api.budget.document(NOV);
    for (const l of draft.find((g) => g.key === 'var')!.lines) l.amount = 100;
    expect(group(await api.budget.view(NOV, draft), 'var')).toMatchObject({ subtotal: 900, over: true });
  });

  it('does not flag a variable group that spent exactly its caps', async () => {
    const api = createFakeApi();
    const spent = await api.budget.view(NOV);
    const draft = await api.budget.document(NOV);
    const variable = draft.find((g) => g.key === 'var')!;
    variable.lines.forEach((l, i) => (l.amount = (group(spent, 'var').lines[i]!.cells[1] as { amount: number }).amount));
    expect(group(await api.budget.view(NOV, draft), 'var')).toMatchObject({ subtotal: 1460, subtotalSpent: 1460, over: false });
  });

  it('carries the marker of an income line', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(line(view, 'rec', 'Renda extra').marks).toEqual([{ text: 'Não garantida', tone: 'neutral' }]);
    expect(line(view, 'rec', 'Renda extra').cells.slice(0, 2)).toEqual([
      { type: 'money', amount: 1500 },
      { type: 'money', amount: 200 },
    ]);
  });

  it('reads a provision its balance and next bill from the pot', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(line(view, 'prov', 'Gás e IPTU').cells).toEqual([
      { type: 'money', amount: 120 },
      { type: 'money', amount: 600 },
      { type: 'next-charge', charge: { description: 'IPTU', day: '2027-01-10' } },
    ]);
    expect(line(view, 'prov', 'Remédios').cells[2]).toEqual({ type: 'next-charge', charge: null });
    expect(group(view, 'prov').subtotal).toBe(320);
    expect(group(view, 'prov').lines.every((l) => !l.expandable)).toBe(true);
  });

  it('reads a wish reserve its balance from the pot and its spending from the month', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(line(view, 'res', 'Viagens').cells).toEqual([
      { type: 'money', amount: 600 },
      { type: 'money', amount: 2350 },
      { type: 'money', amount: 420 },
    ]);
  });

  it('lists the rainy funds by their pots: switched off at the target, on again below it', async () => {
    const view = await createFakeApi().budget.view(NOV);
    const rainy = group(view, 'rf');
    expect(rainy.lines.map((l) => l.name)).toEqual(['Saúde', 'Pets', 'Casa', 'Tech']);
    expect(rainy.subtotal).toBe(11200);
    expect(rainy.lines[1]).toMatchObject({ bar: 84 });
    expect(rainy.lines[1]?.cells).toEqual([
      { type: 'money', amount: 5000 },
      { type: 'money', amount: 4200 },
      { type: 'rainy-deposit', switchedOn: true },
    ]);
    expect(rainy.lines[0]?.cells[2]).toEqual({ type: 'rainy-deposit', switchedOn: false });
    expect(rainy.lines[0]?.bar).toBe(100);
  });

  it('shows what was spent on fees and interest', async () => {
    const view = await createFakeApi().budget.view(NOV);
    expect(group(view, 'tar')).toMatchObject({ subtotal: 25 });
    expect(line(view, 'tar', 'Tarifas e juros')).toMatchObject({ expandable: true });
  });

  it('counts the investment as made once the deposit is, and in a closed month as whole', async () => {
    const api = createFakeApi();
    expect(line(await api.budget.view(NOV), 'inv', 'Investimento').cells[1]).toEqual({ type: 'money', amount: 0 });
    expect(line(await api.budget.view(OCT), 'inv', 'Investimento').cells[1]).toEqual({ type: 'money', amount: 3800 });
    const state = prototypeState();
    state.plannedContributions.find((item) => item.name === 'Investimento')!.state = 'done';
    expect(line(await createFakeApi({ state }).budget.view(NOV), 'inv', 'Investimento').cells[1]).toEqual({
      type: 'money',
      amount: 3800,
    });
  });

  it('marks a budget that outgrows its income, with a negative investment and rate', async () => {
    const api = createFakeApi();
    const draft = await withAmount(api, 'rec', 'Salário', 1000);
    const view = await api.budget.view(NOV, draft);
    const investment = line(view, 'inv', 'Investimento');
    expect(investment.over).toBe(true);
    expect(investment.marks).toEqual([{ text: 'Budget maior que a receita', tone: 'bad' }]);
    expect(group(view, 'inv')).toMatchObject({ subtotal: -5200, over: true });
    expect(view.investmentRate).toBeLessThan(0);
    expect(view.income).toBe(3000);
  });

  it('evaluates a draft without keeping it', async () => {
    const api = createFakeApi();
    const draft = await withAmount(api, 'fix', 'Saúde', 900);
    expect(group(await api.budget.view(NOV, draft), 'fix').subtotal).toBe(4300);
    expect(group(await api.budget.view(NOV), 'fix').subtotal).toBe(3900);
  });

  it('treats an amount that is not a number as zero', async () => {
    const api = createFakeApi();
    const draft = await withAmount(api, 'fix', 'Saúde', Number.NaN);
    expect(group(await api.budget.view(NOV, draft), 'fix').subtotal).toBe(3400);
  });
});

describe('the establishments of a line', () => {
  it('adds up what each established place put into the line this month, biggest first', async () => {
    const list = await createFakeApi().budget.establishments(NOV, 'Delivery');
    expect(list).toEqual([
      { key: 'iFood', name: 'iFood', amount: 133.3, count: 2 },
      { key: 'Rappi', name: 'Rappi', amount: 72.7, count: 1 },
    ]);
  });

  it('is empty for a month with nothing classified', async () => {
    expect(await createFakeApi().budget.establishments(DEC, 'Delivery')).toEqual([]);
  });

  it('leaves out what does not count: a purchase marked as not counting drops out of its place', async () => {
    const api = createFakeApi();
    const ifood = (await api.transactions.list({ month: NOV, period: 'month' })).find((t) => t.name === 'iFood')!;
    await api.transactions.setExcluded(ifood.id, true);
    await api.transactions.setExclusionReason(ifood.id, 'Compra de outra pessoa');
    const list = await api.budget.establishments(NOV, 'Delivery');
    expect(list.find((place) => place.key === 'iFood')).toMatchObject({ count: 1 });
  });
});

describe('versioning by month', () => {
  it('holds a save from its month on and leaves the closed month as it was', async () => {
    const api = createFakeApi();
    await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    const amountIn = async (month: string) => line(await api.budget.view(month), 'fix', 'Saúde').cells[0];
    expect(await amountIn(NOV)).toEqual({ type: 'money', amount: 900 });
    expect(await amountIn(DEC)).toEqual({ type: 'money', amount: 900 });
    expect(await amountIn(OCT)).toEqual({ type: 'money', amount: 500 });
  });

  it('leaves the months before the one saved alone', async () => {
    const api = createFakeApi();
    const doc = await api.budget.document(DEC);
    doc.find((g) => g.key === 'fix')!.lines.find((l) => l.name === 'Saúde')!.amount = 700;
    await api.budget.save(DEC, doc);
    expect(line(await api.budget.view(NOV), 'fix', 'Saúde').cells[0]).toEqual({ type: 'money', amount: 500 });
    expect(line(await api.budget.view(DEC), 'fix', 'Saúde').cells[0]).toEqual({ type: 'money', amount: 700 });
  });

  it('replaces a later version a month had of its own', async () => {
    const api = createFakeApi();
    const dec = await api.budget.document(DEC);
    dec.find((g) => g.key === 'fix')!.lines.find((l) => l.name === 'Saúde')!.amount = 700;
    await api.budget.save(DEC, dec);
    await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    expect(line(await api.budget.view(DEC), 'fix', 'Saúde').cells[0]).toEqual({ type: 'money', amount: 900 });
  });

  it('keeps a month closed after the saved one as it was, and resumes the new budget the month after it', async () => {
    const state = prototypeState();
    state.shell.lastMonth = '2027-01';
    state.shell.closedMonths.push({ month: DEC, closedOn: '2026-12-30' });
    const api = createFakeApi({ state });
    await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    const health = async (month: string) => line(await api.budget.view(month), 'fix', 'Saúde').cells[0];
    expect(await health(NOV)).toEqual({ type: 'money', amount: 900 });
    expect(await health(DEC)).toEqual({ type: 'money', amount: 500 });
    expect(await health('2027-01')).toEqual({ type: 'money', amount: 900 });
  });

  it('lets the other screens see the new budget of the current month', async () => {
    const api = createFakeApi();
    const before = await api.home.summary(NOV);
    const draft = await api.budget.document(NOV);
    draft.find((g) => g.key === 'var')!.lines.find((l) => l.name === 'Mercado')!.amount = 1000;
    await api.budget.save(NOV, draft);
    expect((await api.home.summary(NOV)).limit.cap).toBe(before.limit.cap + 600);
  });

  it('does not touch the current budget when a later month is saved', async () => {
    const api = createFakeApi();
    const before = await api.home.summary(NOV);
    const dec = await api.budget.document(DEC);
    dec.find((g) => g.key === 'var')!.lines[0]!.amount = 9999;
    await api.budget.save(DEC, dec);
    expect((await api.home.summary(NOV)).limit.cap).toBe(before.limit.cap);
  });

  it('refuses to save into a closed month and changes nothing', async () => {
    const api = createFakeApi();
    const draft = await api.budget.document(OCT);
    draft[0]!.lines[0]!.amount = 1;
    await expect(api.budget.save(OCT, draft)).rejects.toThrow(/closed/);
    await expect(api.budget.applyScenario(OCT, draft)).rejects.toThrow(/closed/);
    expect(line(await api.budget.view(OCT), 'rec', 'Salário').cells[0]).toEqual({ type: 'money', amount: 10000 });
  });

  it('keeps a renamed, added and reordered line as the draft had them', async () => {
    const api = createFakeApi();
    const draft = await api.budget.document(NOV);
    const fixed = draft.find((g) => g.key === 'fix')!;
    fixed.lines.reverse();
    fixed.lines.push({ name: 'Nova linha', amount: 10 });
    await api.budget.save(NOV, draft);
    const names = (await api.budget.document(NOV)).find((g) => g.key === 'fix')!.lines.map((l) => l.name);
    expect(names).toEqual(['Transporte para o trabalho', 'Pets', 'Saúde', 'Contas da casa', 'Nova linha']);
  });

  it('answers a save with a change that takes it back,', async () => {
    const api = createFakeApi();
    const draft = await withAmount(api, 'fix', 'Saúde', 900);
    draft.find((g) => g.key === 'var')!.lines[0]!.amount = 1000;
    const { changeId } = await api.budget.save(NOV, draft);
    expect(changeId).toMatch(/^change-/);
    expect((await api.home.summary(NOV)).limit.cap).toBe(3480);
    await api.changes.undo(changeId);
    expect(line(await api.budget.view(NOV), 'fix', 'Saúde').cells[0]).toEqual({ type: 'money', amount: 500 });
    expect((await api.home.summary(NOV)).limit.cap).toBe(2880);
  });

  it('does not take back a transaction change when it takes back a budget save', async () => {
    const api = createFakeApi();
    const { changeId } = await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    const target = (await api.transactions.list({ month: NOV, period: 'month' })).find((t) => t.name === 'iFood')!;
    await api.transactions.setLine(target.id, 'Lazer');
    await api.changes.undo(changeId);
    const after = (await api.transactions.list({ month: NOV, period: 'month' })).find((t) => t.id === target.id)!;
    expect(after.line).toBe('Lazer');
  });
});

describe('scenarios', () => {
  async function withoutExtraIncome(api: FinwiseApi): Promise<BudgetDoc> {
    return withAmount(api, 'rec', 'Renda extra', 0);
  }

  it('weighs a scenario against the budget: investment, rate, difference and the lines that changed', async () => {
    const api = createFakeApi();
    const impact = await api.budget.evaluateScenario(NOV, await withoutExtraIncome(api));
    expect(impact.current).toEqual({ investment: 3800, rate: 32 });
    expect(impact.scenario).toEqual({ investment: 2300, rate: 22 });
    expect(impact.difference).toBe(-1500);
    expect(impact.changed.flat().filter(Boolean)).toHaveLength(1);
    expect(impact.changed[0]).toEqual([false, true, false]);
  });

  it('reports no difference for an untouched copy', async () => {
    const api = createFakeApi();
    const impact = await api.budget.evaluateScenario(NOV, await api.budget.document(NOV));
    expect(impact.difference).toBe(0);
    expect(impact.changed.flat().some(Boolean)).toBe(false);
  });

  it('weighs against the budget of the month asked, not of another', async () => {
    const api = createFakeApi();
    await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    const impact = await api.budget.evaluateScenario(OCT, await api.budget.document(OCT));
    expect(impact.current.investment).toBe(3800);
  });

  it('keeps a saved scenario to load again, under a name that stands in for none', async () => {
    const api = createFakeApi();
    expect(await api.budget.scenarios()).toEqual([]);
    const saved = await api.budget.saveScenario('  ', await withoutExtraIncome(api));
    expect(saved.name).toBe('Cenário sem nome');
    const [kept] = await api.budget.scenarios();
    expect(kept?.name).toBe('Cenário sem nome');
    expect(kept?.budget[0]?.lines[1]?.amount).toBe(0);
  });

  it('replaces a scenario saved under the same name', async () => {
    const api = createFakeApi();
    await api.budget.saveScenario('Sem extra', await withoutExtraIncome(api));
    await api.budget.saveScenario('Sem extra', await api.budget.document(NOV));
    const kept = await api.budget.scenarios();
    expect(kept).toHaveLength(1);
    expect(kept[0]?.budget[0]?.lines[1]?.amount).toBe(1500);
  });

  it('takes a saved scenario back with its change, and keeps the ones saved before it', async () => {
    const api = createFakeApi();
    await api.budget.saveScenario('Primeiro', await api.budget.document(NOV));
    const { changeId } = await api.budget.saveScenario('Sem extra', await withoutExtraIncome(api));
    await api.changes.undo(changeId);
    expect((await api.budget.scenarios()).map((scenario) => scenario.name)).toEqual(['Primeiro']);
  });

  it('applies a scenario as the budget from the month on', async () => {
    const api = createFakeApi();
    const { changeId } = await api.budget.applyScenario(NOV, await withoutExtraIncome(api));
    const view = await api.budget.view(DEC);
    expect(view.income).toBe(10500);
    expect(group(view, 'inv').subtotal).toBe(2300);
    await api.changes.undo(changeId);
    expect((await api.budget.view(DEC)).income).toBe(12000);
  });
});

describe('line ids', () => {
  // The stand-in mints no id; it keeps the ones it is given, as the server will answer them.
  async function withIds(api: FinwiseApi): Promise<BudgetDoc> {
    const doc = await api.budget.document(NOV);
    let next = 1;
    for (const groupDoc of doc) for (const lineDoc of groupDoc.lines) lineDoc.id = String(next++);
    return doc;
  }
  const idsOf = (doc: BudgetDoc) => doc.map((groupDoc) => groupDoc.lines.map((lineDoc) => [lineDoc.id, lineDoc.name]));
  const hasAnyId = (doc: BudgetDoc) => doc.some((groupDoc) => groupDoc.lines.some((lineDoc) => 'id' in lineDoc));

  it('keeps the id of every line through a save', async () => {
    const api = createFakeApi();
    const draft = await withIds(api);
    await api.budget.save(NOV, draft);
    expect(idsOf(await api.budget.document(NOV))).toEqual(idsOf(draft));
    expect(idsOf(await api.budget.document(DEC))).toEqual(idsOf(draft));
  });

  it('keeps the id of every line through an applied scenario and a saved one', async () => {
    const api = createFakeApi();
    const draft = await withIds(api);
    await api.budget.applyScenario(NOV, draft);
    expect(idsOf(await api.budget.document(NOV))).toEqual(idsOf(draft));
    await api.budget.saveScenario('Com ids', draft);
    const [kept] = await api.budget.scenarios();
    expect(idsOf(kept!.budget)).toEqual(idsOf(draft));
  });

  it('mints none: a draft saved without ids is stored without any', async () => {
    const api = createFakeApi();
    expect(hasAnyId(await api.budget.document(NOV))).toBe(false);
    await api.budget.save(NOV, await withAmount(api, 'fix', 'Saúde', 900));
    await api.budget.saveScenario('Sem ids', await api.budget.document(NOV));
    expect(hasAnyId(await api.budget.document(NOV))).toBe(false);
    expect(hasAnyId((await api.budget.scenarios())[0]!.budget)).toBe(false);
  });

  it('figures a draft the same with ids as without', async () => {
    const api = createFakeApi();
    const plain = await withAmount(api, 'rec', 'Renda extra', 0);
    const marked = structuredClone(plain);
    let next = 1;
    for (const groupDoc of marked) for (const lineDoc of groupDoc.lines) lineDoc.id = String(next++);
    expect(await api.budget.view(NOV, marked)).toEqual(await api.budget.view(NOV, plain));
    expect(await api.budget.evaluateScenario(NOV, marked)).toEqual(await api.budget.evaluateScenario(NOV, plain));
  });
});
