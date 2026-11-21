<script setup lang="ts">
import { ref } from 'vue';
import type { SubscriptionCharge } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { brl, longDay } from '@/lib/format';

defineProps<{ charges: SubscriptionCharge[] }>();

const openIndex = ref<number | null>(null);

function toggle(index: number): void {
  openIndex.value = openIndex.value === index ? null : index;
}
</script>

<template>
  <TitledCard title="Histórico de cobranças">
    <template v-for="(charge, index) in charges" :key="charge.day">
      <button
        class="charge"
        :class="{ foreign: charge.foreign }"
        :aria-expanded="charge.foreign ? openIndex === index : undefined"
        @click="toggle(index)"
      >
        <span class="when">
          <span class="chevron">{{ charge.foreign ? (openIndex === index ? '▾' : '▸') : '' }}</span>
          <span>{{ longDay(charge.day) }}</span>
          <span class="original">{{ charge.foreign ? `· ${charge.foreign.originalAmount}` : '' }}</span>
        </span>
        <span class="amount">{{ brl(charge.amount) }}</span>
      </button>
      <div v-if="charge.foreign && openIndex === index" class="iof">
        <span>IOF · {{ brl(charge.foreign.iof) }}</span>
        <span>já incluído no valor acima</span>
      </div>
    </template>
  </TitledCard>
</template>

<style scoped>
.charge {
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 46px;
  align-items: center;
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: none;
  font-size: var(--font-size-body);
  text-align: left;
  cursor: default;
  color: var(--color-text-ink);
  font-variant-numeric: tabular-nums;
}

.charge.foreign {
  cursor: pointer;
}

.when {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.chevron {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted-2);
  width: 8px;
}

.original {
  color: var(--color-text-muted);
}

.amount {
  font-weight: var(--font-weight-medium);
}

.iof {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-8) var(--space-16) var(--space-8) 34px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  background: var(--color-surface-nav);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}
</style>
