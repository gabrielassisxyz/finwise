import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import type { FakeTransaction } from './model';
import { prototypeState } from './prototype-data';
import { contributions, netWorth, spentByLine, toContract } from './rules';

function purchase(overrides: Partial<FakeTransaction> = {}): FakeTransaction {
  const template = prototypeState().transactions.find((transaction) => transaction.id === 't2')!;
  return { ...template, ...overrides };
}

describe('contributions', () => {
  it('puts a purchase on its line for what was paid', () => {
    expect(contributions(purchase({ amount: -180, line: 'Transporte' }))).toEqual([['Transporte', 180]]);
  });

  it('counts nothing while a review is pending, even with a line set', () => {
    const pending = { reason: 'Baixa confiança', suggestion: 'Transporte', confidence: 60, justification: '' };
    expect(contributions(purchase({ pending }))).toEqual([]);
  });

  it('counts nothing for a movement between own accounts', () => {
    expect(contributions(purchase({ movement: true }))).toEqual([]);
  });

  it('counts nothing once marked as not counting with a reason', () => {
    expect(contributions(purchase({ excludedReason: 'Compra da empresa' }))).toEqual([]);
  });

  it('takes a linked reimbursement off the purchase', () => {
    expect(contributions(purchase({ amount: -300, line: 'Lazer', reimbursedAmount: 150 }))).toEqual([['Lazer', 150]]);
  });

  it('spreads a split purchase over its parts and drops a part without a line', () => {
    const split = [
      { line: 'Mercado', amount: 100 },
      { line: 'Casa', amount: 60 },
      { line: null, amount: 20 },
    ];
    expect(contributions(purchase({ amount: -180, split }))).toEqual([
      ['Mercado', 100],
      ['Casa', 60],
    ]);
  });

  it('ignores a credit unless it is a refund pointed at a line, which it then reduces', () => {
    expect(contributions(purchase({ amount: 200, line: 'Renda extra' }))).toEqual([]);
    expect(contributions(purchase({ amount: 120, refund: true }))).toEqual([]);
    expect(contributions(purchase({ amount: 120, refund: true, refundOffsetsLine: 'Tarifas e juros' }))).toEqual([
      ['Tarifas e juros', -120],
    ]);
  });
});

describe('spentByLine', () => {
  it('is the baseline while nothing changed', () => {
    const state = prototypeState();
    const original = structuredClone(state.transactions);
    expect(spentByLine(state, original, '2026-11').Mercado).toBe(308);
  });

  it('moves with a change made in the current month', () => {
    const state = prototypeState();
    const original = structuredClone(state.transactions);
    state.transactions.find((transaction) => transaction.id === 't2')!.line = 'Lazer';
    const spent = spentByLine(state, original, '2026-11');
    expect(spent.Transporte).toBe(0);
    expect(spent.Lazer).toBe(245.1);
  });

  it('leaves another month alone', () => {
    const state = prototypeState();
    const original = structuredClone(state.transactions);
    state.transactions.find((transaction) => transaction.id === 't61')!.line = 'Lazer';
    expect(spentByLine(state, original, '2026-10').Transporte).toBe(265);
  });
});

describe('netWorth', () => {
  it('adds account balances to what is available and sorts savings by bucket', () => {
    expect(netWorth(prototypeState())).toEqual({ available: 8608.75, reserved: 34150, invested: 37800 });
  });

  it('leaves a pot without a bucket out of every total', () => {
    const state = prototypeState();
    state.savingsPots.find((pot) => pot.id === 'sn')!.balance = 99999;
    expect(netWorth(state).available).toBe(8608.75);
  });
});

describe('toContract', () => {
  const state = prototypeState();
  const find = (id: string) => toContract(state, state.transactions.find((transaction) => transaction.id === id)!);

  it('shows the nickname and keeps the original name', () => {
    expect(find('t34')).toMatchObject({ name: 'Aluguel', originalName: 'João P. Santos' });
  });

  it('adds the fixed tags of the establishment without repeating one', () => {
    expect(find('t16').tags).toEqual(['#viagem']);
  });

  it('flags a model classification under the threshold, and only that', () => {
    expect(find('t4')).toMatchObject({ classifiedBy: 'IA', confidence: 62, belowThreshold: true });
    expect(find('t2').belowThreshold).toBe(false);
  });

  it('keeps movements out of the totals', () => {
    expect(find('t52').countsInTotals).toBe(false);
    expect(find('t2').countsInTotals).toBe(true);
  });
});

describe('home summary', () => {
  it('reports the month limit over the variable lines, the net worth and the pending count', async () => {
    const summary = await createFakeApi().home.summary('2026-11');
    expect(summary.limit).toEqual({ cap: 2880, spent: 1460 });
    expect(summary.netWorth).toEqual({ available: 8608.75, reserved: 34150, invested: 37800 });
    expect(summary.pendingCount).toBe(7);
    expect(summary.weeks.find((week) => week.current)).toMatchObject({ guide: 355, spent: 210 });
  });
});
