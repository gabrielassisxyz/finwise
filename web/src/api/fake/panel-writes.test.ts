import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';

function setup() {
  const api = createFakeApi();
  const detail = async (id: string) => (await api.transactions.detail(id))!;
  const spent = async () => (await api.home.summary('2026-11')).limit.spent;
  const pendingCount = async () => (await api.shell.overview()).pendingCount;
  return { api, detail, spent, pendingCount };
}

const BASELINE_SPENT = 1460;

describe('confirming the model', () => {
  it('settles a pending review on its suggestion and starts counting it', async () => {
    const { api, detail, spent, pendingCount } = setup();
    await api.transactions.confirmLine('t5');
    expect(await detail('t5')).toMatchObject({
      line: 'Restaurantes e bares',
      classifiedBy: 'Você',
      confidence: null,
      pending: null,
      justification: null,
    });
    expect((await detail('t5')).history[0]?.text).toBe('Você confirmou Restaurantes e bares');
    expect(await pendingCount()).toBe(6);
    expect(await spent()).toBe(BASELINE_SPENT + 64);
  });

  it('turns the model\'s own classification into the user\'s, keeping the line', async () => {
    const { api, detail } = setup();
    await api.transactions.confirmLine('t4');
    expect(await detail('t4')).toMatchObject({ line: 'Mercado', classifiedBy: 'Você', belowThreshold: false });
  });

  it('does nothing for a transaction with neither a line nor a suggestion', async () => {
    const { api } = setup();
    expect(await api.transactions.confirmLine('t52')).toBeNull();
    expect(await api.transactions.confirmLine('missing')).toBeNull();
  });
});

describe('correcting the model', () => {
  it('records the step and asks why only when the model was corrected', async () => {
    const { api, detail } = setup();
    await api.transactions.setLine('t4', 'Casa');
    expect(await detail('t4')).toMatchObject({ askReason: true, reason: '' });
    expect((await detail('t4')).history[0]).toMatchObject({ text: 'Você alterou para Casa', day: '2026-11-28', undoable: true });

    await api.transactions.setLine('t2', 'Lazer');
    expect((await detail('t2')).askReason).toBe(false);
    await api.transactions.setLine('t5', 'Casa');
    expect((await detail('t5')).askReason).toBe(true);
  });

  it('keeps the answer to "why" on the step it explains', async () => {
    const { api, detail } = setup();
    await api.transactions.setLine('t4', 'Casa');
    await api.transactions.setReason('t4', 'é do apartamento');
    const after = await detail('t4');
    expect(after.reason).toBe('é do apartamento');
    expect(after.history[0]?.reason).toBe('é do apartamento');
    expect(after.history[1]?.reason).toBeNull();
  });

  it('takes the step back to the model\'s classification and stops asking why', async () => {
    const { api, detail, pendingCount } = setup();
    await api.transactions.setLine('t5', 'Casa');
    const [step] = (await detail('t5')).history;
    await api.transactions.undoHistoryStep('t5', step!.id);
    expect(await detail('t5')).toMatchObject({ line: null, classifiedBy: 'Regra', askReason: false });
    expect((await detail('t5')).pending).toMatchObject({ suggestion: 'Restaurantes e bares', confidence: 58 });
    expect((await detail('t5')).history).toHaveLength(1);
    expect(await pendingCount()).toBe(7);
  });

  it('writes a history step on every transaction an "all purchases" change reaches', async () => {
    const { api, detail } = setup();
    const result = await api.transactions.setLine('t4', 'Casa');
    await api.transactions.applyLineToEstablishment(result!.changeId);
    expect((await detail('t9')).history[0]).toMatchObject({
      text: 'Regra aplicou Casa (você pediu para todas em Pão de Açúcar)',
      undoable: true,
    });
    await api.transactions.undoHistoryStep('t9', (await detail('t9')).history[0]!.id);
    expect(await detail('t9')).toMatchObject({ line: 'Mercado', classifiedBy: 'Regra' });
  });
});

