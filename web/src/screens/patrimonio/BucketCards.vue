<script setup lang="ts">
import type { Bucket, NetWorthBucket } from '@/api/types';
import { brl } from '@/lib/format';

defineProps<{ buckets: NetWorthBucket[]; total: number; open: Bucket | null }>();
defineEmits<{ toggle: [bucket: Bucket] }>();

// The same three tones the chart stacks, so a card and its slice read as one thing.
const DOT_TOKENS: Record<Bucket, string> = {
  Disponível: 'dot-available',
  Reservado: 'dot-reserved',
  Investido: 'dot-invested',
};
</script>

<template>
  <div class="cell-strip">
    <button
      v-for="item in buckets"
      :key="item.bucket"
      class="cell bucket"
      :class="{ open: open === item.bucket }"
      :aria-expanded="open === item.bucket"
      @click="$emit('toggle', item.bucket)"
    >
      <span class="label">
        <span class="dot" :class="DOT_TOKENS[item.bucket]" />{{ item.bucket }} {{ open === item.bucket ? '▾' : '▸' }}
      </span>
      <span class="figure">{{ brl(item.total) }}</span>
    </button>
    <div class="cell total">
      <span class="label total-label">Patrimônio total</span>
      <span class="figure">{{ brl(total) }}</span>
    </div>
  </div>
</template>

<style scoped>
.bucket {
  background: var(--color-surface-card);
  text-align: left;
  cursor: pointer;
  color: inherit;
}

.bucket:hover {
  background: var(--color-surface-hover-row);
}

.bucket.open {
  background: var(--color-accent-card);
}

.total {
  background: var(--color-accent-card);
}

.label {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
}

.total-label {
  color: var(--color-accent-card-label);
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 2px; /* token-exempt: a swatch corner smaller than any radius token */
}

.dot-available {
  background: var(--color-chart-1);
}

.dot-reserved {
  background: var(--color-chart-2);
}

.dot-invested {
  background: var(--color-chart-3);
}

.figure {
  font-size: var(--font-size-value);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}
</style>
