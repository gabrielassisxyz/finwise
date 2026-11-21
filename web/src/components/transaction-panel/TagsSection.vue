<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import PanelSection from './PanelSection.vue';

defineProps<{ detail: TransactionDetail }>();
</script>

<template>
  <PanelSection title="Tags">
    <div class="chips">
      <button v-for="tag in detail.ownTags" :key="tag" class="chip" title="Remover">{{ tag }} ×</button>
      <input class="field" placeholder="Adicionar tag" />
    </div>
    <div v-if="detail.suggestedTags.length" class="chips">
      <span class="suggestions-label">Sugestões</span>
      <button v-for="tag in detail.suggestedTags" :key="tag" class="suggestion">{{ tag }}</button>
    </div>
  </PanelSection>
</template>

<style scoped>
.chips {
  display: flex;
  gap: var(--space-6);
  flex-wrap: wrap;
  align-items: center;
}

.chip {
  border: 0;
  font-size: var(--font-size-caption);
  font-family: var(--font-family-mono);
  padding: var(--space-4) var(--space-8);
  border-radius: var(--radius-badge);
  background: var(--color-surface-count);
  color: var(--color-text-ink-3);
  cursor: pointer;
}

.field {
  flex: 1;
  min-width: 110px;
  height: 32px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-secondary);
  outline: none;
}

.suggestions-label {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted-2);
}

.suggestion {
  border: 1px dashed var(--color-border-strong);
  background: var(--color-surface-card);
  font-size: var(--font-size-caption);
  font-family: var(--font-family-mono);
  padding: 3px var(--space-8);
  border-radius: var(--radius-badge);
  color: var(--color-text-ink-3);
  cursor: pointer;
}
</style>
