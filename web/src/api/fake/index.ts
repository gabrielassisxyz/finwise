import type { ChangePinResult, FinwiseApi, SessionStatus, ShellOverview, SyncStatus, UnlockResult } from '../types';
import { createFakeAccounts } from './accounts';
import { createFakeBudget } from './budget';
import { createFakeCard } from './card';
import { createFakeHome } from './home';
import { createChangeLog } from './changes';
import { createFakeNetWorth } from './net-worth';
import { createFakeReview } from './review';
import { createFakeSavingsPots } from './savings-pots';
import { createFakeSettings } from './settings';
import { createFakeSubscriptions } from './subscriptions';
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
  let pin = options.pin ?? PROTOTYPE_PIN;
  const lockoutSeconds = options.lockoutSeconds ?? 300;
  const now = options.now ?? (() => Date.now() / 1000);
  const state = options.state ?? prototypeState();
  // Spending is a baseline plus what changed since; this is the "since".
  const original = structuredClone(state.transactions);

  const log = createChangeLog(state);

  let locked = options.startLocked ?? true;
  let wrongPins = 0;
  let lockedOutUntil = 0;

  /** An expired consent is the one thing the header warns about; the connections hold the fact. */
  function syncStatus(): SyncStatus {
    const expired = state.settings.connections.find((connection) => connection.consentExpired);
    return expired ? { state: 'warning', message: `${expired.bank}: consentimento expirado` } : state.shell.sync;
  }

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
      async changePin(current: string, next: string, repeat: string): Promise<ChangePinResult> {
        if (current !== pin) return { ok: false, reason: 'wrong-current' };
        if (!/^\d{6}$/.test(next)) return { ok: false, reason: 'invalid-new' };
        if (next !== repeat) return { ok: false, reason: 'mismatch' };
        pin = next;
        return { ok: true };
      },
    },
    shell: {
      async overview(): Promise<ShellOverview> {
        return { ...structuredClone(state.shell), sync: structuredClone(syncStatus()), pendingCount: pendingCount(state) };
      },
    },
    home: createFakeHome(state, original),
    transactions: createFakeTransactions(state, log),
    changes: log.api,
    savingsPots: createFakeSavingsPots(state),
    netWorth: createFakeNetWorth(state),
    budget: createFakeBudget(state, original, log),
    subscriptions: createFakeSubscriptions(state, log),
    review: createFakeReview(state, original, log),
    accounts: createFakeAccounts(state),
    card: createFakeCard(state),
    settings: createFakeSettings(state, log),
  };
}
