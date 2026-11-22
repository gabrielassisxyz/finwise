import type { HistoryEntry, TransactionDetail } from '@/api/types';
import { brl, shortDay } from '@/lib/format';

const NBSP = '\u00a0';

export function signedAmount(amount: number, shown: number): string {
  return (amount > 0 ? `+${NBSP}` : '') + brl(shown);
}

/** The line the pill names: the model's suggestion stands in while a review is pending. */
export function lineLabel(detail: TransactionDetail): string {
  return detail.line ?? detail.pending?.suggestion ?? 'Sem linha';
}

/** Who is behind the line, in the sentence that follows its name; nothing while nobody has classified it. */
export function originSentence(detail: TransactionDetail): string | null {
  if (detail.movement && !detail.line) return 'movimentação não entra no budget';
  if (detail.pending) {
    return detail.pending.confidence === null ? detail.pending.reason : `sugestão da IA, ${detail.pending.confidence}% de confiança`;
  }
  if (detail.classifiedBy === null) return null;
  if (detail.classifiedBy === 'IA') return `IA, ${detail.confidence}% de confiança`;
  return `definida por ${detail.classifiedBy}`;
}

/** Whether the panel has a line to confirm: the suggestion of a pending review, or a line the model set. */
export function hasLineToConfirm(detail: TransactionDetail): boolean {
  if (detail.pending) return detail.pending.suggestion !== null;
  return detail.classifiedBy === 'IA';
}

/** The key and value pairs of the panel's details, by the way the money moved. */
export function detailRows(detail: TransactionDetail): [label: string, value: string][] {
  if (detail.kind === 'cartao') {
    const rows: [string, string][] = [
      ['Cartão', detail.accountDescription],
      ['Fatura', detail.bill ?? '—'],
      ['Parcela', detail.installment ?? 'À vista'],
    ];
    if (detail.originalAmount) rows.push(['Valor original', detail.originalAmount], ['IOF', brl(detail.iof ?? 0)]);
    if (!detail.posted) rows.push(['Situação', 'Ainda não lançada pelo banco']);
    return rows;
  }
  if (detail.kind === 'boleto') return [['Descrição do boleto', detail.rawDescription], ['Conta', detail.account]];
  if (detail.kind === 'pix') return [['Contraparte', detail.originalName], ['Conta', detail.account]];
  return [['Tipo', detail.subtype ?? ''], ['Conta', detail.account]];
}

export function historyText(entry: HistoryEntry): string {
  return `${entry.text} · ${shortDay(entry.day)}${entry.reason ? ` · "${entry.reason}"` : ''}`;
}
