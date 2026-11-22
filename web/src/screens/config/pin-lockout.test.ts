import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '@/App.vue';
import { createFakeApi } from '@/api/fake';
import { createAppRouter } from '@/router';
import { failingOne, mountWithFailures } from '@/test/readOnly';

const PIN_SCREEN = '[data-screen-label="PIN"]';

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});
afterEach(() => vi.restoreAllMocks());

describe('the PIN form answered locked during a hold', () => {
  it('brings the PIN screen back with its keys held and the wait, with no toast', async () => {
    const stand = createFakeApi({ startLocked: false });
    for (let attempt = 0; attempt < 3; attempt += 1) await stand.session.unlock('000000');
    expect(await stand.session.status()).toMatchObject({ retryAfterSeconds: 300 });
    const api = failingOne(stand, 'session.changePin', 'locked');
    const mounted = await mountWithFailures(App, [], '/config', { api, router: createAppRouter(), frame: false });
    const section = () => mounted.wrapper.find('[data-section="pin"]');
    await section().find('.change').trigger('click');
    const fields = section().findAll('input');
    for (const [index, value] of ['123456', '135790', '135790'].entries()) {
      (fields[index]!.element as HTMLInputElement).value = value;
      await fields[index]!.trigger('input');
    }

    await mounted.wrapper.findAll('button').find((button) => button.text() === 'Salvar PIN')!.trigger('click');
    await mounted.settle();

    expect(mounted.wrapper.find(PIN_SCREEN).exists()).toBe(true);
    expect(mounted.wrapper.find(`${PIN_SCREEN} .message`).text()).toBe('Muitas tentativas. Tente de novo em 5 minutos.');
    const keys = mounted.wrapper.find(PIN_SCREEN).findAll('button.key');
    expect(keys.length).toBeGreaterThan(0);
    expect(keys.every((key) => key.attributes('disabled') !== undefined)).toBe(true);
    expect(mounted.toasts).toEqual([]);
    expect(mounted.unclaimed).toEqual([]);
  });
});
