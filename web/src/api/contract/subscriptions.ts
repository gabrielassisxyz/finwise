import type { ChangeResult } from './changes';
import type { IsoDay } from './common';

export type SubscriptionCycle = 'Mensal' | 'Trimestral' | 'Semestral' | 'Anual' | 'Bienal';

export type SubscriptionStatus = 'Manter' | 'Analisar' | 'Backup e cancelar' | 'Cancelar';

/** `warning` is amber, `bad` is red. */
export type SubscriptionMarkTone = 'warning' | 'bad';

export interface SubscriptionMark {
  text: string;
  tone: SubscriptionMarkTone;
}

/** One subscription as the list shows it; every figure arrives computed. */
export interface SubscriptionRow {
  id: string;
  name: string;
  amount: number;
  cycle: SubscriptionCycle;
  nextCharge: IsoDay;
  /** The amount spread over the months of its cycle, to the cent. */
  monthlyEquivalent: number;
  /** The line's name in the current month's budget; null when it has none or that budget lacks it. */
  line: string | null;
  status: SubscriptionStatus;
  reviewOn: IsoDay;
  /** The review date has arrived or passed. */
  reviewOverdue: boolean;
  marks: SubscriptionMark[];
}

/** A status heading with its count and monthly total, or the whole list when it is not grouped. */
export interface SubscriptionGroup {
  status: SubscriptionStatus | null;
  monthlyTotal: number;
  subscriptions: SubscriptionRow[];
}

/** A card purchase the app believes repeats like a subscription and asks to confirm. */
export interface SubscriptionSuggestion {
  name: string;
}

export interface SubscriptionList {
  monthlyTotal: number;
  annualTotal: number;
  groups: SubscriptionGroup[];
  suggestion: SubscriptionSuggestion | null;
}

/** A card purchase a subscription can be created from. */
export interface SubscriptionCandidate {
  transactionId: string;
  name: string;
  amount: number;
}

export interface SubscriptionCharge {
  day: IsoDay;
  amount: number;
  /** Present for a charge made in a foreign currency. */
  foreign: { originalAmount: string; iof: number } | null;
}

export interface SubscriptionDetail {
  id: string;
  name: string;
  cycle: SubscriptionCycle;
  amount: number;
  marks: SubscriptionMark[];
  nextCharge: IsoDay;
  monthlyEquivalent: number;
  annualEquivalent: number;
  /** The line's name in the current month's budget; null when it has none or that budget lacks it. */
  line: string | null;
  status: SubscriptionStatus;
  reviewOn: IsoDay;
  note: string;
  /** Latest first. */
  charges: SubscriptionCharge[];
  /** Sentences the model wrote about cheaper plans; empty when it found none. */
  alternatives: string[];
}

export type CadenceUnit = 'days' | 'weeks' | 'months';

/** A purchase that repeats without being a subscription. */
export interface RecurringPurchase {
  name: string;
  cadence: { every: number; unit: CadenceUnit };
  lastPurchase: IsoDay;
  nextExpected: IsoDay;
  averageAmount: number;
  /** The next expected day has arrived. */
  due: boolean;
}

export interface CreatedSubscription extends ChangeResult {
  name: string;
}

export interface SubscriptionsApi {
  list(options: { grouped: boolean }): Promise<SubscriptionList>;
  /** Null when no subscription has that id. */
  detail(id: string): Promise<SubscriptionDetail | null>;
  candidates(): Promise<SubscriptionCandidate[]>;
  recurring(): Promise<RecurringPurchase[]>;
  /** Null when the purchase cannot become a subscription or its establishment already has one. */
  create(transactionId: string): Promise<CreatedSubscription | null>;
  /** Null when there is no open suggestion. */
  acceptSuggestion(): Promise<CreatedSubscription | null>;
  rejectSuggestion(): Promise<ChangeResult | null>;
  setStatus(id: string, status: SubscriptionStatus): Promise<ChangeResult>;
  setLine(id: string, line: string): Promise<ChangeResult>;
  setReviewDate(id: string, day: IsoDay): Promise<ChangeResult>;
  setNote(id: string, note: string): Promise<ChangeResult>;
}
