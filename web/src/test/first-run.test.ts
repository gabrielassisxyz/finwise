import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { emptyLedgerState, freshInstallState } from '@/api/fake/first-run-data';
import type { FakeState } from '@/api/fake/model';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { NOBODY, createSweep, declaredRoutePatterns, idsOf, monthsBetween, pathsOf } from './sweep';

const PATHS_OF = pathsOf(NOBODY);

const STATES: { name: string; build: () => FakeState }[] = [
  { name: 'a fresh install', build: freshInstallState },
  { name: 'an empty ledger', build: emptyLedgerState },
];

describe('every route of the router is covered', () => {
  it('has a path table entry for each route, and no entry for a route that is gone', () => {
    expect(declaredRoutePatterns()).toEqual(Object.keys(PATHS_OF).sort());
  });
});

describe.each(STATES)('every screen on $name', ({ build }) => {
  const state = build();
  const api = createFakeApi({ state, startLocked: false });
  const { problems, rejections, host, router, settle, start, stop, inspect, expandEverything } = createSweep(api);

  beforeAll(start);
  afterAll(stop);

  it('opens the app past the PIN with its frame in place', () => {
    expect(host.querySelector('.shell')).not.toBeNull();
  });

  it('renders every route, in every month the frame offers, without an error or a broken figure', async () => {
    problems.length = 0;
    const ids = await idsOf(api);
    const month = useMonthStore();
    const months = monthsBetween(month.first, month.last);
    const visited: string[] = [];
    for (const selected of months) {
      month.selected = selected;
      for (const paths of Object.values(PATHS_OF)) {
        for (const path of paths(ids)) {
          await router.push(path);
          await settle();
          expect(router.currentRoute.value.fullPath).toBe(path);
          inspect(`${path} in ${selected}`);
          await expandEverything(`${path} in ${selected}`);
          visited.push(path);
        }
      }
    }
    expect(new Set(visited).size).toBeGreaterThanOrEqual(Object.keys(PATHS_OF).length);
    expect(problems).toEqual([]);
  });

  it('opens the panel of every transaction the state holds', async () => {
    problems.length = 0;
    const panel = usePanelStore();
    await router.push('/');
    await settle();
    for (const transaction of state.transactions) {
      panel.open(transaction.id);
      await settle();
      expect(host.querySelector('.panel'), transaction.id).not.toBeNull();
      inspect(`panel of ${transaction.id}`);
    }
    panel.close();
    expect(problems).toEqual([]);
  });

  it('leaves no promise rejection unhandled', async () => {
    await settle();
    expect(rejections).toEqual([]);
  });
});
