<script setup lang="ts">
import type { SavingsPotDetail } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { monthInSentence, signedBrl } from '@/lib/format';
import { movementDay } from './potText';

defineProps<{ pot: SavingsPotDetail }>();
</script>

<template>
  <TitledCard title="Movimentos">
    <div v-for="movement in pot.movements" :key="movement.day + movement.description" class="movement">
      <span>
        {{ movement.description }}
        <span class="day">· {{ movementDay(movement.day, pot.yield.month.slice(0, 4)) }}</span>
      </span>
      <span class="amount">{{ signedBrl(movement.amount) }}</span>
    </div>
    <div v-if="pot.movements.length === 0" class="empty">
      Nenhum depósito ou retirada desde {{ monthInSentence(pot.historyStart) }}. Os próximos aparecem aqui no dia
      seguinte.
    </div>
  </TitledCard>
</template>

<style scoped>
.movement {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 46px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.day {
  color: var(--color-text-muted);
}

.amount {
  font-weight: var(--font-weight-medium);
}

.empty {
  padding: 22px var(--space-16);
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  text-wrap: pretty;
}
</style>
