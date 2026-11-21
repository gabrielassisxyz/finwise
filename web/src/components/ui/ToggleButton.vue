<script setup lang="ts">
defineProps<{ pressed: boolean; compact?: boolean }>();
</script>

<template>
  <button class="toggle" :class="{ on: pressed, compact }" :aria-pressed="pressed">
    <slot />
  </button>
</template>

<style scoped>
.toggle {
  height: 36px;
  padding: 0 var(--space-12);
  border-radius: var(--radius-control);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink-3);
}

.toggle.compact {
  height: 32px;
  border-radius: var(--radius-nav);
}

.toggle.on {
  border-color: var(--color-accent-tint-border);
  background: var(--color-accent-tint);
  color: var(--color-accent-ink);
}
</style>
