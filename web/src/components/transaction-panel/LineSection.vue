<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import { saveStored, vStored } from '@/lib/storedField';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';
import { lineLabel, originSentence } from './panelText';

defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();

const anchorOf = (event: Event): HTMLElement => event.currentTarget as HTMLElement;
</script>

<template>
  <PanelSection title="Linha do budget">
    <button class="line-button" @click="actions.pickLine(anchorOf($event), detail)">
      <span class="line">{{ lineLabel(detail) }}</span>
      <span v-if="originSentence(detail)" class="origin">· {{ originSentence(detail) }}</span>
      <span class="chevron">▾</span>
    </button>
    <template v-if="detail.justification !== null">
      <span class="justification">{{ detail.justification }}</span>
      <div class="actions">
        <button class="confirm" @click="actions.confirm()">Confirmar</button>
        <button class="change" @click="actions.pickLine(anchorOf($event), detail)">Alterar</button>
      </div>
    </template>
    <input
      v-if="detail.askReason"
      class="reason"
      v-stored="detail.reason"
      placeholder="Por quê? (opcional)"
      @change="saveStored($event, actions.setReason)"
    />
  </PanelSection>
</template>

<style scoped>
.line-button {
  align-self: flex-start;
  display: flex;
  align-items: center;
  gap: var(--space-8);
  min-height: 36px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  font-size: var(--font-size-body);
  cursor: pointer;
  color: var(--color-text-ink);
  text-align: left;
}

.line-button:hover {
  background: var(--color-surface-hover-item);
}

.line {
  font-weight: var(--font-weight-medium);
}

.origin {
  color: var(--color-text-muted);
}

.chevron {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted-2);
}

.justification {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-3);
  text-wrap: pretty;
}

.actions {
  display: flex;
  gap: var(--space-8);
}

.confirm,
.change {
  height: 34px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-nav);
  font-size: var(--font-size-secondary);
  cursor: pointer;
}

.confirm {
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.confirm:hover {
  background: var(--color-accent-hover);
}

.change {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink-3);
}

.change:hover {
  background: var(--color-surface-hover-item);
}

.reason {
  height: 36px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  color: var(--color-text-ink);
}

.reason:focus {
  border-color: var(--color-accent-base);
}
</style>
