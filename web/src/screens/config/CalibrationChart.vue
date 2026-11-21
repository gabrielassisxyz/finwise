<script setup lang="ts">
import { computed } from 'vue';
import type { CalibrationBand } from '@/api/types';
import { THRESHOLD_RANGE } from '@/api/contract/settings';

const props = defineProps<{ bands: CalibrationBand[]; threshold: number; suggested: number }>();

// The tallest bar the design draws is a share of the chart, not the whole of it.
const BAR_SCALE = 1.6;

// The chart runs from the lowest threshold to 100%.
const CHART_SPAN = 100 - THRESHOLD_RANGE.min;

const positionOf = (percent: number): string => `${((percent - THRESHOLD_RANGE.min) / CHART_SPAN) * 100}%`;

const bandLabel = (band: CalibrationBand): string => `${band.from}–${band.to}`;

const thresholdAt = computed(() => positionOf(props.threshold));
const suggestedAt = computed(() => positionOf(props.suggested));
</script>

<template>
  <div class="chart">
    <div class="scale">
      <span>50%</span>
      <span>25%</span>
      <span>0</span>
    </div>
    <div class="plot">
      <div class="bars">
        <div v-for="band in bands" :key="band.from" class="band">
          <span class="value">{{ band.correctionPercent }}%</span>
          <span class="bar" :style="{ height: `${band.correctionPercent * BAR_SCALE}%` }" />
        </div>
        <span class="marker current" :style="{ left: thresholdAt }" />
        <span class="marker suggested" :style="{ left: suggestedAt }" />
      </div>
      <div class="ranges">
        <span v-for="band in bands" :key="band.from">{{ bandLabel(band) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chart {
  display: flex;
  gap: var(--space-10);
  max-width: 560px;
}

.scale {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: 150px;
  font-size: var(--font-size-badge);
  color: var(--color-text-muted);
  text-align: right;
}

.plot {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.bars {
  position: relative;
  height: 150px;
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  align-items: end;
  border-bottom: 1px solid var(--color-border-strong);
  background: repeating-linear-gradient(
    to bottom,
    var(--color-border-divider) 0,
    var(--color-border-divider) 1px,
    transparent 1px,
    transparent 75px
  );
}

.band {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  height: 100%;
  gap: var(--space-4);
}

.value {
  font-size: var(--font-size-badge);
  color: var(--color-text-ink-3);
  font-variant-numeric: tabular-nums;
}

.bar {
  width: 56%;
  background: var(--color-chart-calibration);
  border-radius: 3px 3px 0 0; /* token-exempt: a bar's rounded top is not one of the design's radii */
}

.marker {
  position: absolute;
  top: 0;
  bottom: 0;
}

.marker.current {
  border-left: 2px solid var(--color-text-ink);
}

.marker.suggested {
  border-left: 2px dashed var(--color-warn-text);
}

.ranges {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  font-size: var(--font-size-badge);
  color: var(--color-text-muted);
  text-align: center;
}
</style>
