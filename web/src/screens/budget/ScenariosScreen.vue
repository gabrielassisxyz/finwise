<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { BudgetDoc, SavedScenario, ScenarioImpact } from '@/api/types';
import { monthInSentence } from '@/lib/format';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import ScenarioCurrent from './ScenarioCurrent.vue';
import ScenarioEditor from './ScenarioEditor.vue';
import { copyBudget } from './useBudgetDraft';

const DEFAULT_NAME = 'Mês sem renda extra';

const api = useApi();
const router = useRouter();
const month = useMonthStore();
const overlay = useOverlayStore();
const revision = useRevisionStore();

const current = ref<BudgetDoc | null>(null);
const draft = ref<BudgetDoc | null>(null);
const name = ref(DEFAULT_NAME);
const impact = ref<ScenarioImpact | null>(null);
const saved = ref<SavedScenario[]>([]);

const ready = computed(() => current.value && draft.value && impact.value);

// A month that is closed has no scenarios, and changing month leaves them.
const leave = () => router.replace('/budget');

async function open(): Promise<void> {
  const selected = month.selected;
  const [budget, kept] = await Promise.all([api.budget.document(selected), api.budget.scenarios()]);
  if (selected !== month.selected) return;
  current.value = budget;
  draft.value = copyBudget(budget);
  saved.value = kept;
}

let latest = 0;
watch(
  draft,
  async (value) => {
    if (!value) return;
    const request = ++latest;
    const loaded = await api.budget.evaluateScenario(month.selected, value);
    if (request === latest) impact.value = loaded;
  },
  { deep: true },
);

// The first month is the frame's own configuration arriving, not the user changing month.
watch(
  () => month.selected,
  (_now, before) => (before ? leave() : open()),
);
watch(() => month.isClosed, (closed) => closed && leave(), { immediate: true });
void open();

function setAmount(group: number, line: number, amount: number): void {
  if (!draft.value) return;
  draft.value = draft.value.map((candidate, at) =>
    at !== group ? candidate : { ...candidate, lines: candidate.lines.map((l, i) => (i === line ? { ...l, amount } : l)) },
  );
}

async function save(): Promise<void> {
  if (!draft.value) return;
  const result = await api.budget.saveScenario(name.value, draft.value);
  name.value = result.name;
  saved.value = await api.budget.scenarios();
  overlay.showToast(`Cenário "${result.name}" salvo.`, []);
}

function load(scenario: SavedScenario): void {
  name.value = scenario.name;
  draft.value = copyBudget(scenario.budget);
}

function askToApply(): void {
  const label = name.value || 'Cenário sem nome';
  overlay.confirm({
    message: `Aplicar "${label}" ao budget? Vale a partir de ${monthInSentence(month.selected)}. Meses fechados não mudam.`,
    confirmLabel: 'Aplicar ao budget',
    onConfirm: () => void apply(),
  });
}

async function apply(): Promise<void> {
  if (!draft.value) return;
  await api.budget.applyScenario(month.selected, draft.value);
  revision.bump();
  await leave();
}
</script>

<template>
  <div class="screen scenarios" data-screen-label="Budget">
    <template v-if="ready">
      <button class="back" @click="router.push('/budget')">‹ Budget</button>
      <h1 class="screen-title">Cenários</h1>
      <div v-if="saved.length > 0" class="saved">
        <span class="saved-label">Salvos</span>
        <button v-for="scenario in saved" :key="scenario.name" class="chip" @click="load(scenario)">
          {{ scenario.name }}
        </button>
      </div>
      <div class="split-columns">
        <ScenarioCurrent :budget="current!" :current="impact!.current" />
        <ScenarioEditor
          :name="name"
          :budget="draft!"
          :impact="impact!"
          @rename="name = $event"
          @set-amount="setAmount"
          @save="save()"
          @apply="askToApply()"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.scenarios {
  gap: var(--space-16);
}

.back {
  align-self: flex-start;
  border: 0;
  background: none;
  padding: 0;
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}

.saved {
  display: flex;
  gap: var(--space-8);
  flex-wrap: wrap;
  align-items: center;
}

.saved-label {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.chip {
  height: 32px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-pill);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
}
</style>
