<script setup lang="ts">
export interface ComparisonBar {
  label: string;
  value: string;
  /** Percent of the track the bar fills. */
  percent: number;
  tone: 'plan' | 'done';
}

defineProps<{ bars: ComparisonBar[] }>();
</script>

<template>
  <div v-for="bar in bars" :key="bar.label" class="bar-row">
    <div class="labels">
      <span class="label">{{ bar.label }}</span>
      <span class="value">{{ bar.value }}</span>
    </div>
    <div class="track"><div class="fill" :class="bar.tone" :style="{ width: `${bar.percent}%` }" /></div>
  </div>
</template>

<style scoped>
.bar-row {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.labels {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-secondary);
  font-variant-numeric: tabular-nums;
}

.label {
  color: var(--color-text-ink-2);
}

.value {
  font-weight: var(--font-weight-medium);
}

.track {
  height: 10px;
  border-radius: var(--radius-badge);
  background: var(--color-surface-track);
  overflow: hidden;
}

.fill {
  height: 100%;
  border-radius: var(--radius-badge);
}

.fill.plan {
  background: var(--color-chart-plan);
}

.fill.done {
  background: var(--color-accent-base);
}
</style>
