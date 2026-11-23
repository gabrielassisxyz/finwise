<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetLineOption, ReviewQueueItem, WeeklyReview, YearMonth } from '@/api/types';
import { createLatestLoad, type IsLatest } from '@/lib/latestLoad';
import { useMonthStore } from '@/stores/month';
import { useRevisionStore } from '@/stores/revision';
import ApprovalsCard from './ApprovalsCard.vue';
import PendingQueue from './PendingQueue.vue';
import WarningsCard from './WarningsCard.vue';
import WeeklyStrip from './WeeklyStrip.vue';

const api = useApi();
const month = useMonthStore();
const revision = useRevisionStore();

const weekly = ref<WeeklyReview | null>(null);
const linesByMonth = ref<Record<YearMonth, BudgetLineOption[]>>({});

// The queue is not bound to the month on screen: each item's lines are those of its own transaction's month. What
// was answered is kept per month until the next revision.
let kept = { revision: -1, byMonth: new Map<YearMonth, BudgetLineOption[]>() };
const loads = createLatestLoad();

async function menusOf(items: ReviewQueueItem[], at: number, isLatest: IsLatest): Promise<Record<YearMonth, BudgetLineOption[]>> {
  if (kept.revision !== at) kept = { revision: at, byMonth: new Map() };
  const { byMonth } = kept;
  const months = [...new Set(items.map((item) => item.day.slice(0, 7)))];
  const missing = months.filter((asked) => !byMonth.has(asked));
  const answers = await Promise.all(missing.map((asked) => api.transactions.filterOptions(asked)));
  if (!isLatest()) return {};
  missing.forEach((asked, index) => byMonth.set(asked, answers[index]!.lines));
  return Object.fromEntries(months.map((asked) => [asked, byMonth.get(asked)!]));
}

watch(
  () => [month.selected, revision.current] as const,
  async ([selected, at]) => {
    const isLatest = loads.start();
    const loaded = await api.review.weekly(selected);
    if (!isLatest() || selected !== month.selected) return;
    // The queue is drawn once its menus arrived, so "Escolher outra linha" never opens empty.
    const menus = await menusOf(loaded.queue.items, at, isLatest);
    if (!isLatest() || selected !== month.selected) return;
    weekly.value = loaded;
    linesByMonth.value = menus;
  },
  { immediate: true },
);
</script>

<template>
  <template v-if="weekly">
    <WeeklyStrip :limit="weekly.limit" :week="weekly.week" />
    <div class="columns">
      <PendingQueue :queue="weekly.queue" :lines-by-month="linesByMonth" />
      <div class="side">
        <ApprovalsCard :approvals="weekly.approvals" />
        <WarningsCard :warnings="weekly.warnings" />
      </div>
    </div>
  </template>
</template>

<style scoped>
.columns {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr);
  gap: var(--space-20);
  align-items: start;
}

.side {
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
  min-width: 0;
}

@media (max-width: 820px) {
  .columns {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
