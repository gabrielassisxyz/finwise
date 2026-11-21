<script setup lang="ts">
defineProps<{ title: string }>();
</script>

<template>
  <section class="card">
    <div class="head">{{ title }}</div>
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
  padding: var(--space-12) var(--space-16);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}
</style>
