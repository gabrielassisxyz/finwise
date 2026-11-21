import { mount, type ComponentMountingOptions } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { createApp, type Component } from 'vue';
import { createMemoryHistory, createRouter, type Router } from 'vue-router';
import { apiKey } from '@/api/context';
import { createFakeApi, type FakeApiOptions } from '@/api/fake';
import type { FinwiseApi } from '@/api/types';
import type { NavId } from '@/navigation';

const Blank = { template: '<div />' };

function testRouter(): Router {
  const paths: [string, NavId][] = [
    ['/', 'inicio'],
    ['/revisao', 'revisao'],
    ['/patrimonio', 'patrimonio'],
    ['/config/dados', 'config'],
  ];
  return createRouter({
    history: createMemoryHistory(),
    routes: paths.map(([path, nav]) => ({ path, component: Blank, meta: { nav } })),
  });
}

/** Runs `use` inside an app that has the fake API and a fresh Pinia, for testing a store alone. */
export function withStores<T>(use: () => T, options: FakeApiOptions = {}): { result: T; api: FinwiseApi } {
  const api = createFakeApi(options);
  const app = createApp({});
  app.provide(apiKey, api);
  app.use(createPinia());
  return { result: app.runWithContext(use), api };
}

/** Mounts a component with the fake API, a fresh Pinia and a router at `path`. */
export async function mountAt<C extends Component>(
  component: C,
  path: string,
  mounting: ComponentMountingOptions<C> = {},
  options: FakeApiOptions = {},
) {
  const api = createFakeApi(options);
  const router = testRouter();
  await router.push(path);
  await router.isReady();
  const wrapper = mount(component, {
    ...mounting,
    global: { plugins: [createPinia(), router], provide: { [apiKey as symbol]: api } },
  } as ComponentMountingOptions<C>);
  return { wrapper, router, api };
}
