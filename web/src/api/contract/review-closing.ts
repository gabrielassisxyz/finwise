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

export interface CardBill {
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
  steps: ClosingStep[];
  pendingCount: number;
  plannedActual: PlannedActualGroup[];
  /** Variable cap minus what the variable lines spent; negative when they went over. */
  leftover: number;
  contributions: ClosingContribution[];
  cardBill: CardBill | null;
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
  /** Refused while anything is waiting for review, and for a month already closed. */
  closeMonth(month: YearMonth): Promise<ChangeResult>;
}
