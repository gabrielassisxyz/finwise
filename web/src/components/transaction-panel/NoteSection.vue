<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import { saveStored, vStored } from '@/lib/storedField';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
</script>

<template>
  <PanelSection title="Nota" tight>
    <textarea class="note" rows="2" placeholder="Escreva algo para lembrar depois" v-stored="detail.note"
      @change="saveStored($event, actions.setNote)"
    />
  </PanelSection>
</template>

<style scoped>
.note {
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: var(--space-8) var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  resize: vertical;
  color: var(--color-text-ink);
}
</style>
