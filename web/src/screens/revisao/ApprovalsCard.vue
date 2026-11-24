<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { ApprovalRequest } from '@/api/types';
import ReviewCard from './ReviewCard.vue';
import { useReviewActions } from './useReviewActions';

const props = defineProps<{ approvals: ApprovalRequest[] }>();

const api = useApi();
const actions = useReviewActions();

const refusingId = ref<string | null>(null);
const refusalReason = ref('');

watch(
  () => props.approvals,
  (list) => {
    if (refusingId.value !== null && !list.some((item) => item.id === refusingId.value)) {
      refusingId.value = null;
      refusalReason.value = '';
    }
  },
);

async function approve(approval: ApprovalRequest): Promise<void> {
  const result = await api.review.approve(approval.id);
  if (result) actions.announce(`Aprovado: ${result.title.toLowerCase()}.`, result.changeId);
  else actions.refresh();
}

function startRefuse(approval: ApprovalRequest): void {
  refusingId.value = approval.id;
  refusalReason.value = '';
}

function cancelRefuse(): void {
  refusingId.value = null;
  refusalReason.value = '';
}

async function confirmRefuse(approval: ApprovalRequest): Promise<void> {
  const trimmed = refusalReason.value.trim();
  const result = trimmed ? await api.review.refuse(approval.id, trimmed) : await api.review.refuse(approval.id);
  refusingId.value = null;
  refusalReason.value = '';
  if (result) actions.notify('Recusado. Nada foi alterado.');
  else actions.refresh();
}
</script>

<template>
  <ReviewCard title="Aguardando seu ok">
    <div v-for="approval in approvals" :key="approval.id" class="request">
      <span class="text">
        <span class="title">{{ approval.title }}</span>
        {{ ' ' }}
        <span class="reason">· {{ approval.reason }}</span>
        <span v-if="approval.proposer" class="proposer"> · proposto por {{ approval.proposer }}</span>
      </span>
      <div v-if="refusingId === approval.id" class="refuse-form">
        <input
          v-model="refusalReason"
          type="text"
          class="refuse-input"
          placeholder="Por quê? (opcional)"
          maxlength="500"
          @keydown.enter.prevent="confirmRefuse(approval)"
          @keydown.escape.prevent="cancelRefuse"
        />
        <div class="buttons">
          <button class="refuse confirm-refuse" @click="confirmRefuse(approval)">Recusar</button>
          <button class="cancel" @click="cancelRefuse">Cancelar</button>
        </div>
      </div>
      <div v-else class="buttons">
        <button class="approve" @click="approve(approval)">Aprovar</button>
        <button class="refuse" @click="startRefuse(approval)">Recusar</button>
      </div>
    </div>
    <div v-if="!approvals.length" class="empty">
      Nada aguardando. Mudanças que precisam do seu ok aparecem aqui depois de cada sincronização ou do relatório do mês.
    </div>
  </ReviewCard>
</template>

<style scoped>
.request {
  padding: var(--space-14) 18px; /* token-exempt: between two steps of the spacing scale */
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  border-bottom: 1px solid var(--color-border-row);
}

.text {
  font-size: var(--font-size-body);
  line-height: 1.45;
  text-wrap: pretty;
}

.title {
  font-weight: var(--font-weight-medium);
}

.reason,
.proposer {
  color: var(--color-text-muted);
}

.refuse-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.refuse-input {
  width: 100%;
  padding: var(--space-8) var(--space-10);
  border: 1px solid var(--color-border-input);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  color: var(--color-text-ink);
  font-size: var(--font-size-secondary);
  font-family: inherit;
}

.refuse-input::placeholder {
  color: var(--color-text-placeholder);
}

.refuse-input:focus {
  outline: none;
  border-color: var(--color-border-strong);
}

.buttons {
  display: flex;
  gap: var(--space-8);
}

.buttons button {
  height: 34px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-nav);
  font-size: var(--font-size-secondary);
  cursor: pointer;
}

.approve {
  border: 0;
  background: var(--color-surface-action-dark);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.approve:hover {
  background: var(--color-surface-action-dark-hover);
}

.refuse {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink-3);
}

.cancel {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-muted);
}

.empty {
  padding: 22px 18px; /* token-exempt: between two steps of the spacing scale */
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  text-wrap: pretty;
}
</style>
