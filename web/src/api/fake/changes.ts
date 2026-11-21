import type { ChangesApi } from '../types';
import type { FakeReview } from './review-model';
import { syncCurrentBudget } from './budget-versions';
import type {
  FakeBudgetVersion,
  FakeEstablishment,
  FakePlannedContribution,
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
  plannedContributions: FakePlannedContribution[];
  review: FakeReview;
  /** Closing a month is a write that reaches the shell's list of closed months. */
  shell: FakeState['shell'];
}

interface RecordedBudget {
  versions: FakeBudgetVersion[];
  scenarios: FakeScenario[];
}

/** A change takes back the records it touched or the budget it replaced; one that touched both says so. */
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
  /** Snapshots the records and the budget together, for a write that reaches both. */
  beginWithBudget(): string;
  originOf(changeId: string): LineChangeOrigin;
  api: ChangesApi;
}

const snapshotRecords = (state: FakeState): RecordedRecords =>
  structuredClone({
    transactions: state.transactions,
    establishments: state.establishments,
    subscriptions: state.subscriptions,
    subscriptionSuggestion: state.subscriptionSuggestion,
    plannedContributions: state.plannedContributions,
    review: state.review,
    shell: state.shell,
  });

const snapshotBudget = (state: FakeState): RecordedBudget =>
  structuredClone({ versions: state.budgetVersions, scenarios: state.scenarios });

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
      return remember({ records: snapshotRecords(state), origin: origin ?? null });
    },
    beginBudget() {
      return remember({ budget: snapshotBudget(state), origin: null });
    },
    beginWithBudget() {
      return remember({ records: snapshotRecords(state), budget: snapshotBudget(state), origin: null });
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
