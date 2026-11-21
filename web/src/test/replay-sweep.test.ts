import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { RECORDED_LEDGERS, replayApi } from './replay';
import { createSweep, declaredRoutePatterns, idsOf, pathsOf, type AbsentIds } from './sweep';

// The ids the Go side records as "nobody has this", so a page for a missing id is opened too.
const ABSENT: AbsentIds = { pot: 'no-such-pot', account: 'no-such-account', subscription: 'sub-1' };
const PATHS_OF = pathsOf(ABSENT);

// The one screen that asks the backend to evaluate a draft as soon as it opens; the read-only backend refuses
// that and the app answers with its read-only toast. A refusal anywhere else is a screen writing on its own.
const OPENS_WITH_A_REFUSED_EVALUATION = '/budget/cenarios';

describe('every route of the router is covered', () => {
  it('has a path table entry for each route, and no entry for a route that is gone', () => {
    expect(declaredRoutePatterns()).toEqual(Object.keys(PATHS_OF).sort());
  });
});

describe.each(RECORDED_LEDGERS)('every screen on the backend\'s recorded answers over the %s ledger', (ledger) => {
  const replay = replayApi(ledger);
  const { problems, rejections, refusals, host, router, settle, start, stop, inspect, expandEverything } = createSweep(replay.api);

  beforeAll(start);
  afterAll(stop);

  it('opens the app past the PIN with its frame in place', () => {
    expect(host.querySelector('.shell')).not.toBeNull();
  });

  it('renders every route in the month the backend recorded, without an error or a broken figure', async () => {
    problems.length = 0;
    const ids = await idsOf(replay.api);
    const month = useMonthStore();
    month.selected = month.last;
    for (const paths of Object.values(PATHS_OF)) {
      for (const path of paths(ids)) {
        const refusedBefore = refusals.length;
        await router.push(path);
        await settle();
        if (path !== OPENS_WITH_A_REFUSED_EVALUATION) expect(refusals.length, `a refused call on ${path}`).toBe(refusedBefore);
        expect(router.currentRoute.value.fullPath).toBe(router.resolve(path).fullPath);
        inspect(`${path} in ${month.selected}`);
        await expandEverything(`${path} in ${month.selected}`);
      }
    }
    expect(replay.unrecorded).toEqual([]);
    expect(problems).toEqual([]);
  });

  it('opens the panel of every transaction the month lists', async () => {
    problems.length = 0;
    const panel = usePanelStore();
    await router.push('/');
    await settle();
    const month = useMonthStore().last;
    const listed = await replay.api.transactions.list({ month, period: 'month' });
    for (const transaction of listed) {
      panel.open(transaction.id);
      await settle();
      expect(host.querySelector('.panel'), transaction.id).not.toBeNull();
      inspect(`panel of ${transaction.id}`);
    }
    panel.close();
    expect(replay.unrecorded).toEqual([]);
    expect(problems).toEqual([]);
  });

  it('leaves no promise rejection unhandled', async () => {
    await settle();
    expect(rejections).toEqual([]);
  });
});
