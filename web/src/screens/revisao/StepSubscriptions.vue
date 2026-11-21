<script setup lang="ts">
import type { SubscriptionToReview } from '@/api/types';
import { brl, longDay } from '@/lib/format';

defineProps<{ subscriptions: SubscriptionToReview[]; readOnly: boolean }>();
defineEmits<{ keep: [id: string]; cancel: [id: string] }>();
</script>

<template>
  <div v-for="subscription in subscriptions" :key="subscription.id" class="row">
    <span class="what">
      <span class="name">{{ subscription.name }}</span>
      <span class="sub">
        {{ brl(subscription.amount) }} · {{ subscription.cycle.toLowerCase() }} · revisar em {{ longDay(subscription.reviewOn) }}
      </span>
    </span>
    <span v-if="!readOnly" class="buttons">
      <button class="keep" @click="$emit('keep', subscription.id)">Manter</button>
      <button class="cancel" @click="$emit('cancel', subscription.id)">Cancelar</button>
    </span>
  </div>
  <span v-if="!subscriptions.length" class="none">
    Nenhuma assinatura com revisão vencida. Elas entram aqui quando a data de “Revisar em” chega.
  </span>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-10);
  flex-wrap: wrap;
  padding: var(--space-8) 0;
  border-bottom: 1px solid var(--color-border-row);
}

.what {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.name {
  font-weight: var(--font-weight-medium);
}

.sub {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.buttons {
  display: flex;
  gap: var(--space-8);
}

.buttons button {
  height: 34px;
  padding: 0 var(--space-12);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
}

.keep {
  border: 1px solid var(--color-border-button);
  color: var(--color-text-ink);
}

.cancel {
  border: 1px solid var(--color-bad-border);
  color: var(--color-bad-text);
}

.none {
  color: var(--color-text-muted);
}
</style>
