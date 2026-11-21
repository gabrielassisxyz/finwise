<script setup lang="ts">
import { useRouter } from 'vue-router';
import type { CardBill } from '@/api/types';
import { brl, shortDay } from '@/lib/format';

defineProps<{ bill: CardBill }>();

const router = useRouter();
</script>

<template>
  <span>Valor a adiantar: <strong>{{ brl(bill.amount) }}</strong></span>
  <div class="list">
    <div v-for="purchase in bill.purchases" :key="`${purchase.name}-${purchase.day}-${purchase.amount}`" class="row">
      <span>{{ purchase.name }} <span class="day">· {{ shortDay(purchase.day) }}</span></span>
      <span>{{ brl(purchase.amount) }}</span>
    </div>
  </div>
  <button class="link" @click="router.push('/contas/cartao')">Abrir {{ bill.cardLabel }} ›</button>
</template>

<style scoped>
strong {
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
}

.list {
  display: flex;
  flex-direction: column;
}

.row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-8) 0;
  border-bottom: 1px solid var(--color-border-row);
  font-size: var(--font-size-secondary);
  font-variant-numeric: tabular-nums;
}

.day {
  color: var(--color-text-muted);
}

.link {
  align-self: flex-start;
  border: 0;
  background: none;
  padding: 0;
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}
</style>
