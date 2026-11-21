import type { HomeApi, HomeSummary, YearMonth } from '../types';
import type { FakeState, FakeTransaction } from './model';
import { groupTotal, netWorth, pendingCount, spentByLine } from './rules';

export function createFakeHome(state: FakeState, original: FakeTransaction[]): HomeApi {
  return {
    async summary(month: YearMonth): Promise<HomeSummary> {
      const spent = spentByLine(state, original, month);
      const variable = state.budget.find((group) => group.key === 'var')?.lines ?? [];
      const spentOnVariable = variable.reduce((total, line) => total + (spent[line.name] ?? 0), 0);
      return {
        limit: { cap: groupTotal(state, 'var'), spent: Math.round(spentOnVariable * 100) / 100 },
        weeks: structuredClone(state.weeks),
        netWorth: netWorth(state),
        pendingCount: pendingCount(state),
        payAhead: state.payAhead ? { ...state.payAhead } : null,
      };
    },
  };
}
