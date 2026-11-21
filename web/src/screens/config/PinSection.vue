<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useApi } from '@/api/context';
import type { ChangePinResult } from '@/api/types';
import ActionButton from '@/components/ui/ActionButton.vue';
import TitledCard from '@/components/ui/TitledCard.vue';
import { useOverlayStore } from '@/stores/overlay';

const PIN_LENGTH = 6;

const FIELDS = [
  { key: 'current', label: 'PIN atual' },
  { key: 'next', label: 'Novo PIN' },
  { key: 'repeat', label: 'Repetir novo PIN' },
] as const;

const FAULTS: Record<Exclude<ChangePinResult, { ok: true }>['reason'], string> = {
  'wrong-current': 'PIN atual incorreto',
  'invalid-new': 'O novo PIN precisa ter 6 dígitos',
  mismatch: 'Os dois campos do novo PIN não conferem',
};

const api = useApi();
const overlay = useOverlayStore();

const open = ref(false);
const error = ref('');
const pins = reactive({ current: '', next: '', repeat: '' });

function show(): void {
  Object.assign(pins, { current: '', next: '', repeat: '' });
  error.value = '';
  open.value = true;
}

function type(key: (typeof FIELDS)[number]['key'], input: HTMLInputElement): void {
  pins[key] = input.value.replace(/\D/g, '').slice(0, PIN_LENGTH);
  // A refused character leaves the stored text unchanged, so the field has to be put back by hand.
  input.value = pins[key];
  error.value = '';
}

async function save(): Promise<void> {
  const result = await api.session.changePin(pins.current, pins.next, pins.repeat);
  if (!result.ok) {
    error.value = FAULTS[result.reason];
    return;
  }
  open.value = false;
  overlay.showToast('PIN alterado.');
}
</script>

<template>
  <TitledCard title="PIN">
    <div class="body">
      <ActionButton v-if="!open" class="change" @click="show">Trocar PIN</ActionButton>
      <template v-else>
        <div class="fields">
          <label v-for="field in FIELDS" :key="field.key" class="field">
            {{ field.label }}
            <input
              type="password"
              inputmode="numeric"
              :maxlength="PIN_LENGTH"
              :value="pins[field.key]"
              @input="type(field.key, $event.target as HTMLInputElement)"
            />
          </label>
        </div>
        <span class="error">{{ error }}</span>
        <div class="buttons">
          <ActionButton primary class="save" @click="save">Salvar PIN</ActionButton>
          <ActionButton class="cancel" @click="open = false">Cancelar</ActionButton>
        </div>
      </template>
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

.change {
  align-self: flex-start;
}

.fields {
  display: flex;
  gap: var(--space-10);
  flex-wrap: wrap;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-2);
}

.field input {
  width: 120px;
  height: 36px;
  box-sizing: border-box;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-section);
  letter-spacing: 0.2em;
  outline: none;
}

.error {
  font-size: var(--font-size-secondary);
  color: var(--color-bad-text);
  min-height: 18px;
}

.buttons {
  display: flex;
  gap: var(--space-8);
}

.buttons .save:hover {
  background: var(--color-accent-base);
}

.cancel {
  color: var(--color-text-ink-3);
}
</style>
