import { flushPromises } from '@vue/test-utils';
import { createPinia, type Pinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createApp } from 'vue';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useSessionStore } from '@/stores/session';
import { useShellStore } from '@/stores/shell';
import { apiKey } from './context';
import { createFakeApi } from './fake';
import {
  createFailureHandler,
  failureListeners,
  installFailureHandling,
  LEDGER_UNAVAILABLE_MESSAGE,
  LOAD_FAILED_MESSAGE,
  READ_ONLY_MESSAGE,
  RETRY_LABEL,
} from './failures';
import { ApiFailure } from './http/failure';
import type { ApiErrorKind, FinwiseApi } from './types';

function provided(api: FinwiseApi = createFakeApi({ startLocked: false })) {
  const app = createApp({});
  const pinia = createPinia();
  app.provide(apiKey, api);
  app.use(pinia);
  return { app, pinia, api };
}

function stores(pinia: Pinia) {
  return { overlay: useOverlayStore(pinia), revision: useRevisionStore(pinia), session: useSessionStore(pinia), shell: useShellStore(pinia) };
}

afterEach(() => vi.restoreAllMocks());

describe('read_only', () => {
  it('shows the one message, with no action, and makes every screen fetch again', () => {
    const { pinia } = provided();
    const { overlay, revision } = stores(pinia);

    expect(createFailureHandler(pinia)(new ApiFailure('read_only'))).toBe(true);

    expect(overlay.toast).toEqual({ message: READ_ONLY_MESSAGE, actions: [] });
    expect(revision.current).toBe(1);
  });

  it('says exactly what the interface promises', () => {
    expect(READ_ONLY_MESSAGE).toBe('Somente leitura por enquanto. Nada foi alterado.');
  });
});

describe('locked', () => {
  it('locks the session store and shows no toast', () => {
    const { pinia } = provided();
    const { overlay, session, revision } = stores(pinia);
    session.locked = false;
    session.pin = '12';

    expect(createFailureHandler(pinia)(new ApiFailure('locked'))).toBe(true);

    expect(session.locked).toBe(true);
    expect(session.pin).toBe('');
    expect(overlay.toast).toBeNull();
    expect(revision.current).toBe(0);
  });
});

describe.each([
  ['ledger_unavailable', LEDGER_UNAVAILABLE_MESSAGE],
  ['bad_request', LOAD_FAILED_MESSAGE],
  ['unknown_method', LOAD_FAILED_MESSAGE],
  ['internal', LOAD_FAILED_MESSAGE],
  ['network', LOAD_FAILED_MESSAGE],
] as [ApiErrorKind, string][])('%s', (kind, message) => {
  it('shows its message with "Tentar de novo"', () => {
    const { pinia } = provided();
    const { overlay, revision } = stores(pinia);

    expect(createFailureHandler(pinia)(new ApiFailure(kind))).toBe(true);

    expect(overlay.toast?.message).toBe(message);
    expect(overlay.toast?.actions.map((action) => action.label)).toEqual([RETRY_LABEL]);
    expect(revision.current).toBe(0);
  });

  it('fetches again on "Tentar de novo" and takes the toast away', () => {
    const { pinia } = provided();
    const { overlay, revision } = stores(pinia);
    createFailureHandler(pinia)(new ApiFailure(kind));

    overlay.toast!.actions[0]!.run();

    expect(revision.current).toBe(1);
    expect(overlay.toast).toBeNull();
  });
});

describe('the words of the interface', () => {
  it('are the three sentences and the one label', () => {
    expect([LEDGER_UNAVAILABLE_MESSAGE, LOAD_FAILED_MESSAGE, RETRY_LABEL]).toEqual([
      'Dados do banco indisponíveis no momento.',
      'Não foi possível carregar os dados.',
      'Tentar de novo',
    ]);
  });
});

describe('an error that is not the API\'s', () => {
  it('is not handled and shows nothing', () => {
    const { pinia } = provided();
    const { overlay, revision } = stores(pinia);
    const handle = createFailureHandler(pinia);

    for (const error of [new TypeError('x is undefined'), 'text', undefined, { kind: 'read_only' }]) {
      expect(handle(error)).toBe(false);
    }

    expect(overlay.toast).toBeNull();
    expect(revision.current).toBe(0);
  });
});

