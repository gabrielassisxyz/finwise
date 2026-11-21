import type {
  Bucket,
  InvestmentRate,
  MonthContribution,
  PotBelowTarget,
  PotContribution,
  PotInvestment,
  PotLinkedLine,
  PotMovement,
  PotYieldDay,
} from '../types';
import type { FakePlannedContribution, FakeSavingsPot, FakeState } from './model';
import { groupTotal, percentOfIncome, plannedInvestmentOf } from './rules';

// The rules behind the pots and the net-worth screens. The server will own
// these; the stand-in applies the same ones.

const cents = (amount: number): number => Math.round(amount * 100) / 100;

const sum = (amounts: number[]): number => amounts.reduce((total, amount) => total + amount, 0);

// The pots' yield for a month, as a share of the balance per point of the CDI.
// It is the prototype's own invented formula, not a market rate.
const YIELD_PER_CDI_POINT = 0.000105;

/** What an open pot earned this month; a closed pot earns nothing. */
export function monthlyYield(pot: FakeSavingsPot): number {
  return pot.closed ? 0 : cents(pot.balance * pot.cdiPercent * YIELD_PER_CDI_POINT);
}

// Days of the month the invented ledger holds a yield for: business days up to the last sync.
const YIELD_DAYS = [27, 26, 25, 24, 23, 19, 18, 17, 16, 13, 12, 11, 10, 9, 6, 5, 4, 3];

/** The month's yield spread over its days; the last day takes the rounding so the days add up to the total. */
export function yieldDays(state: FakeState, pot: FakeSavingsPot): PotYieldDay[] {
  const total = monthlyYield(pot);
  if (total === 0) return [];
  const base = total / YIELD_DAYS.length;
  let accumulated = 0;
  return YIELD_DAYS.map((day, index) => {
    const isLast = index === YIELD_DAYS.length - 1;
    const amount = isLast ? cents(total - accumulated) : cents(base * (1 + ((day * 7) % 5 - 2) * 0.03));
    accumulated = cents(accumulated + amount);
    return { day: `${state.currentMonth}-${String(day).padStart(2, '0')}`, amount };
  });
}

/** The planned deposit that belongs to a pot; every investment shares the one called "Investimento". */
export function plannedFor(state: FakeState, pot: FakeSavingsPot): FakePlannedContribution | undefined {
  return state.plannedContributions.find(
    (planned) => planned.name === pot.name || (pot.investment !== null && planned.name === 'Investimento'),
  );
}

export function potContribution(state: FakeState, pot: FakeSavingsPot): PotContribution {
  if (pot.closed) return { state: 'closed' };
  if (pot.target !== null && pot.balance >= pot.target) return { state: 'switched-off' };
  const planned = plannedFor(state, pot);
  if (!planned && pot.target === null) return { state: 'none-planned' };
  if (planned?.state === 'done' && planned.doneOn) return { state: 'done', doneOn: planned.doneOn };
  return { state: 'pending' };
}

export function belowTarget(pot: FakeSavingsPot): PotBelowTarget | null {
  if (pot.target === null || pot.balance >= pot.target) return null;
  return { target: pot.target, shortfall: cents(pot.target - pot.balance) };
}

export function linkedLines(pot: FakeSavingsPot): PotLinkedLine[] {
  if (pot.lineShares) return pot.lineShares.map(({ line, amount }) => ({ name: line, share: amount }));
  return pot.linkedLines.map((name) => ({ name, share: null }));
}

export function investmentFigures(pot: FakeSavingsPot): PotInvestment | null {
  if (!pot.investment) return null;
  return {
    applied: pot.investment.applied,
    gross: pot.balance,
    tax: pot.investment.tax,
    net: cents(pot.balance - pot.investment.tax),
    redeemableOn: pot.maturity,
  };
}

/** The month's deposit when it was made, then what the ledger already holds. */
export function potMovements(state: FakeState, pot: FakeSavingsPot): PotMovement[] {
  const planned = plannedFor(state, pot);
  const deposit: PotMovement[] =
    planned?.state === 'done' && planned.doneOn && planned.amount
      ? [{ day: planned.doneOn, description: 'Depósito · aporte do mês', amount: planned.amount }]
      : [];
  return [...deposit, ...pot.movements];
}

const BUCKET_ORDER: (Bucket | null)[] = ['Reservado', 'Investido', 'Disponível', null];

export function groupByBucket(pots: FakeSavingsPot[]): { bucket: Bucket | null; pots: FakeSavingsPot[]; subtotal: number }[] {
  return BUCKET_ORDER.map((bucket) => {
    const inBucket = pots.filter((pot) => pot.bucket === bucket);
    return { bucket, pots: inBucket, subtotal: cents(sum(inBucket.map((pot) => pot.balance))) };
  }).filter((group) => group.pots.length > 0);
}

export function plannedInvestment(state: FakeState): number {
  return plannedInvestmentOf(state.budget);
}

export function monthContribution(state: FakeState): MonthContribution {
  const planned = cents(groupTotal(state, 'prov') + groupTotal(state, 'res') + plannedInvestment(state));
  const done = cents(sum(state.plannedContributions.filter((item) => item.state === 'done').map((item) => item.amount)));
  return { planned, done, progress: planned > 0 ? Math.min(100, (done / planned) * 100) : 0 };
}

export function investmentRate(state: FakeState): InvestmentRate {
  const income = groupTotal(state, 'rec');
  const planned = plannedInvestment(state);
  const invested = state.plannedContributions.find((item) => item.name === 'Investimento')?.state === 'done';
  return {
    month: state.currentMonth,
    planned: percentOfIncome(income, planned),
    actual: percentOfIncome(income, invested ? planned : 0),
  };
}