describe('the establishment', () => {
  it('takes a nickname for every transaction there, and a blank one clears it', async () => {
    const { api, detail } = setup();
    await api.transactions.setNickname('t4', '  Pão  ');
    expect((await detail('t9')).name).toBe('Pão');
    expect((await detail('t4')).counterparty?.nickname).toBe('Pão');
    await api.transactions.setNickname('t4', '   ');
    expect((await detail('t9')).name).toBe('Pão de Açúcar');
    expect((await detail('t4')).counterparty?.nickname).toBeNull();
  });

  it('sets the default line without touching the transactions', async () => {
    const { api, detail } = setup();
    await api.transactions.setDefaultLine('t16', 'Lazer');
    expect((await detail('t16')).counterparty?.defaultLine).toBe('Lazer');
    expect((await detail('t16')).line).toBe('Viagens');
  });

  it('adds and removes fixed tags, which then show on every transaction there', async () => {
    const { api, detail } = setup();
    await api.transactions.addFixedTag('t4', '#casa-nova');
    await api.transactions.addFixedTag('t4', '#casa-nova');
    expect((await detail('t9')).tags).toEqual(['#casa-nova']);
    expect((await detail('t4')).counterparty?.fixedTags).toEqual(['#casa-nova']);
    await api.transactions.removeFixedTag('t4', '#casa-nova');
    expect((await detail('t9')).tags).toEqual([]);
  });

  it('gives a person the same nickname for every PIX with them', async () => {
    const { api, detail } = setup();
    await api.transactions.setNickname('t31', 'Marcos');
    expect((await detail('t31')).name).toBe('Marcos');
  });
});

describe('tags and the note', () => {
  it('stores a tag with its # and hyphens, once', async () => {
    const { api, detail } = setup();
    await api.transactions.addTag('t2', ' viagem de trabalho ');
    await api.transactions.addTag('t2', '#viagem-de-trabalho');
    await api.transactions.addTag('t2', '#');
    expect((await detail('t2')).ownTags).toEqual(['#viagem-de-trabalho']);
  });

  it('stops suggesting a tag the transaction has, and removes it again', async () => {
    const { api, detail } = setup();
    await api.transactions.addTag('t2', '#trabalho');
    expect((await detail('t2')).suggestedTags).not.toContain('#trabalho');
    await api.transactions.removeTag('t2', '#trabalho');
    expect((await detail('t2')).suggestedTags).toContain('#trabalho');
    expect((await detail('t2')).ownTags).toEqual([]);
  });

  it('keeps the note', async () => {
    const { api, detail } = setup();
    await api.transactions.setNote('t2', 'abastecer na sexta');
    expect((await detail('t2')).note).toBe('abastecer na sexta');
  });
});

describe('splitting', () => {
  it('says what a split still lacks, when it passes and when it closes', async () => {
    const { api, detail } = setup();
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 100 }, { line: null, amount: 0 }]);
    expect(await detail('t2')).toMatchObject({ split: true, splitRemaining: 80 });
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 100 }, { line: 'Lazer', amount: 90 }]);
    expect((await detail('t2')).splitRemaining).toBe(-10);
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 100 }, { line: 'Lazer', amount: 80 }]);
    expect((await detail('t2')).splitRemaining).toBe(0);
  });

  it('sends each part to its own line and drops a part without one', async () => {
    const { api, spent } = setup();
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 100 }, { line: 'Saúde', amount: 80 }]);
    expect(await spent()).toBe(BASELINE_SPENT - 80);
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 100 }, { line: null, amount: 80 }]);
    expect(await spent()).toBe(BASELINE_SPENT - 80);
    await api.transactions.setSplit('t2', null);
    expect(await spent()).toBe(BASELINE_SPENT);
  });

  it('takes the split away', async () => {
    const { api, detail } = setup();
    await api.transactions.setSplit('t2', [{ line: 'Transporte', amount: 180 }, { line: null, amount: 0 }]);
    await api.transactions.setSplit('t2', null);
    expect(await detail('t2')).toMatchObject({ split: false, splitParts: null, splitRemaining: 0 });
  });
});

