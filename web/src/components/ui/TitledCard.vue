<script setup lang="ts">
defineProps<{ title: string; aside?: string }>();
</script>

<template>
  <section class="card">
    <div class="head">
      <span>{{ title }}</span>
      <span v-if="aside" class="aside">{{ aside }}</span>
    </div>
    <slot />
  </section>
</template>

<style scoped>
.card {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}
.aside {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-regular);
  color: var(--color-text-muted);
}
</style>
