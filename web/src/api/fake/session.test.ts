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

  describe('changing the PIN', () => {
    it('opens with the new PIN afterwards and no longer with the old one', async () => {
      const { api } = lockedApi();
      expect(await api.session.changePin('424242', '135790', '135790')).toEqual({ ok: true });
      await api.session.lock();
      expect(await api.session.unlock('424242')).toEqual({ ok: false, reason: 'wrong-pin' });
      expect(await api.session.unlock('135790')).toEqual({ ok: true });
    });

    it('refuses a wrong current PIN and leaves the PIN as it was', async () => {
      const { api } = lockedApi();
      expect(await api.session.changePin('000000', '135790', '135790')).toEqual({ ok: false, reason: 'wrong-current' });
      expect(await api.session.unlock('424242')).toEqual({ ok: true });
    });

    it('refuses a new PIN that is not six digits', async () => {
      const { api } = lockedApi();
      for (const bad of ['12345', '1234567', '12345a', '']) {
        expect(await api.session.changePin('424242', bad, bad)).toEqual({ ok: false, reason: 'invalid-new' });
      }
    });

    it('refuses two new PINs that differ', async () => {
      const { api } = lockedApi();
      expect(await api.session.changePin('424242', '135790', '135791')).toEqual({ ok: false, reason: 'mismatch' });
    });

    it('reports the wrong current PIN before the other two faults', async () => {
      const { api } = lockedApi();
      expect(await api.session.changePin('000000', '12', '34')).toEqual({ ok: false, reason: 'wrong-current' });
      expect(await api.session.changePin('424242', '12', '34')).toEqual({ ok: false, reason: 'invalid-new' });
    });
  });
});
