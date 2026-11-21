<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
</script>

<template>
  <PanelSection v-if="detail.counterparty" title="Apelido da pessoa" tight>
    <input class="field" :value="detail.counterparty.nickname ?? ''" placeholder="Aluguel"
      @change="actions.saveNickname(($event.target as HTMLInputElement).value)"
    />
    <span class="hint">Vale para todo PIX com {{ detail.originalName }}.</span>
  </PanelSection>
</template>

<style scoped>
.field {
  height: 36px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  color: var(--color-text-ink);
}

.field:focus {
  border-color: var(--color-accent-base);
}

.hint {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}
</style>
