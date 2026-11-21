<script setup lang="ts">
import { computed } from 'vue';
import type { SavingsPotDetail } from '@/api/types';
import { brl, longDay } from '@/lib/format';
import { contributionText, linkedLinesText } from './potText';

const props = defineProps<{ pot: SavingsPotDetail }>();

const investmentFacts = computed(() => {
  const { investment } = props.pot;
  if (!investment) return [];
  return [
    ['Valor aplicado', brl(investment.applied)],
    ['Valor bruto', brl(investment.gross)],
    ['Imposto', brl(investment.tax)],
    ['Valor líquido', brl(investment.net)],
    ['Pode resgatar em', investment.redeemableOn ? longDay(investment.redeemableOn) : '—'],
  ] as const;
});
</script>

<template>
  <div class="cell-strip facts">
    <div class="cell contribution">
      <span class="label">Aporte do mês</span>
      <span class="value" :class="{ pending: pot.contribution.state === 'pending' }">
        {{ contributionText(pot.contribution) }}
      </span>
    </div>
    <div class="cell lines">
      <span class="label">Linhas do budget ligadas</span>
      <span class="value">{{ linkedLinesText(pot.linkedLines) }}</span>
    </div>
    <div v-for="[label, value] in investmentFacts" :key="label" class="cell investment">
      <span class="label">{{ label }}</span>
      <span class="value">{{ value }}</span>
    </div>
  </div>
</template>

<style scoped>
.facts {
  --cell-padding: var(--space-14) var(--space-20);
}

.contribution {
  --cell-flex: 1 1 220px;
}

.lines {
  --cell-flex: 2 1 280px;
}

.investment {
  --cell-flex: 1 1 150px;
}

.label {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.value {
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-medium);
}

.lines .value {
  font-variant-numeric: tabular-nums;
}

.value.pending {
  color: var(--color-warn-text);
}
</style>
