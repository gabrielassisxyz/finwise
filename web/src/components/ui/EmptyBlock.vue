<script setup lang="ts">
defineProps<{ title: string; hint: string }>();
</script>

<template>
  <div class="empty">
    <span class="empty-title">{{ title }}</span>
    <span class="empty-hint">{{ hint }}</span>
  </div>
</template>

<style scoped>
.empty {
  padding: 44px var(--space-20);
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  align-items: center;
}

.empty-title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.empty-hint {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  max-width: 420px;
  text-wrap: pretty;
}
</style>
