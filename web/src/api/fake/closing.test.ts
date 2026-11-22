import { describe, expect, it } from 'vitest';
import type { ClosingStepId } from '../types';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

const MONTH = '2026-11';

type Api = ReturnType<typeof createFakeApi>;

const stepStates = async (api: Api, month = MONTH) => Object.fromEntries((await api.review.closing(month)).steps.map((step) => [step.id, step.state]));

async function clearQueue(api: Api): Promise<void> {
  for (const item of (await api.review.weekly(MONTH)).queue.items) {
    await api.review.acceptPending(item.transactionId, { line: item.pending.suggestion!, always: false, nickname: '', reason: '' });
  }
}

describe('the steps of the closing', () => {
  it('lists the eight steps in order, the first one pending while reviews wait', async () => {
    const { steps, pendingCount } = await createFakeApi().review.closing(MONTH);
    expect(steps.map((step) => step.id)).toEqual([
      'pending',
      'planned-actual',
      'leftover',
      'contributions',
      'card-bill',
      'subscriptions',
      'report',
      'close',
    ]);
    expect(steps[0]).toEqual({ id: 'pending', state: 'pending', markable: false });
    expect(steps.slice(1).every((step) => step.state === 'todo')).toBe(true);
    expect(pendingCount).toBe(7);
  });

  it('lets the person mark steps 2 to 7 of an open month, and none of a closed one', async () => {
    const api = createFakeApi();
    const markable = async (month: string) => (await api.review.closing(month)).steps.map((step) => step.markable);
    expect(await markable(MONTH)).toEqual([false, true, true, true, true, true, true, false]);
    expect(await markable('2026-10')).toEqual(Array(8).fill(false));
  });

  it('finishes the first step by itself once nothing waits for review', async () => {
    const api = createFakeApi();
    await clearQueue(api);
    expect((await stepStates(api)).pending).toBe('done');
  });

  it('marks a step by hand, only in its own month, and the mark is taken back with the change', async () => {
    const api = createFakeApi();
    const marked = await api.review.markStepDone(MONTH, 'contributions');
    expect((await stepStates(api)).contributions).toBe('done');
    expect((await stepStates(api, '2026-12')).contributions).toBe('todo');
    await api.changes.undo(marked!.changeId);
    expect((await stepStates(api)).contributions).toBe('todo');
  });

  it.each<ClosingStepId>(['pending', 'close'])('does not let the step %s be marked by hand', async (step) => {
    const api = createFakeApi();
    expect(await api.review.markStepDone(MONTH, step)).toBeNull();
    expect((await stepStates(api))[step]).not.toBe('done');
  });

  it('does not mark a step of a closed month', async () => {
    expect(await createFakeApi().review.markStepDone('2026-10', 'report')).toBeNull();
  });
});

describe('planned against actual', () => {
  it('compares each group and line with what was spent, to the cent', async () => {
    const { plannedActual } = await createFakeApi().review.closing(MONTH);
    expect(plannedActual.map((group) => group.name)).toEqual([
      'Receita',
      'Fixos e essenciais',
      'Variáveis',
      'Provisões',
      'Reservas de desejo',
    ]);
    const fixed = plannedActual[1]!;
    expect(fixed).toMatchObject({ planned: 3900, actual: 3835.9, difference: 64.1, contributions: false });
    expect(fixed.lines[0]).toEqual({ name: 'Contas da casa', planned: 2000, actual: 2059.9, difference: -59.9 });
  });

  it('counts income that fell short as a negative difference, spending that went over the same way', async () => {
    const { plannedActual } = await createFakeApi().review.closing(MONTH);
    const income = plannedActual[0]!;
    expect(income).toMatchObject({ planned: 12000, actual: 10350, difference: -1650 });
    expect(income.lines.find((line) => line.name === 'Renda extra')?.difference).toBe(-1300);
    const delivery = plannedActual[2]!.lines.find((line) => line.name === 'Delivery');
    expect(delivery).toEqual({ name: 'Delivery', planned: 200, actual: 206, difference: -6 });
  });

  it('compares provisions and reserves with what was set aside, not with what was spent', async () => {
    const { plannedActual } = await createFakeApi().review.closing(MONTH);
    expect(plannedActual[3]).toMatchObject({ contributions: true, planned: 320, actual: 320, difference: 0 });
    expect(plannedActual[4]).toMatchObject({ contributions: true, planned: 1100, actual: 0, difference: 1100 });
  });

  it('follows a transaction put on a line', async () => {
    const api = createFakeApi();
    await api.review.acceptPending('t5', { line: 'Restaurantes e bares', always: false, nickname: '', reason: '' });
    const line = (await api.review.closing(MONTH)).plannedActual[2]!.lines.find((candidate) => candidate.name === 'Restaurantes e bares');
    expect(line?.actual).toBe(388.15);
  });

  it('takes a closed month as having made every contribution it planned', async () => {
    const { plannedActual } = await createFakeApi().review.closing('2026-10');
    expect(plannedActual[4]).toMatchObject({ planned: 1100, actual: 1100, difference: 0 });
  });
});

