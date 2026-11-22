import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

const MONTH = '2026-11';

const answer = (line: string, extra: Partial<Parameters<ReturnType<typeof createFakeApi>['review']['acceptPending']>[1]> = {}) => ({
  line,
  always: false,
  nickname: '',
  reason: '',
  ...extra,
});

const queueNames = async (api = createFakeApi()) => (await api.review.weekly(MONTH)).queue.items.map((item) => item.name);
const transactionOf = async (api: ReturnType<typeof createFakeApi>, id: string) =>
  (await api.transactions.list({ month: MONTH, period: 'month' })).find((transaction) => transaction.id === id)!;

describe('the weekly summary', () => {
  it('reads the month limit and the current week from the same rules as the home screen', async () => {
    const api = createFakeApi();
    const weekly = await api.review.weekly(MONTH);
    const home = await api.home.summary(MONTH);
    expect(weekly.limit.cap).toBe(home.limit.cap);
    expect(weekly.limit.spent).toBe(home.limit.spent);
    expect(weekly.limit.remaining).toBe(1420);
    expect(weekly.limit.over).toBe(false);
    expect(weekly.week).toMatchObject({ guide: 355, spent: 210 });
    expect(weekly.week?.fill).toBeCloseTo(59.15, 1);
  });

  it('reports a limit that was passed as over, with a negative remainder and a full bar', async () => {
    const state = prototypeState();
    state.spentBaseline[MONTH]!['Mercado'] = 5000;
    const { limit } = await createFakeApi({ state }).review.weekly(MONTH);
    expect(limit.over).toBe(true);
    expect(limit.remaining).toBeLessThan(0);
    expect(limit.fill).toBe(100);
  });
});

