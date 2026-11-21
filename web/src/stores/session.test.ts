import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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

  it('is locked from outside, without a call to the server, and forgets what was typed', async () => {
    const { result: session, api } = withStores(() => useSessionStore(), { pin: '424242', startLocked: false });
    const status = vi.spyOn(api.session, 'status');
    await session.load();
    type(session, '42');

    session.markLocked();

    expect(session.locked).toBe(true);
    expect(session.pin).toBe('');
    expect(status).toHaveBeenCalledTimes(1);
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
