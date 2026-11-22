/// <reference types="node" />
import { flushPromises, mount, type ComponentMountingOptions } from '@vue/test-utils';
import { createPinia, type Pinia } from 'pinia';
import { afterEach } from 'vitest';
import type { Component } from 'vue';
import { createMemoryHistory, createRouter, type RouteRecordRaw, type Router } from 'vue-router';
import { apiKey } from '@/api/context';
import { failureListeners } from '@/api/failures';
import { createFakeApi, type FakeApiOptions } from '@/api/fake';
import { METHOD_CLASSES } from '@/api/contract/classification';
import type { ApiErrorKind, RefusalReason } from '@/api/contract/errors';
import { ApiFailure } from '@/api/http/failure';
import type { FinwiseApi } from '@/api/types';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useShellStore } from '@/stores/shell';
import { prototypeOverview } from '@/test/mount';

type Method = (...args: unknown[]) => Promise<unknown>;

/** The API as the read-only backend answers it: every write and evaluate method is refused, the rest is untouched. */
export function readOnlyApi(api: FinwiseApi): FinwiseApi {
  const refused: Method = async () => {
    throw new ApiFailure('read_only');
  };
  const wrapped: Record<string, Record<string, Method>> = {};
  for (const [domain, classes] of Object.entries(METHOD_CLASSES)) {
    const methods = (api as unknown as Record<string, Record<string, Method>>)[domain]!;
    wrapped[domain] = Object.fromEntries(
      Object.entries(classes).map(([name, kind]) => [name, kind === 'write' || kind === 'evaluate' ? refused : methods[name]!]),
    );
  }
  return wrapped as unknown as FinwiseApi;
}

/** Every method of the contract as `domain.name`, so a name the contract lacks does not compile. */
export type ApiMethodName = {
  [Domain in keyof FinwiseApi & string]: `${Domain}.${keyof FinwiseApi[Domain] & string}`;
}[keyof FinwiseApi & string];

/**
 * The API with one method failing as a server would fail it: with the kind given, the reason given for `refused`,
 * and the class the method has in the contract. Every other method is the wrapped API's own.
 */
export function failingOne(api: FinwiseApi, method: ApiMethodName, kind: ApiErrorKind, reason?: RefusalReason): FinwiseApi {
  const [domain = '', name = ''] = method.split('.');
  const classes = METHOD_CLASSES as unknown as Record<string, Record<string, ApiFailure['methodClass']> | undefined>;
  const methodClass = classes[domain]?.[name];
  if (!methodClass) throw new Error(`failingOne: the contract has no method ${method}`);
  const methods = api as unknown as Record<string, Record<string, Method>>;
  const failing: Method = async () => {
    throw new ApiFailure(kind, { methodClass, reason });
  };
  return { ...methods, [domain]: { ...methods[domain], [name]: failing } } as unknown as FinwiseApi;
}

/**
 * Node reports a promise nobody handled and the browser fires `unhandledrejection` on the window; jsdom does the
 * second for no one. This carries the first to the second, and keeps what no listener claimed.
 */
function carryRejectionsToWindow(): { unclaimed: unknown[]; stop: () => void } {
  const saved = process.listeners('unhandledRejection');
  process.removeAllListeners('unhandledRejection');
  const unclaimed: unknown[] = [];
  const carry = (reason: unknown, promise: Promise<unknown>): void => {
    const event = new Event('unhandledrejection', { cancelable: true });
    Object.assign(event, { reason, promise });
    window.dispatchEvent(event);
    if (!event.defaultPrevented) unclaimed.push(reason);
  };
  process.on('unhandledRejection', carry);
  return {
    unclaimed,
    stop() {
      process.off('unhandledRejection', carry);
      for (const listener of saved) process.on('unhandledRejection', listener);
    },
  };
}

const cleanups: (() => void)[] = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

export interface FailureMount {
  wrapper: ReturnType<typeof mount>;
  api: FinwiseApi;
  pinia: Pinia;
  /** Every message shown as a toast, in order, whatever replaced it since. */
  toasts: string[];
  /** Rejections that reached neither Vue's handler nor the window's: what a browser would have logged as uncaught. */
  unclaimed: unknown[];
  revisionNow: () => number;
  /** Lets the writes, the refetches and the rejections nobody awaited run to the end. */
  settle: () => Promise<void>;
}

export interface FailureMountOptions {
  state?: FakeApiOptions['state'];
  api?: FinwiseApi;
  /** Replaces the router built from `routes`, for mounting the whole app. */
  router?: Router;
  /** Set to false when the screen under test is the app itself, which loads its own frame. */
  frame?: boolean;
  mounting?: ComponentMountingOptions<Component>;
}

/**
 * Mounts a screen as the app does: with the failure handling installed on Vue and on the window, over the fake API
 * made read-only unless `api` says otherwise, and with the month and the frame set as the shell would have set them.
 */
export async function mountWithFailures(
  component: Component,
  routes: RouteRecordRaw[],
  path: string,
  options: FailureMountOptions = {},
): Promise<FailureMount> {
  const api = options.api ?? readOnlyApi(createFakeApi({ startLocked: false, state: options.state }));
  const pinia = createPinia();
  const router = options.router ?? createRouter({ history: createMemoryHistory(), routes });
  await router.push(path);
  await router.isReady();

  const { onVueError, onUnhandledRejection } = failureListeners(pinia);
  window.addEventListener('unhandledrejection', onUnhandledRejection);
  const carried = carryRejectionsToWindow();
  cleanups.push(() => {
    window.removeEventListener('unhandledrejection', onUnhandledRejection);
    carried.stop();
  });

  const withFrame = options.frame ?? true;
  if (withFrame) useMonthStore(pinia).configure(prototypeOverview());
  const wrapper = mount(component, {
    ...options.mounting,
    global: { plugins: [pinia, router], provide: { [apiKey as symbol]: api }, config: { errorHandler: onVueError } },
  } as ComponentMountingOptions<Component>);

  if (withFrame) useShellStore(pinia).overview = prototypeOverview();
  const toasts: string[] = [];
  useOverlayStore(pinia).$onAction(({ name, args }) => {
    if (name === 'showToast') toasts.push(String(args[0]));
  });

  const settle = async (): Promise<void> => {
    await flushPromises();
    await new Promise((done) => setTimeout(done, 0));
    await flushPromises();
  };
  await settle();
  return { wrapper, api, pinia, toasts, unclaimed: carried.unclaimed, revisionNow: () => useRevisionStore(pinia).current, settle };
}