describe('what is left of the month', () => {
  it('is the variable cap minus what the variable lines spent', async () => {
    expect((await createFakeApi().review.closing(MONTH)).leftover).toBe(1420);
  });

  it('turns negative when the variable lines went over', async () => {
    const state = prototypeState();
    state.spentBaseline[MONTH]!['Mercado'] = 2000;
    expect((await createFakeApi({ state }).review.closing(MONTH)).leftover).toBe(-272);
  });
});

describe('the contributions, the card bill and the subscriptions to review', () => {
  it('names each planned transfer as done, on target or not found', async () => {
    const { contributions } = await createFakeApi().review.closing(MONTH);
    expect(contributions.map((item) => [item.name, item.state])).toEqual([
      ['RF Casa', 'on-target'],
      ['Gás e IPTU', 'done'],
      ['Assinaturas anuais', 'done'],
      ['Remédios', 'done'],
      ['Compras', 'not-found'],
      ['Viagens', 'not-found'],
      ['Investimento', 'not-found'],
    ]);
  });

  it('follows a transfer marked as done on the payday tab', async () => {
    const api = createFakeApi();
    await api.review.markTransferDone('Compras');
    expect((await api.review.closing(MONTH)).contributions.find((item) => item.name === 'Compras')?.state).toBe('done');
  });

  it('shows every contribution of a closed month as done except the one on target', async () => {
    const { contributions } = await createFakeApi().review.closing('2026-10');
    expect(contributions.map((item) => item.state)).toEqual(['on-target', 'done', 'done', 'done', 'done', 'done', 'done']);
  });

  it('lists the purchases that fall on the next bill, adding up to the amount to pay ahead', async () => {
    const { cardBill } = await createFakeApi().review.closing(MONTH);
    expect(cardBill?.amount).toBe(386.4);
    expect(cardBill?.cardLabel).toBe('C6 Carbon · final 1234');
    expect(cardBill?.purchases.map((purchase) => [purchase.name, purchase.day, purchase.amount])).toEqual([
      ['Cinema Belas Artes', '2026-11-28', 65.1],
      ['Posto Shell', '2026-11-28', 180],
      ['iFood', '2026-11-28', 54.9],
      ['Pão de Açúcar', '2026-11-28', 86.4],
    ]);
    const total = cardBill!.purchases.reduce((sum, purchase) => sum + purchase.amount, 0);
    expect(Math.round(total * 100) / 100).toBe(cardBill!.amount);
  });

  it('lists the subscriptions whose review falls by the end of the month, and none already being cancelled', async () => {
    const api = createFakeApi();
    expect((await api.review.closing(MONTH)).subscriptionsToReview.map((item) => item.id)).toEqual(['ap', 'rv']);
    await api.review.approve('revista');
    expect((await api.review.closing(MONTH)).subscriptionsToReview.map((item) => item.id)).toEqual(['ap']);
  });

  it('lists, for a month that ended before it, only what had come due by then', async () => {
    const october = (await createFakeApi().review.closing('2026-10')).subscriptionsToReview;
    expect(october.map((item) => item.id)).toEqual(['rv']);
  });

  it('keeps one under review and schedules the next review six months after the month ends', async () => {
    const api = createFakeApi();
    const kept = await api.review.keepSubscription(MONTH, 'rv');
    expect(kept).toMatchObject({ name: 'Revista digital', nextReview: '2027-05-30' });
    expect((await api.subscriptions.detail('rv'))).toMatchObject({ status: 'Manter', reviewOn: '2027-05-30' });
    expect((await api.review.closing(MONTH)).subscriptionsToReview.map((item) => item.id)).toEqual(['ap']);
    await api.changes.undo(kept!.changeId);
    expect((await api.subscriptions.detail('rv'))).toMatchObject({ status: 'Analisar', reviewOn: '2026-09-15' });
  });

  it('marks one to cancel, which takes it off the list', async () => {
    const api = createFakeApi();
    const cancelled = await api.review.cancelSubscription('ap');
    expect(cancelled?.name).toBe('Amazon Prime');
    expect((await api.subscriptions.detail('ap'))?.status).toBe('Cancelar');
    expect((await api.review.closing(MONTH)).subscriptionsToReview.map((item) => item.id)).toEqual(['rv']);
  });

  it('knows no subscription it was not given', async () => {
    const api = createFakeApi();
    expect(await api.review.keepSubscription(MONTH, 'nope')).toBeNull();
    expect(await api.review.cancelSubscription('nope')).toBeNull();
  });
});

