import type { ChangeResult } from './changes';
import type { IsoDay, YearMonth } from './common';
import type { RefusalReason } from './errors';
import type { ClosingApi } from './review-closing';
import type { PaydayApi } from './review-payday';
import type { SubscriptionStatus } from './subscriptions';
import type { SplitPart } from './transaction-detail';
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
  /** The name the proposer gave when submitting; null for proposals without one. */
  proposer: string | null;
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

export interface RefusedProposal {
  title: string;
}

/** Closed union of writes that may be proposed for approval. */
export type ProposedChange =
  | { method: 'transactions.addTag'; arguments: [string, string] }
  | { method: 'transactions.removeTag'; arguments: [string, string] }
  | { method: 'transactions.addTagToMany'; arguments: [string[], string] }
  | { method: 'transactions.addFixedTag'; arguments: [string, string] }
  | { method: 'transactions.removeFixedTag'; arguments: [string, string] }
  | { method: 'transactions.setNote'; arguments: [string, string] }
  | { method: 'transactions.setDefaultLine'; arguments: [string, string] }
  | { method: 'transactions.setLine'; arguments: [string, string] }
  | { method: 'transactions.setExcluded'; arguments: [string, false] }
  | { method: 'transactions.setExclusionReason'; arguments: [string, string] }
  | { method: 'transactions.excludeMany'; arguments: [string[], string] }
  | { method: 'transactions.setSplit'; arguments: [string, SplitPart[] | null] }
  | { method: 'transactions.setReimbursable'; arguments: [string, boolean] }
  | { method: 'transactions.setExpectedReimbursement'; arguments: [string, number | null] }
  | { method: 'transactions.linkReimbursement'; arguments: [string, string] }
  | { method: 'transactions.unlinkReimbursement'; arguments: [string] }
  | { method: 'subscriptions.create'; arguments: [string] }
  | { method: 'subscriptions.setStatus'; arguments: [string, SubscriptionStatus] }
  | { method: 'subscriptions.setLine'; arguments: [string, string] }
  | { method: 'subscriptions.setReviewDate'; arguments: [string, IsoDay] }
  | { method: 'subscriptions.setNote'; arguments: [string, string] };

export interface ProposalDraft {
  change: ProposedChange;
  reason: string;
  proposer: string;
}

export interface SubmittedProposal {
  proposalId: string;
  title: string;
}

export type ProposalOutcome = 'waiting' | 'approved' | 'refused' | 'stale';

export interface ProposalRecord {
  id: string;
  title: string;
  /** Null for a seeded prototype proposal whose effect is outside ProposedChange. */
  change: ProposedChange | null;
  reason: string;
  proposer: string | null;
  submittedAt: string;
  outcome: ProposalOutcome;
  answeredAt: string | null;
  refusal: string | null;
  staleReason: RefusalReason | null;
  changeId: string | null;
  undone: boolean;
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
  /** Closes a waiting proposal without changing anything else. Null when nothing by that id waits. */
  refuse(id: string, reason?: string): Promise<RefusedProposal | null>;
  /** Puts a change the month's report suggested in front of the person. Null when it is unknown or already waiting. */
  propose(id: string): Promise<ChangeResult | null>;
  /** Validates a proposed change with a dry run and queues it for approval. Null when the change already holds. */
  submitProposal(draft: ProposalDraft): Promise<SubmittedProposal | null>;
  /** Every proposal with its current outcome, newest first. */
  proposals(): Promise<ProposalRecord[]>;
  dismissWarning(id: string): Promise<ChangeResult | null>;
  runWarningAction(id: string, action: WarningActionId): Promise<WarningActionResult | null>;
}
