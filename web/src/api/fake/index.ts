import type { FinwiseApi, SessionStatus, ShellOverview, UnlockResult } from '../types';
import { PROTOTYPE_PIN, PROTOTYPE_SHELL } from './prototype-data';

const MAX_WRONG_PINS = 3;

export interface FakeApiOptions {
  pin?: string;
  lockoutSeconds?: number;
  startLocked?: boolean;
  shell?: ShellOverview;
  /** Seconds since an arbitrary origin; injected so tests can move time. */
  now?: () => number;
}

/**
 * The in-memory stand-in for the backend. It answers with the prototype's
 * dataset and applies the same rules the server will, so screens and tests
 * run without a server.
 */
export function createFakeApi(options: FakeApiOptions = {}): FinwiseApi {
  const pin = options.pin ?? PROTOTYPE_PIN;
  const lockoutSeconds = options.lockoutSeconds ?? 300;
  const now = options.now ?? (() => Date.now() / 1000);
  const shell = options.shell ?? PROTOTYPE_SHELL;

  let locked = options.startLocked ?? true;
  let wrongPins = 0;
  let lockedOutUntil = 0;

  function retryAfterSeconds(): number {
    return Math.max(0, Math.ceil(lockedOutUntil - now()));
  }

  return {
    session: {
      async status(): Promise<SessionStatus> {
        return { locked, retryAfterSeconds: retryAfterSeconds() };
      },
      async unlock(attempt: string): Promise<UnlockResult> {
        const wait = retryAfterSeconds();
        if (wait > 0) return { ok: false, reason: 'locked-out', retryAfterSeconds: wait };
        if (attempt === pin) {
          locked = false;
          wrongPins = 0;
          return { ok: true };
        }
        wrongPins += 1;
        if (wrongPins < MAX_WRONG_PINS) return { ok: false, reason: 'wrong-pin' };
        wrongPins = 0;
        lockedOutUntil = now() + lockoutSeconds;
        return { ok: false, reason: 'locked-out', retryAfterSeconds: lockoutSeconds };
      },
      async lock(): Promise<void> {
        locked = true;
      },
    },
    shell: {
      async overview(): Promise<ShellOverview> {
        return structuredClone(shell);
      },
    },
  };
}
