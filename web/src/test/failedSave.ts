import type { ApiErrorKind } from '@/api/contract/errors';
import { ApiFailure } from '@/api/http/failure';
import type { FinwiseApi } from '@/api/types';
import { useOverlayStore } from '@/stores/overlay';
import { failingOne, type ApiMethodName, type FailureMount } from '@/test/readOnly';

type Method = (...args: unknown[]) => Promise<unknown>;

/**
 * The API when a write is stored and the answer is lost on the way back: the stand-in carries the write out, then the
 * call fails as `network`, which is how a write that did go through looks to the person.
 */
export function storedButUnanswered(api: FinwiseApi, method: ApiMethodName): FinwiseApi {
  const [domain = '', name = ''] = method.split('.');
  const methods = api as unknown as Record<string, Record<string, Method>>;
  const carriedOut = methods[domain]![name]!;
  const lost: Method = async (...args) => {
    await carriedOut(...args);
    throw new ApiFailure('network', { methodClass: 'write' });
  };
  return { ...methods, [domain]: { ...methods[domain], [name]: lost } } as unknown as FinwiseApi;
}

/** The API whose method works for the first `succeeding` calls and fails as `kind` from then on. */
export function failingAfter(api: FinwiseApi, method: ApiMethodName, kind: ApiErrorKind, succeeding: number): FinwiseApi {
  const [domain = '', name = ''] = method.split('.');
  const methods = api as unknown as Record<string, Record<string, Method>>;
  const working = methods[domain]![name]!;
  const failing = (failingOne(api, method, kind) as unknown as Record<string, Record<string, Method>>)[domain]![name]!;
  let calls = 0;
  const eventually: Method = (...args) => (calls++ < succeeding ? working(...args) : failing(...args));
  return { ...methods, [domain]: { ...methods[domain], [name]: eventually } } as unknown as FinwiseApi;
}

export interface Mark {
  revision: number;
  toasts: number;
}

export const mark = (mounted: FailureMount): Mark => ({ revision: mounted.revisionNow(), toasts: mounted.toasts.length });

/** What a write left: the toasts shown since the mark, the actions of the one showing, the bumps, what escaped. */
export function since(mounted: FailureMount, at: Mark) {
  return {
    toasts: mounted.toasts.slice(at.toasts),
    actions: useOverlayStore(mounted.pinia).toast?.actions.map((action) => action.label),
    bumps: mounted.revisionNow() - at.revision,
    unclaimed: mounted.unclaimed,
  };
}

/** A failed save reaches the one handler once: one toast with nothing to retry, one fetch, nothing uncaught. */
export const shownOnce = (message: string) => ({ toasts: [message], actions: [], bumps: 1, unclaimed: [] });
