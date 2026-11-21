<script setup lang="ts">
import type { Mark } from './marks';

defineProps<{ mark: Mark }>();
</script>

<template>
  <span class="mark" :class="mark.tone">{{ mark.text }}</span>
</template>

<style scoped>
.mark {
  font-size: var(--font-size-badge);
  line-height: 1.5;
  padding: 0 var(--space-6);
  border-radius: var(--radius-badge);
  white-space: nowrap;
  border: 1px solid transparent;
}

.neutral {
  color: var(--color-text-ink-2);
  background: var(--color-surface-card);
  border-color: var(--color-border-button);
}

.warning {
  color: var(--color-warn-text);
  background: var(--color-warn-bg);
  border-color: var(--color-warn-border);
}

.positive {
  color: var(--color-accent-positive);
  background: var(--color-accent-badge-bg);
  border-color: var(--color-accent-badge-border);
}

.bad {
  color: var(--color-bad-text);
  background: var(--color-bad-bg);
  border-color: var(--color-bad-border);
}

.tag {
  color: var(--color-text-ink-3);
  background: var(--color-surface-count);
}
</style>
