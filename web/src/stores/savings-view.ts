import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { Bucket } from '@/api/types';

/** What the pots and net-worth screens keep switched while the user moves between screens. */
export const useSavingsViewStore = defineStore('savings-view', () => {
  const showClosed = ref(false);
  const openBucket = ref<Bucket | null>(null);
  const stackedChart = ref(false);

  return { showClosed, openBucket, stackedChart };
});
