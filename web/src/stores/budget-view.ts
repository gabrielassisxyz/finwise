import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { BudgetGroupKey } from '@/api/types';

/** Which groups of the budget are open; they stay as they were while the user visits other screens. */
export const useBudgetViewStore = defineStore('budget-view', () => {
  const open = reactive<Record<BudgetGroupKey, boolean>>({
    rec: true,
    fix: true,
    var: true,
    prov: true,
    res: true,
    rf: true,
    tar: true,
    inv: true,
  });

  function toggle(key: BudgetGroupKey): void {
    open[key] = !open[key];
  }

  return { open, toggle };
});
