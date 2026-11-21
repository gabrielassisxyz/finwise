<script setup lang="ts">
import { computed } from 'vue';
import type { InvestmentRate, MonthContribution } from '@/api/types';
import { brl, monthLabel } from '@/lib/format';
import ComparisonBars, { type ComparisonBar } from './ComparisonBars.vue';

const props = defineProps<{ contribution: MonthContribution; rate: InvestmentRate }>();

const contributionBars = computed<ComparisonBar[]>(() => [
  { label: 'Planejado', value: brl(props.contribution.planned), percent: 100, tone: 'plan' },
  { label: 'Feito', value: brl(props.contribution.done), percent: props.contribution.progress, tone: 'done' },
]);

// A negative plan has nothing to draw.
const rateBars = computed<ComparisonBar[]>(() => [
  { label: 'Planejada', value: `${props.rate.planned}%`, percent: Math.max(0, props.rate.planned), tone: 'plan' },
  { label: 'Real até agora', value: `${props.rate.actual}%`, percent: props.rate.actual, tone: 'done' },
]);
</script>

<template>
  <div class="split-columns">
    <section class="block">
      <span class="title">Aporte do mês</span>
      <ComparisonBars :bars="contributionBars" />
    </section>
    <section class="block">
      <div class="head">
        <span class="title">Taxa de investimento</span>
        <span class="month">{{ monthLabel(rate.month) }}</span>
      </div>
      <ComparisonBars :bars="rateBars" />
      <span class="month">Um mês por linha. Os próximos entram a cada fechamento.</span>
    </section>
  </div>
</template>

<style scoped>
.block {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  padding: var(--space-16) var(--space-20);
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
}

.head {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  align-items: baseline;
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.month {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}
</style>
