<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetLineOption, WeeklyReview } from '@/api/types';
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
const lines = ref<BudgetLineOption[]>([]);

watch(
  () => [month.selected, revision.current] as const,
  async ([selected]) => {
    const [loaded, options] = await Promise.all([api.review.weekly(selected), api.transactions.filterOptions()]);
    if (selected !== month.selected) return;
    weekly.value = loaded;
    lines.value = options.lines;
  },
  { immediate: true },
);
</script>

<template>
  <template v-if="weekly">
    <WeeklyStrip :limit="weekly.limit" :week="weekly.week" />
    <div class="columns">
      <PendingQueue :queue="weekly.queue" :lines="lines" />
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
