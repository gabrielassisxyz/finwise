import type { Classifier, IsoDay, PendingReview, ShellOverview, TransactionKind, WeekGuide, YearMonth } from '../types';

// What the stand-in keeps in memory. It is wider than the contract: the
// contract is what a screen needs, this is what the rules are computed from.

export interface FakeTransaction {
  id: string;
  /** Breaks ties between transactions of the same day, oldest entry first. */
  sequence: number;
  day: IsoDay;
  name: string;
  establishmentKey: string;
  raw: string;
  kind: TransactionKind;
  subtype: string | null;
  account: string;
  amount: number;
  line: string | null;
  by: Classifier;
  confidence: number | null;
  pending: PendingReview | null;
  posted: boolean;
  tags: string[];
  movement: boolean;
  person: boolean;
  refund: boolean;
  /** The line a refund is taken off; a refund without one touches no total. */
  refundOffsetsLine: string | null;
  reimbursementIncome: boolean;
  /** Set when the transaction was marked as not counting; it only takes effect with a reason. */
  excludedReason: string | null;
  split: { line: string | null; amount: number }[] | null;
  /** Amount of the credit linked as this purchase's reimbursement. */
  reimbursedAmount: number | null;
  installment: string | null;
  bill: string | null;
  originalAmount: string | null;
  iof: number | null;
}

export interface FakeEstablishment {
  nickname?: string;
  line?: string;
  tags?: string[];
}

export type BudgetGroupKey = 'rec' | 'fix' | 'var' | 'prov' | 'res';

export interface FakeBudgetLine {
  name: string;
  amount: number;
  flag?: string;
}

export interface FakeBudgetGroup {
  key: BudgetGroupKey;
  name: string;
  lines: FakeBudgetLine[];
}

export type Bucket = 'Disponível' | 'Reservado' | 'Investido';

export interface FakeSavingsPot {
  id: string;
  name: string;
  bank: string;
  bucket: Bucket | null;
  balance: number;
}

export interface FakeAccount {
  id: string;
  bank: string;
  name: string;
  kind: 'Conta' | 'Cartão';
  balance: number;
}

export interface FakeState {
  shell: Omit<ShellOverview, 'pendingCount'>;
  /** The month the baseline of spending was taken in; only its transactions move the totals. */
  currentMonth: YearMonth;
  confidenceThreshold: number;
  transactions: FakeTransaction[];
  establishments: Record<string, FakeEstablishment>;
  budget: FakeBudgetGroup[];
  /** What each line had spent per month before any change made in this session. */
  spentBaseline: Record<YearMonth, Record<string, number>>;
  savingsPots: FakeSavingsPot[];
  accounts: FakeAccount[];
  tags: string[];
  weeks: WeekGuide[];
  payAhead: { amount: number; cardLabel: string } | null;
}
