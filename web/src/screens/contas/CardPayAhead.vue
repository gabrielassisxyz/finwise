<script setup lang="ts">
import type { CardOverview } from '@/api/types';
import { brl, orUnknown } from '@/lib/format';
import { usePanelStore } from '@/stores/panel';
import CardPurchaseButton from './CardPurchaseButton.vue';

defineProps<{ card: CardOverview }>();

const panel = usePanelStore();
</script>

<template>
  <section class="block">
    <div class="head">
      <span class="title">Valor a adiantar: {{ orUnknown(card.payAhead.amount, brl) }}</span>
      <span class="explanation">
        Compras feitas a partir do fechamento. Pagando adiantado, o gasto do cartão fica igual ao do mês.
      </span>
    </div>
    <CardPurchaseButton
      v-for="purchase in card.payAhead.purchases"
      :key="purchase.transactionId"
      rule="below"
      :purchase="purchase"
      @open="panel.open(purchase.transactionId)"
    />
    <div class="maybe-title">Podem entrar na próxima fatura</div>
    <div class="maybe-note">Compras dos dois dias antes do fechamento. O banco ainda pode lançá-las depois.</div>
    <CardPurchaseButton
      v-for="purchase in card.mayFallOnNextBill"
      :key="purchase.transactionId"
      :purchase="purchase"
      @open="panel.open(purchase.transactionId)"
    />
  </section>
</template>

<style scoped>
.block {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  padding: var(--space-14) var(--space-16);
  background: var(--color-accent-card);
  border-bottom: 1px solid var(--color-border-default);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.title {
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
}

.explanation {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-3);
  text-wrap: pretty;
}

.maybe-title {
  padding: var(--space-14) var(--space-16) var(--space-6);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-semibold);
}

.maybe-note {
  padding: 0 var(--space-16) var(--space-8);
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
  text-wrap: pretty;
}
</style>
