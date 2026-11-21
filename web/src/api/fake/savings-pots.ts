import type { SavingsPotDetail, SavingsPotRow, SavingsPotsApi } from '../types';
import type { FakeSavingsPot, FakeState } from './model';
import {
  belowTarget,
  groupByBucket,
  investmentFigures,
  linkedLines,
  monthlyYield,
  potContribution,
  potMovements,
  yieldDays,
} from './savings-rules';

function toRow(pot: FakeSavingsPot): SavingsPotRow {
  return {
    id: pot.id,
    name: pot.name || null,
    bank: pot.bank,
    bucket: pot.bucket,
    balance: pot.balance,
    cdiPercent: pot.cdiPercent,
    monthlyYield: monthlyYield(pot),
    maturity: pot.maturity,
    target: pot.target === null ? null : { amount: pot.target, progress: Math.min(100, (pot.balance / pot.target) * 100) },
    closed: pot.closed,
  };
}

function toDetail(state: FakeState, pot: FakeSavingsPot): SavingsPotDetail {
  return {
    id: pot.id,
    name: pot.name || null,
    bank: pot.bank,
    bucket: pot.bucket,
    cdiPercent: pot.cdiPercent,
    balance: pot.balance,
    belowTarget: belowTarget(pot),
    contribution: potContribution(state, pot),
    linkedLines: linkedLines(pot),
    investment: investmentFigures(pot),
    movements: potMovements(state, pot),
    yield: { month: state.currentMonth, total: monthlyYield(pot), days: yieldDays(state, pot) },
    historyStart: state.currentMonth,
  };
}

export function createFakeSavingsPots(state: FakeState): SavingsPotsApi {
  return {
    async list({ includeClosed }) {
      const shown = state.savingsPots.filter((pot) => includeClosed || !pot.closed);
      return groupByBucket(shown).map(({ bucket, subtotal, pots }) => ({ bucket, subtotal, pots: pots.map(toRow) }));
    },
    async detail(id) {
      const pot = state.savingsPots.find((candidate) => candidate.id === id);
      return pot ? toDetail(state, pot) : null;
    },
  };
}
