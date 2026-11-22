import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import type { Component } from 'vue';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { prototypeOverview } from '@/test/mount';
import { createMemoryHistory, createRouter } from 'vue-router';
import { apiKey } from '@/api/context';
import { createFakeApi } from '@/api/fake';
import type { FinwiseApi } from '@/api/types';
import BudgetScreen from './BudgetScreen.vue';
import ScenariosScreen from './ScenariosScreen.vue';

const Blank = { template: '<div />' };

export const routes = [
  { path: '/', component: Blank },
  { path: '/budget', component: BudgetScreen as Component },
  { path: '/budget/cenarios', component: ScenariosScreen as Component },
];

export const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/**
 * Mounts a budget screen with the month store set as the frame would set it,
 * before the screen exists. `earlier` starts on the month before the current one; `api` is a stand-in
 * prepared beforehand.
 */
export async function mountBudgetAt(path: string, component: Component, options: { earlier?: boolean; api?: FinwiseApi } = {}) {
  const api = options.api ?? createFakeApi();
  const pinia = createPinia();
  setActivePinia(pinia);
  const month = useMonthStore(pinia);
  month.configure(prototypeOverview());
  if (options.earlier) month.shift(-1);
  const router = createRouter({ history: createMemoryHistory(), routes });
  await router.push(path);
  await router.isReady();
  const wrapper = mount(component, { global: { plugins: [pinia, router], provide: { [apiKey as symbol]: api } } });
  await flushPromises();
  return { wrapper, router, api, month, overlay: useOverlayStore(pinia) };
}

export async function typeInto(input: { element: unknown; trigger: (event: string) => Promise<void> }, value: string) {
  (input.element as HTMLInputElement).value = value;
  await input.trigger('change');
  await flushPromises();
}
