import type { Transaction } from '@/api/types';

export type MarkTone = 'neutral' | 'warning' | 'positive' | 'bad' | 'tag';

export interface Mark {
  text: string;
  tone: MarkTone;
}

/** The badges beside a transaction's name, in the order they are read. */
export function marksOf(transaction: Transaction): Mark[] {
  const marks: Mark[] = [];
  const add = (text: string, tone: MarkTone) => marks.push({ text, tone });

  if (!transaction.posted) add('Pendente', 'warning');
  if (transaction.pending) add(`IA ${transaction.pending.confidence}%`, 'warning');
  else if (transaction.belowThreshold) add(`IA ${transaction.confidence}%`, 'warning');
  if (transaction.refund) add('Estorno', 'positive');
  if (transaction.reimbursementIncome) add('Reembolso', 'positive');
  if (transaction.reimbursed) add('Reembolsado', 'positive');
  if (transaction.excluded) add('Não conta', 'neutral');
  if (transaction.movement) add('Movimentação', 'neutral');
  if (transaction.originalAmount) add('Internacional', 'neutral');
  if (transaction.split) add('Dividida', 'neutral');
  for (const tag of transaction.tags) add(tag, 'tag');
  return marks;
}
