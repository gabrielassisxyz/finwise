import type { FakeState, FakeTransaction, Restorable } from './model';

export interface HistoryRecorder {
  /**
   * Runs `apply` and adds a step to the transaction's history that puts back
   * the listed fields as they were before it.
   */
  record(transaction: FakeTransaction, text: string, keys: (keyof Restorable)[], apply: () => void): void;
}

export function createHistoryRecorder(state: FakeState): HistoryRecorder {
  let counter = 0;
  return {
    record(transaction, text, keys, apply) {
      const restore: Record<string, unknown> = {};
      for (const key of keys) restore[key] = structuredClone(transaction[key]);
      apply();
      counter += 1;
      transaction.history.push({
        id: `step-${counter}`,
        text,
        day: state.shell.today,
        reason: null,
        restore: restore as Restorable,
      });
    },
  };
}
