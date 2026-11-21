<script setup lang="ts">
import { computed } from 'vue';
import type { NetWorthOverview, NetWorthPoint, YearMonth } from '@/api/types';
import ToggleButton from '@/components/ui/ToggleButton.vue';
import { monthInSentence, monthLabel } from '@/lib/format';
import { axisLabels, columnHeight, stackShares } from './chartGeometry';

const props = defineProps<{ chart: NetWorthOverview['chart']; historyStart: YearMonth; stacked: boolean }>();
defineEmits<{ 'update:stacked': [stacked: boolean] }>();

const columns = computed(() =>
  props.chart.months.map((month) => ({
    month,
    label: monthLabel(month).slice(0, 3),
    point: props.chart.points.find((candidate) => candidate.month === month) ?? null,
  })),
);

const labels = computed(() => axisLabels(props.chart.max));

// A line needs two points; until then the single month is a dot and the note says why.
const note = computed(() => {
  const start = `O histórico começa em ${monthInSentence(props.historyStart)}.`;
  return props.chart.points.length < 2 ? `${start} A linha aparece quando houver dois meses registrados.` : start;
});

const height = (point: NetWorthPoint) => `${columnHeight(point.total, props.chart.max)}%`;
</script>

<template>
  <section class="chart-card">
    <div class="head">
      <span class="title">Patrimônio no tempo</span>
      <ToggleButton compact :pressed="stacked" @click="$emit('update:stacked', !stacked)">
        Empilhar por balde
      </ToggleButton>
    </div>
    <div class="plot-row">
      <div class="axis"><span v-for="(label, index) in labels" :key="index">{{ label }}</span></div>
      <div class="plot-column">
        <div class="plot" :style="{ gridTemplateColumns: `repeat(${columns.length}, 1fr)` }">
          <div v-for="column in columns" :key="column.month" class="slot">
            <template v-if="column.point">
              <span v-if="!stacked" class="dot" :style="{ bottom: height(column.point) }" />
              <span v-else class="stack" :style="{ height: height(column.point) }">
                <span
                  v-for="slice in stackShares(column.point)"
                  :key="slice.tone"
                  class="slice"
                  :class="slice.tone"
                  :style="{ flex: `0 0 ${slice.height}%` }"
                />
              </span>
            </template>
          </div>
        </div>
        <div class="months" :style="{ gridTemplateColumns: `repeat(${columns.length}, 1fr)` }">
          <span v-for="column in columns" :key="column.month">{{ column.label }}</span>
        </div>
      </div>
    </div>
    <span class="note">{{ note }}</span>
  </section>
</template>

<style scoped>
.chart-card {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  padding: var(--space-16) var(--space-20) 18px;
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  flex-wrap: wrap;
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.plot-row {
  display: flex;
  gap: var(--space-10);
}

.axis {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: 200px;
  font-size: var(--font-size-badge);
  color: var(--color-text-muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.plot-column {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.plot {
  height: 200px;
  display: grid;
  border-bottom: 1px solid var(--color-border-strong);
  background: repeating-linear-gradient(
    to bottom,
    var(--color-border-divider) 0,
    var(--color-border-divider) 1px,
    transparent 1px,
    transparent 50px
  );
}

.slot {
  position: relative;
  height: 100%;
}

.dot {
  position: absolute;
  left: 50%;
  width: 10px;
  height: 10px;
  margin: 0 0 -5px -5px;
  border-radius: 50%;
  background: var(--color-accent-base);
  box-shadow: 0 0 0 3px var(--color-surface-card); /* token-exempt: a ring that lifts the point off the grid lines */
}

.stack {
  position: absolute;
  left: 50%;
  bottom: 0;
  width: 14px;
  margin-left: -7px;
  display: flex;
  flex-direction: column-reverse;
  border-radius: 3px 3px 0 0; /* token-exempt: the top corners of a slim column, below the smallest radius token */
  overflow: hidden;
}

.slice.invested {
  background: var(--color-chart-3);
}

.slice.reserved {
  background: var(--color-chart-2);
}

.slice.available {
  background: var(--color-chart-1);
}

.months {
  display: grid;
  font-size: var(--font-size-badge);
  color: var(--color-text-muted);
  text-align: center;
}

.note {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  text-wrap: pretty;
}
</style>
