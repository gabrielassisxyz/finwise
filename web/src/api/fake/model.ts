import type { Bucket, Classifier, ClosingMode, Moment, IsoDay, PendingReview, ShellOverview, TransactionKind, WeekGuide, YearMonth } from '../types';
import type { CadenceUnit, SubscriptionCycle, SubscriptionStatus } from '../types';
import type { FakeReview } from './review-model';

// What the stand-in keeps in memory. It is wider than the contract: the
// contract is what a screen needs, this is what the rules are computed from.

/** What a history step puts back when it is taken back. */
export type Restorable = Partial<
  Pick<
    FakeTransaction,
    'line' | 'by' | 'confidence' | 'pending' | 'askReason' | 'reimbursement' | 'excludedReason' | 'refund' | 'refundOffsetsLine'
  >
>;

export interface FakeHistoryEntry {
  id: string;
  text: string;
  day: IsoDay;
  reason: string | null;
  /** Null for a step that only records what happened. */
  restore: Restorable | null;
}

export interface FakeReimbursement {
  expected: number | null;
  /** The received PIX that paid the purchase back, with the amount that came in. */
  link: { transactionId: string; amount: number } | null;
}

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
  by: Classifier | null;
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
  /** Set when the transaction was marked as not counting; it only takes effect with a reason, so '' is on but idle. */
  excludedReason: string | null;
  split: { line: string | null; amount: number }[] | null;
  /** Null while the purchase is not marked reimbursable. */
  reimbursement: FakeReimbursement | null;
  /** The model's sentence for a classification that stands; a pending review carries its own. */
  justification: string | null;
  /** True right after the user corrected the model, when the panel asks why. */
  askReason: boolean;
  reasonText: string;
  note: string;
  /** Oldest first. */
  history: FakeHistoryEntry[];
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
  id?: string;
  name: string;
  amount: number;
  flag?: string;
}

export interface FakeBudgetGroup {
  key: BudgetGroupKey;
  name: string;
  lines: FakeBudgetLine[];
}

/** A budget that holds from a month until the next version takes over. */
export interface FakeBudgetVersion {
  from: YearMonth;
  groups: FakeBudgetGroup[];
}

export interface FakeScenario {
  name: string;
  groups: FakeBudgetGroup[];
}

export interface FakeSavingsPot {
  id: string;
  name: string;
  bank: string;
  bucket: Bucket | null;
  balance: number;
  /** Rate the pot pays, as a percent of the CDI; null when the bank does not say. */
  cdiPercent: number | null;
  target: number | null;
  /** Budget lines the pot feeds. */
  linkedLines: string[];
  /** For a pot shared by several lines: how much of the balance each one owns. */
  lineShares: { line: string; amount: number }[] | null;
  maturity: IsoDay | null;
  closed: boolean;
  /** Days of the month whose yield the ledger cannot support; the month's yield is then unknown too. */
  unknownYieldDays?: number[];
  /** Present for an investment. */
  investment: { applied: number; tax: number } | null;
  /** The next bill a provision is saving for, when one is known. */
  nextCharge?: { description: string; day: IsoDay } | null;
  /** Deposits and withdrawals the ledger already holds, besides the month's planned deposit. */
  movements: { day: IsoDay; description: string; amount: number }[];
}

/** The month's planned deposit into a pot or an investment, and whether it has been made. */
export interface FakePlannedContribution {
  /** Its identity in the list, which a mark names. */
  key: string;
  /** The pot's name, or "Investimento" for the deposit shared by every investment. */
  name: string;
  amount: number;
  state: 'on-target' | 'done' | 'pending';
  doneOn: IsoDay | null;
  /** True when the person marked it, not the sync that found the transfer. */
  markedByHand?: boolean;
}

export interface FakeAccount {
  id: string;
  bank: string;
  name: string;
  kind: 'Conta' | 'Cartão';
  balance: number;
}

/** What the model detected about a subscription that is not derived from its figures. */
export type SubscriptionFlag = 'overlapping' | 'charged-after-cancel';

export interface FakeSubscription {
  id: string;
  name: string;
  amount: number;
  cycle: SubscriptionCycle;
  nextCharge: IsoDay;
  line: string | null;
  status: SubscriptionStatus;
  reviewOn: IsoDay;
  note: string;
  flags: SubscriptionFlag[];
  foreign: { originalAmount: string; iof: number } | null;
  /** What the charges before the latest one cost, when the price changed. */
  previousAmount: number | null;
  alternatives: string[];
}

/** A purchase the app saw repeat that is not a subscription. */
export interface FakeRecurringPurchase {
  name: string;
  cadence: { every: number; unit: CadenceUnit };
  lastPurchase: IsoDay;
  amount: number;
}

/** A card purchase the app asks to confirm as a subscription. */
export interface FakeSubscriptionSuggestion {
  transactionId: string;
  dismissed: boolean;
}

export interface FakeState {
  shell: Omit<ShellOverview, 'pendingCount'>;
  /** The month the baseline of spending was taken in; only its transactions move the totals. */
  currentMonth: YearMonth;
  confidenceThreshold: number;
  transactions: FakeTransaction[];
  establishments: Record<string, FakeEstablishment>;
  /** The budget of the current month: what the other screens read. It follows `budgetVersions`. */
  budget: FakeBudgetGroup[];
  /** Oldest first. A month uses the latest version that starts on or before it. */
  budgetVersions: FakeBudgetVersion[];
  scenarios: FakeScenario[];
  /** What each line had spent per month before any change made in this session. */
  spentBaseline: Record<YearMonth, Record<string, number>>;
  savingsPots: FakeSavingsPot[];
  accounts: FakeAccount[];
  plannedContributions: FakePlannedContribution[];
  tags: string[];
  weeks: WeekGuide[];
  payAhead: { amount: number; cardLabel: string } | null;
  subscriptions: FakeSubscription[];
  recurringPurchases: FakeRecurringPurchase[];
  subscriptionSuggestion: FakeSubscriptionSuggestion | null;
  review: FakeReview;
  /** Null when the ledger holds no card. */
  card: FakeCard | null;
  settings: FakeSettings;
}

/** A bill that already closed: what it held is a fact, so its dates are stored, not derived. */
export interface FakeCardBill {
  month: YearMonth;
  closedOn: IsoDay;
  dueOn: IsoDay;
  items: { name: string; day: IsoDay | null; amount: number }[];
}

export interface FakeCard {
  limit: number;
  /** The day the bank's own rule closes the bill on; null for a bank whose rule is not known. */
  bankClosingDay: number | null;
  /** The bank's due date, as a day of the month after the closing. */
  dueDay: number;
  /** Newest first. */
  bills: FakeCardBill[];
}

export interface FakeConnection {
  bank: string;
  consentExpired: boolean;
}

export interface FakeSettings {
  minExamples: number;
  /** Percent of corrections in each band of confidence, from 50-59 up to 90-100. */
  calibration: (number | null)[];
  cardLabel: string;
  closingMode: ClosingMode;
  manualClosingDay: number;
  connections: FakeConnection[];
  lastSync: Moment | null;
  lastBackup: Moment | null;
}
