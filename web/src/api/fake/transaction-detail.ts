import type {
  CounterpartyDetail,
  PanelSections,
  ReimbursementCandidate,
  ReimbursementDetail,
  TransactionDetail,
} from '../types';
import type { FakeState, FakeTransaction } from './model';
import { PROTOTYPE_CARD_FULL_LABEL } from './prototype-data';
import { toContract } from './rules';

// What the transaction panel shows, derived from the stand-in's state. The
// panel holds no rule of its own: which sections apply, what a split still
// lacks and which PIX could pay a purchase back are all decided here.

// Pay is income, never a refund of something bought.
const SALARY_LINE = 'Salário';

const cents = (amount: number): number => Math.round(amount * 100) / 100;

export function nameOf(state: FakeState, transaction: FakeTransaction): string {
  return state.establishments[transaction.establishmentKey]?.nickname ?? transaction.name;
}

function sectionsOf(transaction: FakeTransaction): PanelSections {
  const spend = transaction.amount < 0 && !transaction.movement;
  return {
    establishment: !transaction.person && !transaction.movement,
    person: transaction.person,
    split: spend,
    reimbursement: spend,
    refund: transaction.amount > 0,
  };
}

function counterpartyOf(state: FakeState, transaction: FakeTransaction): CounterpartyDetail | null {
  if (transaction.movement) return null;
  const record = state.establishments[transaction.establishmentKey];
  return {
    key: transaction.establishmentKey,
    nickname: record?.nickname ?? null,
    defaultLine: record?.line ?? null,
    fixedTags: [...(record?.tags ?? [])],
  };
}

function reimbursementOf(state: FakeState, transaction: FakeTransaction): ReimbursementDetail | null {
  const { reimbursement } = transaction;
  if (!reimbursement) return null;
  const credit = state.transactions.find((candidate) => candidate.id === reimbursement.link?.transactionId);
  const link =
    reimbursement.link && credit
      ? {
          transactionId: credit.id,
          name: nameOf(state, credit),
          day: credit.day,
          amount: reimbursement.link.amount,
        }
      : null;
  return { expected: reimbursement.expected, link };
}

/** Received PIX that no purchase has claimed yet, pay excluded. */
export function candidatesFor(state: FakeState): ReimbursementCandidate[] {
  const claimed = new Set(state.transactions.map((candidate) => candidate.reimbursement?.link?.transactionId));
  return state.transactions
    .filter((credit) => credit.kind === 'pix' && credit.amount > 0 && credit.line !== SALARY_LINE && !claimed.has(credit.id))
    .map((credit) => ({
      transactionId: credit.id,
      name: nameOf(state, credit),
      day: credit.day,
      amount: credit.amount,
    }));
}

function splitRemaining(transaction: FakeTransaction): number {
  if (!transaction.split) return 0;
  const assigned = transaction.split.reduce((total, part) => total + (Number(part.amount) || 0), 0);
  return cents(Math.abs(transaction.amount) - assigned);
}

export function toDetail(state: FakeState, transaction: FakeTransaction): TransactionDetail {
  const sections = sectionsOf(transaction);
  const modelClassified = !!transaction.pending || transaction.by === 'IA';
  return {
    ...toContract(state, transaction),
    accountDescription: transaction.kind === 'cartao' ? PROTOTYPE_CARD_FULL_LABEL : transaction.account,
    justification: modelClassified ? (transaction.pending?.justification ?? transaction.justification ?? '') : null,
    askReason: transaction.askReason,
    reason: transaction.reasonText,
    sections,
    counterparty: sections.establishment || sections.person ? counterpartyOf(state, transaction) : null,
    ownTags: [...transaction.tags],
    suggestedTags: state.tags.filter((tag) => !transaction.tags.includes(tag)),
    note: transaction.note,
    splitParts: transaction.split ? structuredClone(transaction.split) : null,
    splitRemaining: splitRemaining(transaction),
    reimbursement: reimbursementOf(state, transaction),
    reimbursementCandidates: sections.reimbursement ? candidatesFor(state) : [],
    exclusion: transaction.excludedReason === null ? null : { reason: transaction.excludedReason },
    refundLine: transaction.refundOffsetsLine,
    history: transaction.history
      .map((entry) => ({
        id: entry.id,
        text: entry.text,
        day: entry.day,
        reason: entry.reason,
        undoable: !!entry.restore,
      }))
      .reverse(),
  };
}
