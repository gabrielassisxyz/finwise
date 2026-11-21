<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import type { CardBill } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { brl, shortDay } from '@/lib/format';
import { billTitle } from './cardText';

defineProps<{ bills: CardBill[] }>();

const router = useRouter();
// One bill is open at a time; pressing the open one closes it.
const openMonth = ref<string | null>(null);

function toggle(month: string): void {
  openMonth.value = openMonth.value === month ? null : month;
}
</script>

<template>
  <TitledCard title="Faturas anteriores">
    <template v-for="bill in bills" :key="bill.month">
      <button class="bill" :aria-expanded="openMonth === bill.month" @click="toggle(bill.month)">
        <span class="lead">
          <span class="chevron">{{ openMonth === bill.month ? '▾' : '▸' }}</span>
          <span class="text">
            <span class="month">{{ billTitle(bill.month) }}</span>
            <span class="dates">Fechou em {{ shortDay(bill.closedOn) }} · Venceu em {{ shortDay(bill.dueOn) }}</span>
          </span>
        </span>
        <span class="total">{{ brl(bill.total) }}</span>
      </button>
      <template v-if="openMonth === bill.month">
        <div v-for="item in bill.items" :key="item.name + (item.day ?? '')" class="item">
          <span>{{ item.name }} <span class="day">· {{ item.day ? shortDay(item.day) : '—' }}</span></span>
          <span>{{ brl(item.amount) }}</span>
        </div>
      </template>
    </template>
    <button class="adjust" @click="router.push('/config/cartao')">Ajustar dia de fechamento ›</button>
  </TitledCard>
</template>

<style scoped>
.bill {
  width: 100%;
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-8) var(--space-16);
  min-height: 54px;
  align-items: center;
  border: 0;
  border-bottom: 1px solid var(--color-border-row);
  background: none;
  font-size: var(--font-size-body);
  text-align: left;
  cursor: pointer;
  color: var(--color-text-ink);
  font-variant-numeric: tabular-nums;
}

.bill:hover {
  background: var(--color-surface-hover-row);
}

.lead {
  display: flex;
  gap: var(--space-8);
  align-items: center;
}

.chevron {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted-2);
}

.text {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.month,
.total {
  font-weight: var(--font-weight-medium);
}

.dates {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.item {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-8) var(--space-16) var(--space-8) 34px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-pill);
  background: var(--color-surface-nav);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.day {
  color: var(--color-text-muted);
}

.adjust {
  margin: var(--space-6) var(--space-16) var(--space-10);
  border: 0;
  background: none;
  padding: 0;
  min-height: 36px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}
</style>
