import { methods } from '../../../../internal/contract/contract.json';
import type { MethodClass } from '../contract/classification';
import { API_ERROR_KINDS, type ApiErrorKind } from '../contract/errors';
import type { FinwiseApi } from '../types';
import { ApiFailure } from './failure';

interface ManifestMethod {
  domain: string;
  name: string;
  route: string;
  class: MethodClass;
  result: object;
}

interface WireFailure {
  kind: ApiErrorKind;
  reason?: unknown;
}

type Fetch = typeof fetch;
type Method = (...args: unknown[]) => Promise<unknown>;

const wireKinds: readonly ApiErrorKind[] = API_ERROR_KINDS.filter((kind) => kind !== 'network');

function returnsNothing({ result }: ManifestMethod): boolean {
  return 'type' in result && result.type === 'null';
}

/**
 * A server that answers with anything but a known kind is reported as its own fault, not guessed at. A refusal keeps
 * its kind whatever its reason: the server did say that nothing was changed.
 */
async function wireFailure(response: Response): Promise<WireFailure> {
  try {
    const body: unknown = await response.json();
    const error = (body as { error?: { kind?: unknown; reason?: unknown } } | null)?.error;
    const kind = wireKinds.find((known) => known === error?.kind) ?? 'internal';
    return kind === 'refused' ? { kind, reason: error?.reason } : { kind };
  } catch {
    return { kind: 'internal' };
  }
}

async function answerOf(response: Response, methodClass: MethodClass): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new ApiFailure('internal', { methodClass });
  }
}

function methodOf(entry: ManifestMethod, send: Fetch): Method {
  const nothing = returnsNothing(entry);
  const methodClass = entry.class;
  return async (...args) => {
    let response: Response;
    try {
      response = await send(entry.route, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify(args),
      });
    } catch {
      throw new ApiFailure('network', { methodClass });
    }
    if (!response.ok) {
      const { kind, reason } = await wireFailure(response);
      throw new ApiFailure(kind, { methodClass, reason });
    }
    const answer = await answerOf(response, methodClass);
    return nothing && answer === null ? undefined : answer;
  };
}

/**
 * The API over HTTP. It is built from the generated manifest rather than as a
 * proxy, so it has exactly the contract's methods and a call to one that does
 * not exist cannot turn into a request.
 */
export function createHttpApi(send: Fetch = (...args) => fetch(...args)): FinwiseApi {
  const api: Record<string, Record<string, Method>> = {};
  for (const entry of methods as ManifestMethod[]) {
    const domain = (api[entry.domain] ??= {});
    domain[entry.name] = methodOf(entry, send);
  }
  return api as unknown as FinwiseApi;
}
