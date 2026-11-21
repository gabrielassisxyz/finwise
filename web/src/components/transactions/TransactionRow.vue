<script setup lang="ts">
import { computed } from 'vue';
import type { Transaction } from '@/api/types';
import { brl, longDay, shortDay } from '@/lib/format';
import BudgetLinePill from './BudgetLinePill.vue';
import MarkBadge from './MarkBadge.vue';
import { marksOf } from './marks';
import type { TransactionTab } from './useTransactionList';

const props = defineProps<{
  transaction: Transaction;
  tab: TransactionTab;
  selected: boolean;
  iofOpen: boolean;
}>();

const emit = defineEmits<{
  open: [];
  'toggle-selected': [];
  'toggle-iof': [];
  'pick-line': [anchor: HTMLElement];
}>();

const KIND_LABELS = { cartao: 'Cartão', pix: 'PIX', boleto: 'Boleto' } as const;

const isCardTab = computed(() => props.tab === 'cartao');
const showKind = computed(() => props.tab === 'todas' || props.tab === 'outros');
const marks = computed(() => marksOf(props.transaction));

const kindLabel = computed(() => {
  const { kind, subtype } = props.transaction;
  return kind === 'outros' ? subtype : KIND_LABELS[kind];
});

const amountText = computed(() => {
  const sign = props.transaction.amount > 0 ? '+ ' : '';
  return sign + brl(props.transaction.effectiveAmount);
});

const phoneMeta = computed(() => {
  const { day, accountLabel, installment, originalAmount } = props.transaction;
  return [longDay(day), accountLabel, installment ? `Parcela ${installment}` : '', originalAmount ?? '']
    .filter(Boolean)
    .join(' · ');
});
</script>

<template>
  <div class="row desktop-only" :class="{ faded: transaction.excluded }" @click="emit('open')">
    <input
      type="checkbox"
      class="check"
      :checked="selected"
      aria-label="Selecionar"
      @click.stop
      @change="emit('toggle-selected')"
    />
    <span class="date">{{ shortDay(transaction.day) }}</span>
    <span class="description">
      <button v-if="transaction.iof" class="iof-toggle" aria-label="Mostrar IOF" @click.stop="emit('toggle-iof')">
        {{ iofOpen ? '▾' : '▸' }}
      </button>
      <span class="name-block">
        <span class="name-line">
          <span class="name">{{ transaction.name }}</span>
          <MarkBadge v-for="mark in marks" :key="mark.text" :mark="mark" />
        </span>
        <span class="raw">{{ transaction.rawDescription }}</span>
      </span>
    </span>
    <span v-if="showKind" class="kind">
      <span class="kind-badge">{{ kindLabel }}</span>
    </span>
    <span v-if="isCardTab" class="card">{{ transaction.accountLabel }}</span>
    <span class="line">
      <BudgetLinePill :transaction="transaction" @open="emit('pick-line', $event)" />
    </span>
    <template v-if="isCardTab">
      <span class="installment">{{ transaction.installment ?? '—' }}</span>
      <span class="bill">{{ transaction.bill ?? '—' }}</span>
    </template>
    <span v-else class="account">{{ transaction.accountLabel }}</span>
    <span class="amount">
      <span class="amount-value" :class="{ incoming: transaction.amount > 0 }">{{ amountText }}</span>
      <span v-if="transaction.reimbursed" class="amount-before">{{ brl(transaction.amount) }}</span>
    </span>
    <span v-if="isCardTab" class="original">{{ transaction.originalAmount ?? '' }}</span>
  </div>

  <div class="card-row phone-only" :class="{ faded: transaction.excluded }" @click="emit('open')">
    <div class="card-top">
      <span class="card-name">{{ transaction.name }}</span>
      <span class="card-amount" :class="{ incoming: transaction.amount > 0 }">{{ amountText }}</span>
    </div>
    <div class="card-meta">{{ phoneMeta }}</div>
    <div class="card-marks">
      <BudgetLinePill :transaction="transaction" roomy @open="emit('pick-line', $event)" />
      <MarkBadge v-for="mark in marks" :key="mark.text" :mark="mark" />
      <button v-if="transaction.iof" class="card-iof" @click.stop="emit('toggle-iof')">
        IOF {{ iofOpen ? '▾' : '▸' }}
      </button>
    </div>
  </div>

  <div v-if="transaction.iof && iofOpen" class="iof-row">
    <span>IOF · {{ brl(transaction.iof) }}</span>
    <span class="iof-note">já incluído no valor acima</span>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  gap: var(--space-10);
  padding: var(--space-6) var(--space-12);
  min-height: 40px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  cursor: pointer;
}

