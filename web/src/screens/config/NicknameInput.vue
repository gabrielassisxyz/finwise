<script setup lang="ts">
// A text field that commits when it loses focus or Enter is pressed, and shows what was kept.
defineProps<{ value: string; label: string; placeholder?: string }>();
defineEmits<{ commit: [text: string] }>();
</script>

<template>
  <input
    class="field"
    :value="value"
    :aria-label="label"
    :placeholder="placeholder"
    @change="$emit('commit', ($event.target as HTMLInputElement).value)"
  />
</template>

<style scoped>
.field {
  width: 100%;
  box-sizing: border-box;
  height: 32px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-item);
  padding: 0 var(--space-8);
  font-size: var(--font-size-secondary);
  outline: none;
}
</style>
