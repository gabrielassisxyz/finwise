<script setup lang="ts">
import { useOverlayStore } from '@/stores/overlay';

const overlay = useOverlayStore();

function accept(): void {
  const onConfirm = overlay.confirmation?.onConfirm;
  overlay.closeConfirmation();
  onConfirm?.();
}
</script>

<template>
  <div v-if="overlay.confirmation" class="scrim">
    <div class="dialog" role="dialog" aria-modal="true">
      <span class="message">{{ overlay.confirmation.message }}</span>
      <div class="buttons">
        <button class="cancel" @click="overlay.closeConfirmation()">Cancelar</button>
        <button class="confirm" @click="accept()">{{ overlay.confirmation.confirmLabel }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.scrim {
  position: fixed;
  inset: 0;
  z-index: 70;
  background: var(--color-overlay-dialog);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-20);
}

.dialog {
  width: 100%;
  max-width: 400px;
  background: var(--color-surface-card);
  border-radius: var(--radius-dialog);
  padding: 22px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  box-shadow: var(--shadow-dialog);
}

.message {
  font-size: var(--font-size-section);
  line-height: 1.5;
  text-wrap: pretty;
}

.buttons {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-8);
}

.cancel,
.confirm {
  height: 38px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-control);
  font-size: var(--font-size-body);
  cursor: pointer;
}

.cancel {
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink-3);
}

.confirm {
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.confirm:hover {
  background: var(--color-accent-hover);
}
</style>
