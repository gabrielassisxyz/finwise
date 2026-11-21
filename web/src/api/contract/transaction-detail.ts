import type { ChangeResult } from './changes';
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

/** Reads and writes of the transaction panel. Every write answers with the handle that takes it back. */
export interface TransactionDetailApi {
  /** Null for an unknown id. */
  detail(id: string): Promise<TransactionDetail | null>;
  /** Accepts the model's classification as it stands, or its suggestion for a pending one. */
  confirmLine(id: string): Promise<ChangeResult | null>;
  /** The optional answer to "why" after correcting the model, kept on the history step. */
  setReason(id: string, reason: string): Promise<ChangeResult>;
  /** A blank nickname clears it. */
  setNickname(id: string, nickname: string): Promise<ChangeResult>;
  setDefaultLine(id: string, line: string): Promise<ChangeResult>;
  addFixedTag(id: string, tag: string): Promise<ChangeResult>;
  removeFixedTag(id: string, tag: string): Promise<ChangeResult>;
  /** The tag is stored with its leading `#`, spaces turned into hyphens. */
  addTag(id: string, tag: string): Promise<ChangeResult>;
  removeTag(id: string, tag: string): Promise<ChangeResult>;
  setNote(id: string, note: string): Promise<ChangeResult>;
  /** Null takes the split away. */
  setSplit(id: string, parts: SplitPart[] | null): Promise<ChangeResult>;
  setReimbursable(id: string, on: boolean): Promise<ChangeResult>;
  setExpectedReimbursement(id: string, amount: number | null): Promise<ChangeResult>;
  linkReimbursement(id: string, creditId: string): Promise<ChangeResult>;
  unlinkReimbursement(id: string): Promise<ChangeResult>;
  /** Turning it on starts without a reason; it only takes effect once `setExclusionReason` gives one. */
  setExcluded(id: string, on: boolean): Promise<ChangeResult>;
  setExclusionReason(id: string, reason: string): Promise<ChangeResult>;
  setRefund(id: string, on: boolean): Promise<ChangeResult>;
  setRefundLine(id: string, line: string): Promise<ChangeResult>;
  /** Restores the state before that step alone and drops the step from the history. */
  undoHistoryStep(id: string, stepId: string): Promise<ChangeResult | null>;
}
