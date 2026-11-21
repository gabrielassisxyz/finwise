<script setup lang="ts">
import type { CardPurchase } from '@/api/types';
import { brl, shortDay } from '@/lib/format';

// The pay-ahead list is ruled below each row and the list after it above, as the design draws them.
defineProps<{ purchase: CardPurchase; rule?: 'above' | 'below' }>();
defineEmits<{ open: [] }>();
</script>

<template>
  <button class="purchase" :class="rule ?? 'above'" @click="$emit('open')">
    <span>{{ purchase.name }} <span class="day">· {{ shortDay(purchase.day) }}</span></span>
    <span class="amount">{{ brl(purchase.amount) }}</span>
  </button>
</template>

<style scoped>
.purchase {
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 46px;
  align-items: center;
  border: 0;
  background: none;
  font-size: var(--font-size-body);
  text-align: left;
  cursor: pointer;
  color: var(--color-text-ink);
  font-variant-numeric: tabular-nums;
}

.purchase.above {
  border-top: 1px solid var(--color-border-row);
}

.purchase.below {
  border-bottom: 1px solid var(--color-border-row);
}

.purchase:hover {
  background: var(--color-surface-hover-row);
}

.day {
  color: var(--color-text-muted);
}

.amount {
  font-weight: var(--font-weight-medium);
}
</style>
