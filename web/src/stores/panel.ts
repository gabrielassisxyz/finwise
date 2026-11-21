import { defineStore } from 'pinia';
import { ref } from 'vue';

/** Which transaction the side panel is open on, if any. */
export const usePanelStore = defineStore('panel', () => {
  const openId = ref<string | null>(null);

  function open(id: string): void {
    openId.value = id;
  }

  function close(): void {
    openId.value = null;
  }

  return { openId, open, close };
});
