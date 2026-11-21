import { flushPromises, mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import App from '@/App.vue';
import { apiKey } from '@/api/context';
import type { FinwiseApi } from '@/api/types';
import { shiftMonth } from '@/lib/format';
import { createAppRouter } from '@/router';
import { CONFIG_SECTIONS } from '@/screens/config/sections';
import { useSubscriptionsViewStore } from '@/stores/subscriptions-view';

// What a screen must never print: the traces of a figure that was computed from nothing.
export const BROKEN_TEXT = /\b(NaN|undefined|Infinity|null)\b|Invalid Date/;

export interface Ids {
  pots: string[];
  accounts: string[];
  subscriptions: string[];
}

/** The id each family of pages is asked for when nobody has it: the backend answers `null` and the page says so. */
export interface AbsentIds {
  pot: string;
  account: string;
  subscription: string;
}

export const NOBODY: AbsentIds = { pot: 'nobody', account: 'nobody', subscription: 'nobody' };

/** Every route pattern of the router, with the concrete paths that stand for it in a state. */
export function pathsOf(absent: AbsentIds): Record<string, (ids: Ids) => string[]> {
  return {
    '/': () => ['/'],
    '/revisao': () => ['/revisao'],
    '/revisao/fechamento': () => ['/revisao/fechamento'],
    '/revisao/salario': () => ['/revisao/salario'],
    '/budget': () => ['/budget'],
    '/budget/cenarios': () => ['/budget/cenarios'],
    '/cofrinhos': () => ['/cofrinhos'],
    '/cofrinhos/:id': (ids) => [...ids.pots, absent.pot].map((id) => `/cofrinhos/${id}`),
    '/assinaturas': () => ['/assinaturas'],
    '/assinaturas/:id': (ids) => [...ids.subscriptions, absent.subscription].map((id) => `/assinaturas/${id}`),
    '/patrimonio': () => ['/patrimonio'],
    '/contas': () => ['/contas'],
    '/contas/cartao': () => ['/contas/cartao'],
    '/contas/:id': (ids) => [...ids.accounts, absent.account].map((id) => `/contas/${id}`),
    '/config/:section?': () => ['/config', ...CONFIG_SECTIONS.map((section) => `/config/${section.id}`)],
  };
}

export function declaredRoutePatterns(): string[] {
  const declared = createAppRouter()
    .getRoutes()
    .map((route) => route.path);
  return [...new Set(declared)].sort();
}

export async function idsOf(api: FinwiseApi): Promise<Ids> {
  const pots = (await api.savingsPots.list({ includeClosed: true })).flatMap((group) => group.pots);
  const subscriptions = (await api.subscriptions.list({ grouped: false })).groups.flatMap((group) => group.subscriptions);
  const accounts = await api.accounts.list();
  return { pots: pots.map((pot) => pot.id), accounts: accounts.map((account) => account.id), subscriptions: subscriptions.map((row) => row.id) };
}

export function monthsBetween(first: string, last: string): string[] {
  const months: string[] = [];
  for (let month = first; month <= last; month = shiftMonth(month, 1)) months.push(month);
  return months;
}

export interface Sweep {
  /** What went wrong on screen or in Vue, each with the place it was seen. */
  problems: string[];
  rejections: unknown[];
  host: HTMLElement;
  router: ReturnType<typeof createAppRouter>;
  settle: () => Promise<void>;
  start: () => Promise<void>;
  stop: () => void;
  inspect: (place: string) => void;
  expandEverything: (place: string) => Promise<void>;
}

/** The whole app mounted over an API, and the means to look at what it prints. */
export function createSweep(api: FinwiseApi): Sweep {
  const problems: string[] = [];
  const rejections: unknown[] = [];
  const onRejection = (reason: unknown) => rejections.push(reason);
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

  async function start(): Promise<void> {
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
  }

  function stop(): void {
    process.off('unhandledRejection', onRejection);
    wrapper.unmount();
    host.remove();
  }

  return { problems, rejections, host, router, settle, start, stop, inspect, expandEverything };
}
