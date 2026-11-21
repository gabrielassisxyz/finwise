<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import PanelSection from './PanelSection.vue';

defineProps<{ detail: TransactionDetail }>();
</script>

<template>
  <PanelSection v-if="detail.counterparty" title="Estabelecimento">
    <div class="nickname">
      <span class="name">{{ detail.name }}</span>
      <button class="link">Editar apelido</button>
    </div>
    <div class="row">
      <span class="row-label">Linha padrão</span>
      <button class="select">{{ detail.counterparty.defaultLine ?? 'Nenhuma' }} ▾</button>
    </div>
    <div class="row">
      <span class="row-label">Tags fixas</span>
      <span class="chips">
        <button v-for="tag in detail.counterparty.fixedTags" :key="tag" class="chip" title="Remover">{{ tag }} ×</button>
        <button class="add">+ tag</button>
      </span>
    </div>
  </PanelSection>
</template>

<style scoped>
.nickname {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  flex-wrap: wrap;
}

.name {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.link {
  border: 0;
  background: none;
  padding: 0;
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}

.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  font-size: var(--font-size-secondary);
}

.row-label {
  color: var(--color-text-ink-2);
}

.select {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  border-radius: var(--radius-nav);
  height: 32px;
  padding: 0 var(--space-10);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
}

.chips {
  display: flex;
  gap: var(--space-6);
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
}

.chip {
  border: 0;
  font-size: var(--font-size-caption);
  font-family: var(--font-family-mono);
  padding: 3px var(--space-8);
  border-radius: var(--radius-badge);
  background: var(--color-surface-count);
  color: var(--color-text-ink-3);
  cursor: pointer;
}

.add {
  border: 1px dashed var(--color-border-strong);
  background: var(--color-surface-card);
  border-radius: var(--radius-badge);
  height: 26px;
  padding: 0 var(--space-8);
  font-size: var(--font-size-caption);
  cursor: pointer;
  color: var(--color-text-ink-2);
}
</style>
