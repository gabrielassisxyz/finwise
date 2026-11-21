import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';

async function detailOf(id: string) {
  const detail = await createFakeApi().transactions.detail(id);
  if (!detail) throw new Error(`no transaction ${id}`);
  return detail;
}

describe('detail', () => {
  it('is null for an unknown id', async () => {
    expect(await createFakeApi().transactions.detail('nope')).toBeNull();
  });

  it('carries what the list shows, nickname and effective amount included', async () => {
    const detail = await detailOf('t34');
    expect(detail).toMatchObject({ id: 't34', name: 'Aluguel', originalName: 'João P. Santos', effectiveAmount: -1500 });
  });

  it('names a card by its last digits and any other account by its name', async () => {
    expect((await detailOf('t2')).accountDescription).toBe('C6 Carbon · final 1234');
    expect((await detailOf('t34')).accountDescription).toBe('Itaú');
  });
});

describe('which sections apply', () => {
  const sectionsOf = async (id: string) => (await detailOf(id)).sections;

  it('gives a card purchase the establishment, the split and the reimbursement, and no refund', async () => {
    expect(await sectionsOf('t2')).toEqual({
      establishment: true,
      person: false,
      split: true,
      reimbursement: true,
      refund: false,
    });
  });

  it('gives a PIX to a person the nickname instead of the establishment', async () => {
    expect(await sectionsOf('t31')).toMatchObject({ establishment: false, person: true, split: true });
  });

  it('gives a credit the refund switch and neither the split nor the reimbursement', async () => {
    expect(await sectionsOf('t10')).toEqual({
      establishment: true,
      person: false,
      split: false,
      reimbursement: false,
      refund: true,
    });
  });

  it('gives a movement nothing but the sections every transaction has', async () => {
    const detail = await detailOf('t52');
    expect(detail.sections).toEqual({
      establishment: false,
      person: false,
      split: false,
      reimbursement: false,
      refund: false,
    });
    expect(detail.counterparty).toBeNull();
  });
});

describe('the model\'s justification', () => {
  it('is the pending review\'s own sentence', async () => {
    expect((await detailOf('t5')).justification).toBe('O nome sugere bar, mas a compra foi no horário do almoço.');
  });

  it('is the sentence of a classification the model made', async () => {
    expect((await detailOf('t4')).justification).toBe('Compras anteriores neste estabelecimento foram todas em Mercado.');
  });

  it('is absent when a rule or the user classified it', async () => {
    expect((await detailOf('t2')).justification).toBeNull();
    expect((await detailOf('t36')).justification).toBeNull();
  });
});

describe('the establishment', () => {
  it('shows its nickname, default line and fixed tags', async () => {
    expect((await detailOf('t16')).counterparty).toEqual({
      key: 'Pousada Serra Azul',
      nickname: null,
      defaultLine: null,
      fixedTags: ['#viagem'],
    });
    expect((await detailOf('t4')).counterparty).toMatchObject({ defaultLine: 'Mercado' });
  });

  it('keeps the transaction\'s own tags apart from the fixed ones, and suggests the rest', async () => {
    const detail = await detailOf('t20');
    expect(detail.ownTags).toEqual(['#com-ana']);
    expect(detail.suggestedTags).toEqual(['#viagem', '#trabalho', '#presente', '#casa-nova']);
    expect((await detailOf('t16')).ownTags).toEqual(['#viagem']);
  });
});

describe('the history', () => {
  it('opens with what put the transaction where it is', async () => {
    expect((await detailOf('t2')).history).toEqual([
      { id: 't2-h0', text: 'Regra classificou como Transporte', day: '2026-11-28', reason: null, undoable: false },
    ]);
    expect((await detailOf('t5')).history[0]?.text).toBe('IA sugeriu Restaurantes e bares (58%)');
    expect((await detailOf('t52')).history[0]?.text).toBe('Regra marcou como Movimentação');
  });

  it('lists the newest step first and offers to take back only the ones that changed something', async () => {
    const { history } = await detailOf('t36');
    expect(history.map((entry) => [entry.text, entry.day, entry.reason, entry.undoable])).toEqual([
      ['Você alterou para Mercado', '2026-11-04', 'é o mercado do bairro', true],
      ['IA classificou como Lazer', '2026-11-03', null, false],
    ]);
  });
});

describe('the details a PIX or a boleto carries', () => {
  it('keeps the original name of the counterparty beside the nickname', async () => {
    const detail = await detailOf('t34');
    expect(detail.originalName).toBe('João P. Santos');
    expect(detail.account).toBe('Itaú');
  });

  it('keeps the foreign purchase\'s original amount, tax and bill', async () => {
    expect(await detailOf('t7')).toMatchObject({ originalAmount: 'US$ 20,00', iof: 3.85, bill: 'Dez/26', posted: true });
    expect(await detailOf('t1')).toMatchObject({ posted: false, bill: 'Jan/27' });
  });
});

describe('the switches before anything was touched', () => {
  it('starts with no split, reimbursement or exclusion, and no refund line', async () => {
    expect(await detailOf('t2')).toMatchObject({
      splitParts: null,
      splitRemaining: 0,
      reimbursement: null,
      exclusion: null,
      note: '',
      askReason: false,
    });
    expect(await detailOf('t10')).toMatchObject({ refund: true, refundLine: null });
  });

  it('offers received PIX as reimbursement candidates, pay left out', async () => {
    const candidates = (await detailOf('t2')).reimbursementCandidates;
    expect(candidates.map((candidate) => candidate.transactionId)).toEqual(['t32', 't33']);
    expect(candidates[0]).toEqual({ transactionId: 't32', name: 'Ana', day: '2026-11-12', amount: 150 });
    expect((await detailOf('t10')).reimbursementCandidates).toEqual([]);
  });
});
