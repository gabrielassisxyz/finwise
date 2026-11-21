<script setup lang="ts">
import type { NetWorthOverview } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { signedBrl } from '@/lib/format';

defineProps<{ yields: NetWorthOverview['yields'] }>();
</script>

<template>
  <TitledCard title="Rendimento do mês">
    <div v-for="pot in yields.pots" :key="pot.name ?? 'unnamed'" class="row">
      <span>{{ pot.name ?? 'Sem nome' }} <span class="cdi">· {{ pot.cdiPercent }}% do CDI</span></span>
      <span>{{ signedBrl(pot.amount) }}</span>
    </div>
    <div class="sum">
      <span>Total</span>
      <span class="sum-amount">{{ signedBrl(yields.total) }}</span>
    </div>
  </TitledCard>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 42px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.cdi {
  color: var(--color-text-muted);
}

.sum {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
  background: var(--color-surface-nav);
}

.sum-amount {
  color: var(--color-accent-positive);
}
</style>
