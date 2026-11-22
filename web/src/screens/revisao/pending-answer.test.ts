import { describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import type { FakeState } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import { useReviewViewStore } from '@/stores/review-view';
import { stateWithoutJustification, stateWithoutSuggestion } from '@/test/pendingStates';
import { withStores } from '@/test/mount';
import { usePendingAnswer } from './usePendingAnswer';

const MONTH = '2026-11';

async function answeringOver(state: FakeState) {
  const { result, api } = withStores(() => ({ answer: usePendingAnswer(ref([])), view: useReviewViewStore() }), { state });
  const accepted = vi.spyOn(api.review, 'acceptPending');
  const [item] = (await api.review.weekly(MONTH)).queue.items;
  return { ...result, accepted, item: item! };
}

describe('answering a pending review with no suggestion', () => {
  it('sends nothing while no line is picked', async () => {
    const { answer, accepted, item } = await answeringOver(stateWithoutSuggestion());
    expect(item.pending.suggestion).toBeNull();
    await answer.accept(item);
    expect(accepted).not.toHaveBeenCalled();
  });

  it('sends the line the person picked', async () => {
    const { answer, view, accepted, item } = await answeringOver(stateWithoutSuggestion());
    view.draft.line = 'Mercado';
    await answer.accept(item);
    expect(accepted).toHaveBeenCalledTimes(1);
    expect(accepted).toHaveBeenCalledWith(item.transactionId, { line: 'Mercado', always: false, nickname: '', reason: '' });
  });
});

describe('answering a pending review that has a suggestion', () => {
  it('sends the suggestion when no other line is picked, with or without a justification', async () => {
    for (const state of [prototypeState(), stateWithoutJustification()]) {
      const { answer, accepted, item } = await answeringOver(state);
      await answer.accept(item);
      expect(accepted).toHaveBeenCalledWith(item.transactionId, expect.objectContaining({ line: item.pending.suggestion }));
    }
  });
});
