import type { ClosingStepId, ClosingStepState } from '@/api/types';
import { longDay, monthInSentence } from '@/lib/format';
import type { YearMonth } from '@/api/types';

export const STEP_TITLES: Record<ClosingStepId, string> = {
  pending: 'Pendências',
  'planned-actual': 'Orçado × realizado',
  leftover: 'Sobra do mês',
  contributions: 'Aportes',
  'card-bill': 'Fatura',
  subscriptions: 'Assinaturas a revisar',
  report: 'Relatório do mês',
  close: 'Fechar o mês',
};

export function stepStatus(state: ClosingStepState): string {
  if (state === 'done') return 'Feito';
  return state === 'pending' ? 'Pendente' : '';
}

/** "Novembro de 2026", the month as a sentence starts with it. */
export function capitalizedMonth(month: YearMonth): string {
  const text = monthInSentence(month);
  return text.charAt(0).toUpperCase() + text.slice(1);
}

export function closedOnText(month: YearMonth, closedOn: string | null): string {
  const day = closedOn ? longDay(closedOn) : '—';
  return `${capitalizedMonth(month)} foi fechado em ${day}. O budget deste mês está guardado e não muda mais.`;
}
