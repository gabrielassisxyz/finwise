import { describe, expect, it } from 'vitest';
import { createFakeApi } from '.';
import { emptyLedgerState, freshInstallState } from './first-run-data';

describe('the net worth chart', () => {
  it('draws no history on an empty ledger, because no balance was ever read', async () => {
    const { chart } = await createFakeApi({ state: emptyLedgerState(), startLocked: false }).netWorth.overview();
    expect(chart).toEqual({ max: 0, months: [], points: [] });
  });

  it('still draws the month of the balances the bank reported on a fresh install', async () => {
    const { chart } = await createFakeApi({ state: freshInstallState(), startLocked: false }).netWorth.overview();
    expect(chart.points.map((point) => point.month)).toEqual(['2026-11']);
    expect(chart.months).toHaveLength(6);
  });
});