describe('the queue of pending reviews', () => {
  it('lists the pending transactions newest first, with the model reason and doubt', async () => {
    const { queue } = await createFakeApi().review.weekly(MONTH);
    expect(queue.items.map((item) => item.name)).toEqual([
      'Bar do Zé',
      'Padaria Estrela',
      'Marcos A. Lima',
      'Uber',
      'Mercado Livre',
      'Farmácia São João',
      'Loja Verde Vida',
    ]);
    expect(queue.items[0]).toMatchObject({
      transactionId: 't5',
      day: '2026-11-27',
      accountLabel: 'C6 Carbon · final 1234',
      person: false,
      pending: { reason: 'Baixa confiança', suggestion: 'Restaurantes e bares', confidence: 58 },
    });
    expect(queue.resolved).toBe(0);
  });

  it('flags a person and names the account it was paid from', async () => {
    const { queue } = await createFakeApi().review.weekly(MONTH);
    const person = queue.items.find((item) => item.name === 'Marcos A. Lima');
    expect(person).toMatchObject({ person: true, accountLabel: 'Itaú' });
  });

  it('accepts the suggestion: the transaction takes the line, leaves the queue and counts as answered', async () => {
    const api = createFakeApi();
    const accepted = await api.review.acceptPending('t5', answer('Restaurantes e bares'));
    expect(accepted?.name).toBe('Bar do Zé');
    const weekly = await api.review.weekly(MONTH);
    expect(weekly.queue.items.map((item) => item.transactionId)).not.toContain('t5');
    expect(weekly.queue.resolved).toBe(1);
    expect(await transactionOf(api, 't5')).toMatchObject({ line: 'Restaurantes e bares', classifiedBy: 'Você', pending: null });
  });

  it('records "aceitou" for the suggestion and "alterou" for another line, with the reason typed', async () => {
    const api = createFakeApi();
    await api.review.acceptPending('t5', answer('Restaurantes e bares'));
    await api.review.acceptPending('t12', answer('Transporte para o trabalho', { reason: 'vai ao escritório' }));
    const first = await api.transactions.detail('t5');
    const second = await api.transactions.detail('t12');
    expect(first?.history.at(0)?.text).toBe('Você aceitou Restaurantes e bares');
    expect(second?.history.at(0)).toMatchObject({ text: 'Você alterou para Transporte para o trabalho', reason: 'vai ao escritório' });
    expect(second?.reason).toBe('vai ao escritório');
    expect(first?.reason).toBe('');
  });

  it('never hands out the same history step id twice for one transaction, whichever writer added the step', async () => {
    const api = createFakeApi();
    await api.review.acceptPending('t5', answer('Restaurantes e bares', { always: true }));
    await api.transactions.setRefund('t5', true);
    const steps = (await api.transactions.detail('t5'))!.history;
    expect(new Set(steps.map((step) => step.id)).size).toBe(steps.length);
    const refundStep = steps.find((step) => step.text === 'Você marcou como estorno')!;
    await api.transactions.undoHistoryStep('t5', refundStep.id);
    const after = (await api.transactions.detail('t5'))!.history.map((step) => step.text);
    expect(after).toContain('Você aceitou Restaurantes e bares');
    expect(after).not.toContain('Você marcou como estorno');
  });

  it('moves the spending of the line the transaction was put on', async () => {
    const api = createFakeApi();
    const before = (await api.budget.view(MONTH)).groups.flatMap((group) => group.lines).find((line) => line.name === 'Restaurantes e bares');
    await api.review.acceptPending('t5', answer('Restaurantes e bares'));
    const after = (await api.budget.view(MONTH)).groups.flatMap((group) => group.lines).find((line) => line.name === 'Restaurantes e bares');
    expect(before?.cells[1]).toEqual({ type: 'money', amount: 324.15 });
    expect(after?.cells[1]).toEqual({ type: 'money', amount: 388.15 });
  });

  it('ties the establishment to the line when asked, and says so in the history', async () => {
    const api = createFakeApi();
    await api.review.acceptPending('t5', answer('Restaurantes e bares', { always: true }));
    const detail = await api.transactions.detail('t5');
    expect(detail?.history.slice(0, 2).map((entry) => entry.text)).toEqual([
      'Regra criada: Bar do Zé sempre em Restaurantes e bares',
      'Você aceitou Restaurantes e bares',
    ]);
    expect(detail?.history.at(0)?.undoable).toBe(false);
    expect(detail?.counterparty?.defaultLine).toBe('Restaurantes e bares');
  });

  it('gives a person a nickname, and ignores a rule for it', async () => {
    const api = createFakeApi();
    const accepted = await api.review.acceptPending('t31', answer('Lazer', { always: true, nickname: ' Aluguel ' }));
    expect(accepted?.name).toBe('Aluguel');
    expect((await transactionOf(api, 't31')).name).toBe('Aluguel');
    expect((await api.transactions.detail('t31'))?.counterparty?.defaultLine ?? null).toBeNull();
  });

  it('ignores a nickname offered for an establishment', async () => {
    const api = createFakeApi();
    const accepted = await api.review.acceptPending('t5', answer('Restaurantes e bares', { nickname: 'Bar' }));
    expect(accepted?.name).toBe('Bar do Zé');
    expect((await transactionOf(api, 't5')).name).toBe('Bar do Zé');
  });

  it('is taken back whole by one undo: the line, the rule, the nickname and the counter', async () => {
    const api = createFakeApi();
    const before = await queueNames(api);
    const accepted = await api.review.acceptPending('t5', answer('Delivery', { always: true, reason: 'x' }));
    await api.changes.undo(accepted!.changeId);
    const weekly = await api.review.weekly(MONTH);
    expect(weekly.queue.items.map((item) => item.name)).toEqual(before);
    expect(weekly.queue.resolved).toBe(0);
    expect((await api.transactions.detail('t5'))?.counterparty?.defaultLine ?? null).toBeNull();
  });

  it('answers nothing for a transaction that is not waiting', async () => {
    const api = createFakeApi();
    expect(await api.review.acceptPending('t2', answer('Transporte'))).toBeNull();
    expect(await api.review.acceptPending('nope', answer('Transporte'))).toBeNull();
  });

  it('empties the queue after every review is answered', async () => {
    const api = createFakeApi();
    for (const item of (await api.review.weekly(MONTH)).queue.items) {
      await api.review.acceptPending(item.transactionId, answer(item.pending.suggestion!));
    }
    const { queue } = await api.review.weekly(MONTH);
    expect(queue.items).toEqual([]);
    expect(queue.resolved).toBe(7);
    expect((await api.shell.overview()).pendingCount).toBe(0);
  });
});

