import { describe, expect, it } from 'vitest';
import { createFakeApi } from '.';
import { emptyLedgerState } from './first-run-data';

describe('the subscriptions of an empty ledger', () => {
  it('make no group, ungrouped or grouped', async () => {
    const api = createFakeApi({ state: emptyLedgerState(), startLocked: false });
    expect((await api.subscriptions.list({ grouped: false })).groups).toEqual([]);
    expect((await api.subscriptions.list({ grouped: true })).groups).toEqual([]);
  });
});
