<script setup lang="ts">
import type { WeeklyGuide, WeeklyLimit } from '@/api/types';
import { brl } from '@/lib/format';

defineProps<{ limit: WeeklyLimit; week: WeeklyGuide | null }>();
</script>

<template>
  <div class="cell-strip strip">
    <div class="cell limit">
      <span class="caption">Limite do mês</span>
      <div class="bar"><div class="fill" :class="{ over: limit.over }" :style="{ width: `${limit.fill}%` }" /></div>
      <span class="detail">
        Gasto {{ brl(limit.spent) }} de {{ brl(limit.cap) }} ·
        <strong>{{ limit.over ? 'Passou' : 'Restam' }} {{ brl(limit.remaining) }}</strong>
      </span>
    </div>
    <div v-if="week" class="cell week">
      <span class="caption">Esta semana</span>
      <div class="bar"><div class="fill" :style="{ width: `${week.fill}%` }" /></div>
      <span class="detail">Guia {{ brl(week.guide) }} · gasto {{ brl(week.spent) }}</span>
    </div>
  </div>
</template>

<style scoped>
.strip {
  --cell-padding: var(--space-14) var(--space-20);
}

.cell-strip > .limit {
  --cell-flex: 2 1 280px;
  gap: var(--space-8);
}

.cell-strip > .week {
  --cell-flex: 1 1 220px;
  gap: var(--space-8);
}

.caption {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.bar {
  height: 8px;
  border-radius: var(--radius-bar);
  background: var(--color-surface-track);
  overflow: hidden;
}

.fill {
  height: 100%;
  background: var(--color-accent-base);
  border-radius: var(--radius-bar);
}

.fill.over {
  background: var(--color-bad-text);
}

.detail {
  font-size: var(--font-size-body);
  font-variant-numeric: tabular-nums;
}

.detail strong {
  font-weight: var(--font-weight-semibold);
}
</style>
