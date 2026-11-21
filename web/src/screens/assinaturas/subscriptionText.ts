import type { CadenceUnit, RecurringPurchase, SubscriptionRow, SubscriptionStatus } from '@/api/types';
import type { MarkTone } from '@/components/transactions/marks';
import { brl } from '@/lib/format';
import { plural } from '@/lib/text';

const UNITS: Record<CadenceUnit, [string, string]> = {
  days: ['dia', 'dias'],
  weeks: ['semana', 'semanas'],
  months: ['mês', 'meses'],
};

/** "A cada 3 semanas" */
export function cadenceText({ every, unit }: RecurringPurchase['cadence']): string {
  const [one, many] = UNITS[unit];
  return `A cada ${plural(every, one, many)}`;
}

/** What sits under the name when the cycle, equivalent and line columns are not shown. */
export function rowSubtitle(row: SubscriptionRow): string {
  return `${row.cycle} · ${brl(row.monthlyEquivalent)} por mês · ${row.line}`;
}

export function statusTone(status: SubscriptionStatus): MarkTone {
  if (status === 'Cancelar') return 'danger';
  return status === 'Manter' ? 'neutral' : 'warning';
}

export const STATUSES: SubscriptionStatus[] = ['Manter', 'Analisar', 'Backup e cancelar', 'Cancelar'];
