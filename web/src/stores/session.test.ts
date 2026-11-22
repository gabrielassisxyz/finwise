import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiFailure } from '@/api/http/failure';
import type { SessionStatus } from '@/api/types';
import { withStores } from '@/test/mount';
import { useSessionStore } from './session';

function type(session: ReturnType<typeof useSessionStore>, digits: string): void {
  for (const digit of digits) session.pressDigit(digit);
}

describe('session store', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function setup() {
    return withStores(() => useSessionStore(), { pin: '424242', lockoutSeconds: 300 }).result;
  }

  it('reports the server state once loaded', async () => {
    const session = setup();
    expect(session.ready).toBe(false);
    await session.load();
    expect(session.ready).toBe(true);
    expect(session.locked).toBe(true);
  });

  it('unlocks by itself after the sixth right digit', async () => {
    const session = setup();
    type(session, '42424');
    await vi.advanceTimersByTimeAsync(1000);
    expect(session.locked).toBe(true);

    session.pressDigit('2');
    expect(session.pin).toBe('424242');
    await vi.advanceTimersByTimeAsync(220);
    expect(session.locked).toBe(false);
    expect(session.pin).toBe('');
  });

  describe('locked from outside', () => {
    async function unlockedWithStatus(answer: () => Promise<SessionStatus>) {
      const { result: session, api } = withStores(() => useSessionStore(), { pin: '424242', startLocked: false });
      await session.load();
      const status = vi.spyOn(api.session, 'status').mockImplementation(answer);
      return { session, status };
    }

    it('is locked before the status answers, forgets what was typed, and asks the status once', async () => {
      const { session, status } = await unlockedWithStatus(() => new Promise<SessionStatus>(() => undefined));
      type(session, '42');

      session.markLocked();

      expect(session.locked).toBe(true);
      expect(session.pin).toBe('');
      expect(status).toHaveBeenCalledTimes(1);
    });

    it('holds the keypad for the wait the status reports, and releases it when the wait ends', async () => {
      const { session } = await unlockedWithStatus(async () => ({ locked: true, retryAfterSeconds: 120 }));

      session.markLocked();
      await vi.advanceTimersByTimeAsync(0);

      expect(session.keypadOnHold).toBe(true);
      expect(session.message).toBe('Muitas tentativas. Tente de novo em 2 minutos.');
      await vi.advanceTimersByTimeAsync(119_999);
      expect(session.keypadOnHold).toBe(true);
      await vi.advanceTimersByTimeAsync(1);
      expect(session.keypadOnHold).toBe(false);
      expect(session.message).toBe('');
    });

    it('leaves the keypad free and says nothing when the status reports no wait', async () => {
      const { session } = await unlockedWithStatus(async () => ({ locked: true, retryAfterSeconds: 0 }));

      session.markLocked();
      await vi.advanceTimersByTimeAsync(0);

      expect(session.keypadOnHold).toBe(false);
      expect(session.message).toBe('');
    });

    it('stays locked when the status says the session is open, because the server said it is gone', async () => {
      const { session } = await unlockedWithStatus(async () => ({ locked: false, retryAfterSeconds: 0 }));

      session.markLocked();
      await vi.advanceTimersByTimeAsync(0);

      expect(session.locked).toBe(true);
    });

    it('shows nothing and keeps the keypad free when the status call fails', async () => {
      const { session } = await unlockedWithStatus(() => Promise.reject(new ApiFailure('network')));

      session.markLocked();
      await vi.advanceTimersByTimeAsync(0);

      expect(session.locked).toBe(true);
      expect(session.keypadOnHold).toBe(false);
      expect(session.message).toBe('');
    });
  });

  it('clears the digits, shakes and says so on a wrong PIN', async () => {
    const session = setup();
    type(session, '000000');
    await vi.advanceTimersByTimeAsync(220);
    expect(session.locked).toBe(true);
    expect(session.pin).toBe('');
    expect(session.message).toBe('PIN incorreto');
    expect(session.shakeCount).toBe(1);
  });

  it('drops the error as soon as a new digit is typed', async () => {
    const session = setup();
    type(session, '000000');
    await vi.advanceTimersByTimeAsync(220);
    session.pressDigit('4');
    expect(session.message).toBe('');
  });

  it('erases the last digit', () => {
    const session = setup();
    type(session, '123');
    session.eraseDigit();
    expect(session.pin).toBe('12');
  });

  it('holds the keypad after three wrong PINs and releases it when the wait ends', async () => {
    const session = setup();
    for (let attempt = 0; attempt < 3; attempt += 1) {
      type(session, '000000');
      await vi.advanceTimersByTimeAsync(220);
    }
    expect(session.keypadOnHold).toBe(true);
    expect(session.message).toBe('Muitas tentativas. Tente de novo em 5 minutos.');

    session.pressDigit('4');
    expect(session.pin).toBe('');

    await vi.advanceTimersByTimeAsync(300_000);
    expect(session.keypadOnHold).toBe(false);
    expect(session.message).toBe('');
  });

  it('locks again and forgets what was being typed', async () => {
    const session = setup();
    type(session, '424242');
    await vi.advanceTimersByTimeAsync(220);
    session.pressDigit('1');
    await session.lock();
    expect(session.locked).toBe(true);
    expect(session.pin).toBe('');
  });
});
