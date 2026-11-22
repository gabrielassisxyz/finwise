<script setup lang="ts">
import { computed, ref } from 'vue';
import type { TransactionDetail } from '@/api/types';
import SwitchToggle from '@/components/ui/SwitchToggle.vue';
import { brl, shortDay } from '@/lib/format';
import { saveStored, vStored } from '@/lib/storedField';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();
const linking = ref(false);

async function link(creditId: string): Promise<void> {
  await actions.link(creditId);
  linking.value = false;
}

function expectedFrom(typed: string): number | null {
  return typed === '' ? null : Number(typed);
}

const linked = computed(() => props.detail.reimbursement?.link ?? null);
</script>

<template>
  <PanelSection>
    <div class="head">
      <span class="title">Reembolsável</span>
      <SwitchToggle :on="!!detail.reimbursement" label="Reembolsável" @toggle="actions.setReimbursable(!detail.reimbursement)" />
    </div>
    <template v-if="detail.reimbursement && !linked">
      <label class="expected">
        Espero receber R$
        <input
          type="number"
          v-stored="detail.reimbursement.expected"
          @change="saveStored($event, (typed) => actions.setExpected(expectedFrom(typed)))"
        />
      </label>
      <button class="link-button" @click="linking = !linking">Ligar a um PIX recebido</button>
      <div v-if="linking" class="candidates">
        <button
          v-for="credit in detail.reimbursementCandidates"
          :key="credit.transactionId"
          class="candidate"
          @click="link(credit.transactionId)"
        >
          <span>{{ credit.name }} · {{ shortDay(credit.day) }}</span>
          <span class="candidate-amount">+&nbsp;{{ brl(credit.amount) }}</span>
        </button>
      </div>
    </template>
    <div v-if="linked" class="linked">
      <span class="linked-title">Reembolsado: {{ brl(linked.amount) }} de {{ linked.name }} em {{ shortDay(linked.day) }}</span>
      <span class="linked-net">Esta compra conta por {{ brl(Math.abs(detail.effectiveAmount)) }}.</span>
      <button class="unlink" @click="actions.unlink()">Desfazer ligação</button>
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

.candidates {
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-control);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.candidate {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  min-height: 44px;
  align-items: center;
  padding: 0 var(--space-12);
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
}

.candidate:hover {
  background: var(--color-accent-card);
}

.candidate-amount {
  font-weight: var(--font-weight-medium);
  font-variant-numeric: tabular-nums;
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
