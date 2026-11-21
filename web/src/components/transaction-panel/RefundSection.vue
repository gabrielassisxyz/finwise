<script setup lang="ts">
import type { TransactionDetail } from '@/api/types';
import SwitchToggle from '@/components/ui/SwitchToggle.vue';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();

const anchorOf = (event: Event): HTMLElement => event.currentTarget as HTMLElement;
</script>

<template>
  <PanelSection>
    <div class="head">
      <span class="titles">
        <span class="kicker">Estorno</span>
        <span class="title">Marcar como estorno</span>
      </span>
      <SwitchToggle :on="detail.refund" label="Marcar como estorno" @toggle="actions.setRefund(!detail.refund)" />
    </div>
    <div v-if="detail.refund" class="offsets">
      <span class="offsets-label">Abate a linha:</span>
      <button class="select" @click="actions.pickRefundLine(anchorOf($event), detail.refundLine)">{{ detail.refundLine ?? 'Nenhuma' }} ▾</button>
    </div>
  </PanelSection>
</template>

<style scoped>
.head,
.offsets {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
}

.titles {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.kicker {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.offsets {
  font-size: var(--font-size-secondary);
}

.offsets-label {
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
</style>
