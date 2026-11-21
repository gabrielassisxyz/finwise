<script setup lang="ts">
import type { PlannedActualGroup } from '@/api/types';
import { brl, signedBrl } from '@/lib/format';

defineProps<{ groups: PlannedActualGroup[] }>();
</script>

<template>
  <div class="table">
    <div class="head desktop-only">
      <span>Linha</span>
      <span class="right">Orçado</span>
      <span class="right">Realizado</span>
      <span class="right">Diferença</span>
    </div>
    <template v-for="group in groups" :key="group.name">
      <div class="row group stack-row">
        <span class="stack-full">{{ group.name }}{{ group.contributions ? ' (aportes)' : '' }}</span>
        <span class="right" data-label="Orçado">{{ brl(group.planned) }}</span>
        <span class="right" data-label="Realizado">{{ brl(group.actual) }}</span>
        <span class="right" :class="{ bad: group.difference < 0 }" data-label="Diferença">{{ signedBrl(group.difference) }}</span>
      </div>
      <div v-for="line in group.lines" :key="line.name" class="row line stack-row">
        <span class="stack-full">{{ line.name }}</span>
        <span class="right" data-label="Orçado">{{ brl(line.planned) }}</span>
        <span class="right" data-label="Realizado">{{ brl(line.actual) }}</span>
        <span class="right" :class="{ bad: line.difference < 0 }" data-label="Diferença">{{ signedBrl(line.difference) }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.table {
  border: 1px solid var(--color-border-divider);
  border-radius: var(--radius-control);
  overflow: hidden;
}

.head,
.row {
  display: grid;
  grid-template-columns: minmax(0, 2fr) repeat(3, minmax(0, 1fr));
  gap: var(--space-16);
}

.head {
  padding: 9px var(--space-14); /* token-exempt: between two steps of the spacing scale */
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  background: var(--color-surface-head);
}

.row {
  padding: var(--space-8) var(--space-14);
  font-size: var(--font-size-secondary);
  border-top: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.row.group {
  padding: 9px var(--space-14); /* token-exempt: between two steps of the spacing scale */
  font-weight: var(--font-weight-semibold);
  border-top-color: var(--color-border-divider);
  background: var(--color-surface-nav);
}

.line > :first-child {
  color: var(--color-text-pill);
}

.right {
  text-align: right;
}

.row .right:last-child {
  color: var(--color-text-ink-2);
}

.row .right.bad {
  color: var(--color-bad-text);
}
</style>
