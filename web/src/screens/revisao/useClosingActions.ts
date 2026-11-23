import { useApi } from '@/api/context';
import type { ClosingStepId, YearMonth } from '@/api/types';
import { longDay } from '@/lib/format';
import { useRevisionStore } from '@/stores/revision';
import { useReviewActions } from './useReviewActions';

/**
 * The writes of the month closing. Marking a step, closing the month and reopening it carry no toast: the screen itself
 * shows the result. A refused reopen is left to the failure handling, which says why and refetches.
 */
export function useClosingActions() {
  const api = useApi();
  const revision = useRevisionStore();
  const actions = useReviewActions();

  async function markStepDone(month: YearMonth, step: ClosingStepId): Promise<void> {
    await api.review.markStepDone(month, step);
    revision.bump();
  }

  async function closeMonth(month: YearMonth): Promise<void> {
    await api.review.closeMonth(month);
    revision.bump();
  }

  /** Reopens a month by undoing the change that closed it, the id the closing read answered. */
  async function reopenMonth(closeChangeId: string): Promise<void> {
    await api.changes.undo(closeChangeId);
    revision.bump();
  }

  async function keepSubscription(month: YearMonth, id: string): Promise<void> {
    const kept = await api.review.keepSubscription(month, id);
    if (kept) actions.announce(`${kept.name}: mantida. Nova revisão em ${longDay(kept.nextReview)}.`, kept.changeId);
  }

  async function cancelSubscription(id: string): Promise<void> {
    const cancelled = await api.review.cancelSubscription(id);
    if (cancelled) actions.announce(`${cancelled.name}: marcada para cancelar.`, cancelled.changeId);
  }

  async function sendSuggestion(proposalId: string): Promise<void> {
    const result = await api.review.propose(proposalId);
    if (result) actions.announce('Sugestão enviada para Aguardando seu ok.', result.changeId);
  }

  return { markStepDone, closeMonth, reopenMonth, keepSubscription, cancelSubscription, sendSuggestion };
}
