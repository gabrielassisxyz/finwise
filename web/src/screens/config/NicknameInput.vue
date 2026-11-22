<script setup lang="ts">
import { saveStored, vStored } from '@/lib/storedField';

// A text field that saves when it loses focus or Enter is pressed, and shows what was kept.
defineProps<{ value: string; label: string; placeholder?: string; save: (text: string) => Promise<unknown> }>();
</script>

<template>
  <input
    class="field"
    v-stored="value"
    :aria-label="label"
    :placeholder="placeholder"
    @change="saveStored($event, save)"
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
