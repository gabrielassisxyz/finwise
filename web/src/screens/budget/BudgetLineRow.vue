<script setup lang="ts">
import { computed } from 'vue';
import type { BudgetGroupView, BudgetLineDoc, BudgetLineView } from '@/api/types';
import MarkBadge from '@/components/transactions/MarkBadge.vue';
import { cellClasses, cellText, type Columns } from './budgetText';
import { typedAmount } from './typedAmount';

const props = defineProps<{
  group: BudgetGroupView;
  line: BudgetLineView;
  columns: Columns;
  open: boolean;
  /** Present while the line is being edited. */
  draft: BudgetLineDoc | null;
}>();

const emit = defineEmits<{
  toggle: [];
  rename: [name: string];
  setAmount: [amount: number];
  up: [];
  down: [];
  remove: [];
}>();

const clickable = computed(() => props.line.expandable && !props.draft);

function typed(event: Event): string {
  return (event.target as HTMLInputElement).value;
}
</script>

<template>
  <div
    class="row stack-row"
    :class="{ clickable }"
    :role="clickable ? 'button' : undefined"
    :aria-expanded="clickable ? open : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="clickable && emit('toggle')"
    @keydown.enter.self="clickable && emit('toggle')"
  >
    <span class="name-cell stack-full">
      <template v-if="!draft">
        <span class="chev">{{ clickable ? (open ? '▾' : '▸') : '' }}</span>
        <span>{{ line.name }}</span>
      </template>
      <template v-else>
        <input class="name-input" :value="draft.name" aria-label="Nome da linha" @change="emit('rename', typed($event))" />
        <button class="step" aria-label="Subir" @click="emit('up')">↑</button>
        <button class="step" aria-label="Descer" @click="emit('down')">↓</button>
        <button class="step remove" aria-label="Remover linha" @click="emit('remove')">×</button>
      </template>
      <MarkBadge v-for="mark in line.marks" :key="mark.text" :mark="mark" />
    </span>
    <span class="value" :data-label="columns[0]">
      <input
        v-if="draft"
        class="amount-input"
        type="number"
        :value="draft.amount"
        aria-label="Valor"
        @change="emit('setAmount', typedAmount(typed($event)))"
      />
      <template v-else>{{ cellText(line.cells[0]) }}</template>
    </span>
    <span class="value" :data-label="columns[1]">{{ cellText(line.cells[1]) }}</span>
    <span class="value" :class="cellClasses(group.key, line, 2)" :data-label="columns[2]">{{ cellText(line.cells[2]) }}</span>
    <span>
      <span v-if="line.bar !== null" class="bar">
        <span class="fill" :class="{ over: line.over }" :style="{ width: `${line.bar}%` }" />
      </span>
    </span>
  </div>
</template>

<style scoped>
.row {
  display: grid;
  grid-template-columns: minmax(0, 2fr) repeat(3, minmax(0, 1fr)) minmax(0, 1.1fr);
  gap: var(--space-16);
  padding: 0 var(--space-16);
  min-height: 46px;
  align-items: center;
  font-size: var(--font-size-body);
  border-top: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.row:hover {
  background: var(--color-surface-hover-row);
}

.row.clickable {
  cursor: pointer;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  min-width: 0;
  flex-wrap: wrap;
}

.chev {
  font-size: var(--font-size-micro);
  color: var(--color-text-muted-2);
  width: 10px;
  flex-shrink: 0;
}

.value {
  text-align: right;
}

.bad {
  color: var(--color-bad-text);
}

.warning {
  color: var(--color-warn-text);
}

.quiet {
  color: var(--color-text-muted-2);
}

.secondary {
  color: var(--color-text-ink-2);
}

.strong {
  font-weight: var(--font-weight-semibold);
}

.medium {
  font-weight: var(--font-weight-medium);
}

.bar {
  display: block;
  height: 6px;
  border-radius: var(--radius-bar);
  background: var(--color-surface-track);
  overflow: hidden;
}

.fill {
  display: block;
  height: 100%;
  background: var(--color-accent-base);
  border-radius: var(--radius-bar);
}

.fill.over {
  background: var(--color-bad-text);
}

.name-input,
.amount-input {
  height: 32px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-item);
  padding: 0 var(--space-8);
  font-size: var(--font-size-body);
  outline: none;
}

.name-input {
  /* Content-box on purpose: the design's name field is 2px taller than its value. */
  flex: 1;
  min-width: 90px;
}

.amount-input {
  box-sizing: border-box;
  width: 100%;
  max-width: 110px;
  text-align: right;
}

.step {
  width: 28px;
  height: 32px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-item);
  background: var(--color-surface-card);
  cursor: pointer;
  color: var(--color-text-ink-3);
}

.step.remove {
  border-color: var(--color-bad-border);
  color: var(--color-bad-text);
}
</style>
