<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import { isApiFailure } from '@/api/http/failure';
import type { BudgetEstablishment, BudgetView, EditableGroupKey } from '@/api/types';
import ActionButton from '@/components/ui/ActionButton.vue';
import { brl, monthInSentence, percentText } from '@/lib/format';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import BudgetGroup from './BudgetGroup.vue';
import { useBudgetDraft } from './useBudgetDraft';
import { useOpenLine } from './useOpenLine';

const api = useApi();
const router = useRouter();
const month = useMonthStore();
const overlay = useOverlayStore();
const revision = useRevisionStore();

const view = ref<BudgetView | null>(null);
const budget = useBudgetDraft();
const openLine = useOpenLine(computed(() => month.selected));
const editing = computed(() => budget.draft.value !== null);

// Every answer is checked against the request that is still the latest, so a slow one never lands over a newer one.
let latest = 0;
watch(
  () => [month.selected, revision.current, budget.draft.value] as const,
  async ([selected, , draft]) => {
    const request = ++latest;
    try {
      const loaded = await api.budget.view(selected, draft ?? undefined);
      if (request === latest) view.value = loaded;
    } catch (error) {
      // A draft the backend refuses to evaluate would be asked again after every refusal, so the edit ends here.
      if (draft && isApiFailure(error) && error.kind === 'read_only') budget.discard();
      throw error;
    }
  },
  { immediate: true },
);

// Changing the month ends the edit and closes the open line.
watch(
  () => month.selected,
  () => {
    budget.discard();
    openLine.close();
  },
);

async function startEdit(): Promise<void> {
  openLine.close();
  budget.start(await api.budget.document(month.selected));
}

function askToSave(): void {
  overlay.confirm({
    message: `Vale a partir de ${monthInSentence(month.selected)}. Meses fechados não mudam.`,
    confirmLabel: 'Salvar',
    onConfirm: () => void save(),
  });
}

async function save(): Promise<void> {
  if (!budget.draft.value) return;
  await api.budget.save(month.selected, budget.draft.value);
  budget.discard();
  revision.bump();
}

function askToRemove(key: EditableGroupKey, index: number): void {
  const name = budget.draft.value?.find((group) => group.key === key)?.lines[index]?.name ?? '';
  overlay.confirm({
    message: `Remover a linha ${name}? As transações dela ficam sem categoria a partir de ${monthInSentence(month.selected)}.`,
    confirmLabel: 'Remover linha',
    onConfirm: () => budget.remove(key, index),
  });
}

function showEstablishment(establishment: BudgetEstablishment): void {
  void router.push({ path: '/', query: { establishment: establishment.key } });
}

// The groups below the editable ones are derived, so they are never empty of a budget by themselves.
const EDITABLE_KEYS: string[] = ['rec', 'fix', 'var', 'prov', 'res'];
const noLines = computed(
  () => view.value?.groups.filter((group) => EDITABLE_KEYS.includes(group.key)).every((group) => group.lines.length === 0) ?? false,
);

const figures = computed(() => {
  const current = view.value;
  if (!current) return [];
  const inv = current.groups.find((group) => group.key === 'inv');
  return [
    { label: 'Receita', text: brl(current.income), tone: '' },
    { label: 'Destinado', text: brl(current.allocated), tone: '' },
    { label: 'Taxa de investimento', text: percentText(current.investmentRate), tone: inv?.over ? 'bad' : 'positive' },
  ];
});
</script>

<template>
  <div class="screen budget" data-screen-label="Budget">
    <template v-if="view">
      <div class="top">
        <h1 class="screen-title">Budget</h1>
        <div class="actions">
          <template v-if="!month.isClosed && !editing">
            <ActionButton @click="router.push('/budget/cenarios')">Cenários</ActionButton>
            <ActionButton primary @click="startEdit()">Editar budget</ActionButton>
          </template>
          <template v-if="editing">
            <ActionButton @click="budget.discard()">Cancelar</ActionButton>
            <ActionButton primary class="save" @click="askToSave()">Salvar</ActionButton>
          </template>
        </div>
      </div>
      <div v-if="month.isClosed" class="notice">Budget de {{ monthInSentence(month.selected) }} · fechado</div>
      <div v-if="noLines && !month.isClosed && !editing" class="notice">
        Nenhuma linha no budget ainda. Use Editar budget para montar o seu.
      </div>
      <div class="cell-strip figures">
        <div v-for="figure in figures" :key="figure.label" class="cell fig">
          <span class="label">{{ figure.label }}</span>
          <span class="value" :class="figure.tone">{{ figure.text }}</span>
        </div>
      </div>
      <BudgetGroup
        v-for="group in view.groups"
        :key="group.key"
        :group="group"
        :month="month.selected"
        :open-line="openLine.key.value"
        :establishments="openLine.establishments.value"
        :draft="budget.draft.value?.find((candidate) => candidate.key === group.key) ?? null"
        @toggle-line="openLine.toggle"
        @pick="showEstablishment"
        @rename="(index, name) => budget.rename(group.key as EditableGroupKey, index, name)"
        @set-amount="(index, amount) => budget.setAmount(group.key as EditableGroupKey, index, amount)"
        @move="(index, by) => budget.move(group.key as EditableGroupKey, index, by)"
        @remove="(index) => askToRemove(group.key as EditableGroupKey, index)"
        @add="budget.add(group.key as EditableGroupKey)"
      />
    </template>
  </div>
</template>

<style scoped>
.budget {
  gap: var(--space-16);
}

.top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-12);
  flex-wrap: wrap;
}

.actions {
  display: flex;
  gap: var(--space-8);
}

.actions > .save {
  padding: 0 var(--space-16);
}

.notice {
  padding: var(--space-10) var(--space-14);
  border-radius: var(--radius-control);
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-nav);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.figures {
  --cell-flex: 1 1 180px;
  --cell-padding: var(--space-14) var(--space-20);
}

.figures > .cell.fig {
  gap: var(--space-4);
}

.label {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.value {
  font-size: var(--font-size-value);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.value.positive {
  color: var(--color-accent-positive);
}

.value.bad {
  color: var(--color-bad-text);
}
</style>
