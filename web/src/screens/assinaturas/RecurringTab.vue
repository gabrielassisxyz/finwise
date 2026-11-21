<script setup lang="ts">
import { ref } from 'vue';
import { useApi } from '@/api/context';
import type { RecurringPurchase } from '@/api/types';
import MarkBadge from '@/components/transactions/MarkBadge.vue';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import { brl, longDay } from '@/lib/format';
import { cadenceText } from './subscriptionText';

const purchases = ref<RecurringPurchase[] | null>(null);
void useApi()
  .subscriptions.recurring()
  .then((loaded) => {
    purchases.value = loaded;
  });
</script>

<template>
  <span class="intro">Compras que se repetem sem ser assinatura. O app aprende a cadência pelas compras anteriores.</span>
  <div class="table stack-table">
    <div class="head desktop-only">
      <span>Item</span>
      <span>Cadência esperada</span>
      <span>Última compra</span>
      <span>Próxima esperada</span>
      <span class="right">Valor médio</span>
    </div>
    <EmptyBlock
      v-if="purchases && purchases.length === 0"
      title="Nenhuma compra recorrente ainda"
      hint="Elas aparecem aqui quando o app vê uma compra se repetir."
    />
    <div v-for="purchase in purchases" :key="purchase.name" class="row stack-row">
      <span class="name-cell stack-full">
        <span class="name">{{ purchase.name }}</span>
        <MarkBadge v-if="purchase.due" :mark="{ text: 'Está na hora', tone: 'warning' }" />
      </span>
      <span class="secondary" data-label="Cadência esperada">{{ cadenceText(purchase.cadence) }}</span>
      <span class="secondary" data-label="Última compra">{{ longDay(purchase.lastPurchase) }}</span>
      <span data-label="Próxima esperada">{{ longDay(purchase.nextExpected) }}</span>
      <span class="right" data-label="Valor médio">{{ brl(purchase.averageAmount) }}</span>
    </div>
  </div>
</template>

<style scoped>
.intro {
  font-size: var(--font-size-body);
  color: var(--color-text-ink-2);
  text-wrap: pretty;
}

.table {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
  --cols: minmax(0, 1.6fr) minmax(0, 1.2fr) repeat(3, minmax(0, 1fr));
}

.head,
.row {
  display: grid;
  grid-template-columns: var(--cols);
  gap: var(--space-14);
  padding: 0 var(--space-16);
}

.head {
  padding-top: var(--space-10);
  padding-bottom: var(--space-10);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  background: var(--color-surface-head);
}

.row {
  min-height: 50px;
  align-items: center;
  font-size: var(--font-size-body);
  border-top: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.name-cell {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  flex-wrap: wrap;
}

.name {
  font-weight: var(--font-weight-medium);
}

.secondary {
  color: var(--color-text-ink-2);
}

.right {
  text-align: right;
}
</style>
