import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '@/App.vue';
import { createFakeApi } from '@/api/fake';
import { createAppRouter } from '@/router';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useSessionStore } from '@/stores/session';
import { mountWithFailures, type FailureMount } from '@/test/readOnly';
import { LEDGER_UNAVAILABLE_MESSAGE, LOAD_FAILED_MESSAGE, RETRY_LABEL } from './failures';
import { ApiFailure } from './http/failure';

const PIN_SCREEN = '[data-screen-label="PIN"]';
const KEYPRESS_AND_DELAY_MS = 300;

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});
afterEach(() => vi.restoreAllMocks());

function openApp(api = createFakeApi({ startLocked: false })) {
  return { api, mount: () => mountWithFailures(App, [], '/', { api, router: createAppRouter(), frame: false }) };
}

const toastActions = (mounted: FailureMount) => useOverlayStore(mounted.pinia).toast?.actions.map((action) => action.label);

/** What is on the page: the toast only counts once a ToastHost draws it. */
const shownToast = (mounted: FailureMount) => (mounted.wrapper.find('.toast').exists() ? plain(mounted.wrapper.find('.toast').text()) : null);
const plain = (text: string) => text.replace(/\s+/g, ' ').trim();

async function runToastAction(mounted: FailureMount): Promise<void> {
  useOverlayStore(mounted.pinia).toast!.actions[0]!.run();
  await mounted.settle();
}

async function typePin(mounted: FailureMount, digits: string): Promise<void> {
  const session = useSessionStore(mounted.pinia);
  for (const digit of digits) session.pressDigit(digit);
  await new Promise((done) => setTimeout(done, KEYPRESS_AND_DELAY_MS));
  await mounted.settle();
}

describe('a call rejected with locked', () => {
  it('brings the PIN screen back over the screen that was showing', async () => {
    const { api, mount } = openApp();
    const mounted = await mount();
    expect(mounted.wrapper.find(PIN_SCREEN).exists()).toBe(false);
    expect(mounted.wrapper.find('main').exists()).toBe(true);
    vi.spyOn(api.home, 'summary').mockRejectedValueOnce(new ApiFailure('locked'));

    useRevisionStore(mounted.pinia).bump();
    await mounted.settle();

    expect(mounted.wrapper.find(PIN_SCREEN).exists()).toBe(true);
    expect(mounted.toasts).toEqual([]);
    expect(mounted.unclaimed).toEqual([]);
  });
});

describe('a read rejected with ledger_unavailable', () => {
  it('shows its toast, and "Tentar de novo" fetches again and renders once the read succeeds', async () => {
    const { api, mount } = openApp();
    const summary = vi.spyOn(api.home, 'summary').mockRejectedValueOnce(new ApiFailure('ledger_unavailable'));

    const mounted = await mount();

    expect(mounted.toasts).toEqual([LEDGER_UNAVAILABLE_MESSAGE]);
    expect(toastActions(mounted)).toEqual([RETRY_LABEL]);
    expect(shownToast(mounted)).toBe(`${LEDGER_UNAVAILABLE_MESSAGE}${RETRY_LABEL}`);
    expect(mounted.wrapper.text()).not.toContain('Restam');
    expect(summary).toHaveBeenCalledTimes(1);

    await runToastAction(mounted);

    expect(summary).toHaveBeenCalledTimes(2);
    expect(mounted.wrapper.text()).toContain('Restam');
    expect(useOverlayStore(mounted.pinia).toast).toBeNull();
    expect(mounted.unclaimed).toEqual([]);
  });

  it('says it could not load the data for any other failure, and retries the same way', async () => {
    const { api, mount } = openApp();
    vi.spyOn(api.home, 'summary').mockRejectedValueOnce(new ApiFailure('network'));

    const mounted = await mount();

    expect(mounted.toasts).toEqual([LOAD_FAILED_MESSAGE]);
    await runToastAction(mounted);
    expect(mounted.wrapper.text()).toContain('Restam');
  });
});

describe('the first reads of the app', () => {
  it('can be tried again when the session status never arrived', async () => {
    const { api, mount } = openApp();
    vi.spyOn(api.session, 'status').mockRejectedValueOnce(new ApiFailure('network'));

    const mounted = await mount();
    expect(mounted.toasts).toEqual([LOAD_FAILED_MESSAGE]);
    expect(shownToast(mounted)).toBe(`${LOAD_FAILED_MESSAGE}${RETRY_LABEL}`);
    expect(mounted.wrapper.find('main').exists()).toBe(false);

    await runToastAction(mounted);

    expect(mounted.wrapper.find('main').exists()).toBe(true);
  });

  it('can be tried again when the frame never arrived', async () => {
    const { api, mount } = openApp();
    vi.spyOn(api.shell, 'overview').mockRejectedValueOnce(new ApiFailure('ledger_unavailable'));

    const mounted = await mount();
    expect(mounted.toasts).toEqual([LEDGER_UNAVAILABLE_MESSAGE]);
    expect(shownToast(mounted)).toBe(`${LEDGER_UNAVAILABLE_MESSAGE}${RETRY_LABEL}`);
    expect(mounted.wrapper.find('main').exists()).toBe(false);

    await runToastAction(mounted);

    expect(mounted.wrapper.find('main').exists()).toBe(true);
  });
});

describe('the PIN screen when the server does not answer', () => {
  it('says so, and lets the PIN be typed again', async () => {
    const { api, mount } = openApp(createFakeApi({ startLocked: true }));
    vi.spyOn(api.session, 'unlock').mockRejectedValueOnce(new ApiFailure('network'));
    const mounted = await mount();

    await typePin(mounted, '123456');

    expect(mounted.toasts).toEqual([LOAD_FAILED_MESSAGE]);
    expect(shownToast(mounted)).toBe(`${LOAD_FAILED_MESSAGE}${RETRY_LABEL}`);
    expect(mounted.wrapper.find(PIN_SCREEN).exists()).toBe(true);
    expect(useSessionStore(mounted.pinia).pin).toBe('');

    await typePin(mounted, '123456');

    expect(mounted.wrapper.find(PIN_SCREEN).exists()).toBe(false);
    expect(mounted.wrapper.find('main').exists()).toBe(true);
  });
});

describe('an error that is not the API\'s', () => {
  it('thrown where Vue is waiting for it reaches the console and shows no toast', async () => {
    const { api, mount } = openApp();
    const bug = new TypeError('summary is undefined');
    vi.spyOn(api.home, 'summary').mockRejectedValueOnce(bug);
    const logged = vi.spyOn(console, 'error').mockImplementation(() => undefined);

    const mounted = await mount();

    expect(logged).toHaveBeenCalledWith(bug);
    expect(mounted.toasts).toEqual([]);
  });

  it('left unhandled on the window is not claimed, so the browser reports it', async () => {
    const { mount } = openApp();
    const mounted = await mount();
    const bug = new TypeError('forgotten promise');

    void Promise.reject(bug);
    await mounted.settle();

    expect(mounted.unclaimed).toEqual([bug]);
    expect(mounted.toasts).toEqual([]);
  });
});
