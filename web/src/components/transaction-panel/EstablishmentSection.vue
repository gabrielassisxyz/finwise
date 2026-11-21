<script setup lang="ts">
import { ref } from 'vue';
import type { TransactionDetail } from '@/api/types';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
const editing = ref(false);
const draft = ref('');

const anchorOf = (event: Event): HTMLElement => event.currentTarget as HTMLElement;

function startEditing(): void {
  draft.value = props.detail.name;
  editing.value = true;
}

async function saveNickname(): Promise<void> {
  await actions.saveNickname(draft.value);
  editing.value = false;
}
</script>

<template>
  <PanelSection v-if="detail.counterparty" title="Estabelecimento">
    <div class="nickname">
      <template v-if="!editing">
        <span class="name">{{ detail.name }}</span>
        <button class="link" @click="startEditing()">Editar apelido</button>
      </template>
      <template v-else>
        <input v-model="draft" class="nickname-field" aria-label="Apelido" />
        <button class="save" @click="saveNickname()">Salvar</button>
      </template>
    </div>
    <div class="row">
      <span class="row-label">Linha padrão</span>
      <button class="select" @click="actions.pickDefaultLine(anchorOf($event), detail.counterparty?.defaultLine ?? null)">{{ detail.counterparty.defaultLine ?? 'Nenhuma' }} ▾</button>
    </div>
    <div class="row">
      <span class="row-label">Tags fixas</span>
      <span class="chips">
        <button v-for="tag in detail.counterparty.fixedTags" :key="tag" class="chip" title="Remover" @click="actions.removeFixedTag(tag)">{{ tag }} ×</button>
        <button class="add" @click="actions.pickFixedTag(anchorOf($event), detail)">+ tag</button>
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

.nickname-field {
  flex: 1;
  min-width: 0;
  height: 34px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
}

.save {
  height: 34px;
  padding: 0 var(--space-12);
  border-radius: var(--radius-nav);
  border: 0;
  background: var(--color-text-ink);
  color: var(--color-text-on-accent);
  font-size: var(--font-size-secondary);
  cursor: pointer;
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
