<script setup lang="ts">
import { computed } from 'vue';
import type { TransactionDetail } from '@/api/types';
import PanelSection from './PanelSection.vue';
import { detailRows } from './panelText';

const props = defineProps<{ detail: TransactionDetail }>();

const rows = computed(() => detailRows(props.detail));
</script>

<template>
  <PanelSection title="Detalhes" tight>
    <div v-for="[label, value] in rows" :key="label" class="pair">
      <span class="key">{{ label }}</span>
      <span class="value">{{ value }}</span>
    </div>
  </PanelSection>
</template>

<style scoped>
.pair {
  display: flex;
  justify-content: space-between;
  gap: var(--space-16);
  font-size: var(--font-size-secondary);
}

.key {
  color: var(--color-text-ink-2);
}

.value {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
