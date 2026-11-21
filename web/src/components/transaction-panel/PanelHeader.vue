<script setup lang="ts">
import { computed } from 'vue';
import type { TransactionDetail } from '@/api/types';
import { longDay } from '@/lib/format';
import MarkBadge from '@/components/transactions/MarkBadge.vue';
import { marksOf } from '@/components/transactions/marks';
import { signedAmount } from './panelText';

const props = defineProps<{ detail: TransactionDetail }>();
defineEmits<{ close: [] }>();

const marks = computed(() => marksOf(props.detail));
const amount = computed(() => signedAmount(props.detail.amount, props.detail.effectiveAmount));
const subtitle = computed(() => `${longDay(props.detail.day)} · ${props.detail.accountDescription}`);
</script>

<template>
  <header class="header">
    <div class="top">
      <span class="kicker">Transação</span>
      <button class="close" aria-label="Fechar" @click="$emit('close')">×</button>
    </div>
    <div class="headline">
      <span class="name">{{ detail.name }}</span>
      <span class="amount">{{ amount }}</span>
    </div>
    <span class="subtitle">{{ subtitle }}</span>
    <div class="marks">
      <MarkBadge v-for="mark in marks" :key="mark.text" :mark="mark" />
    </div>
  </header>
</template>

<style scoped>
.header {
  padding: var(--space-14) var(--space-20) 18px;
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  position: sticky;
  top: 0;
  background: var(--color-surface-card);
  z-index: 1;
  border-bottom: 1px solid var(--color-border-divider);
}

.top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
}

.kicker {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
}

/* The button is a 44px touch target pulled back so it does not add height to the row. */
.close {
  width: 44px;
  height: 44px;
  margin: -8px -12px -8px 0; /* token-exempt: the negative margin offsets the touch target */
  border: 0;
  background: none;
  font-size: var(--font-size-value);
  color: var(--color-text-ink-2);
  cursor: pointer;
  border-radius: var(--radius-control);
}

.close:hover {
  background: var(--color-surface-hover-nav);
}

.headline {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-16);
  font-size: var(--font-size-panel-title);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
}

.name {
  min-width: 0;
  text-wrap: pretty;
}

.amount {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.subtitle {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.marks {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
}

.marks :deep(.mark) {
  line-height: 1.6;
}
</style>
