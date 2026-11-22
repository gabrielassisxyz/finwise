<script setup lang="ts">
import { computed } from 'vue';
import type { BudgetDoc, ScenarioImpact } from '@/api/types';
import ActionButton from '@/components/ui/ActionButton.vue';
import { brl, brlWhole, percentText, signedBrl } from '@/lib/format';
import { typedAmount } from './typedAmount';

const props = defineProps<{ name: string; budget: BudgetDoc; impact: ScenarioImpact }>();

const emit = defineEmits<{
  rename: [name: string];
  setAmount: [group: number, line: number, amount: number];
  save: [];
  apply: [];
}>();

const MINUS = '−';
const NBSP = ' ';

const tone = computed(() => (props.impact.difference < 0 ? 'bad' : props.impact.difference > 0 ? 'positive' : ''));

const investmentText = computed(() => {
  const { current, scenario, difference } = props.impact;
  const sign = scenario.investment < 0 ? `${MINUS}${NBSP}` : '';
  const change = difference === 0 ? 'sem diferença' : signedBrl(difference).replace(',00', '');
  return `Investimento ${brlWhole(current.investment)} → ${sign}${brlWhole(scenario.investment)} (${change})`;
});

const rateText = computed(() => `Taxa de investimento ${percentText(props.impact.current.rate)} → ${percentText(props.impact.scenario.rate)}`);

function typed(event: Event): string {
  return (event.target as HTMLInputElement).value;
}
</script>

<template>
  <section class="card">
    <div class="head">
      <input
        class="name"
        :value="name"
        aria-label="Nome do cenário"
        placeholder="Nome do cenário"
        @change="emit('rename', typed($event))"
      />
      <span class="difference" :class="tone">{{ investmentText }}</span>
      <span class="rate">{{ rateText }}</span>
    </div>
    <template v-for="(group, groupIndex) in budget" :key="group.key">
      <div class="group">{{ group.name }}</div>
      <div v-for="(line, lineIndex) in group.lines" :key="lineIndex" class="line">
        <span :class="{ changed: impact.changed[groupIndex]?.[lineIndex] }">{{ line.name }}</span>
        <input
          class="amount"
          :class="{ changed: impact.changed[groupIndex]?.[lineIndex] }"
          type="number"
          :value="line.amount"
          :aria-label="line.name"
          @change="emit('setAmount', groupIndex, lineIndex, typedAmount(typed($event)))"
        />
      </div>
    </template>
    <div class="total">
      <span>Investimento</span>
      <span :class="tone">{{ brl(impact.scenario.investment) }}</span>
    </div>
    <div class="actions">
      <ActionButton @click="emit('save')">Salvar cenário</ActionButton>
      <ActionButton primary @click="emit('apply')">Aplicar ao budget</ActionButton>
    </div>
  </section>
</template>

<style scoped>
.card {
  background: var(--color-surface-card);
  border: 1px solid var(--color-accent-tint-border);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  padding: var(--space-14) var(--space-16);
  background: var(--color-accent-card);
  border-bottom: 1px solid var(--color-border-default);
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.name {
  height: 34px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
  outline: none;
  background: var(--color-surface-card);
  color: var(--color-text-ink);
}

.difference {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  font-variant-numeric: tabular-nums;
}

.rate {
  font-size: var(--font-size-caption);
  color: var(--color-accent-card-label);
  font-variant-numeric: tabular-nums;
}

.positive {
  color: var(--color-accent-positive);
}

.bad {
  color: var(--color-bad-text);
}

.group {
  padding: var(--space-10) var(--space-16) var(--space-4);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-ink-2);
}

.line {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 38px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
}

span.changed {
  font-weight: var(--font-weight-semibold);
}

.amount {
  width: 104px;
  box-sizing: border-box;
  height: 30px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-item);
  padding: 0 var(--space-8);
  font-size: var(--font-size-body);
  text-align: right;
  outline: none;
  font-variant-numeric: tabular-nums;
}

.amount.changed {
  border-color: var(--color-chart-2);
}

.total {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
}

.actions {
  display: flex;
  gap: var(--space-8);
  flex-wrap: wrap;
  padding: var(--space-12) var(--space-16) var(--space-16);
  border-top: 1px solid var(--color-border-divider);
}
</style>
