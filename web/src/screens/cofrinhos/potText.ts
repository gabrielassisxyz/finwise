import type { PotContribution, PotLinkedLine, SavingsPotDetail } from '@/api/types';
import { brl, shortDay } from '@/lib/format';

export function contributionText(contribution: PotContribution): string {
  switch (contribution.state) {
    case 'closed':
      return 'Encerrado';
    case 'switched-off':
      return 'Desligado: está no alvo';
    case 'none-planned':
      return 'Sem aporte planejado';
    case 'done':
      return `Feito em ${shortDay(contribution.doneOn)}`;
    case 'pending':
      return 'Ainda não feito';
  }
}

export function linkedLinesText(lines: PotLinkedLine[]): string {
  if (lines.length === 0) return 'Nenhuma linha ligada';
  return lines.map(({ name, share }) => (share === null ? name : `${name} ${brl(share)}`)).join(' · ');
}

export function potSubtitle(pot: Pick<SavingsPotDetail, 'bank' | 'bucket' | 'cdiPercent'>): string {
  const rate = pot.cdiPercent === null ? [] : [`${pot.cdiPercent}% do CDI`];
  return [pot.bank, pot.bucket ?? 'Sem balde', ...rate].join(' · ');
}

/** A day inside the year the page is about shows without the year. */
export function movementDay(day: string, currentYear: string): string {
  const [year, month, date] = day.split('-');
  return year === currentYear ? `${date}/${month}` : `${date}/${month}/${year}`;
}
