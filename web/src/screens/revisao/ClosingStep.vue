<script setup lang="ts">
import type { ClosingStepState } from '@/api/types';
import { stepStatus } from './closingText';

defineProps<{ number: number; title: string; state: ClosingStepState; open: boolean; canMarkDone: boolean }>();
defineEmits<{ toggle: []; done: [] }>();
</script>

<template>
  <section class="step">
    <button class="head" @click="$emit('toggle')">
      <span class="number" :class="{ done: state === 'done' }">{{ state === 'done' ? '✓' : number }}</span>
      <span class="title">{{ title }}</span>
      <span class="status" :class="{ done: state === 'done' }">{{ stepStatus(state) }}</span>
      <span class="chevron">{{ open ? '▾' : '▸' }}</span>
    </button>
    <div v-if="open" class="body">
      <slot />
      <button v-if="canMarkDone" class="done-button" @click="$emit('done')">Marcar como feito e seguir</button>
    </div>
  </section>
</template>

<style scoped>
.step {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16);
  min-height: 52px;
  border: 0;
  background: none;
  cursor: pointer;
  text-align: left;
  color: var(--color-text-ink);
}

.head:hover {
  background: var(--color-surface-hover-row);
}

.number {
  width: 26px;
  height: 26px;
  flex-shrink: 0;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-semibold);
  font-variant-numeric: tabular-nums;
  background: var(--color-surface-count);
  color: var(--color-text-ink-3);
}

.number.done {
  background: var(--color-accent-step-done);
  color: var(--color-accent-link-hover);
}

.title {
  flex: 1;
  min-width: 0;
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-medium);
}

.status {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-warn-text);
}

.status.done {
  color: var(--color-accent-positive);
}

.chevron {
  font-size: var(--font-size-badge);
  color: var(--color-text-muted-2);
}

.body {
  padding: var(--space-4) var(--space-16) 18px 54px; /* token-exempt: between two steps of the spacing scale */
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  font-size: var(--font-size-body);
}

.done-button {
  align-self: flex-start;
  height: 36px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-control);
  border: 0;
  background: var(--color-surface-action-dark);
  color: var(--color-text-on-accent);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
}

.done-button:hover {
  background: var(--color-surface-action-dark-hover);
}
</style>
