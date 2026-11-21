<script setup lang="ts">
import { CONFIG_SECTIONS, type ConfigSectionId } from './sections';

defineProps<{ active: ConfigSectionId }>();
defineEmits<{ select: [id: ConfigSectionId] }>();
</script>

<template>
  <div class="chips phone-only">
    <button
      v-for="section in CONFIG_SECTIONS"
      :key="section.id"
      class="chip"
      :class="{ active: section.id === active }"
      :aria-pressed="section.id === active"
      @click="$emit('select', section.id)"
    >
      {{ section.label }}
    </button>
  </div>
</template>

<style scoped>
.chips {
  display: flex;
  gap: var(--space-6);
  overflow-x: auto;
  padding-bottom: var(--space-2);
}

.chip {
  flex-shrink: 0;
  min-height: 40px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-pill);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink-3);
  font-weight: var(--font-weight-regular);
}

.chip.active {
  border-color: var(--color-accent-tint-border);
  background: var(--color-accent-tint);
  color: var(--color-accent-ink);
  font-weight: var(--font-weight-medium);
}
</style>
