import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import { holdFirstCall } from '@/test/holdCall';
import { withStores } from '@/test/mount';
import { useMonthStore } from './month';
import { useRevisionStore } from './revision';
import { useShellStore } from './shell';

function stateReadyToClose() {
  const state = prototypeState();
  for (const transaction of state.transactions) {
    if (transaction.pending) {
      Object.assign(transaction, { line: transaction.pending.suggestion ?? null, by: 'Você', pending: null });
    }
  }
  return state;
}

describe('shell store', () => {
  it('drops a late initial load that answers after a revision reload, keeping closed-month state aligned', async () => {
    const { result: stores, api } = withStores(
      () => ({ shell: useShellStore(), month: useMonthStore(), revision: useRevisionStore() }),
      { state: stateReadyToClose() },
    );
    await stores.shell.load();
    expect(stores.month.selected).toBe('2026-11');
    expect(stores.month.isClosed).toBe(false);

    const first = holdFirstCall(api.shell, 'overview');
    const initial = stores.shell.load();
    await flushPromises();

    await api.review.closeMonth('2026-11');
    stores.revision.bump();
    await flushPromises();
    expect(stores.month.isClosed).toBe(true);
    expect(stores.shell.overview!.closedMonths.map((closed) => closed.month)).toContain('2026-11');

    first.release();
    await initial;
    await flushPromises();
    expect(stores.month.isClosed).toBe(true);
    expect(stores.shell.overview!.closedMonths.map((closed) => closed.month)).toContain('2026-11');
  });

  it('does not let an earlier revision reload replace a later one', async () => {
    const { result: stores, api } = withStores(() => ({
      shell: useShellStore(),
      month: useMonthStore(),
      revision: useRevisionStore(),
    }));
    await stores.shell.load();
    expect(stores.shell.overview!.pendingCount).toBe(7);

    const first = holdFirstCall(api.shell, 'overview');
    stores.revision.bump();
    await flushPromises();

    await api.transactions.setLine('t5', 'Restaurantes e bares');
    stores.revision.bump();
    await flushPromises();
    expect(stores.shell.overview!.pendingCount).toBe(6);

    first.release();
    await flushPromises();
    expect(stores.shell.overview!.pendingCount).toBe(6);
  });
});
