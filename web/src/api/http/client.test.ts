import { describe, expect, it, vi } from 'vitest';
import { METHOD_CLASSES } from '../contract/classification';
import { createHttpApi } from './client';
import { ApiFailure } from './failure';

type Reply = () => Response | Promise<Response>;

const json = (body: unknown, status = 200): Reply => () =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

function clientAnswering(reply: Reply) {
  const send = vi.fn<typeof fetch>(async () => reply());
  return { api: createHttpApi(send), send };
}

const callable = (api: unknown, domain: string, name: string) =>
  (api as Record<string, Record<string, (...args: unknown[]) => Promise<unknown>> | undefined>)[domain]?.[name]!;

async function failureOf(call: Promise<unknown>): Promise<ApiFailure> {
  const failure = await call.then(
    () => undefined,
    (error: unknown) => error,
  );
  expect(failure).toBeInstanceOf(ApiFailure);
  return failure as ApiFailure;
}

describe('the HTTP client', () => {
  it('has exactly the methods of the contract, domain by domain', () => {
    const api = createHttpApi(vi.fn());
    const shape = (tree: object) =>
      Object.fromEntries(Object.entries(tree).map(([domain, group]) => [domain, Object.keys(group as object).sort()]));
    expect(shape(api)).toEqual(shape(METHOD_CLASSES));
  });

  it('has no method the contract does not list, so such a call cannot become a request', () => {
    const { api, send } = clientAnswering(json(null));
    expect(callable(api, 'home', 'deleteEverything')).toBeUndefined();
    expect(callable(api, 'nowhere', 'summary')).toBeUndefined();
    expect(send).not.toHaveBeenCalled();
  });

  it.each(Object.entries(METHOD_CLASSES).flatMap(([domain, group]) => Object.keys(group).map((name) => [domain, name])))(
    'posts %s.%s to its route with the arguments as a JSON array',
    async (domain, name) => {
      const { api, send } = clientAnswering(json(null));
      await callable(api, domain!, name!)('a', 2, null);
      expect(send).toHaveBeenCalledTimes(1);
      const [url, init] = send.mock.calls[0]!;
      expect(url).toBe(`/api/${domain}/${name}`);
      expect(init).toMatchObject({ method: 'POST', credentials: 'same-origin', body: '["a",2,null]' });
      expect(new Headers(init!.headers).get('Content-Type')).toBe('application/json');
    },
  );

  it('sends an empty array for a method without arguments', async () => {
    const { api, send } = clientAnswering(json({ locked: true, retryAfterSeconds: 0 }));
    await api.session.status();
    expect(send.mock.calls[0]![1]!.body).toBe('[]');
  });

  it('resolves with the parsed answer', async () => {
    const { api } = clientAnswering(json({ locked: false, retryAfterSeconds: 0 }));
    await expect(api.session.status()).resolves.toEqual({ locked: false, retryAfterSeconds: 0 });
  });

  it('resolves a null answer to undefined for a method that returns nothing', async () => {
    const { api } = clientAnswering(json(null));
    await expect(api.session.lock()).resolves.toBeUndefined();
    await expect(api.changes.undo('c1')).resolves.toBeUndefined();
  });

  it('keeps a null answer for a method whose result can be null', async () => {
    const { api } = clientAnswering(json(null));
    await expect(api.transactions.setLine('t1', 'Lazer')).resolves.toBeNull();
  });

  it('rejects a 403 carrying read_only with an ApiFailure of that kind', async () => {
    const { api } = clientAnswering(json({ error: { kind: 'read_only' } }, 403));
    expect((await failureOf(api.transactions.setLine('t1', 'Lazer'))).kind).toBe('read_only');
  });

  it.each(['locked', 'ledger_unavailable', 'bad_request', 'unknown_method', 'internal'] as const)(
    'rejects with the kind %s the server sent',
    async (kind) => {
      const { api } = clientAnswering(json({ error: { kind } }, 500));
      expect((await failureOf(api.home.summary('2026-11'))).kind).toBe(kind);
    },
  );

  it('rejects with network when fetch itself fails', async () => {
    const { api } = clientAnswering(() => Promise.reject(new TypeError('Failed to fetch')));
    expect((await failureOf(api.home.summary('2026-11'))).kind).toBe('network');
  });

  it('reports an error answer it cannot read as internal', async () => {
    const replies: Reply[] = [
      () => new Response('<html>bad gateway</html>', { status: 502 }),
      json({ error: { kind: 'made_up' } }, 500),
      json({ error: { kind: 'network' } }, 500),
      json({ oops: true }, 500),
      json(null, 500),
    ];
    for (const reply of replies) {
      const { api } = clientAnswering(reply);
      expect((await failureOf(api.home.summary('2026-11'))).kind).toBe('internal');
    }
  });

  it('reports a success answer that is not JSON as internal', async () => {
    const { api } = clientAnswering(() => new Response('not json', { status: 200 }));
    expect((await failureOf(api.home.summary('2026-11'))).kind).toBe('internal');
  });
});
