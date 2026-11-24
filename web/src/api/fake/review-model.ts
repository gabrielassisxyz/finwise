import type { ClosingStepId, ProposalOutcome, RefusalReason, ReportSection, SubscriptionStatus, WarningActionId, YearMonth } from '../types';

// What the review screens keep besides the transactions, the budget and the
// subscriptions they act on.

/** What approving a proposal does, applied through the same rules as a hand-made change. */
export type ProposalEffect =
  | { kind: 'cap'; line: string; amount: number }
  | { kind: 'exclude'; transactionId: string; reason: string }
  | { kind: 'status'; subscriptionId: string; status: SubscriptionStatus };

/** A change the model proposed. It exists whether or not it is waiting for approval. */
export interface FakeProposal {
  id: string;
  title: string;
  reason: string;
  effect: ProposalEffect;
  /** The proposer's name; absent or null for proposals not submitted by an agent. */
  proposer?: string | null;
  submittedAt?: string;
  outcome?: ProposalOutcome;
  answeredAt?: string | null;
  refusal?: string | null;
  staleReason?: RefusalReason | null;
  changeId?: string | null;
}

export type WarningKind = 'price' | 'new-recurring' | 'overlap' | 'renewal' | 'charged-after-cancel';

/** A warning the sync raised. The stand-in stores them; the server will detect them. */
export interface FakeWarning {
  id: string;
  kind: WarningKind;
  text: string;
  subscriptionId: string | null;
  actions: { id: WarningActionId; label: string }[];
}

export interface FakeReview {
  proposals: FakeProposal[];
  /** The proposals waiting for a yes or a no, oldest first. */
  awaiting: string[];
  warnings: FakeWarning[];
  /** Reviews answered through the queue so far. */
  resolved: number;
  /** The closing steps marked by hand, per month. */
  stepsDone: Record<YearMonth, ClosingStepId[]>;
  /** The report the model wrote for any month; absent, the prototype's sentences stand in for it. */
  report?: ReportSection[];
}