describe('the changes waiting for approval', () => {
  it('lists them in the order they arrived', async () => {
    const { approvals } = await createFakeApi().review.weekly(MONTH);
    expect(approvals.map((approval) => approval.id)).toEqual(['teto', 'estorno', 'revista']);
    expect(approvals[0]).toEqual({
      id: 'teto',
      title: 'Subir o teto de Delivery de R$ 200 para R$ 250',
      reason: 'estourou três meses seguidos',
    });
  });

  it('applies a cap for real, from the open month, leaving the closed one as it was', async () => {
    const api = createFakeApi();
    const capOf = async (month: string) =>
      (await api.budget.document(month)).flatMap((group) => group.lines).find((line) => line.name === 'Delivery')?.amount;
    await api.review.approve('teto');
    expect(await capOf(MONTH)).toBe(250);
    expect(await capOf('2026-10')).toBe(200);
    expect((await api.review.weekly(MONTH)).limit.cap).toBe(2930);
  });

  it('holds a cap from the next open month once the month is closed', async () => {
    const api = createFakeApi();
    await api.review.markStepDone(MONTH, 'leftover');
    for (const item of (await api.review.weekly(MONTH)).queue.items) {
      await api.review.acceptPending(item.transactionId, answer(item.pending.suggestion!));
    }
    await api.review.closeMonth(MONTH);
    await api.review.approve('teto');
    const cap = async (month: string) =>
      (await api.budget.document(month)).flatMap((group) => group.lines).find((line) => line.name === 'Delivery')?.amount;
    expect(await cap(MONTH)).toBe(200);
    expect(await cap('2026-12')).toBe(250);
  });

  it('marks a transaction as not counting, and leaves the totals of its line', async () => {
    const api = createFakeApi();
    expect((await transactionOf(api, 't10')).excluded).toBe(false);
    await api.review.approve('estorno');
    expect(await transactionOf(api, 't10')).toMatchObject({ excluded: true });
    const history = (await api.transactions.detail('t10'))?.history.at(0);
    expect(history?.text).toBe('IA marcou como não conta (você aprovou)');
  });

  it('changes the status of a subscription', async () => {
    const api = createFakeApi();
    expect((await api.subscriptions.detail('rv'))?.status).toBe('Analisar');
    const approved = await api.review.approve('revista');
    expect(approved?.title).toBe('Mudar status de Revista digital para Cancelar');
    expect((await api.subscriptions.detail('rv'))?.status).toBe('Cancelar');
  });

  it('takes the approved change out of the waiting list', async () => {
    const api = createFakeApi();
    await api.review.approve('estorno');
    expect((await api.review.weekly(MONTH)).approvals.map((approval) => approval.id)).toEqual(['teto', 'revista']);
    expect(await api.review.approve('estorno')).toBeNull();
  });

  it('takes an approval back whole: the effect and the place in the list', async () => {
    const api = createFakeApi();
    const approved = await api.review.approve('teto');
    await api.changes.undo(approved!.changeId);
    const delivery = (await api.budget.document(MONTH)).flatMap((group) => group.lines).find((line) => line.name === 'Delivery');
    expect(delivery?.amount).toBe(200);
    expect((await api.review.weekly(MONTH)).approvals.map((approval) => approval.id)).toContain('teto');
  });

  it('refuses without changing anything but the list', async () => {
    const api = createFakeApi();
    await api.review.refuse('revista');
    expect((await api.subscriptions.detail('rv'))?.status).toBe('Analisar');
    expect((await api.review.weekly(MONTH)).approvals.map((approval) => approval.id)).toEqual(['teto', 'estorno']);
  });

  it('knows nothing of an id that is not waiting', async () => {
    const api = createFakeApi();
    expect(await api.review.approve('lazer')).toBeNull();
    expect(await api.review.refuse('lazer')).toBeNull();
    expect(await api.review.approve('nope')).toBeNull();
  });

  it('puts a suggestion of the report in the list once, at the end', async () => {
    const api = createFakeApi();
    expect(await api.review.propose('lazer')).not.toBeNull();
    expect(await api.review.propose('lazer')).toBeNull();
    expect(await api.review.propose('nope')).toBeNull();
    expect((await api.review.weekly(MONTH)).approvals.map((approval) => approval.id)).toEqual(['teto', 'estorno', 'revista', 'lazer']);
  });
});

