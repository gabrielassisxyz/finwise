import { flushPromises } from '@vue/test-utils';
import type { Component } from 'vue';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import BudgetScreen from './BudgetScreen.vue';

const Blank = { template: '<div />' };

export const routes = [
  { path: '/', component: Blank },
  { path: '/budget', component: BudgetScreen as Component },
];

export const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** Mounts a budget screen with the month store set as the frame would set it. */
export async function mountBudgetAt(path: string, component: Component) {
  const mounted = await mountRouted(component, routes, path);
  const month = useMonthStore();
  month.configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, month, overlay: useOverlayStore() };
}

export async function typeInto(input: { element: unknown; trigger: (event: string) => Promise<void> }, value: string) {
  (input.element as HTMLInputElement).value = value;
  await input.trigger('change');
  await flushPromises();
}
