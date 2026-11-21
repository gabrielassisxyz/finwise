import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import type { FinwiseApi } from '@/api/types';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { RECORDED_LEDGERS, replayApi } from './replay';
import { createSweep, declaredRoutePatterns, idsOf, pathsOf, type AbsentIds } from './sweep';

// The ids the Go side records as "nobody has this", so a page for a missing id is opened too.
const ABSENT: AbsentIds = { pot: 'no-such-pot', account: 'no-such-account', subscription: 'sub-1' };
const PATHS_OF = pathsOf(ABSENT);

// The backend refuses to evaluate a scenario until its arithmetic exists, and the scenarios screen ends itself on that
// refusal, so over the plain replay it could never be seen drawn. The evaluation alone is answered by the stand-in, so
// that the screen is swept as it will be and a refusal on any route still fails the sweep.
function withScenarioEvaluationAnswered(api: FinwiseApi): FinwiseApi {
  const stand = createFakeApi({ startLocked: false });
  return { ...api, budget: { ...api.budget, evaluateScenario: stand.budget.evaluateScenario } };
}

describe('every route of the router is covered', () => {
  it('has a path table entry for each route, and no entry for a route that is gone', () => {
    expect(declaredRoutePatterns()).toEqual(Object.keys(PATHS_OF).sort());
  });
});

describe.each(RECORDED_LEDGERS)('every screen on the backend\'s recorded answers over the %s ledger', (ledger) => {
  const replay = replayApi(ledger);
  const { problems, rejections, refusals, host, router, settle, start, stop, inspect, expandEverything } = createSweep(withScenarioEvaluationAnswered(replay.api));

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
        expect(refusals.length, `a refused call on ${path}`).toBe(refusedBefore);
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
