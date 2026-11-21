import type { ChangeResult } from './changes';
import type { IsoDay, YearMonth } from './common';
import type { ClosingApi } from './review-closing';
import type { PaydayApi } from './review-payday';
import type { PendingReview } from './transactions';

/** One transaction the model would not classify by itself, as the queue shows it. */
export interface ReviewQueueItem {
  transactionId: string;
  day: IsoDay;
  /** The nickname when there is one. */
  name: string;
  rawDescription: string;
  /** Signed: negative is money out. */
  amount: number;
  accountLabel: string;
  /** The other side is a person: it can be given a nickname, and a rule for it makes no sense. */
  person: boolean;
  pending: PendingReview;
}

export interface ReviewQueue {
  /** Newest first. */
  items: ReviewQueueItem[];
  /** Reviews answered through the queue so far, so the counter reads "3 de 8" while the queue shrinks. */
  resolved: number;
}

/** A change the model proposed that waits for a yes or a no. */
export interface ApprovalRequest {
  id: string;
  title: string;
  reason: string;
}

export type WarningActionId = 'accept-suggestion' | 'reject-suggestion' | 'silence-pair';

export interface ReviewWarning {
  id: string;
  text: string;
  /** The subscription the warning is about; null opens the list. */
  subscriptionId: string | null;
  actions: { id: WarningActionId; label: string }[];
}

export interface WeeklyLimit {
  cap: number;
  spent: number;
  /** Cap minus spent; negative once the cap is passed. */
  remaining: number;
  over: boolean;
  /** How much of the cap is used, 0 to 100. */
  fill: number;
}

export interface WeeklyGuide {
  guide: number;
  spent: number;
  fill: number;
}

export interface WeeklyReview {
  limit: WeeklyLimit;
  /** Null when the month has no current week. */
  week: WeeklyGuide | null;
  queue: ReviewQueue;
  approvals: ApprovalRequest[];
  warnings: ReviewWarning[];
}

/** What the person decided about one pending review. */
export interface PendingAnswer {
  /** The line to put the transaction on: the suggestion, or the one picked instead. */
  line: string;
  /** Ties the establishment to the line for the purchases to come. Ignored for a person. */
  always: boolean;
  /** Only a person can be given one; ignored for an establishment. */
  nickname: string;
  reason: string;
}

export interface PendingAccepted extends ChangeResult {
  /** What the toast calls it: the nickname given now, else the establishment's name. */
  name: string;
}

export interface Approved extends ChangeResult {
  title: string;
}

export type WarningOutcome = 'subscription-created' | 'suggestion-rejected' | 'pair-silenced';

export interface WarningActionResult extends ChangeResult {
  outcome: WarningOutcome;
  name: string;
}

export interface ReviewApi extends ClosingApi, PaydayApi {
  /** Month limit and the week guide are read for the month; the queue and the rest are the account's now. */
  weekly(month: YearMonth): Promise<WeeklyReview>;
  /** Null when the transaction is not waiting for review. */
  acceptPending(transactionId: string, answer: PendingAnswer): Promise<PendingAccepted | null>;
  /** Applies the change for real. Null when nothing by that id is waiting. */
  approve(id: string): Promise<Approved | null>;
  refuse(id: string): Promise<ChangeResult | null>;
  /** Puts a change the month's report suggested in front of the person. Null when it is unknown or already waiting. */
  propose(id: string): Promise<ChangeResult | null>;
  dismissWarning(id: string): Promise<ChangeResult | null>;
  runWarningAction(id: string, action: WarningActionId): Promise<WarningActionResult | null>;
}
