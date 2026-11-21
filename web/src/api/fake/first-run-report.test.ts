import { describe, expect, it } from 'vitest';
import { createFakeApi } from '.';
import { emptyLedgerState, freshInstallState } from './first-run-data';

describe.each([
  ['a fresh install', freshInstallState],
  ['an empty ledger', emptyLedgerState],
])('on %s', (_name, build) => {
  it('writes no report for the closing, so no sentence about money is invented', async () => {
    const api = createFakeApi({ state: build(), startLocked: false });
    expect((await api.review.closing('2026-11')).report).toEqual([]);
  });
});