describe('the warnings', () => {
  const ids = async (api = createFakeApi()) => (await api.review.weekly(MONTH)).warnings.map((warning) => warning.id);

  it('lists them with the subscription each opens and the actions some carry', async () => {
    const { warnings } = await createFakeApi().review.weekly(MONTH);
    expect(warnings.map((warning) => [warning.id, warning.subscriptionId, warning.actions.map((action) => action.label)])).toEqual([
      ['preco', 'sp', []],
      ['nova', null, ['Sim', 'Não']],
      ['sobre', 'ai', ['Silenciar este par']],
      ['renov', 'ap', []],
      ['cobrou', null, []],
    ]);
  });

  it('dismisses one, and takes the dismissal back', async () => {
    const api = createFakeApi();
    const dismissed = await api.review.dismissWarning('preco');
    expect(await ids(api)).toEqual(['nova', 'sobre', 'renov', 'cobrou']);
    await api.changes.undo(dismissed!.changeId);
    expect(await ids(api)).toContain('preco');
  });

  it('creates the subscription for "Sim" and the warning goes with the suggestion', async () => {
    const api = createFakeApi();
    const result = await api.review.runWarningAction('nova', 'accept-suggestion');
    expect(result).toMatchObject({ outcome: 'subscription-created', name: 'Serviço de nuvem' });
    expect(await ids(api)).not.toContain('nova');
    expect((await api.subscriptions.list({ grouped: false })).groups[0]?.subscriptions.map((row) => row.name)).toContain('Serviço de nuvem');
    await api.changes.undo(result!.changeId);
    expect(await ids(api)).toContain('nova');
  });

  it('rejects the suggestion for "Não"', async () => {
    const api = createFakeApi();
    const result = await api.review.runWarningAction('nova', 'reject-suggestion');
    expect(result).toMatchObject({ outcome: 'suggestion-rejected', name: 'Serviço de nuvem' });
    expect(await ids(api)).not.toContain('nova');
    expect((await api.subscriptions.list({ grouped: false })).suggestion).toBeNull();
  });

  it('hides the question once the suggestion was answered on the subscriptions screen', async () => {
    const api = createFakeApi();
    await api.subscriptions.rejectSuggestion();
    expect(await ids(api)).not.toContain('nova');
  });

  it('silences an overlapping pair: the mark goes from the subscription and the warning from the list', async () => {
    const api = createFakeApi();
    const rowMarks = async () =>
      (await api.subscriptions.list({ grouped: false })).groups[0]!.subscriptions.find((row) => row.id === 'ai')!.marks.map((mark) => mark.text);
    expect(await rowMarks()).toContain('Sobreposta');
    const result = await api.review.runWarningAction('sobre', 'silence-pair');
    expect(result?.outcome).toBe('pair-silenced');
    expect(await rowMarks()).not.toContain('Sobreposta');
    expect(await ids(api)).not.toContain('sobre');
    await api.changes.undo(result!.changeId);
    expect(await rowMarks()).toContain('Sobreposta');
  });

  it('refuses an action the warning does not offer', async () => {
    const api = createFakeApi();
    expect(await api.review.runWarningAction('preco', 'silence-pair')).toBeNull();
    expect(await api.review.runWarningAction('sobre', 'accept-suggestion')).toBeNull();
  });
});
