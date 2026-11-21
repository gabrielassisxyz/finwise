<script setup lang="ts">
import type { TransactionPeriod } from '@/api/types';
import { useOverlayStore, type MenuItem } from '@/stores/overlay';
import type { TransactionListState } from './useTransactionList';

const props = defineProps<{ list: TransactionListState; hideAccount?: boolean }>();

const overlay = useOverlayStore();

const PERIODS: { value: TransactionPeriod; label: string }[] = [
  { value: 'month', label: 'Mês selecionado' },
  { value: 'last-3-months', label: 'Últimos 3 meses' },
  { value: 'year', label: 'Este ano' },
];

function open(event: MouseEvent, title: string, items: MenuItem[]): void {
  overlay.openMenu(event.currentTarget as HTMLElement, { title, items });
}

function openPeriod(event: MouseEvent): void {
  const { filters } = props.list;
  open(
    event,
    'Período',
    PERIODS.map(({ value, label }) => ({ label, current: filters.period === value, onPick: () => (filters.period = value) })),
  );
}

function openAccount(event: MouseEvent): void {
  const { filters, options } = props.list;
  open(
    event,
    'Conta',
    options.value.accounts.map(({ value, label }) => ({
      label,
      current: filters.account === value,
      onPick: () => (filters.account = value),
    })),
  );
}

function openLine(event: MouseEvent): void {
  const { filters, options } = props.list;
  overlay.openMenu(event.currentTarget as HTMLElement, {
    search: true,
    placeholder: 'Buscar linha',
    items: options.value.lines.map(({ name, group }) => ({
      label: name,
      hint: group,
      current: filters.line === name,
      onPick: () => (filters.line = name),
    })),
  });
}

function openTag(event: MouseEvent): void {
  const { filters, options } = props.list;
  open(
    event,
    'Tag',
    options.value.tags.map((tag) => ({ label: tag, current: filters.tag === tag, onPick: () => (filters.tag = tag) })),
  );
}
</script>

<template>
  <div class="bar">
    <input
      v-model="list.filters.query"
      class="search"
      placeholder="Buscar por descrição, estabelecimento, valor ou tag"
      aria-label="Buscar transações"
    />
    <button class="filter" @click="openPeriod">Período ▾</button>
    <button v-if="!hideAccount" class="filter" @click="openAccount">Conta ▾</button>
    <button class="filter" @click="openLine">Linha do budget ▾</button>
    <button class="filter" @click="openTag">Tag ▾</button>
    <button
      class="filter"
      :class="{ on: list.filters.onlyPending }"
      :aria-pressed="list.filters.onlyPending"
      @click="list.filters.onlyPending = !list.filters.onlyPending"
    >
      Só pendentes
    </button>
    <button
      class="filter"
      :class="{ on: list.filters.uncategorised }"
      :aria-pressed="list.filters.uncategorised"
      @click="list.filters.uncategorised = !list.filters.uncategorised"
    >
      Sem categoria
    </button>
  </div>
  <div v-if="list.chips.value.length" class="chips">
    <span v-for="chip in list.chips.value" :key="chip.label" class="chip">
      {{ chip.label }}<button class="chip-remove" aria-label="Remover filtro" @click="chip.remove()">×</button>
    </span>
  </div>
</template>

<style scoped>
.bar {
  display: flex;
  gap: var(--space-8);
  flex-wrap: wrap;
  align-items: center;
}

.search {
  flex: 1 1 300px;
  min-width: 0;
  height: 40px;
  border: 1px solid var(--color-border-input);
  border-radius: var(--radius-control);
  padding: 0 var(--space-14);
  font-size: var(--font-size-body);
  outline: none;
  background: var(--color-surface-card);
  color: var(--color-text-ink);
  box-sizing: border-box;
}

.search:focus {
  border-color: var(--color-accent-base);
}

.filter {
  height: 40px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-regular);
  color: var(--color-text-ink-3);
  cursor: pointer;
  white-space: nowrap;
}

.filter:hover {
  background: var(--color-surface-hover-item);
}

.filter.on,
.filter.on:hover {
  background: var(--color-accent-tint);
  color: var(--color-accent-ink);
  border-color: var(--color-accent-tint-border);
  font-weight: var(--font-weight-medium);
}

.chips {
  display: flex;
  gap: var(--space-6);
  flex-wrap: wrap;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-secondary);
  padding: 0 var(--space-2) 0 var(--space-10);
  border-radius: var(--radius-pill);
  background: var(--color-accent-tint);
  color: var(--color-accent-ink);
}

.chip-remove {
  border: 0;
  background: none;
  width: 28px;
  height: 28px;
  cursor: pointer;
  color: inherit;
  font-size: var(--font-size-section);
  border-radius: 50%;
}
</style>