describe('retrying the first reads of the app', () => {
  it('loads the session again when its status never arrived', async () => {
    const { pinia, api } = provided();
    const { overlay, session } = stores(pinia);
    const status = vi.spyOn(api.session, 'status');
    status.mockRejectedValueOnce(new ApiFailure('network'));
    await session.load().catch((error: unknown) => createFailureHandler(pinia)(error));
    expect(session.ready).toBe(false);

    overlay.toast!.actions[0]!.run();
    await flushPromises();

    expect(status).toHaveBeenCalledTimes(2);
    expect(session.ready).toBe(true);
  });

  it('loads the frame again when its overview never arrived', async () => {
    const { pinia, api } = provided();
    const { overlay, session, shell } = stores(pinia);
    await session.load();
    vi.spyOn(api.shell, 'overview').mockRejectedValueOnce(new ApiFailure('ledger_unavailable'));
    await shell.load().catch((error: unknown) => createFailureHandler(pinia)(error));
    expect(shell.overview).toBeNull();

    overlay.toast!.actions[0]!.run();
    await flushPromises();

    expect(shell.overview).not.toBeNull();
  });

  it('does not load the frame behind the PIN screen', async () => {
    const { pinia, api } = provided(createFakeApi({ startLocked: true }));
    const { overlay, session } = stores(pinia);
    await session.load();
    const overview = vi.spyOn(api.shell, 'overview');
    createFailureHandler(pinia)(new ApiFailure('network'));

    overlay.toast!.actions[0]!.run();
    await flushPromises();

    expect(overview).not.toHaveBeenCalled();
  });
});

describe('the two doors', () => {
  it('Vue\'s handler reports an error that is not the API\'s on the console and leaves the toast alone', () => {
    const { pinia } = provided();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const bug = new TypeError('x is undefined');

    failureListeners(pinia).onVueError(bug);

    expect(logged).toHaveBeenCalledWith(bug);
    expect(useOverlayStore(pinia).toast).toBeNull();
  });

  it('Vue\'s handler does not log an error it handled', () => {
    const { pinia } = provided();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => undefined);

    failureListeners(pinia).onVueError(new ApiFailure('read_only'));

    expect(logged).not.toHaveBeenCalled();
  });

  const rejection = (reason: unknown) => Object.assign(new Event('unhandledrejection', { cancelable: true }), { reason }) as PromiseRejectionEvent;

  it('the window\'s listener claims a rejection of the API so the browser does not report it', () => {
    const { pinia } = provided();
    const event = rejection(new ApiFailure('read_only'));

    failureListeners(pinia).onUnhandledRejection(event);

    expect(event.defaultPrevented).toBe(true);
    expect(useOverlayStore(pinia).toast?.message).toBe(READ_ONLY_MESSAGE);
  });

  it('the window\'s listener leaves any other rejection to the browser, which logs it', () => {
    const { pinia } = provided();
    const event = rejection(new TypeError('x is undefined'));

    failureListeners(pinia).onUnhandledRejection(event);

    expect(event.defaultPrevented).toBe(false);
    expect(useOverlayStore(pinia).toast).toBeNull();
  });

  it('reaches the stores through what installFailureHandling put on the app and on the window', () => {
    const { app, pinia } = provided();
    const target = new EventTarget() as unknown as Window;
    installFailureHandling(app, pinia, target);

    app.config.errorHandler!(new ApiFailure('read_only'), null, 'native event handler');
    expect(useOverlayStore(pinia).toast?.message).toBe(READ_ONLY_MESSAGE);

    useOverlayStore(pinia).dismissToast();
    target.dispatchEvent(Object.assign(new Event('unhandledrejection'), { reason: new ApiFailure('internal') }));
    expect(useOverlayStore(pinia).toast?.message).toBe(LOAD_FAILED_MESSAGE);
  });

  it('is installed on the app and on the window, and comes off again', () => {
    const { app, pinia } = provided();
    const target = new EventTarget() as unknown as Window;
    const added = vi.spyOn(target, 'addEventListener');
    const removed = vi.spyOn(target, 'removeEventListener');

    const uninstall = installFailureHandling(app, pinia, target);

    expect(app.config.errorHandler).toBeTypeOf('function');
    expect(added).toHaveBeenCalledWith('unhandledrejection', expect.any(Function));
    uninstall();
    expect(removed).toHaveBeenCalledWith('unhandledrejection', added.mock.calls[0]![1]);
  });
});
