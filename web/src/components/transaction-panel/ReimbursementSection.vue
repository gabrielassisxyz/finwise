<script setup lang="ts">
import { computed } from 'vue';
import type { TransactionDetail } from '@/api/types';
import SwitchToggle from '@/components/ui/SwitchToggle.vue';
import { brl, shortDay } from '@/lib/format';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const link = computed(() => props.detail.reimbursement?.link ?? null);
</script>

<template>
  <PanelSection>
    <div class="head">
      <span class="title">Reembolsável</span>
      <SwitchToggle :on="!!detail.reimbursement" label="Reembolsável" />
    </div>
    <template v-if="detail.reimbursement && !link">
      <label class="expected">
        Espero receber R$
        <input type="number" :value="detail.reimbursement.expected ?? ''" />
      </label>
      <button class="link-button">Ligar a um PIX recebido</button>
    </template>
    <div v-if="link" class="linked">
      <span class="linked-title">Reembolsado: {{ brl(link.amount) }} de {{ link.name }} em {{ shortDay(link.day) }}</span>
      <span class="linked-net">Esta compra conta por {{ brl(Math.abs(detail.effectiveAmount)) }}.</span>
      <button class="unlink">Desfazer ligação</button>
    </div>
  </PanelSection>
</template>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.expected {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  font-size: var(--font-size-body);
  color: var(--color-text-pill);
}

.expected input {
  width: 104px;
  height: 34px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  text-align: right;
  outline: none;
  box-sizing: border-box;
}

.link-button {
  align-self: flex-start;
  height: 34px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink);
  cursor: pointer;
}

.link-button:hover {
  background: var(--color-surface-hover-item);
}

.linked {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-10) var(--space-12);
  border-radius: var(--radius-control);
  background: var(--color-accent-card);
  font-size: var(--font-size-secondary);
}

.linked-title {
  font-weight: var(--font-weight-medium);
  color: var(--color-accent-ink);
}

.linked-net {
  color: var(--color-text-ink-3);
}

.unlink {
  align-self: flex-start;
  border: 0;
  background: none;
  padding: 0;
  min-height: 28px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}
</style>
