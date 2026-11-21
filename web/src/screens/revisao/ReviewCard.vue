<script setup lang="ts">
defineProps<{ title: string; aside?: string }>();
</script>

<template>
  <section class="card">
    <div class="head">
      <span class="title">{{ title }}</span>
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
  align-items: center;
  gap: var(--space-12);
  padding: var(--space-12) 18px; /* token-exempt: between two steps of the spacing scale */
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.aside {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
