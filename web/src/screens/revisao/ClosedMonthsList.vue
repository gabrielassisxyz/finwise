<script setup lang="ts">
import type { ClosedMonthEntry } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import { longDay } from '@/lib/format';
import { capitalizedMonth } from './closingText';

defineProps<{ months: ClosedMonthEntry[] }>();
defineEmits<{ open: [month: string] }>();
</script>

<template>
  <div class="closed">
    <h2 class="heading">Meses fechados</h2>
    <EmptyBlock v-if="months.length === 0" title="Nenhum mês fechado ainda" hint="Os meses que você fechar ficam guardados aqui." />
    <div class="list">
      <button v-for="entry in months" :key="entry.month" class="entry" @click="$emit('open', entry.month)">
        <span class="label">{{ capitalizedMonth(entry.month) }}</span>
        <span class="sub">Fechado em {{ longDay(entry.closedOn) }} ›</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.closed {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
}

.heading {
  margin: var(--space-8) 0 0;
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-semibold);
}

.list {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.entry {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  min-height: 48px;
  padding: 0 var(--space-16);
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: none;
  font-size: var(--font-size-body);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
}

.entry:hover {
  background: var(--color-surface-hover-row);
}

.label {
  font-weight: var(--font-weight-medium);
}

.sub {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}
</style>
