import { flushPromises, mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import App from '@/App.vue';
import { apiKey } from '@/api/context';
import { createFakeApi } from '@/api/fake';
import { emptyLedgerState, freshInstallState } from '@/api/fake/first-run-data';
import type { FakeState } from '@/api/fake/model';
import type { FinwiseApi } from '@/api/types';
import { shiftMonth } from '@/lib/format';
import { createAppRouter } from '@/router';
import { CONFIG_SECTIONS } from '@/screens/config/sections';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { useSubscriptionsViewStore } from '@/stores/subscriptions-view';

// What a screen must never print: the traces of a figure that was computed from nothing.
const BROKEN_TEXT = /\b(NaN|undefined|Infinity|null)\b|Invalid Date/;

interface Ids {
  pots: string[];
  accounts: string[];
  subscriptions: string[];
}

/** Every route pattern of the router, with the concrete paths that stand for it in a state. */
const PATHS_OF: Record<string, (ids: Ids) => string[]> = {
  '/': () => ['/'],
  '/revisao': () => ['/revisao'],
  '/revisao/fechamento': () => ['/revisao/fechamento'],
  '/revisao/salario': () => ['/revisao/salario'],
  '/budget': () => ['/budget'],
  '/budget/cenarios': () => ['/budget/cenarios'],
  '/cofrinhos': () => ['/cofrinhos'],
  '/cofrinhos/:id': (ids) => [...ids.pots, 'nobody'].map((id) => `/cofrinhos/${id}`),
  '/assinaturas': () => ['/assinaturas'],
  '/assinaturas/:id': (ids) => [...ids.subscriptions, 'nobody'].map((id) => `/assinaturas/${id}`),
  '/patrimonio': () => ['/patrimonio'],
  '/contas': () => ['/contas'],
  '/contas/cartao': () => ['/contas/cartao'],
  '/contas/:id': (ids) => [...ids.accounts, 'nobody'].map((id) => `/contas/${id}`),
  '/config/:section?': () => ['/config', ...CONFIG_SECTIONS.map((section) => `/config/${section.id}`)],
};

const STATES: { name: string; build: () => FakeState }[] = [
  { name: 'a fresh install', build: freshInstallState },
  { name: 'an empty ledger', build: emptyLedgerState },
];

async function idsOf(api: FinwiseApi): Promise<Ids> {
  const pots = (await api.savingsPots.list({ includeClosed: true })).flatMap((group) => group.pots);
  const subscriptions = (await api.subscriptions.list({ grouped: false })).groups.flatMap((group) => group.subscriptions);
  const accounts = await api.accounts.list();
  return { pots: pots.map((pot) => pot.id), accounts: accounts.map((account) => account.id), subscriptions: subscriptions.map((row) => row.id) };
}

describe('every route of the router is covered', () => {
  it('has a path table entry for each route, and no entry for a route that is gone', () => {
    const declared = createAppRouter()
      .getRoutes()
      .map((route) => route.path);
    expect([...new Set(declared)].sort()).toEqual(Object.keys(PATHS_OF).sort());
  });
});

describe.each(STATES)('every screen on $name', ({ build }) => {
  const problems: string[] = [];
  const rejections: unknown[] = [];
  const onRejection = (reason: unknown) => rejections.push(reason);
  const state = build();
  const api = createFakeApi({ state, startLocked: false });
  const router = createAppRouter();
  const host = document.createElement('div');
  let wrapper: ReturnType<typeof mount>;

  async function settle(): Promise<void> {
    for (let round = 0; round < 3; round += 1) await flushPromises();
  }

  /** Every text node on screen, set apart: sibling elements carry no whitespace, which would glue "null" to the next word. */
  function textOnScreen(): string {
    const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT);
    const pieces: string[] = [];
    for (let node = walker.nextNode(); node; node = walker.nextNode()) pieces.push(node.textContent ?? '');
    return pieces.join(' ');
  }

  /** Text on screen, checked against the broken forms; the place is named in the failure. */
  function inspect(place: string): void {
    const text = textOnScreen();
    const found = text.match(BROKEN_TEXT);
    if (found) problems.push(`${place}: "${found[0]}" in "${text.slice(Math.max(0, (found.index ?? 0) - 40), (found.index ?? 0) + 40)}"`);
  }

  /** Whatever a screen keeps folded is part of the screen: open each fold, and the other tab of Assinaturas, and look again. */
  async function expandEverything(place: string): Promise<void> {
    const subscriptions = useSubscriptionsViewStore();
    for (const tab of ['assinaturas', 'recorrentes'] as const) {
      subscriptions.tab = tab;
      subscriptions.grouped = tab === 'recorrentes';
      await settle();
      if (router.currentRoute.value.path === '/assinaturas') inspect(`${place}, tab ${tab}`);
      for (const fold of Array.from(host.querySelectorAll('button')).filter((button) => button.textContent?.includes('▸'))) {
        fold.click();
        await settle();
        inspect(`${place}, after opening "${fold.textContent?.trim().slice(0, 30)}"`);
      }
    }
    subscriptions.tab = 'assinaturas';
    subscriptions.grouped = false;
  }

  beforeAll(async () => {
    process.on('unhandledRejection', onRejection);
    window.scrollTo = () => {};
    document.body.append(host);
    wrapper = mount(App, {
      attachTo: host,
      global: {
        plugins: [createPinia(), router],
        provide: { [apiKey as symbol]: api },
        config: { errorHandler: (error, _instance, info) => problems.push(`${info}: ${String(error)}`) },
      },
    });
    await router.push('/');
    await settle();
  });

  afterAll(() => {
    process.off('unhandledRejection', onRejection);
    wrapper.unmount();
    host.remove();
  });

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

function monthsBetween(first: string, last: string): string[] {
  const months: string[] = [];
  for (let month = first; month <= last; month = shiftMonth(month, 1)) months.push(month);
  return months;
}
