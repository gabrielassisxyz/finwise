<script setup lang="ts">
defineProps<{ on: boolean; label: string }>();
defineEmits<{ toggle: [] }>();
</script>

<template>
  <button class="switch" :class="{ on }" role="switch" :aria-checked="on" :aria-label="label" @click="$emit('toggle')">
    <span class="thumb" />
  </button>
</template>

<style scoped>
.switch {
  width: 40px;
  height: 24px;
  border-radius: var(--radius-dialog);
  border: 0;
  padding: 3px;
  cursor: pointer;
  background: var(--color-surface-switch-off);
  display: flex;
  flex-shrink: 0;
}

.switch.on {
  background: var(--color-accent-base);
}

.thumb {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--color-surface-card);
  margin-left: 0;
  transition: margin var(--duration-switch);
}

.on .thumb {
  margin-left: var(--space-16);
}
</style>
