<script setup lang="ts">
defineProps<{ primary?: boolean }>();
</script>

<template>
  <button class="action" :class="{ primary }"><slot /></button>
</template>

<style scoped>
.action {
  height: 36px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-control);
  font-size: var(--font-size-body);
  cursor: pointer;
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink);
}

.action:hover {
  background: var(--color-surface-hover-item);
}

.action.primary {
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.action.primary:hover {
  background: var(--color-accent-hover);
}
</style>
