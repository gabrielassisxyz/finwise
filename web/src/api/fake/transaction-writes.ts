import type { ChangeResult, LineChangeResult } from '../types';
import type { ChangeLog } from './changes';
import { createHistoryRecorder } from './history';
import type { FakeState, FakeTransaction } from './model';
import { createTransactionEdits } from './transaction-edits';

type Classification = Pick<FakeTransaction, 'line' | 'by' | 'confidence' | 'pending'>;

/** A decision settles the review: whoever made it, the model's doubt no longer applies. */
function decided(line: string, by: 'Você' | 'Regra'): Classification {
  return { line, by, confidence: null, pending: null };
}

function nameOf(state: FakeState, transaction: FakeTransaction): string {
  return state.establishments[transaction.establishmentKey]?.nickname ?? transaction.name;
}

export function createTransactionWrites(state: FakeState, log: ChangeLog) {
  const history = createHistoryRecorder(state);
  const find = (id: string) => state.transactions.find((transaction) => transaction.id === id);

  function changeMany(ids: string[], apply: (transaction: FakeTransaction) => void): ChangeResult {
    const changeId = log.begin();
    const wanted = new Set(ids);
    state.transactions.filter((transaction) => wanted.has(transaction.id)).forEach(apply);
    return { changeId };
  }

  return {
    ...createTransactionEdits(state, log, history),
    async setLine(id: string, line: string): Promise<LineChangeResult | null> {
      const transaction = find(id);
      if (!transaction || (transaction.line === line && !transaction.pending)) return null;
      const changeId = log.begin({ transactionId: id, line, establishmentKey: transaction.establishmentKey });
      // The panel asks why only when what is being corrected was the model's.
      const askReason = transaction.by === 'IA' || !!transaction.pending;
      history.record(transaction, `Você alterou para ${line}`, ['line', 'by', 'confidence', 'pending'], () => {
        Object.assign(transaction, decided(line, 'Você'), { askReason });
      });
      const restore = transaction.history.at(-1)?.restore;
      if (restore) restore.askReason = false;
      const askable = !transaction.person && !transaction.movement;
      return {
        changeId,
        establishment: askable ? { key: transaction.establishmentKey, name: nameOf(state, transaction) } : null,
      };
    },

    async applyLineToEstablishment(changeId: string): Promise<ChangeResult & { changed: number }> {
      const { transactionId, line, establishmentKey } = log.originOf(changeId);
      state.establishments[establishmentKey] = { ...state.establishments[establishmentKey], line };
      const others = state.transactions.filter(
        (transaction) =>
          transaction.establishmentKey === establishmentKey && transaction.id !== transactionId && transaction.line !== line,
      );
      const origin = find(transactionId);
      const name = origin ? nameOf(state, origin) : establishmentKey;
      const text = `Regra aplicou ${line} (você pediu para todas em ${name})`;
      others.forEach((transaction) =>
        history.record(transaction, text, ['line', 'by', 'confidence', 'pending'], () => {
          Object.assign(transaction, decided(line, 'Regra'));
        }),
      );
      return { changeId, changed: others.length };
    },

    async setLineForMany(ids: string[], line: string): Promise<ChangeResult> {
      return changeMany(ids, (transaction) => Object.assign(transaction, decided(line, 'Você')));
    },

    async addTagToMany(ids: string[], tag: string): Promise<ChangeResult> {
      return changeMany(ids, (transaction) => {
        if (!transaction.tags.includes(tag)) transaction.tags = [...transaction.tags, tag];
      });
    },

    async excludeMany(ids: string[], reason: string): Promise<ChangeResult> {
      return changeMany(ids, (transaction) => {
        transaction.excludedReason = reason;
      });
    },
  };
}
