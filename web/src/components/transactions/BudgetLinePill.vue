<script setup lang="ts">
import { computed } from 'vue';
import type { Transaction } from '@/api/types';
import { lineDotColor } from '@/lib/lineColor';

const props = defineProps<{ transaction: Transaction; roomy?: boolean }>();

defineEmits<{ open: [anchor: HTMLElement] }>();

// Without a line the pill shows what the model would pick, as a question; a review with nothing to pick says it waits.
const label = computed(() => {
  const { line, pending } = props.transaction;
  if (line) return line;
  if (!pending) return '—';
  return pending.suggestion === null ? 'Pendente' : `${pending.suggestion}?`;
});

const dotStyle = computed(() => (props.transaction.line ? { background: lineDotColor(props.transaction.line) } : {}));
</script>

<template>
  <button
    class="pill-button"
    :class="{ roomy }"
    title="Alterar linha"
    @click.stop="$emit('open', $event.currentTarget as HTMLElement)"
  >
    <span class="pill" :class="{ unset: !transaction.line }">
      <span class="dot" :style="dotStyle" />
      <span class="label">{{ label }}</span>
      <span class="chevron">▾</span>
    </span>
  </button>
</template>

<style scoped>
.pill-button {
  border: 0;
  background: none;
  padding: 0;
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  cursor: pointer;
  font-size: var(--font-size-caption);
}

/* On a phone the pill keeps its size and the button around it grows to a touch target. */
.pill-button.roomy {
  min-height: 44px;
}

.pill {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
  max-width: 100%;
  box-sizing: border-box;
  padding: 3px 9px;
  border-radius: var(--radius-pill);
  background: var(--color-surface-pill);
  color: var(--color-text-pill);
  border: 1px solid var(--color-border-strong);
}

.roomy .pill {
  padding: var(--space-4) var(--space-10);
}

.pill.unset {
  background: var(--color-surface-card);
  color: var(--color-text-ink-2);
  border-style: dashed;
}

.pill-button:hover .pill {
  background: var(--color-surface-pressed);
}

.dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--color-border-strong);
}

.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chevron {
  font-size: 9px; /* token-exempt: glyph size of the pill's chevron */
  color: var(--color-text-muted-2);
}

.roomy .chevron {
  font-size: inherit;
  color: inherit;
}
</style>
