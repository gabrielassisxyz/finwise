import type { ChangesApi } from '../types';
import { syncCurrentBudget } from './budget-versions';
import type {
  FakeBudgetVersion,
  FakeEstablishment,
  FakeScenario,
  FakeState,
  FakeSubscription,
  FakeSubscriptionSuggestion,
  FakeTransaction,
} from './model';

/** A line change that "all purchases here" may still extend. */
export interface LineChangeOrigin {
  transactionId: string;
  line: string;
  establishmentKey: string;
}

/** What a write over a transaction, an establishment or a subscription may have touched. */
interface RecordedRecords {
  transactions: FakeTransaction[];
  establishments: Record<string, FakeEstablishment>;
  subscriptions: FakeSubscription[];
  subscriptionSuggestion: FakeSubscriptionSuggestion | null;
}

interface RecordedBudget {
  versions: FakeBudgetVersion[];
  scenarios: FakeScenario[];
}

/** A change takes back the records it touched or the budget it replaced, never both. */
interface RecordedChange {
  records?: RecordedRecords;
  budget?: RecordedBudget;
  origin: LineChangeOrigin | null;
}

export interface ChangeLog {
  /** Snapshots what is about to change and returns the id that restores it. */
  begin(origin?: LineChangeOrigin): string;
  /** Snapshots the budget versions and the saved scenarios; the records are left as they are. */
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

  function remember(change: RecordedChange): string {
    counter += 1;
    const changeId = `change-${counter}`;
    recorded.set(changeId, change);
    return changeId;
  }

  return {
    begin(origin) {
      return remember({
        records: structuredClone({
          transactions: state.transactions,
          establishments: state.establishments,
          subscriptions: state.subscriptions,
          subscriptionSuggestion: state.subscriptionSuggestion,
        }),
        origin: origin ?? null,
      });
    },
    beginBudget() {
      return remember({
        budget: structuredClone({ versions: state.budgetVersions, scenarios: state.scenarios }),
        origin: null,
      });
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
        if (change.records) Object.assign(state, change.records);
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
