import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';

function lockedApi() {
  let clock = 1000;
  const api = createFakeApi({ pin: '424242', lockoutSeconds: 300, now: () => clock });
  return { api, advance: (seconds: number) => (clock += seconds) };
}

describe('fake session', () => {
  it('starts locked and opens with the right PIN', async () => {
    const { api } = lockedApi();
    expect(await api.session.status()).toEqual({ locked: true, retryAfterSeconds: 0 });
    expect(await api.session.unlock('424242')).toEqual({ ok: true });
    expect((await api.session.status()).locked).toBe(false);
  });

  it('locks again on request', async () => {
    const { api } = lockedApi();
    await api.session.unlock('424242');
    await api.session.lock();
    expect((await api.session.status()).locked).toBe(true);
  });

  it('puts the keypad on hold at the third wrong PIN', async () => {
    const { api } = lockedApi();
    expect(await api.session.unlock('000000')).toEqual({ ok: false, reason: 'wrong-pin' });
    expect(await api.session.unlock('000000')).toEqual({ ok: false, reason: 'wrong-pin' });
    expect(await api.session.unlock('000000')).toEqual({ ok: false, reason: 'locked-out', retryAfterSeconds: 300 });
  });

  it('refuses even the right PIN while on hold, then accepts it', async () => {
    const { api, advance } = lockedApi();
    for (let attempt = 0; attempt < 3; attempt += 1) await api.session.unlock('000000');

    advance(120);
    expect(await api.session.unlock('424242')).toEqual({ ok: false, reason: 'locked-out', retryAfterSeconds: 180 });

    advance(180);
    expect(await api.session.unlock('424242')).toEqual({ ok: true });
  });

  it('forgets earlier wrong PINs after a right one', async () => {
    const { api } = lockedApi();
    await api.session.unlock('000000');
    await api.session.unlock('000000');
    await api.session.unlock('424242');
    await api.session.lock();
    expect(await api.session.unlock('000000')).toEqual({ ok: false, reason: 'wrong-pin' });
  });
});
