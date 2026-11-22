import { describe, expect, it } from 'vitest';
import { createFakeApi } from '.';
import { emptyLedgerState, freshInstallState } from './first-run-data';

describe.each([
  ['a fresh install', freshInstallState],
  ['an empty ledger', emptyLedgerState],
])('on %s', (_name, build) => {
  it('offers no budget line to move a transaction to, the fees line included', async () => {
    const api = createFakeApi({ state: build(), startLocked: false });
    expect((await api.transactions.filterOptions('2026-11')).lines).toEqual([]);
  });
});
