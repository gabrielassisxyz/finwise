<script setup lang="ts">
import { computed } from 'vue';
import type { BudgetLineOption, ReviewQueue, YearMonth } from '@/api/types';
import { useReviewViewStore } from '@/stores/review-view';
import PendingItem from './PendingItem.vue';
import ReviewCard from './ReviewCard.vue';
import { usePendingAnswer } from './usePendingAnswer';

const props = defineProps<{ queue: ReviewQueue; linesByMonth: Record<YearMonth, BudgetLineOption[]> }>();

const view = useReviewViewStore();

const index = computed(() => (props.queue.items.length ? view.position % props.queue.items.length : 0));
const current = computed(() => props.queue.items[index.value] ?? null);
const answer = usePendingAnswer(computed(() => (current.value ? props.linesByMonth[current.value.day.slice(0, 7)] ?? [] : [])));
const counter = computed(() => {
  if (!current.value) return '';
  return `${props.queue.resolved + index.value + 1} de ${props.queue.resolved + props.queue.items.length}`;
});
</script>

<template>
  <ReviewCard title="Pendências" :aside="counter">
    <PendingItem
      v-if="current"
      :item="current"
      @accept="answer.accept(current)"
      @pick="answer.pickLine($event, current)"
      @skip="view.skip(index)"
    />
    <div v-else class="empty">
      <span class="done">Tudo revisado.</span>
      <span class="note">Novas pendências aparecem aqui depois da sincronização da madrugada.</span>
    </div>
  </ReviewCard>
</template>

<style scoped>
.empty {
  padding: 44px var(--space-20); /* token-exempt: between two steps of the spacing scale */
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  align-items: center;
}

.done {
  font-size: 16px; /* token-exempt: between the section and panel title sizes */
  font-weight: var(--font-weight-semibold);
}

.note {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  max-width: 340px;
  text-wrap: pretty;
}
</style>
