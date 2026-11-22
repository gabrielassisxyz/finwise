<script setup lang="ts">
import { ref, watch } from 'vue';
import { CLOSING_DAY_RANGE } from '@/api/contract/settings';
import type { CardClosing, ClosingMode } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import TitledCard from '@/components/ui/TitledCard.vue';
import { restoreOnFailure, saveStored, vStored } from '@/lib/storedField';
import { useSettingsWrites } from './useSettingsWrites';

const props = defineProps<{ title: string | null; closing: CardClosing }>();

const { api, write } = useSettingsWrites();

const manualDay = ref(props.closing.manualDay);
watch(
  () => props.closing.manualDay,
  (kept) => (manualDay.value = kept),
);

const autoRadio = ref<HTMLInputElement | null>(null);
const manualRadio = ref<HTMLInputElement | null>(null);

// Picking a radio checks it in the element and unchecks the other, and neither binding moves when the write fails.
function showStoredMode(): void {
  if (autoRadio.value) autoRadio.value.checked = props.closing.mode === 'auto';
  if (manualRadio.value) manualRadio.value.checked = props.closing.mode === 'manual';
}

function choose(mode: ClosingMode): Promise<void> {
  return restoreOnFailure(showStoredMode, () => write(() => api.settings.setClosing(mode, props.closing.manualDay)));
}

function commitDay(event: Event): Promise<void> {
  return saveStored(event, (typed) => write(() => api.settings.setClosing('manual', Number(typed))));
}
</script>

<template>
  <TitledCard title="Cartão">
    <EmptyBlock
      v-if="title === null"
      title="Nenhum cartão encontrado"
      hint="O dia de fechamento aparece aqui depois que o banco enviar a fatura."
    />
    <div v-else class="body">
      <span class="caption">Dia de fechamento · {{ title }}</span>
      <label class="option">
        <input ref="autoRadio" class="radio" type="radio" name="closing" :checked="closing.mode === 'auto'" @change="choose('auto')" />
        Automático (pela regra do banco)
      </label>
      <label class="option wrapping">
        <input ref="manualRadio" class="radio" type="radio" name="closing" :checked="closing.mode === 'manual'" @change="choose('manual')" />
        Manual
        <span v-if="closing.mode === 'manual'" class="day">
          dia
          <input
            class="number"
            type="number"
            aria-label="Dia de fechamento"
            :min="CLOSING_DAY_RANGE.min"
            :max="CLOSING_DAY_RANGE.max"
            v-stored="manualDay"
            @change="commitDay"
          />
        </span>
      </label>
    </div>
  </TitledCard>
</template>

<style scoped>
.body {
  padding: var(--space-16);
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  font-size: var(--font-size-body);
}

.caption {
  color: var(--color-text-ink-2);
}

.option {
  display: flex;
  align-items: center;
  gap: var(--space-10);
  min-height: 32px;
  cursor: pointer;
}

.option.wrapping {
  flex-wrap: wrap;
}

.radio {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--color-accent-base);
}

.day {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  color: var(--color-text-ink-2);
}

.number {
  width: 64px;
  height: 34px;
  box-sizing: border-box;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-8);
  font-size: var(--font-size-body);
  text-align: right;
  outline: none;
}
</style>
