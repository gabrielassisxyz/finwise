import type { YearMonth } from '@/api/types';
import { monthInSentence } from '@/lib/format';

/** "Novembro de 2026", the name of a bill by the month it falls due in. */
export function billTitle(month: YearMonth): string {
  const text = monthInSentence(month);
  return text.charAt(0).toUpperCase() + text.slice(1);
}
