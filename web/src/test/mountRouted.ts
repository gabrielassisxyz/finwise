import { mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import type { Component } from 'vue';
import { createMemoryHistory, createRouter, type RouteRecordRaw } from 'vue-router';
import { apiKey } from '@/api/context';
import { createFakeApi, type FakeApiOptions } from '@/api/fake';

/** Like `mountAt`, for a screen that reads or changes the route: the routes are the caller's. */
export async function mountRouted(
  component: Component,
  routes: RouteRecordRaw[],
  path: string,
  options: FakeApiOptions = {},
) {
  const api = createFakeApi(options);
  const router = createRouter({ history: createMemoryHistory(), routes });
  await router.push(path);
  await router.isReady();
  const wrapper = mount(component, { global: { plugins: [createPinia(), router], provide: { [apiKey as symbol]: api } } });
  return { wrapper, router, api };
}
