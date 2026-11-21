<script setup lang="ts">
import { computed, toRef } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetLineOption, SubscriptionDetail } from '@/api/types';
import { useLineChange } from '@/components/transactions/useLineChange';
import { brl, longDay } from '@/lib/format';
import { useRevisionStore } from '@/stores/revision';
import StatusPill from './StatusPill.vue';
import { useSubscriptionActions } from './useSubscriptionActions';

const props = defineProps<{ subscription: SubscriptionDetail; lines: BudgetLineOption[] }>();

const api = useApi();
const revision = useRevisionStore();
const actions = useSubscriptionActions();
const { openLineMenu } = useLineChange(toRef(props, 'lines'));

const facts = computed(() => [
  ['Próxima cobrança', longDay(props.subscription.nextCharge)],
  ['Equivalente mensal', brl(props.subscription.monthlyEquivalent)],
  ['Equivalente anual', brl(props.subscription.annualEquivalent)],
]);

async function write(change: Promise<unknown>): Promise<void> {
  await change;
  revision.bump();
}

function pickLine(anchor: HTMLElement): void {
  openLineMenu(anchor, props.subscription.line, (line) => void write(api.subscriptions.setLine(props.subscription.id, line)));
}

function changeReviewDate(event: Event): void {
  const day = (event.target as HTMLInputElement).value;
  if (day) void write(api.subscriptions.setReviewDate(props.subscription.id, day));
}

function changeNote(event: Event): void {
  void write(api.subscriptions.setNote(props.subscription.id, (event.target as HTMLTextAreaElement).value));
}
</script>

<template>
  <section class="editor">
    <div v-for="[label, value] in facts" :key="label" class="fact">
      <span class="label">{{ label }}</span>
      <span>{{ value }}</span>
    </div>
    <div class="field">
      <span class="label">Linha do budget</span>
      <button class="line-button" @click="pickLine($event.currentTarget as HTMLElement)">{{ subscription.line }} ▾</button>
    </div>
    <div class="field">
      <span class="label">Status</span>
      <StatusPill tall :status="subscription.status" @click="actions.openStatusMenu($event.currentTarget as HTMLElement, subscription)" />
    </div>
    <div class="field">
      <span class="label">Revisar em</span>
      <input class="date" type="date" aria-label="Revisar em" :value="subscription.reviewOn" @change="changeReviewDate" />
    </div>
    <label class="note">
      Nota
      <textarea
        :value="subscription.note"
        rows="2"
        placeholder="Por que manter, o que conferir antes de cancelar"
        @change="changeNote"
      />
    </label>
  </section>
</template>

<style scoped>
.editor {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  padding: var(--space-6) var(--space-16) var(--space-16);
  display: flex;
  flex-direction: column;
}

.fact,
.field {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
}

.fact {
  min-height: 44px;
  font-variant-numeric: tabular-nums;
}

.field {
  min-height: 48px;
}

.label {
  color: var(--color-text-ink-2);
}

.line-button,
.date {
  height: 34px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink);
}

.line-button {
  padding: 0 var(--space-10);
  cursor: pointer;
}

.date {
  padding: 0 var(--space-8);
  outline: none;
}

.note {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  padding-top: var(--space-14);
}

.note textarea {
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: var(--space-8) var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  resize: vertical;
  color: var(--color-text-ink);
  font-weight: var(--font-weight-regular);
}
</style>
