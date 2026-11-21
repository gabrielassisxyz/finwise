<script setup lang="ts">
import { computed, ref } from 'vue';
import type { SavingsPotDetail } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { monthInSentence, orUnknown, shortDay, signedBrl } from '@/lib/format';

const props = defineProps<{ pot: SavingsPotDetail }>();

const open = ref(false);

const expanded = computed(() => open.value && props.pot.yield.days.length > 0);

// "novembro de 2026" without its year, capitalised: the heading of the month's row.
const monthName = computed(() => {
  const name = monthInSentence(props.pot.yield.month).split(' ')[0] ?? '';
  return name.charAt(0).toUpperCase() + name.slice(1);
});
</script>

<template>
  <TitledCard title="Rendimento">
    <button class="month" @click="open = !open">
      <span class="month-name"><span class="chevron">{{ expanded ? '▾' : '▸' }}</span>{{ monthName }}</span>
      <span class="total">{{ orUnknown(pot.yield.total, signedBrl) }}</span>
    </button>
    <template v-if="expanded">
      <div v-for="day in pot.yield.days" :key="day.day" class="yield-day">
        <span>{{ shortDay(day.day) }}</span>
        <span>{{ signedBrl(day.amount) }}</span>
      </div>
    </template>
    <div class="note">O histórico começa em {{ monthInSentence(pot.historyStart) }}.</div>
  </TitledCard>
</template>

<style scoped>
.month {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 48px;
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: none;
  font-size: var(--font-size-body);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
  font-variant-numeric: tabular-nums;
}

.month:hover {
  background: var(--color-surface-hover-row);
}

.month-name {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.chevron {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted-2);
}

.total {
  font-weight: var(--font-weight-medium);
}

.yield-day {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 7px var(--space-16) 7px 34px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-3);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
  background: var(--color-surface-nav);
}

.note {
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}
</style>
