import { brl } from '@/lib/format';
import type { ChangeResult, SplitPart } from '../types';
import type { ChangeLog } from './changes';
import type { HistoryRecorder } from './history';
import type { FakeEstablishment, FakeState, FakeTransaction } from './model';
import { candidatesFor } from './transaction-detail';

// What the panel edits besides the line: the establishment, tags, note,
// split, reimbursement, exclusion and refund. A step that the panel lists in
// the history also records what it would put back.

const cents = (amount: number): number => Math.round(amount * 100) / 100;

/** A tag is stored with its `#`, and a space inside it is a hyphen. */
function normalizeTag(tag: string): string {
  const body = tag.trim().replace(/\s+/g, '-').replace(/^#+/, '');
  return body ? `#${body}` : '';
}

export function createTransactionEdits(state: FakeState, log: ChangeLog, history: HistoryRecorder) {
  const find = (id: string) => state.transactions.find((transaction) => transaction.id === id);

  /** One snapshot, then the change, whether or not the transaction exists. */
  function change(id: string, apply: (transaction: FakeTransaction) => void): ChangeResult {
    const changeId = log.begin();
    const transaction = find(id);
    if (transaction) apply(transaction);
    return { changeId };
  }

  function updateEstablishment(transaction: FakeTransaction, patch: Partial<FakeEstablishment>): void {
    const merged = { ...state.establishments[transaction.establishmentKey], ...patch };
    for (const key of Object.keys(merged) as (keyof FakeEstablishment)[]) {
      if (merged[key] === undefined) delete merged[key];
    }
    state.establishments[transaction.establishmentKey] = merged;
  }

  return {
    async confirmLine(id: string): Promise<ChangeResult | null> {
      const transaction = find(id);
      const line = transaction?.line ?? transaction?.pending?.suggestion;
      if (!transaction || !line) return null;
      return change(id, (target) =>
        history.record(target, `Você confirmou ${line}`, ['line', 'by', 'confidence', 'pending'], () => {
          Object.assign(target, { line, by: 'Você', confidence: null, pending: null });
        }),
      );
    },

    async setReason(id: string, reason: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.reasonText = reason;
        const latest = transaction.history.at(-1);
        if (latest) latest.reason = reason;
      });
    },

    async setNickname(id: string, nickname: string): Promise<ChangeResult> {
      return change(id, (transaction) => updateEstablishment(transaction, { nickname: nickname.trim() || undefined }));
    },

    async setDefaultLine(id: string, line: string): Promise<ChangeResult> {
      return change(id, (transaction) => updateEstablishment(transaction, { line }));
    },

    async addFixedTag(id: string, tag: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        const fixed = state.establishments[transaction.establishmentKey]?.tags ?? [];
        if (!fixed.includes(tag)) updateEstablishment(transaction, { tags: [...fixed, tag] });
      });
    },

    async removeFixedTag(id: string, tag: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        const fixed = state.establishments[transaction.establishmentKey]?.tags ?? [];
        updateEstablishment(transaction, { tags: fixed.filter((existing) => existing !== tag) });
      });
    },

    async addTag(id: string, tag: string): Promise<ChangeResult> {
      const normalized = normalizeTag(tag);
      return change(id, (transaction) => {
        if (normalized && !transaction.tags.includes(normalized)) transaction.tags = [...transaction.tags, normalized];
      });
    },

    async removeTag(id: string, tag: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.tags = transaction.tags.filter((existing) => existing !== tag);
      });
    },

    async setNote(id: string, note: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.note = note;
      });
    },

    async setSplit(id: string, parts: SplitPart[] | null): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.split = parts ? parts.map((part) => ({ line: part.line, amount: cents(Number(part.amount) || 0) })) : null;
      });
    },

    async setReimbursable(id: string, on: boolean): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.reimbursement = on ? { expected: cents(Math.abs(transaction.amount) / 2), link: null } : null;
      });
    },

    async setExpectedReimbursement(id: string, amount: number | null): Promise<ChangeResult> {
      return change(id, (transaction) => {
        if (transaction.reimbursement) transaction.reimbursement.expected = amount;
      });
    },

    async linkReimbursement(id: string, creditId: string): Promise<ChangeResult> {
      const credit = candidatesFor(state).find((candidate) => candidate.transactionId === creditId);
      return change(id, (transaction) => {
        if (!credit || !transaction.reimbursement) return;
        const text = `Você ligou ao PIX de ${credit.name} · ${brl(credit.amount)}`;
        history.record(transaction, text, ['reimbursement'], () => {
          transaction.reimbursement = {
            expected: transaction.reimbursement?.expected ?? null,
            link: { transactionId: credit.transactionId, amount: credit.amount },
          };
        });
      });
    },

    async unlinkReimbursement(id: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        const current = transaction.reimbursement;
        if (!current) return;
        history.record(transaction, 'Você desfez a ligação com o PIX', ['reimbursement'], () => {
          transaction.reimbursement = { expected: current.expected, link: null };
        });
      });
    },

    async setExcluded(id: string, on: boolean): Promise<ChangeResult> {
      return change(id, (transaction) => {
        if (on) {
          transaction.excludedReason = '';
          return;
        }
        history.record(transaction, 'Você voltou a contar esta transação', ['excludedReason'], () => {
          transaction.excludedReason = null;
        });
      });
    },

    async setExclusionReason(id: string, reason: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        transaction.excludedReason = reason.trim();
      });
    },

    async setRefund(id: string, on: boolean): Promise<ChangeResult> {
      return change(id, (transaction) => {
        const text = on ? 'Você marcou como estorno' : 'Você desmarcou o estorno';
        history.record(transaction, text, ['refund'], () => {
          transaction.refund = on;
        });
      });
    },

    async setRefundLine(id: string, line: string): Promise<ChangeResult> {
      return change(id, (transaction) => {
        history.record(transaction, `Você definiu que o estorno abate ${line}`, ['refundOffsetsLine'], () => {
          transaction.refundOffsetsLine = line;
        });
      });
    },

    async undoHistoryStep(id: string, stepId: string): Promise<ChangeResult | null> {
      const step = find(id)?.history.find((entry) => entry.id === stepId);
      if (!step?.restore) return null;
      const { restore } = step;
      return change(id, (transaction) => {
        Object.assign(transaction, structuredClone(restore));
        transaction.history = transaction.history.filter((entry) => entry.id !== stepId);
      });
    },
  };
}
