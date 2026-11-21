import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';

/** What the person has typed or picked for the review in front of them; it ends with the review. */
export interface PendingDraft {
  /** The line picked instead of the suggestion. */
  line: string | null;
  always: boolean;
  nickname: string;
  reason: string;
}

const emptyDraft = (): PendingDraft => ({ line: null, always: false, nickname: '', reason: '' });

/** Where the person is in the queue and in the closing; both stay while they visit other screens. */
export const useReviewViewStore = defineStore('review-view', () => {
  const position = ref(0);
  const draft = reactive<PendingDraft>(emptyDraft());
  /** The step of the closing that is open, or -1 when every one is folded. */
  const openStep = ref(0);

  function resetDraft(): void {
    Object.assign(draft, emptyDraft());
  }

  function skip(current: number): void {
    position.value = current + 1;
    resetDraft();
  }

  return { position, draft, openStep, resetDraft, skip };
});
