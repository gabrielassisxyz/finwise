import type { IsoDay, YearMonth } from '@/api/types';

const MONTH_NAMES = [
  'janeiro',
  'fevereiro',
  'março',
  'abril',
  'maio',
  'junho',
  'julho',
  'agosto',
  'setembro',
  'outubro',
  'novembro',
  'dezembro',
];

function monthName(month: YearMonth): string {
  return MONTH_NAMES[Number(month.slice(5, 7)) - 1] ?? '';
}

/** "novembro 2026", the form the month selector shows. */
export function monthLabel(month: YearMonth): string {
  return `${monthName(month)} ${month.slice(0, 4)}`;
}

/** "novembro de 2026", the form used inside sentences. */
export function monthInSentence(month: YearMonth): string {
  return `${monthName(month)} de ${month.slice(0, 4)}`;
}

export function shiftMonth(month: YearMonth, delta: number): YearMonth {
  const index = Number(month.slice(0, 4)) * 12 + Number(month.slice(5, 7)) - 1 + delta;
  const year = Math.floor(index / 12);
  return `${year}-${String((index % 12) + 1).padStart(2, '0')}`;
}

/** "28/11" */
export function shortDay(day: IsoDay): string {
  return `${day.slice(8, 10)}/${day.slice(5, 7)}`;
}

/** "28/11/2026" */
export function longDay(day: IsoDay): string {
  return day.split('-').reverse().join('/');
}

const NBSP = ' ';
const MINUS = '−';

/** "R$ 1.234,56", unsigned, with a non-breaking space after the symbol. */
export function brl(amount: number): string {
  const digits = Math.abs(amount).toLocaleString('pt-BR', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return `R$${NBSP}${digits}`;
}

/** "+ R$ 10,00" for money in, "− R$ 10,00" for money out. */
export function signedBrl(amount: number): string {
  if (amount === 0) return brl(amount);
  return `${amount < 0 ? MINUS : '+'}${NBSP}${brl(amount)}`;
}
