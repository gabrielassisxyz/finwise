import type { ChangesApi } from '../types';
import type { FakeBudgetVersion, FakeEstablishment, FakeScenario, FakeState, FakeTransaction } from './model';
import { syncCurrentBudget } from './budget-versions';

/** A line change that "all purchases here" may still extend. */
export interface LineChangeOrigin {
  transactionId: string;
  line: string;
  establishmentKey: string;
}

interface RecordedBudget {
  versions: FakeBudgetVersion[];
  scenarios: FakeScenario[];
}

/** A change takes back the transactions it touched or the budget it replaced, never both. */
interface RecordedChange {
  transactions?: FakeTransaction[];
  establishments?: Record<string, FakeEstablishment>;
  budget?: RecordedBudget;
  origin: LineChangeOrigin | null;
}

export interface ChangeLog {
  /** Snapshots what is about to change and returns the id that restores it. */
  begin(origin?: LineChangeOrigin): string;
  /** Snapshots the budget versions and the saved scenarios; transactions are left as they are. */
  beginBudget(): string;
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
    beginBudget() {
      counter += 1;
      const changeId = `change-${counter}`;
      recorded.set(changeId, {
        budget: { versions: structuredClone(state.budgetVersions), scenarios: structuredClone(state.scenarios) },
        origin: null,
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
        if (change.transactions && change.establishments) {
          state.transactions = change.transactions;
          state.establishments = change.establishments;
        }
        if (change.budget) {
          state.budgetVersions = change.budget.versions;
          state.scenarios = change.budget.scenarios;
          syncCurrentBudget(state);
        }
        recorded.delete(changeId);
      },
    },
  };
}
