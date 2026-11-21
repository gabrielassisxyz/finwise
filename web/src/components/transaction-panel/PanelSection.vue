<script setup lang="ts">
defineProps<{ title?: string; last?: boolean; tight?: boolean }>();
</script>

<template>
  <section class="section" :class="{ last, tight }">
    <span v-if="title" class="label">{{ title }}</span>
    <slot />
  </section>
</template>

<style scoped>
.section {
  padding: var(--space-16) var(--space-20);
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  border-bottom: 1px solid var(--color-border-divider);
}

.section.tight {
  gap: var(--space-8);
}

.section.last {
  padding-bottom: 28px;
  border-bottom: 0;
}

.label {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
}
</style>
