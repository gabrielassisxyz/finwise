import type { ChangeResult } from './changes';
import type { IsoDay, YearMonth } from './common';
import type { SubscriptionCycle } from './subscriptions';

export type ClosingStepId =
  | 'pending'
  | 'planned-actual'
  | 'leftover'
  | 'contributions'
  | 'card-bill'
  | 'subscriptions'
  | 'report'
  | 'close';

/** `pending` is a step that blocks the close; `todo` one that is simply not done yet. */
export type ClosingStepState = 'done' | 'pending' | 'todo';

export interface ClosingStep {
  id: ClosingStepId;
  state: ClosingStepState;
  /** The person can mark it as done: steps 2 to 7 of a month that is still open. */
  markable: boolean;
}

export interface PlannedActualLine {
  name: string;
  planned: number;
  actual: number;
  /** Signed so that negative is the bad direction: overspending, or income that fell short. */
  difference: number;
}

export interface PlannedActualGroup extends PlannedActualLine {
  /** Provisions and reserves compare with what was set aside, not with what was spent. */
  contributions: boolean;
  lines: PlannedActualLine[];
}

export type ContributionState = 'done' | 'on-target' | 'not-found';

export interface ClosingContribution {
  name: string;
  amount: number;
  state: ContributionState;
}

export interface CardBillPurchase {
  name: string;
  day: IsoDay;
  amount: number;
}

export interface ClosingCardBill {
  amount: number;
  cardLabel: string;
  purchases: CardBillPurchase[];
}

export interface SubscriptionToReview {
  id: string;
  name: string;
  amount: number;
  cycle: SubscriptionCycle;
  reviewOn: IsoDay;
}

export interface ReportParagraph {
  text: string;
  /** The change this paragraph suggests, when it suggests one. */
  proposalId: string | null;
  /** The suggested change is already waiting for approval. */
  sent: boolean;
}

export interface ReportSection {
  heading: string;
  paragraphs: ReportParagraph[];
}

export interface ClosedMonthEntry {
  month: YearMonth;
  closedOn: IsoDay;
}

export interface ClosingView {
  month: YearMonth;
  closed: boolean;
  closedOn: IsoDay | null;
  /**
   * The change that closed the month, which `changes.undo` takes back to reopen it. Null for an open month, and for a
   * month closed by a change that did more than close it.
   */
  closeChangeId: string | null;
  steps: ClosingStep[];
  pendingCount: number;
  plannedActual: PlannedActualGroup[];
  /** Variable cap minus what the variable lines spent; negative when they went over. */
  leftover: number;
  contributions: ClosingContribution[];
  cardBill: ClosingCardBill | null;
  subscriptionsToReview: SubscriptionToReview[];
  report: ReportSection[];
  /** Closed months, newest first. */
  closedMonths: ClosedMonthEntry[];
}

export interface KeptSubscription extends ChangeResult {
  name: string;
  nextReview: IsoDay;
}

export interface ClosingApi {
  closing(month: YearMonth): Promise<ClosingView>;
  /** Steps 2 to 7 are marked by hand; the others follow from the data. Null for any other step or a closed month. */
  markStepDone(month: YearMonth, step: ClosingStepId): Promise<ChangeResult | null>;
  /** Keeps a subscription under review and schedules the next review. */
  keepSubscription(month: YearMonth, id: string): Promise<KeptSubscription | null>;
  cancelSubscription(id: string): Promise<(ChangeResult & { name: string }) | null>;
  /**
   * Refused for a month already closed, for a month after the current one, and while a transaction
   * of that month or an earlier one waits for review.
   */
  closeMonth(month: YearMonth): Promise<ChangeResult>;
}
