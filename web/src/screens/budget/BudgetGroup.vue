<script setup lang="ts">
import type { BudgetEstablishment, BudgetGroupDoc, BudgetGroupView } from '@/api/types';
import { useBudgetViewStore } from '@/stores/budget-view';
import BudgetEstablishments from './BudgetEstablishments.vue';
import BudgetLineRow from './BudgetLineRow.vue';
import { COLUMNS, subtotalLabel, subtotalText } from './budgetText';

const props = defineProps<{
  group: BudgetGroupView;
  month: string;
  /** The key of the line that is open, `group:line`. */
  openLine: string | null;
  establishments: BudgetEstablishment[];
  /** Present while the group's lines are being edited. */
  draft: BudgetGroupDoc | null;
}>();

const emit = defineEmits<{
  toggleLine: [key: string];
  pick: [establishment: BudgetEstablishment];
  rename: [index: number, name: string];
  setAmount: [index: number, amount: number];
  move: [index: number, by: -1 | 1];
  remove: [index: number];
  add: [];
}>();

const view = useBudgetViewStore();
const columns = COLUMNS[props.group.key];
const lineKey = (name: string) => `${props.group.key}:${name}`;
</script>

<template>
  <section class="group">
    <button class="head" :aria-expanded="view.open[group.key]" @click="view.toggle(group.key)">
      <span class="title">
        <span class="chev">{{ view.open[group.key] ? '▾' : '▸' }}</span>
        {{ group.name }}
      </span>
      <span class="subtotal">
        {{ subtotalLabel(group) }}
        <strong :class="{ bad: group.over }">{{ subtotalText(group) }}</strong>
      </span>
    </button>
    <template v-if="view.open[group.key]">
      <div class="columns desktop-only">
        <span>Linha</span>
        <span class="right">{{ columns[0] }}</span>
        <span class="right">{{ columns[1] }}</span>
        <span class="right">{{ columns[2] }}</span>
        <span />
      </div>
      <template v-for="(line, index) in group.lines" :key="index">
        <BudgetLineRow
          :group="group"
          :line="line"
          :columns="columns"
          :open="openLine === lineKey(line.name)"
          :draft="draft?.lines[index] ?? null"
          @toggle="emit('toggleLine', lineKey(line.name))"
          @rename="(name) => emit('rename', index, name)"
          @set-amount="(amount) => emit('setAmount', index, amount)"
          @up="emit('move', index, -1)"
          @down="emit('move', index, 1)"
          @remove="emit('remove', index)"
        />
        <BudgetEstablishments
          v-if="openLine === lineKey(line.name)"
          :items="establishments"
          :month="month"
          @pick="(item) => emit('pick', item)"
        />
      </template>
      <button v-if="draft" class="add" @click="emit('add')">+ Nova linha</button>
    </template>
  </section>
</template>

<style scoped>
.group {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 46px;
  border: 0;
  background: var(--color-surface-head);
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
}

.title {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.chev {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted);
  width: 10px;
}

.subtotal {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.subtotal strong {
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-ink);
  font-size: var(--font-size-body);
}

.subtotal strong.bad {
  color: var(--color-bad-text);
}

.columns {
  display: grid;
  grid-template-columns: minmax(0, 2fr) repeat(3, minmax(0, 1fr)) minmax(0, 1.1fr);
  gap: var(--space-16);
  padding: var(--space-8) var(--space-16);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  border-top: 1px solid var(--color-border-divider);
}

.right {
  text-align: right;
}

.add {
  margin: var(--space-10) var(--space-16) var(--space-12);
  height: 34px;
  padding: 0 var(--space-12);
  border: 1px dashed var(--color-border-dashed);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink-3);
}
</style>