.row:hover {
  background: var(--color-surface-hover-row);
}

.faded {
  opacity: 0.5;
}

.check {
  width: 15px;
  height: 15px;
  margin: 0;
  flex-shrink: 0;
  accent-color: var(--color-accent-base);
}

.date {
  width: 44px;
  flex-shrink: 0;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.description {
  flex: 2.2 1 0;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-6);
}

.iof-toggle {
  border: 0;
  background: none;
  width: 20px;
  height: 28px;
  margin-left: calc(var(--space-6) * -1);
  cursor: pointer;
  color: var(--color-text-ink-2);
  font-size: var(--font-size-badge);
  flex-shrink: 0;
}

.name-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  line-height: 1.3;
}

.name-line {
  display: flex;
  gap: 5px;
  align-items: center;
  flex-wrap: wrap;
  min-width: 0;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.raw {
  font-size: var(--font-size-badge);
  font-family: var(--font-family-mono);
  color: var(--color-text-muted-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kind {
  width: 52px;
  flex-shrink: 0;
  display: flex;
}

.kind-badge {
  font-size: var(--font-size-caption);
  padding: 1px var(--space-6);
  border-radius: var(--radius-badge);
  border: 1px solid var(--color-border-button);
  color: var(--color-text-ink-2);
  white-space: nowrap;
}

.card {
  width: 128px;
  flex-shrink: 0;
  color: var(--color-text-ink-2);
  font-size: var(--font-size-secondary);
  white-space: nowrap;
}

.line {
  flex: 1.6 1 0;
  min-width: 0;
  display: flex;
}

.installment,
.bill {
  width: 52px;
  flex-shrink: 0;
  color: var(--color-text-ink-2);
  font-size: var(--font-size-secondary);
}

.installment {
  font-variant-numeric: tabular-nums;
}

.account {
  width: 72px;
  flex-shrink: 0;
  color: var(--color-text-ink-2);
  font-size: var(--font-size-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.amount {
  width: 96px;
  flex-shrink: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
  display: flex;
  flex-direction: column;
  line-height: 1.3;
}

.amount-value {
  font-weight: var(--font-weight-medium);
}

.incoming {
  color: var(--color-accent-positive);
}

.amount-before {
  font-size: var(--font-size-badge);
  color: var(--color-text-muted-2);
  text-decoration: line-through;
}

.original {
  width: 88px;
  flex-shrink: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-muted);
  font-size: var(--font-size-secondary);
}

.card-row {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-12) var(--space-14);
  border-bottom: 1px solid var(--color-border-row);
}

.card-top {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  align-items: baseline;
}

.card-name,
.card-amount {
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-medium);
}

.card-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-amount {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.card-meta {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.card-marks {
  display: flex;
  gap: var(--space-6);
  flex-wrap: wrap;
  align-items: center;
}

.card-iof {
  border: 0;
  background: none;
  min-height: 44px;
  padding: 0 var(--space-6);
  font-size: var(--font-size-caption);
  color: var(--color-accent-link);
  cursor: pointer;
}

.iof-row {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 9px var(--space-16) 9px 96px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.iof-note {
  color: var(--color-text-muted-2);
}
</style>
