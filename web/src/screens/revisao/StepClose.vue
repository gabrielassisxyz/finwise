<script setup lang="ts">
import { computed } from 'vue';
import type { ClosingView } from '@/api/types';
import { closedOnText } from './closingText';

const props = defineProps<{ view: ClosingView }>();
defineEmits<{ close: []; reopen: [changeId: string] }>();

const text = computed(() => {
  const { closed, month, closedOn, pendingCount } = props.view;
  if (closed) return closedOnText(month, closedOn);
  if (pendingCount) return `Resolva as ${pendingCount} pendências antes de fechar o mês.`;
  return 'Tudo pronto. Depois de fechado, o mês fica só para leitura.';
});
</script>

<template>
  <span class="text">{{ text }}</span>
  <button v-if="!view.closed" class="close" :disabled="view.pendingCount > 0" @click="$emit('close')">Fechar o mês</button>
  <button v-else-if="view.closeChangeId !== null" class="reopen" @click="$emit('reopen', view.closeChangeId)">Reabrir mês</button>
</template>

<style scoped>
.text {
  text-wrap: pretty;
}

.close {
  align-self: flex-start;
  height: 40px;
  padding: 0 18px; /* token-exempt: between two steps of the spacing scale */
  border-radius: var(--radius-control);
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
}

.close:disabled {
  opacity: 0.4; /* token-exempt: the dimming of a disabled control */
  cursor: default;
}

.reopen {
  align-self: flex-start;
  height: 36px;
  padding: 0 var(--space-14);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  color: var(--color-text-ink);
  font-size: var(--font-size-secondary);
  cursor: pointer;
}
</style>
