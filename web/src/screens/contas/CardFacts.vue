<script setup lang="ts">
import type { CardOverview, IsoDay } from '@/api/types';
import { brl, shortDay, UNKNOWN } from '@/lib/format';

defineProps<{ card: CardOverview }>();

const dayOrUnknown = (day: IsoDay | null): string => (day === null ? UNKNOWN : shortDay(day));
</script>

<template>
  <div class="cell-strip facts">
    <div class="cell open-bill">
      <span class="label">Fatura aberta</span>
      <span class="value">{{ brl(card.openBill) }}</span>
      <span class="dates">
        Fecha em {{ dayOrUnknown(card.closesOn) }} · Vence em {{ shortDay(card.dueOn) }} · Melhor dia de compra:
        {{ dayOrUnknown(card.bestPurchaseDay) }}
      </span>
    </div>
    <div class="cell limit">
      <span class="label">Limite</span>
      <span class="value">{{ brl(card.limit) }}</span>
    </div>
    <div class="cell limit">
      <span class="label">Limite disponível</span>
      <span class="value">{{ brl(card.availableLimit) }}</span>
    </div>
  </div>
</template>

<style scoped>
.open-bill {
  --cell-flex: 2 1 300px;
}

.label {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.value {
  font-size: var(--font-size-value);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.dates {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  font-variant-numeric: tabular-nums;
}
</style>
