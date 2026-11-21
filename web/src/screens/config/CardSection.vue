<script setup lang="ts">
import { ref, watch } from 'vue';
import { CLOSING_DAY_RANGE } from '@/api/contract/settings';
import type { CardClosing, ClosingMode } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { useSettingsWrites } from './useSettingsWrites';

const props = defineProps<{ title: string; closing: CardClosing }>();

const { api, write, stored } = useSettingsWrites();

const manualDay = ref(props.closing.manualDay);
watch(
  () => props.closing.manualDay,
  (kept) => (manualDay.value = kept),
);

function choose(mode: ClosingMode): Promise<void> {
  return write(() => api.settings.setClosing(mode, props.closing.manualDay));
}

async function commitDay(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  await write(() => api.settings.setClosing('manual', Number(input.value)));
  input.value = String((await stored()).closing.manualDay);
}
</script>

<template>
  <TitledCard title="Cartão">
    <div class="body">
      <span class="caption">Dia de fechamento · {{ title }}</span>
      <label class="option">
        <input class="radio" type="radio" name="closing" :checked="closing.mode === 'auto'" @change="choose('auto')" />
        Automático (pela regra do banco)
      </label>
      <label class="option wrapping">
        <input class="radio" type="radio" name="closing" :checked="closing.mode === 'manual'" @change="choose('manual')" />
        Manual
        <span v-if="closing.mode === 'manual'" class="day">
          dia
          <input
            class="number"
            type="number"
            aria-label="Dia de fechamento"
            :min="CLOSING_DAY_RANGE.min"
            :max="CLOSING_DAY_RANGE.max"
            :value="manualDay"
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
