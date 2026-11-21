<script setup lang="ts">
import type { BudgetEstablishment } from '@/api/types';
import { brl, monthInSentence } from '@/lib/format';

defineProps<{ items: BudgetEstablishment[]; month: string }>();
defineEmits<{ pick: [establishment: BudgetEstablishment] }>();
</script>

<template>
  <div class="establishments">
    <button v-for="item in items" :key="item.key" class="establishment" @click="$emit('pick', item)">
      <span>{{ item.name }} ›</span>
      <span>{{ brl(item.amount) }} <span class="count">({{ item.count }})</span></span>
    </button>
    <span v-if="items.length === 0" class="empty">
      Nenhuma transação nesta linha em {{ monthInSentence(month) }}. Elas aparecem aqui assim que forem classificadas.
    </span>
  </div>
</template>

<style scoped>
.establishments {
  padding: var(--space-6) var(--space-16) var(--space-12) 34px;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-nav);
  border-top: 1px solid var(--color-border-row);
}

.establishment {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  min-height: 40px;
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: none;
  font-size: var(--font-size-secondary);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
  font-variant-numeric: tabular-nums;
  padding: 0;
}

.establishment:hover {
  color: var(--color-accent-positive);
}

.count {
  color: var(--color-text-muted);
}

.empty {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  padding: var(--space-8) 0;
}
</style>
