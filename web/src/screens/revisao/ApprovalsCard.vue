<script setup lang="ts">
import { useApi } from '@/api/context';
import type { ApprovalRequest } from '@/api/types';
import ReviewCard from './ReviewCard.vue';
import { useReviewActions } from './useReviewActions';

defineProps<{ approvals: ApprovalRequest[] }>();

const api = useApi();
const actions = useReviewActions();

async function approve(approval: ApprovalRequest): Promise<void> {
  const result = await api.review.approve(approval.id);
  if (result) actions.announce(`Aprovado: ${result.title.toLowerCase()}.`, result.changeId);
}

async function refuse(approval: ApprovalRequest): Promise<void> {
  const result = await api.review.refuse(approval.id);
  if (result) actions.notify('Recusado. Nada foi alterado.');
}
</script>

<template>
  <ReviewCard title="Aguardando seu ok">
    <div v-for="approval in approvals" :key="approval.id" class="request">
      <span class="text">
        <span class="title">{{ approval.title }}</span> <span class="reason">· {{ approval.reason }}</span>
      </span>
      <div class="buttons">
        <button class="approve" @click="approve(approval)">Aprovar</button>
        <button class="refuse" @click="refuse(approval)">Recusar</button>
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

.reason {
  color: var(--color-text-muted);
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

.empty {
  padding: 22px 18px; /* token-exempt: between two steps of the spacing scale */
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  text-wrap: pretty;
}
</style>