describe('a reimbursable purchase', () => {
  it('expects half of it back until told otherwise', async () => {
    const { api, detail } = setup();
    await api.transactions.setReimbursable('t2', true);
    expect((await detail('t2')).reimbursement).toEqual({ expected: 90, link: null });
    await api.transactions.setExpectedReimbursement('t2', 120);
    expect((await detail('t2')).reimbursement?.expected).toBe(120);
    await api.transactions.setReimbursable('t2', false);
    expect((await detail('t2')).reimbursement).toBeNull();
  });

  it('counts for what is left once a received PIX is linked, and says so in the history', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setReimbursable('t2', true);
    await api.transactions.linkReimbursement('t2', 't32');
    const linked = await detail('t2');
    expect(linked.reimbursement?.link).toEqual({ transactionId: 't32', name: 'Ana', day: '2026-11-12', amount: 150 });
    expect(linked).toMatchObject({ effectiveAmount: -30, reimbursed: true });
    expect(linked.history[0]?.text).toBe('Você ligou ao PIX de Ana · R$ 150,00');
    expect(await spent()).toBe(BASELINE_SPENT - 150);
  });

  it('offers a received PIX to one purchase only', async () => {
    const { api, detail } = setup();
    await api.transactions.setReimbursable('t2', true);
    await api.transactions.linkReimbursement('t2', 't32');
    expect((await detail('t3')).reimbursementCandidates.map((c) => c.transactionId)).toEqual(['t33']);
    await api.transactions.linkReimbursement('t3', 't32');
    expect((await detail('t3')).reimbursement).toBeNull();
  });

  it('counts for the full amount again once the link is undone, or the switch turned off', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setReimbursable('t2', true);
    await api.transactions.linkReimbursement('t2', 't32');
    await api.transactions.unlinkReimbursement('t2');
    expect(await spent()).toBe(BASELINE_SPENT);
    expect((await detail('t2')).history[0]?.text).toBe('Você desfez a ligação com o PIX');
    expect((await detail('t2')).reimbursement).toEqual({ expected: 90, link: null });

    await api.transactions.linkReimbursement('t2', 't32');
    await api.transactions.setReimbursable('t2', false);
    expect(await spent()).toBe(BASELINE_SPENT);
    expect((await detail('t3')).reimbursementCandidates.map((c) => c.transactionId)).toContain('t32');
  });

  it('puts the link back when its step is taken back', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setReimbursable('t2', true);
    await api.transactions.linkReimbursement('t2', 't32');
    await api.transactions.undoHistoryStep('t2', (await detail('t2')).history[0]!.id);
    expect((await detail('t2')).reimbursement).toEqual({ expected: 90, link: null });
    expect(await spent()).toBe(BASELINE_SPENT);
  });
});

describe('"não conta"', () => {
  it('keeps counting while no reason was given', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setExcluded('t2', true);
    expect(await detail('t2')).toMatchObject({ exclusion: { reason: '' }, excluded: false, countsInTotals: true });
    expect(await spent()).toBe(BASELINE_SPENT);
    await api.transactions.setExclusionReason('t2', '   ');
    expect(await spent()).toBe(BASELINE_SPENT);
  });

  it('leaves the totals once a reason is given', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setExcluded('t2', true);
    await api.transactions.setExclusionReason('t2', 'Compra da empresa');
    expect(await detail('t2')).toMatchObject({ exclusion: { reason: 'Compra da empresa' }, excluded: true, countsInTotals: false });
    expect(await spent()).toBe(BASELINE_SPENT - 180);
  });

  it('counts again when turned off, and the step can be taken back', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setExcluded('t2', true);
    await api.transactions.setExclusionReason('t2', 'Compra da empresa');
    await api.transactions.setExcluded('t2', false);
    expect(await spent()).toBe(BASELINE_SPENT);
    expect((await detail('t2')).history[0]?.text).toBe('Você voltou a contar esta transação');
    expect((await detail('t2')).exclusion).toBeNull();

    await api.transactions.undoHistoryStep('t2', (await detail('t2')).history[0]!.id);
    expect((await detail('t2')).exclusion).toEqual({ reason: 'Compra da empresa' });
    expect(await spent()).toBe(BASELINE_SPENT - 180);
  });
});

