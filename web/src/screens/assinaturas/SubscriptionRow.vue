<script setup lang="ts">
import type { SubscriptionRow } from '@/api/types';
import MarkBadge from '@/components/transactions/MarkBadge.vue';
import { brl, longDay, UNKNOWN } from '@/lib/format';
import StatusPill from './StatusPill.vue';
import { rowSubtitle } from './subscriptionText';

defineProps<{ row: SubscriptionRow }>();
defineEmits<{ open: []; pickStatus: [anchor: HTMLElement] }>();
</script>

<template>
  <div class="row stack-row" @click="$emit('open')">
    <span class="name-cell stack-full">
      <span class="name">{{ row.name }}</span>
      <MarkBadge v-for="mark in row.marks" :key="mark.text" :mark="mark" />
      <span class="subtitle narrow-only">{{ rowSubtitle(row) }}</span>
    </span>
    <span class="right" data-label="Valor">{{ brl(row.amount) }}</span>
    <span class="secondary wide-only" data-label="Ciclo">{{ row.cycle }}</span>
    <span class="secondary" data-label="Próxima cobrança">{{ longDay(row.nextCharge) }}</span>
    <span class="right wide-only" data-label="Equivalente mensal">{{ brl(row.monthlyEquivalent) }}</span>
    <span class="secondary line wide-only" data-label="Linha do budget">{{ row.line ?? UNKNOWN }}</span>
    <span data-label="Status">
      <StatusPill :status="row.status" @click.stop="$emit('pickStatus', $event.currentTarget as HTMLElement)" />
    </span>
    <span class="review" :class="{ overdue: row.reviewOverdue }" data-label="Revisar em">{{ longDay(row.reviewOn) }}</span>
  </div>
</template>

<style scoped>
.row {
  display: grid;
  grid-template-columns: var(--cols);
  gap: var(--space-14);
  padding: var(--space-6) var(--space-16);
  min-height: 40px;
  align-items: center;
  font-size: var(--font-size-body);
  border-top: 1px solid var(--color-border-row);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}

.row:hover {
  background: var(--color-surface-hover-row);
}

.name-cell {
  display: flex;
  gap: var(--space-6);
  align-items: center;
  flex-wrap: wrap;
  min-width: 0;
}

.name {
  font-weight: var(--font-weight-medium);
}

.subtitle {
  flex-basis: 100%;
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.right {
  text-align: right;
}

.secondary,
.review {
  color: var(--color-text-ink-2);
}

.review.overdue {
  color: var(--color-warn-text);
}

.line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
