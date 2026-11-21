<script setup lang="ts">
import { computed, ref } from 'vue';
import type { TransactionDetail } from '@/api/types';
import { normalize } from '@/lib/text';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
const query = ref('');

const suggestions = computed(() =>
  props.detail.suggestedTags.filter((tag) => !query.value || normalize(tag).includes(normalize(query.value))),
);

async function add(tag: string): Promise<void> {
  await actions.addTag(tag);
  query.value = '';
}

function addTyped(): void {
  if (query.value.trim()) void add(query.value.trim());
}
</script>

<template>
  <PanelSection title="Tags">
    <div class="chips">
      <button v-for="tag in detail.ownTags" :key="tag" class="chip" title="Remover" @click="actions.removeTag(tag)">{{ tag }} ×</button>
      <input v-model="query" class="field" placeholder="Adicionar tag" @keydown.enter="addTyped()" />
    </div>
    <div v-if="suggestions.length" class="chips">
      <span class="suggestions-label">Sugestões</span>
      <button v-for="tag in suggestions" :key="tag" class="suggestion" @click="add(tag)">{{ tag }}</button>
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
