import type { BudgetCell, BudgetGroupKey, BudgetGroupView, BudgetLineView } from '@/api/types';
import { brl, longDay, signedBrl } from '@/lib/format';

// The words of the budget screen. What they say is worked out by the backend.

export type Columns = [string, string, string];

export const COLUMNS: Record<BudgetGroupKey, Columns> = {
  rec: ['Esperado', 'Recebido', ''],
  fix: ['Esperado', 'Gasto', 'Diferença'],
  var: ['Teto', 'Gasto', 'Resta'],
  prov: ['Aporte mensal', 'Saldo', 'Próxima cobrança'],
  res: ['Aporte mensal', 'Saldo', 'Gasto no mês'],
  rf: ['Alvo', 'Saldo', 'Aporte'],
  tar: ['Gasto', '', ''],
  inv: ['Planejado', 'Feito', 'Taxa de investimento'],
};

const SUBTOTAL_LABEL: Record<BudgetGroupKey, string> = {
  rec: 'Subtotal',
  fix: 'Subtotal',
  var: 'Subtotal',
  prov: 'Aporte mensal',
  res: 'Aporte mensal',
  rf: 'Saldo',
  tar: 'Gasto',
  inv: 'O que sobra da receita',
};

const MINUS = '−';
const NBSP = ' ';

/** The investment is the one amount that can fall below zero. */
const moneyText = (amount: number): string => (amount < 0 ? `${MINUS}${NBSP}${brl(amount)}` : brl(amount));

export function cellText(cell: BudgetCell): string {
  switch (cell.type) {
    case 'none':
      return '';
    case 'money':
      return moneyText(cell.amount);
    case 'difference':
      return signedBrl(cell.amount);
    case 'percent':
      return `${cell.value}%`;
    case 'next-charge':
      return cell.charge ? `${cell.charge.description} · ${longDay(cell.charge.day)}` : 'Sem data marcada';
    case 'rainy-deposit':
      return cell.switchedOn ? 'Religado' : 'Desligado';
  }
}

export const subtotalLabel = (group: BudgetGroupView): string => SUBTOTAL_LABEL[group.key];

export function subtotalText(group: BudgetGroupView): string {
  if (group.subtotalSpent === null) return moneyText(group.subtotal);
  return `${brl(group.subtotalSpent)} de ${brl(group.subtotal)}`;
}

/** The classes that colour and weigh one value of a row, by the group it is in. */
export function cellClasses(group: BudgetGroupKey, line: BudgetLineView, index: number): string[] {
  const cell = line.cells[index];
  if (index !== 2 || !cell) return [];
  if (group === 'fix') return line.over ? ['bad', 'strong'] : ['quiet'];
  if (group === 'var') return line.over ? ['bad'] : [];
  if (group === 'prov') return ['secondary'];
  if (group === 'rf') return cell.type === 'rainy-deposit' && cell.switchedOn ? ['warning', 'medium'] : ['secondary', 'medium'];
  if (group === 'inv') return line.over ? ['bad', 'strong'] : ['strong'];
  return [];
}
