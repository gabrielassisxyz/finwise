import type { FinwiseApi, SessionStatus, ShellOverview, UnlockResult } from '../types';
import { createFakeBudget } from './budget';
import { createFakeHome } from './home';
import { createChangeLog } from './changes';
import { createFakeNetWorth } from './net-worth';
import { createFakeSavingsPots } from './savings-pots';
import type { FakeState } from './model';
import { PROTOTYPE_PIN, prototypeState } from './prototype-data';
import { pendingCount } from './rules';
import { createFakeTransactions } from './transactions';

const MAX_WRONG_PINS = 3;

export interface FakeApiOptions {
  pin?: string;
  lockoutSeconds?: number;
  startLocked?: boolean;
  state?: FakeState;
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
  const state = options.state ?? prototypeState();
  // Spending is a baseline plus what changed since; this is the "since".
  const original = structuredClone(state.transactions);

  const log = createChangeLog(state);

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
        return { ...structuredClone(state.shell), pendingCount: pendingCount(state) };
      },
    },
    home: createFakeHome(state, original),
    transactions: createFakeTransactions(state, log),
    changes: log.api,
    savingsPots: createFakeSavingsPots(state),
    netWorth: createFakeNetWorth(state),
    budget: createFakeBudget(state, original, log),
  };
}
