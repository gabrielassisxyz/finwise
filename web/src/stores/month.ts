import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import type { ClosedMonth, ShellOverview, YearMonth } from '@/api/types';
import { shiftMonth } from '@/lib/format';

/** The month every screen is looking at, and the range it may move within. */
export const useMonthStore = defineStore('month', () => {
  const selected = ref<YearMonth>('');
  const first = ref<YearMonth>('');
  const last = ref<YearMonth>('');
  const closedMonths = ref<ClosedMonth[]>([]);

  const canGoBack = computed(() => shiftMonth(selected.value, -1) >= first.value);
  const canGoForward = computed(() => shiftMonth(selected.value, 1) <= last.value);
  const isClosed = computed(() => closedMonths.value.some((closed) => closed.month === selected.value));

  function configure(overview: ShellOverview): void {
    first.value = overview.firstMonth;
    last.value = overview.lastMonth;
    closedMonths.value = overview.closedMonths;
    if (!selected.value) selected.value = overview.today.slice(0, 7);
  }

  function shift(delta: -1 | 1): void {
    const target = shiftMonth(selected.value, delta);
    if (target < first.value || target > last.value) return;
    selected.value = target;
  }

  return { selected, first, last, closedMonths, canGoBack, canGoForward, isClosed, configure, shift };
});
