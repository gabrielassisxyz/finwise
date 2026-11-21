import { describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { detailRows, historyText, lineLabel, originSentence } from './panelText';

async function detailOf(id: string) {
  const detail = await createFakeApi().transactions.detail(id);
  if (!detail) throw new Error(`no transaction ${id}`);
  return detail;
}

describe('originSentence', () => {
  it('says who put the transaction on its line', async () => {
    expect(originSentence(await detailOf('t2'))).toBe('definida por Regra');
    expect(originSentence(await detailOf('t36'))).toBe('definida por Você');
    expect(originSentence(await detailOf('t4'))).toBe('IA, 62% de confiança');
    expect(originSentence(await detailOf('t5'))).toBe('sugestão da IA, 58% de confiança');
    expect(originSentence(await detailOf('t52'))).toBe('movimentação não entra no budget');
  });
});

describe('lineLabel', () => {
  it('names the suggestion while a review is pending', async () => {
    expect(lineLabel(await detailOf('t5'))).toBe('Restaurantes e bares');
    expect(lineLabel(await detailOf('t2'))).toBe('Transporte');
    expect(lineLabel(await detailOf('t52'))).toBe('Sem linha');
  });
});

describe('detailRows', () => {
  it('describes a card purchase by its card, bill and installment', async () => {
    expect(detailRows(await detailOf('t6'))).toEqual([
      ['Cartão', 'C6 Carbon · final 1234'],
      ['Fatura', 'Dez/26'],
      ['Parcela', '2/3'],
    ]);
    expect(detailRows(await detailOf('t2'))[2]).toEqual(['Parcela', 'À vista']);
  });

  it('adds the original amount and tax of a foreign purchase, and says when the bank has not posted it', async () => {
    expect(detailRows(await detailOf('t7')).slice(3)).toEqual([
      ['Valor original', 'US$ 20,00'],
      ['IOF', 'R$\u00a03,85'],
    ]);
    expect(detailRows(await detailOf('t1')).at(-1)).toEqual(['Situação', 'Ainda não lançada pelo banco']);
  });

  it('describes a boleto, a PIX and everything else', async () => {
    expect(detailRows(await detailOf('t40'))).toEqual([
      ['Descrição do boleto', 'VELOX TELECOM S.A.'],
      ['Conta', 'Itaú'],
    ]);
    expect(detailRows(await detailOf('t34'))).toEqual([
      ['Contraparte', 'João P. Santos'],
      ['Conta', 'Itaú'],
    ]);
    expect(detailRows(await detailOf('t52'))).toEqual([
      ['Tipo', 'TEF'],
      ['Conta', 'Itaú'],
    ]);
  });
});

describe('historyText', () => {
  it('ends with the day and the reason the user gave', async () => {
    const [newest, oldest] = (await detailOf('t36')).history;
    expect(historyText(newest!)).toBe('Você alterou para Mercado · 04/11 · "é o mercado do bairro"');
    expect(historyText(oldest!)).toBe('IA classificou como Lazer · 03/11');
  });
});
