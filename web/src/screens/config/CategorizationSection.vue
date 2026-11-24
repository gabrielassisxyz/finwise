<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { MIN_EXAMPLES_RANGE, THRESHOLD_RANGE } from '@/api/contract/settings';
import type { CategorizationSettings } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { orUnknown, shortDay } from '@/lib/format';
import { restoreOnFailure, saveStored, vStored } from '@/lib/storedField';
import CalibrationChart from './CalibrationChart.vue';
import { useSettingsWrites } from './useSettingsWrites';

const props = defineProps<{ categorization: CategorizationSettings }>();

const { api, write } = useSettingsWrites();

// The label and the marker follow the slider while it moves; the write waits for it to be released.
const threshold = ref(props.categorization.threshold);
watch(
  () => props.categorization.threshold,
  (kept) => (threshold.value = kept),
);

function commitThreshold(): Promise<void> {
  return restoreOnFailure(
    () => (threshold.value = props.categorization.threshold),
    () => write(() => api.settings.setThreshold(threshold.value)),
  );
}

const suggestedText = computed(() => orUnknown(props.categorization.suggestedThreshold, (percent) => `${percent}%`));

function useSuggested(): Promise<void> {
  const suggested = props.categorization.suggestedThreshold;
  if (suggested === null) return Promise.resolve();
  return write(() => api.settings.setThreshold(suggested), `Limite alterado para ${suggested}% de confiança.`);
}

/** The only place the model's name is shown: it tells why the chart started from zero after a change of model. */
const modelSentence = computed(() => {
  const model = props.categorization.model;
  if (model === null) return null;
  const { lastRun } = model;
  if (lastRun === null) return `O modelo ${model.name} ainda não foi consultado.`;
  const when = `${shortDay(lastRun.day)} às ${lastRun.time}`;
  if (lastRun.worked) return `Última consulta ao modelo ${model.name}: ${when}.`;
  return `A última consulta ao modelo ${model.name}, em ${when}, falhou. As pendências seguem sem sugestão.`;
});

function commitMinExamples(event: Event): Promise<void> {
  return saveStored(event, (typed) => write(() => api.settings.setMinExamples(Number(typed))));
}
</script>

<template>
  <TitledCard title="Categorização">
    <div class="body">
      <label class="slider">
        <!-- Three items, as the design draws them: its template engine splits a figure from the text around it. -->
        <span>Aplicar sozinho a partir de</span>
        <span>{{ threshold }}</span>
        <span>% de confiança</span>
        <input
          v-model.number="threshold"
          type="range"
          :min="THRESHOLD_RANGE.min"
          :max="THRESHOLD_RANGE.max"
          @change="commitThreshold"
        />
      </label>
      <label class="examples">
        Mínimo de exemplos para aprender:
        <input
          class="number"
          type="number"
          :min="MIN_EXAMPLES_RANGE.min"
          :max="MIN_EXAMPLES_RANGE.max"
          v-stored="categorization.minExamples"
          @change="commitMinExamples"
        />
      </label>
      <div class="calibration">
        <span class="subtitle">Calibração</span>
        <span class="explanation">Com que frequência você corrigiu a IA em cada faixa de confiança.</span>
        <CalibrationChart
          :bands="categorization.calibration"
          :threshold="threshold"
          :suggested="categorization.suggestedThreshold"
        />
        <div class="legend">
          <span class="key"><span class="line current" /><span>Limite atual:</span><span>{{ threshold }}</span><span>%</span></span>
          <span class="key"><span class="line suggested" />Sugerido: {{ suggestedText }}</span>
          <button v-if="categorization.suggestedThreshold !== null" class="use" @click="useSuggested">Usar o sugerido</button>
        </div>
        <span v-if="modelSentence !== null" class="explanation model-run">{{ modelSentence }}</span>
      </div>
    </div>
  </TitledCard>
</template>

<style scoped>
.body {
  padding: var(--space-16);
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.slider {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  font-size: var(--font-size-body);
}

.slider input {
  width: 100%;
  max-width: 420px;
  accent-color: var(--color-accent-base);
}

.examples {
  display: flex;
  align-items: center;
  gap: var(--space-10);
  font-size: var(--font-size-body);
  flex-wrap: wrap;
}

.number {
  width: 72px;
  height: 34px;
  box-sizing: border-box;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-8);
  font-size: var(--font-size-body);
  text-align: right;
  outline: none;
}

.calibration {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
}

.subtitle {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-semibold);
}

.explanation {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  text-wrap: pretty;
}

.legend {
  display: flex;
  gap: var(--space-16);
  flex-wrap: wrap;
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-3);
}

.key {
  display: flex;
  gap: var(--space-6);
  align-items: center;
}

.line {
  width: 14px;
}

.line.current {
  border-top: 2px solid var(--color-text-ink);
}

.line.suggested {
  border-top: 2px dashed var(--color-warn-text);
}

.use {
  border: 0;
  background: none;
  padding: 0;
  font-size: var(--font-size-caption);
  color: var(--color-accent-link);
  cursor: pointer;
  min-height: 24px;
}
</style>
