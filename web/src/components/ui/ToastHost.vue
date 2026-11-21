<script setup lang="ts">
import { useOverlayStore } from '@/stores/overlay';

const overlay = useOverlayStore();
</script>

<template>
  <!-- Whatever floats over the foot of the page stacks here, above the toast. -->
  <div class="float">
    <slot />
    <div v-if="overlay.toast" class="toast" role="status">
      <span class="message">{{ overlay.toast.message }}</span>
      <span class="actions">
        <button v-for="action in overlay.toast.actions" :key="action.label" class="action" @click="action.run()">
          {{ action.label }}
        </button>
      </span>
    </div>
  </div>
</template>

<style scoped>
.float {
  position: fixed;
  left: 12px;
  right: 12px;
  bottom: 20px;
  z-index: 50;
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  align-items: center;
  pointer-events: none;
}

/* On a phone the bottom bar sits where the toast would. */
@media (max-width: 820px) {
  .float {
    bottom: 72px;
  }
}

.toast {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: var(--space-6) var(--space-14);
  flex-wrap: wrap;
  max-width: 680px;
  background: var(--color-surface-toast);
  color: var(--color-text-on-accent);
  border-radius: var(--radius-card);
  padding: var(--space-8) var(--space-8) var(--space-8) var(--space-16);
  box-shadow: var(--shadow-toast);
  font-size: var(--font-size-body);
}

.message {
  text-wrap: pretty;
  padding: var(--space-4) 0;
}

.actions {
  display: flex;
  gap: var(--space-4);
  margin-left: auto;
}

.action {
  height: 34px;
  padding: 0 var(--space-12);
  border: 0;
  border-radius: var(--radius-nav);
  background: var(--color-surface-toast-button);
  color: var(--color-text-on-accent);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
  white-space: nowrap;
}

.action:hover {
  background: var(--color-surface-toast-button-hover);
}
</style>
