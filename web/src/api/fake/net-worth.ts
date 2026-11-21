import type { Bucket, NetWorthApi, NetWorthBucket, NetWorthItem, NetWorthOverview, YearMonth } from '../types';
import type { FakeState } from './model';
import { netWorth } from './rules';
import { investmentRate, monthContribution, monthlyYield } from './savings-rules';

const cents = (amount: number): number => Math.round(amount * 100) / 100;

const sum = (amounts: number[]): number => amounts.reduce((total, amount) => total + amount, 0);

// The chart's top, and how many months it has room for, one of them before the history starts.
const CHART_MAX = 100_000;
const CHART_MONTHS = 6;

function addMonths(month: YearMonth, delta: number): YearMonth {
  const index = Number(month.slice(0, 4)) * 12 + Number(month.slice(5, 7)) - 1 + delta;
  return `${Math.floor(index / 12)}-${String((index % 12) + 1).padStart(2, '0')}`;
}

/** What makes a bucket up: bank accounts for the available one, then the pots in it. */
function itemsOf(state: FakeState, bucket: Bucket): NetWorthItem[] {
  const accounts: NetWorthItem[] =
    bucket === 'Disponível'
      ? state.accounts
          .filter((account) => account.kind === 'Conta')
          .map((account) => ({ name: `${account.bank} · ${account.name}`, detail: 'conta', amount: account.balance }))
      : [];
  const pots = state.savingsPots
    .filter((pot) => pot.bucket === bucket && !pot.closed)
    .map((pot) => ({ name: pot.name || null, detail: pot.bank, amount: pot.balance }));
  return [...accounts, ...pots];
}

export function createFakeNetWorth(state: FakeState): NetWorthApi {
  return {
    async overview(): Promise<NetWorthOverview> {
      const { available, reserved, invested } = netWorth(state);
      const totals: [Bucket, number][] = [
        ['Disponível', available],
        ['Reservado', reserved],
        ['Investido', invested],
      ];
      const buckets: NetWorthBucket[] = totals.map(([bucket, total]) => ({
        bucket,
        total,
        items: itemsOf(state, bucket),
      }));
      const total = cents(available + reserved + invested);
      const hasReadings = state.accounts.length + state.savingsPots.length > 0;
      const openPots = state.savingsPots.filter((pot) => !pot.closed);
      const knownYields = openPots.map(monthlyYield).filter((amount): amount is number => amount !== null);
      return {
        total,
        buckets,
        chart: {
          max: total === 0 ? 0 : CHART_MAX,
          months: hasReadings ? Array.from({ length: CHART_MONTHS }, (_, index) => addMonths(state.currentMonth, index - 1)) : [],
          points: hasReadings ? [{ month: state.currentMonth, available, reserved, invested, total }] : [],
        },
        historyStart: state.currentMonth,
        contribution: monthContribution(state),
        investmentRate: investmentRate(state),
        yields: {
          pots: openPots.map((pot) => ({
            name: pot.name || null,
            cdiPercent: pot.cdiPercent,
            amount: monthlyYield(pot),
          })),
          total: knownYields.length ? cents(sum(knownYields)) : null,
        },
      };
    },
  };
}
