import type { IsoDay, YearMonth } from './common';

export type TransactionKind = 'cartao' | 'pix' | 'boleto' | 'outros';

/** Who put the transaction on its budget line. */
export type Classifier = 'Regra' | 'IA' | 'Você';

/** A classification the app would not apply by itself, waiting for a decision. */
export interface PendingReview {
  reason: string;
  suggestion: string;
  confidence: number;
  justification: string;
}

export interface Transaction {
  id: string;
  day: IsoDay;
  /** The name to show: the nickname when the establishment or person has one. */
  name: string;
  /** The name before any nickname, kept so a search still finds it. */
  originalName: string;
  rawDescription: string;
  /** Groups every transaction of one establishment or one person. */
  establishmentKey: string;
  kind: TransactionKind;
  /** The bank's own type for what is neither card, PIX nor boleto. */
  subtype: string | null;
  account: string;
  accountLabel: string;
  /** Signed: negative is money out. */
  amount: number;
  /** What it counts for once a linked reimbursement is taken off. */
  effectiveAmount: number;
  /** False for a movement between own accounts and for one marked as not counting. */
  countsInTotals: boolean;
  line: string | null;
  classifiedBy: Classifier;
  /** Set when the classifier was the model. */
  confidence: number | null;
  /** True when the model's confidence is under the configured threshold. */
  belowThreshold: boolean;
  pending: PendingReview | null;
  /** False while the bank still reports the charge as pending. */
  posted: boolean;
  /** The transaction's own tags followed by the establishment's fixed ones, without repeats. */
  tags: string[];
  movement: boolean;
  /** The other side is a person, not an establishment. */
  person: boolean;
  refund: boolean;
  /** A credit that pays back a purchase. */
  reimbursementIncome: boolean;
  reimbursed: boolean;
  excluded: boolean;
  split: boolean;
  installment: string | null;
  bill: string | null;
  /** The amount in the currency of a foreign purchase, as text. */
  originalAmount: string | null;
  /** The tax the bank charged on a foreign purchase, already inside `amount`. */
  iof: number | null;
}

export type TransactionPeriod = 'month' | 'last-3-months' | 'year';

export interface TransactionQuery {
  month: YearMonth;
  period: TransactionPeriod;
}

export interface FilterOption {
  value: string;
  label: string;
}

export interface BudgetLineOption {
  name: string;
  group: string;
}

export interface TransactionFilterOptions {
  accounts: FilterOption[];
  tags: string[];
  /** In the order a menu lists them: where spending goes first, income last. */
  lines: BudgetLineOption[];
}

export interface TransactionsApi {
  /** Newest first. */
  list(query: TransactionQuery): Promise<Transaction[]>;
  filterOptions(): Promise<TransactionFilterOptions>;
}
