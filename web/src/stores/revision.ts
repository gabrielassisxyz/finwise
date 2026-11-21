import { defineStore } from 'pinia';
import { ref } from 'vue';

/**
 * A counter that moves after every write or undo. What shows data a write can
 * change watches it and fetches again, so a write does not have to know who
 * is looking.
 */
export const useRevisionStore = defineStore('revision', () => {
  const current = ref(0);

  function bump(): void {
    current.value += 1;
  }

  return { current, bump };
});
