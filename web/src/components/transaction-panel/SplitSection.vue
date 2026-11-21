<script setup lang="ts">
import { computed } from 'vue';
import type { TransactionDetail } from '@/api/types';
import { brl } from '@/lib/format';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();

const anchorOf = (event: Event): HTMLElement => event.currentTarget as HTMLElement;
const numberOf = (event: Event): number => Number((event.target as HTMLInputElement).value) || 0;

const closed = computed(() => props.detail.splitRemaining === 0);
const remainingText = computed(() => {
  const { splitRemaining } = props.detail;
  if (splitRemaining > 0) return `Faltam ${brl(splitRemaining)}`;
  return splitRemaining < 0 ? `Passou ${brl(splitRemaining)}` : 'Fecha com o total';
});
</script>

<template>
  <PanelSection>
    <div class="head">
      <span class="title">Dividir</span>
      <button class="toggle" @click="actions.toggleSplit(detail)">{{ detail.splitParts ? 'Remover divisão' : 'Dividir em linhas' }}</button>
    </div>
    <template v-if="detail.splitParts">
      <div v-for="(part, index) in detail.splitParts" :key="index" class="part">
        <button class="pick" @click="actions.pickSplitLine(anchorOf($event), detail, index)">{{ part.line ?? 'Escolher linha' }} ▾</button>
        <input class="value" type="number" aria-label="Valor" :value="part.amount"
          @change="actions.setSplitAmount(detail, index, numberOf($event))"
        />
        <button class="remove" aria-label="Remover linha" @click="actions.removeSplitPart(detail, index)">×</button>
      </div>
      <div class="foot">
        <button class="add" @click="actions.addSplitPart(detail)">+ Linha</button>
        <span class="remaining" :class="{ closed }">{{ remainingText }}</span>
      </div>
    </template>
  </PanelSection>
</template>

<style scoped>
.head,
.foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.toggle {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  border-radius: var(--radius-nav);
  height: 32px;
  padding: 0 var(--space-12);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink-3);
}

.part {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.pick {
  flex: 1;
  min-width: 0;
  height: 36px;
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-secondary);
  text-align: left;
  cursor: pointer;
  color: var(--color-text-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.value {
  width: 104px;
  height: 36px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  text-align: right;
  outline: none;
  box-sizing: border-box;
  font-variant-numeric: tabular-nums;
}

.remove {
  width: 32px;
  height: 36px;
  border: 0;
  background: none;
  font-size: 18px; /* token-exempt: glyph size of the remove cross */
  color: var(--color-text-muted);
  cursor: pointer;
}

.add {
  border: 0;
  background: none;
  padding: 0;
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}

.remaining {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  font-variant-numeric: tabular-nums;
  color: var(--color-warn-text);
}

.remaining.closed {
  color: var(--color-accent-positive);
}
</style>
