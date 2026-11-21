import type { FakeState, FakeTransaction, Restorable } from './model';

export interface HistoryRecorder {
  /**
   * Runs `apply` and adds a step to the transaction's history that puts back
   * the listed fields as they were before it.
   */
  record(transaction: FakeTransaction, text: string, keys: (keyof Restorable)[], apply: () => void): void;
  /** Adds a step that only records what happened and cannot be taken back on its own. */
  note(transaction: FakeTransaction, text: string): void;
}

// One counter for every recorder: two writers over the same transaction must not hand out the same step id.
let counter = 0;

export function createHistoryRecorder(state: FakeState): HistoryRecorder {
  function push(transaction: FakeTransaction, text: string, restore: Restorable | null): void {
    counter += 1;
    transaction.history.push({ id: `step-${counter}`, text, day: state.shell.today, reason: null, restore });
  }

  return {
    note(transaction, text) {
      push(transaction, text, null);
    },
    record(transaction, text, keys, apply) {
      const restore: Record<string, unknown> = {};
      for (const key of keys) restore[key] = structuredClone(transaction[key]);
      apply();
      push(transaction, text, restore as Restorable);
    },
  };
}
