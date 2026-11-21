<script setup lang="ts">
import type { BudgetDoc, InvestmentFigures } from '@/api/types';
import { brl, brlWhole } from '@/lib/format';

defineProps<{ budget: BudgetDoc; current: InvestmentFigures }>();
</script>

<template>
  <section class="card">
    <div class="head">
      <span class="title">Budget atual</span>
      <span class="summary">
        Investimento {{ brlWhole(current.investment) }} · Taxa de investimento {{ current.rate }}%
      </span>
    </div>
    <template v-for="group in budget" :key="group.key">
      <div class="group">{{ group.name }}</div>
      <div v-for="(line, index) in group.lines" :key="index" class="line">
        <span>{{ line.name }}</span>
        <span>{{ brl(line.amount) }}</span>
      </div>
    </template>
    <div class="total">
      <span>Investimento</span>
      <span>{{ brl(current.investment) }}</span>
    </div>
  </section>
</template>

<style scoped>
.card {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  padding: var(--space-14) var(--space-16);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  /* Content-box on purpose: the design's head is 64px of content, not of box. */
  min-height: 64px;
  justify-content: center;
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.summary {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  font-variant-numeric: tabular-nums;
}

.group {
  padding: var(--space-10) var(--space-16) var(--space-4);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-ink-2);
}

.line {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 38px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.total {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
}
</style>
