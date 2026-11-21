import type { HomeApi, HomeSummary, YearMonth } from '../types';
import type { FakeState, FakeTransaction } from './model';
import { payAheadOf } from './card-rules';
import { groupTotal, netWorth, pendingCount, spentByLine } from './rules';

/** The card is offered for paying ahead only while there is something to pay. */
function payAheadFigure(state: FakeState): HomeSummary['payAhead'] {
  const { amount } = payAheadOf(state);
  return state.payAhead && amount !== null && amount > 0 ? { amount, cardLabel: state.payAhead.cardLabel } : null;
}

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
        payAhead: payAheadFigure(state),
      };
    },
  };
}
