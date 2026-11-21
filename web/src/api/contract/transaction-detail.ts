import type { IsoDay } from './common';
import type { Transaction } from './transactions';

/** One step of a transaction's history, newest first in the detail. */
export interface HistoryEntry {
  id: string;
  /** What happened, without the date. */
  text: string;
  day: IsoDay;
  /** The user's answer to "why", when the step corrected the model. */
  reason: string | null;
  /** True when taking this step back restores a previous state. */
  undoable: boolean;
}

/** The establishment, or the person, behind a transaction. */
export interface CounterpartyDetail {
  key: string;
  nickname: string | null;
  defaultLine: string | null;
  fixedTags: string[];
}

export interface SplitPart {
  line: string | null;
  amount: number;
}

/** A received PIX that could pay a purchase back. */
export interface ReimbursementCandidate {
  transactionId: string;
  name: string;
  day: IsoDay;
  amount: number;
}

export interface ReimbursementLink {
  transactionId: string;
  name: string;
  day: IsoDay;
  amount: number;
}

export interface ReimbursementDetail {
  /** What the user expects to get back; half of the purchase until told otherwise. */
  expected: number | null;
  link: ReimbursementLink | null;
}

/** Which sections of the panel apply to this kind of transaction. */
export interface PanelSections {
  establishment: boolean;
  person: boolean;
  split: boolean;
  reimbursement: boolean;
  refund: boolean;
}

/** Everything the transaction panel shows, derived on the server. */
export interface TransactionDetail extends Transaction {
  /** The account as the panel's subtitle and details name it; a card carries its last digits. */
  accountDescription: string;
  /** Why the model chose the line, for a pending review or a classification of its own. */
  justification: string | null;
  /** True right after the user corrected the model, when the panel asks why. */
  askReason: boolean;
  reason: string;
  sections: PanelSections;
  counterparty: CounterpartyDetail | null;
  /** The transaction's own tags, not the establishment's fixed ones. */
  ownTags: string[];
  /** Known tags the transaction does not have yet. */
  suggestedTags: string[];
  note: string;
  /** Null while the purchase is not split. */
  splitParts: SplitPart[] | null;
  /** What the split still lacks (positive) or exceeds (negative); zero when it closes with the total. */
  splitRemaining: number;
  /** Null while the purchase is not marked reimbursable. */
  reimbursement: ReimbursementDetail | null;
  reimbursementCandidates: ReimbursementCandidate[];
  /** Null while "não conta" is off; a switch that is on with an empty reason does not count yet. */
  exclusion: { reason: string } | null;
  /** The line a refund reduces. */
  refundLine: string | null;
  history: HistoryEntry[];
}

/** What the transaction panel reads. */
export interface TransactionDetailApi {
  /** Null for an unknown id. */
  detail(id: string): Promise<TransactionDetail | null>;
}
