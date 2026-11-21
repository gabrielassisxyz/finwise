<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';
import { historyText } from './panelText';

defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
</script>

<template>
  <PanelSection title="Histórico" last>
    <div v-for="entry in detail.history" :key="entry.id" class="step">
      <span class="text">{{ historyText(entry) }}</span>
      <button v-if="entry.undoable" class="undo" @click="actions.undoStep(entry.id)">Desfazer</button>
    </div>
  </PanelSection>
</template>

<style scoped>
.step {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--space-12);
  font-size: var(--font-size-secondary);
  padding-left: var(--space-12);
  border-left: 2px solid var(--color-accent-badge-border);
}

.text {
  text-wrap: pretty;
  line-height: 1.45;
}

.undo {
  border: 0;
  background: none;
  padding: 0 0 0 var(--space-4);
  min-height: 24px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
  white-space: nowrap;
}
</style>
