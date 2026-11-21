import type { Ref } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetLineOption, ReviewQueueItem } from '@/api/types';
import { useLineChange } from '@/components/transactions/useLineChange';
import { useReviewViewStore } from '@/stores/review-view';
import { useReviewActions } from './useReviewActions';

/** Answering the review in front of the person: picking another line and accepting. */
export function usePendingAnswer(lines: Ref<BudgetLineOption[]>) {
  const api = useApi();
  const view = useReviewViewStore();
  const actions = useReviewActions();
  const { openLineMenu } = useLineChange(lines);

  function pickLine(anchor: HTMLElement, item: ReviewQueueItem): void {
    openLineMenu(anchor, view.draft.line ?? item.pending.suggestion, (line) => {
      view.draft.line = line;
    });
  }

  async function accept(item: ReviewQueueItem): Promise<void> {
    const { draft } = view;
    const line = draft.line ?? item.pending.suggestion;
    const answer = { line, always: draft.always, nickname: draft.nickname, reason: draft.reason };
    const result = await api.review.acceptPending(item.transactionId, answer);
    if (!result) return;
    view.resetDraft();
    const rule = answer.always && !item.person ? ' Vale para as próximas compras neste estabelecimento.' : '';
    actions.announce(`${result.name} foi para ${line}.${rule}`, result.changeId);
  }

  return { pickLine, accept };
}
