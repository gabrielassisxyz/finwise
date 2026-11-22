import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useApi } from '@/api/context';
import type { UnlockResult } from '@/api/types';

const PIN_LENGTH = 6;

// The sixth dot stays filled for a beat before the answer replaces it;
// without the pause the last digit never appears to land.
const SUBMIT_DELAY_MS = 220;

function lockoutMessage(seconds: number): string {
  const minutes = Math.max(1, Math.ceil(seconds / 60));
  const span = minutes === 1 ? '1 minuto' : `${minutes} minutos`;
  return `Muitas tentativas. Tente de novo em ${span}.`;
}

export const useSessionStore = defineStore('session', () => {
  const api = useApi();

  const ready = ref(false);
  const locked = ref(true);
  const pin = ref('');
  const message = ref('');
  const shakeCount = ref(0);
  const keypadOnHold = ref(false);

  let checking = false;
  let releaseTimer: ReturnType<typeof setTimeout> | undefined;

  function holdKeypad(seconds: number): void {
    keypadOnHold.value = true;
    message.value = lockoutMessage(seconds);
    clearTimeout(releaseTimer);
    releaseTimer = setTimeout(() => {
      keypadOnHold.value = false;
      message.value = '';
    }, seconds * 1000);
  }

  async function load(): Promise<void> {
    const status = await api.session.status();
    locked.value = status.locked;
    if (status.retryAfterSeconds > 0) holdKeypad(status.retryAfterSeconds);
    ready.value = true;
  }

  async function submit(): Promise<void> {
    let result: UnlockResult;
    try {
      result = await api.session.unlock(pin.value);
    } finally {
      // A failed call must free the keypad too, or the PIN could never be typed again.
      checking = false;
      pin.value = '';
    }
    if (result.ok) {
      locked.value = false;
      return;
    }
    shakeCount.value += 1;
    if (result.reason === 'wrong-pin') {
      message.value = 'PIN incorreto';
      return;
    }
    holdKeypad(result.retryAfterSeconds);
  }

  function pressDigit(digit: string): void {
    if (keypadOnHold.value || checking || pin.value.length >= PIN_LENGTH) return;
    pin.value += digit;
    message.value = '';
    if (pin.value.length < PIN_LENGTH) return;
    checking = true;
    setTimeout(() => void submit(), SUBMIT_DELAY_MS);
  }

  function eraseDigit(): void {
    if (checking) return;
    pin.value = pin.value.slice(0, -1);
  }

  async function lock(): Promise<void> {
    await api.session.lock();
    markLocked();
  }

  /** The server says the session is gone. The PIN screen is back at once; the status is asked only for a hold. */
  function markLocked(): void {
    locked.value = true;
    pin.value = '';
    message.value = '';
    void showHold();
  }

  // A `locked` answer can arrive while the server holds every attempt, so the PIN screen has to say how long to wait.
  // The status's own `locked` is not read: the session is gone whatever it says. Without the answer the keypad
  // stays free, and the hold shows on the next attempt instead.
  async function showHold(): Promise<void> {
    let retryAfterSeconds: number;
    try {
      ({ retryAfterSeconds } = await api.session.status());
    } catch {
      return;
    }
    if (retryAfterSeconds > 0) holdKeypad(retryAfterSeconds);
  }

  return { ready, locked, pin, message, shakeCount, keypadOnHold, load, pressDigit, eraseDigit, lock, markLocked };
});
