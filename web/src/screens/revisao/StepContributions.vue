<script setup lang="ts">
import type { ClosingContribution, ContributionState } from '@/api/types';
import { brl } from '@/lib/format';

defineProps<{ contributions: ClosingContribution[] }>();

const LABELS: Record<ContributionState, string> = { done: 'Feito', 'on-target': 'No alvo', 'not-found': 'Não encontrado' };
</script>

<template>
  <div class="list">
    <div v-for="item in contributions" :key="item.name" class="row">
      <span>{{ item.name }}</span>
      <span class="figures">
        <span>{{ brl(item.amount) }}</span>
        <span class="state" :class="{ missing: item.state === 'not-found' }">{{ LABELS[item.state] }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
}

.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  padding: 9px 0; /* token-exempt: between two steps of the spacing scale */
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.figures {
  display: flex;
  gap: var(--space-14);
  align-items: center;
}

.state {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  min-width: 104px;
  text-align: right;
  color: var(--color-accent-positive);
}

.state.missing {
  color: var(--color-warn-text);
}
</style>
