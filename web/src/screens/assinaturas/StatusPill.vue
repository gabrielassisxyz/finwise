<script setup lang="ts">
import type { SubscriptionStatus } from '@/api/types';
import { statusTone } from './subscriptionText';

defineProps<{ status: SubscriptionStatus; tall?: boolean }>();
</script>

<template>
  <button class="pill" :class="[statusTone(status), { tall }]">
    {{ status }}<span class="caret">▾</span>
  </button>
</template>

<style scoped>
.pill {
  display: inline-flex;
  align-items: center;
  gap: var(--space-6);
  height: 30px;
  padding: 0 var(--space-10);
  border-radius: var(--radius-nav);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  white-space: nowrap;
  border: 1px solid transparent;
}

.pill.tall {
  height: 34px;
}

.caret {
  font-size: 9px; /* token-exempt: glyph size of the pill's caret */
}

.neutral {
  color: var(--color-text-ink-2);
  background: var(--color-surface-card);
  border-color: var(--color-border-button);
}

.warning {
  color: var(--color-warn-text);
  background: var(--color-warn-bg);
  border-color: var(--color-warn-border);
}

.bad {
  color: var(--color-bad-text);
  background: var(--color-bad-bg);
  border-color: var(--color-bad-border);
}
</style>