describe('a refund', () => {
  it('reduces the line it points at, once marked', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setRefund('t33', true);
    expect(await spent()).toBe(BASELINE_SPENT);
    await api.transactions.setRefundLine('t33', 'Mercado');
    expect(await spent()).toBe(BASELINE_SPENT - 200);
    expect((await detail('t33')).history.map((entry) => entry.text)).toEqual([
      'Você definiu que o estorno abate Mercado',
      'Você marcou como estorno',
      'Regra classificou como Renda extra',
    ]);
  });

  it('stops reducing it when unmarked, and puts the mark back when its step is taken back', async () => {
    const { api, detail, spent } = setup();
    await api.transactions.setRefund('t33', true);
    await api.transactions.setRefundLine('t33', 'Mercado');
    await api.transactions.setRefund('t33', false);
    expect(await spent()).toBe(BASELINE_SPENT);
    expect((await detail('t33')).history[0]?.text).toBe('Você desmarcou o estorno');

    await api.transactions.undoHistoryStep('t33', (await detail('t33')).history[0]!.id);
    expect((await detail('t33')).refund).toBe(true);
    expect(await spent()).toBe(BASELINE_SPENT - 200);
  });

  it('can take the line of an existing refund back to none', async () => {
    const { api, detail } = setup();
    await api.transactions.setRefundLine('t10', 'Tarifas e juros');
    await api.transactions.undoHistoryStep('t10', (await detail('t10')).history[0]!.id);
    expect((await detail('t10')).refundLine).toBeNull();
  });
});

describe('taking a step back', () => {
  it('does not touch what was done after it', async () => {
    const { api, detail } = setup();
    await api.transactions.setRefund('t33', true);
    await api.transactions.setRefundLine('t33', 'Mercado');
    const steps = (await detail('t33')).history;
    await api.transactions.undoHistoryStep('t33', steps[1]!.id);
    expect(await detail('t33')).toMatchObject({ refund: false, refundLine: 'Mercado' });
    expect((await detail('t33')).history).toHaveLength(2);
  });

  it('refuses a step that only records what happened, and an unknown one', async () => {
    const { api, detail } = setup();
    expect(await api.transactions.undoHistoryStep('t2', (await detail('t2')).history[0]!.id)).toBeNull();
    expect(await api.transactions.undoHistoryStep('t2', 'nope')).toBeNull();
    expect(await api.transactions.undoHistoryStep('missing', 'nope')).toBeNull();
  });

  it('can itself be undone', async () => {
    const { api, detail } = setup();
    await api.transactions.setRefund('t33', true);
    const result = await api.transactions.undoHistoryStep('t33', (await detail('t33')).history[0]!.id);
    expect((await detail('t33')).refund).toBe(false);
    await api.changes.undo(result!.changeId);
    expect((await detail('t33')).refund).toBe(true);
  });
});

describe('every write', () => {
  it('answers with a change that takes it back', async () => {
    const { api, detail } = setup();
    const { changeId } = await api.transactions.addTag('t2', '#trabalho');
    expect((await detail('t2')).ownTags).toEqual(['#trabalho']);
    await api.changes.undo(changeId);
    expect((await detail('t2')).ownTags).toEqual([]);
  });
});