describe('the report', () => {
  it('has five sections, the figures of the first computed from the month', async () => {
    const { report } = await createFakeApi().review.closing(MONTH);
    expect(report.map((section) => section.heading)).toEqual([
      'Orçado × realizado',
      'Onde dá para cortar',
      'Fora do padrão',
      'Sugestões de teto',
      'Comparado aos meses anteriores',
    ]);
    expect(report[0]!.paragraphs[0]!.text.replace(/\u00a0/g, ' ')).toMatch(/^As linhas variáveis somam R\$ 1\.460,00 de R\$ 2\.880,00\./);
  });

  it('suggests two caps, each of which can be sent for approval once', async () => {
    const api = createFakeApi();
    const suggestions = async () => (await api.review.closing(MONTH)).report[3]!.paragraphs.map((p) => [p.proposalId, p.sent]);
    expect(await suggestions()).toEqual([['lazer', false], ['teto', true]]);
    await api.review.propose('lazer');
    expect(await suggestions()).toEqual([['lazer', true], ['teto', true]]);
  });
});

describe('closing the month', () => {
  it('is refused while anything waits for review', async () => {
    const api = createFakeApi();
    await expect(api.review.closeMonth(MONTH)).rejects.toThrow(/waiting for review/);
    expect((await api.shell.overview()).closedMonths.map((closed) => closed.month)).toEqual(['2026-10']);
  });

  it('closes the month on today, once the queue is empty, and the shell reports it', async () => {
    const api = createFakeApi();
    await clearQueue(api);
    await api.review.closeMonth(MONTH);
    const { closedMonths } = await api.shell.overview();
    expect(closedMonths).toEqual([
      { month: '2026-10', closedOn: '2026-11-02' },
      { month: '2026-11', closedOn: '2026-11-28' },
    ]);
    const closing = await api.review.closing(MONTH);
    expect(closing).toMatchObject({ closed: true, closedOn: '2026-11-28' });
    expect(closing.steps.every((step) => step.state === 'done')).toBe(true);
    expect(closing.closedMonths.map((closed) => closed.month)).toEqual(['2026-11', '2026-10']);
  });

  it('makes the budget of the month read-only', async () => {
    const api = createFakeApi();
    await clearQueue(api);
    await api.review.closeMonth(MONTH);
    expect((await api.budget.view(MONTH)).closed).toBe(true);
    await expect(api.budget.save(MONTH, await api.budget.document(MONTH))).rejects.toThrow(/closed/);
  });

  it('is refused for a month that is already closed', async () => {
    await expect(createFakeApi().review.closeMonth('2026-10')).rejects.toThrow(/already closed/);
  });

  it('is taken back by its change, which reopens the month', async () => {
    const api = createFakeApi();
    await clearQueue(api);
    const closed = await api.review.closeMonth(MONTH);
    await api.changes.undo(closed.changeId);
    expect((await api.shell.overview()).closedMonths.map((month) => month.month)).toEqual(['2026-10']);
    expect((await api.review.closing(MONTH)).closed).toBe(false);
  });
});
