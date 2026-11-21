import type { ChangesApi } from '../types';
import type { FakeEstablishment, FakeState, FakeTransaction } from './model';

/** A line change that "all purchases here" may still extend. */
export interface LineChangeOrigin {
  transactionId: string;
  line: string;
  establishmentKey: string;
}

interface RecordedChange {
  transactions: FakeTransaction[];
  establishments: Record<string, FakeEstablishment>;
  origin: LineChangeOrigin | null;
}

export interface ChangeLog {
  /** Snapshots what is about to change and returns the id that restores it. */
  begin(origin?: LineChangeOrigin): string;
  originOf(changeId: string): LineChangeOrigin;
  api: ChangesApi;
}

/**
 * A snapshot per change is all the stand-in needs to take a write back. The
 * server will keep the same promise with whatever it stores.
 */
export function createChangeLog(state: FakeState): ChangeLog {
  const recorded = new Map<string, RecordedChange>();
  let counter = 0;

  return {
    begin(origin) {
      counter += 1;
      const changeId = `change-${counter}`;
      recorded.set(changeId, {
        transactions: structuredClone(state.transactions),
        establishments: structuredClone(state.establishments),
        origin: origin ?? null,
      });
      return changeId;
    },
    originOf(changeId) {
      const origin = recorded.get(changeId)?.origin;
      if (!origin) throw new Error(`Change ${changeId} is not a line change that can be extended`);
      return origin;
    },
    api: {
      async undo(changeId) {
        const change = recorded.get(changeId);
        if (!change) throw new Error(`Unknown change ${changeId}`);
        state.transactions = change.transactions;
        state.establishments = change.establishments;
        recorded.delete(changeId);
      },
    },
  };
}
