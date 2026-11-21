<script setup lang="ts">
import { computed, reactive, watch } from 'vue';
import { monthInSentence } from '@/lib/format';
import { useMonthStore } from '@/stores/month';
import { useShellStore } from '@/stores/shell';
import TransactionFilters from './TransactionFilters.vue';
import TransactionRow from './TransactionRow.vue';
import { useTransactionActions } from './useTransactionActions';
import { useTransactionList } from './useTransactionList';

const month = useMonthStore();
const shell = useShellStore();

const list = useTransactionList(computed(() => month.selected));
const { tab, tabs, rows, loading, hasFilter, total, countLabel } = list;

const selected = reactive(new Set<string>());
const iofOpen = reactive(new Set<string>());

const actions = useTransactionActions(list);

const isCardTab = computed(() => tab.value === 'cartao');
const showKind = computed(() => tab.value === 'todas' || tab.value === 'outros');
const allSelected = computed(() => rows.value.length > 0 && rows.value.every((row) => selected.has(row.id)));

// A selection made under one view must not carry into another.
watch([tab, () => ({ ...list.filters }), () => month.selected], () => selected.clear());

function toggle(set: Set<string>, id: string): void {
  if (!set.delete(id)) set.add(id);
}

function toggleAll(): void {
  const selectAll = !allSelected.value;
  selected.clear();
  if (selectAll) rows.value.forEach((row) => selected.add(row.id));
}

const empty = computed(() => {
  if (hasFilter.value) {
    return {
      title: 'Nenhuma transação com esses filtros',
      hint: 'Remova um filtro ou amplie o período para ver mais resultados.',
    };
  }
  const alreadyStarted = month.selected <= (shell.overview?.today ?? '').slice(0, 7);
  return {
    title: `Nenhuma transação em ${monthInSentence(month.selected)} ainda`,
    hint: alreadyStarted
      ? 'Nada nesta aba neste mês. As transações chegam dos bancos toda madrugada.'
      : 'As transações aparecem aqui na manhã seguinte ao movimento no banco.',
  };
});
</script>

<template>
  <div class="list">
    <TransactionFilters :list="list" />

    <div class="tabs">
      <button v-for="item in tabs" :key="item.id" class="tab" :class="{ active: item.id === tab }" @click="tab = item.id">
        {{ item.label }}<span class="tab-count">{{ item.count }}</span>
      </button>
    </div>

    <div class="table">
      <div v-if="hasFilter" class="total">{{ total }}</div>
      <div class="scroller">
        <div class="content" :class="{ wide: isCardTab }">
          <div class="head desktop-only">
            <input type="checkbox" class="check" :checked="allSelected" aria-label="Selecionar todas" @change="toggleAll" />
            <span class="col-date">Data</span>
            <span class="col-description">{{ tab === 'boleto' ? 'Beneficiário' : 'Descrição' }}</span>
            <span v-if="showKind" class="col-kind">{{ tab === 'outros' ? 'Tipo' : 'Meio' }}</span>
            <span v-if="isCardTab" class="col-card">Cartão</span>
            <span class="col-line">Linha do budget</span>
            <template v-if="isCardTab">
              <span class="col-kind">Parcela</span>
              <span class="col-kind">Fatura</span>
            </template>
            <span v-else class="col-account">Conta</span>
            <span class="col-amount">Valor</span>
            <span v-if="isCardTab" class="col-original">Valor original</span>
          </div>

          <template v-if="loading">
            <div v-for="index in 6" :key="index" class="skeleton">
              <span class="bone bone-date" />
              <span class="bone bone-wide" />
              <span class="bone bone-mid" />
              <span class="bone bone-amount" />
            </div>
          </template>
          <template v-else>
            <TransactionRow
              v-for="transaction in rows"
              :key="transaction.id"
              :transaction="transaction"
              :tab="tab"
              :selected="selected.has(transaction.id)"
              :iof-open="iofOpen.has(transaction.id)"
              @toggle-selected="toggle(selected, transaction.id)"
              @toggle-iof="toggle(iofOpen, transaction.id)"
              @pick-line="actions.pickLine($event, transaction)"
            />
            <div v-if="rows.length === 0" class="empty">
              <span class="empty-title">{{ empty.title }}</span>
              <span class="empty-hint">{{ empty.hint }}</span>
            </div>
          </template>
        </div>
      </div>
      <div class="foot">
        <span>{{ countLabel }}</span>
        <span>Movimentações e “não conta” ficam fora dos totais</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
}

.tabs {
  display: flex;
  gap: var(--space-24);
  border-bottom: 1px solid var(--color-border-default);
  overflow-x: auto;
  overflow-y: hidden;
}

.tab {
  border: 0;
  background: none;
  padding: var(--space-10) 0 11px;
  margin-bottom: -1px;
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-regular);
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 7px;
  white-space: nowrap;
  flex-shrink: 0;
  color: var(--color-text-muted);
  border-bottom: 2px solid transparent;
}

.tab.active {
  color: var(--color-text-ink);
  font-weight: var(--font-weight-medium);
  border-bottom-color: var(--color-accent-base);
}

.tab-count {
  font-size: var(--font-size-caption);
  font-family: var(--font-family-mono);
  color: var(--color-text-muted);
  background: var(--color-surface-count);
  padding: 1px var(--space-6);
  border-radius: var(--radius-card);
}

.table {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.total {
  padding: var(--space-10) var(--space-16);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  border-bottom: 1px solid var(--color-border-divider);
  font-variant-numeric: tabular-nums;
}

.scroller {
  overflow-x: auto;
}

.content {
  min-width: 600px;
}

.content.wide {
  min-width: 860px;
}

/* A phone stacks each row as a card, so nothing needs a minimum width there. */
@media (max-width: 820px) {
  .content,
  .content.wide {
    min-width: 0;
  }
}

.head {
  display: flex;
  gap: var(--space-10);
  padding: var(--space-10) var(--space-12);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
  align-items: center;
}

.check {
  width: 15px;
  height: 15px;
  margin: 0;
  flex-shrink: 0;
  accent-color: var(--color-accent-base);
}

.col-date {
  width: 44px;
  flex-shrink: 0;
}

.col-description {
  flex: 2.2 1 0;
  min-width: 0;
}

.col-kind {
  width: 52px;
  flex-shrink: 0;
}

.col-card {
  width: 128px;
  flex-shrink: 0;
}

.col-line {
  flex: 1.6 1 0;
  min-width: 0;
}

.col-account {
  width: 72px;
  flex-shrink: 0;
}

.col-amount {
  width: 96px;
  flex-shrink: 0;
  text-align: right;
}

.col-original {
  width: 88px;
  flex-shrink: 0;
  text-align: right;
}

.skeleton {
  display: flex;
  gap: var(--space-14);
  padding: 0 var(--space-16);
  height: 52px;
  align-items: center;
  border-bottom: 1px solid var(--color-border-row);
}

.bone {
  height: 10px;
  border-radius: var(--radius-badge);
  background: var(--color-surface-track);
}

.bone-date {
  width: 44px;
}

.bone-wide {
  flex: 2;
}

.bone-mid {
  flex: 1;
}

.bone-amount {
  width: 80px;
}

.empty {
  padding: 44px var(--space-20);
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  align-items: center;
}

.empty-title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.empty-hint {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  max-width: 420px;
  text-wrap: pretty;
}

.foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  background: var(--color-surface-nav);
  font-variant-numeric: tabular-nums;
}
</style>
