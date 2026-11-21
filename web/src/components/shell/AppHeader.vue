<script setup lang="ts">
import { computed } from 'vue';
import type { IsoDay, SyncStatus } from '@/api/types';
import { monthLabel, shortDay } from '@/lib/format';
import { useMonthStore } from '@/stores/month';
import { useSessionStore } from '@/stores/session';

const props = defineProps<{ sync: SyncStatus; today: IsoDay }>();

const month = useMonthStore();
const session = useSessionStore();

const syncText = computed(() => {
  if (props.sync.state !== 'ok') return { wide: '', narrow: '' };
  const { lastSyncDay, lastSyncTime } = props.sync;
  if (lastSyncDay === props.today) {
    return { wide: `Atualizado hoje às ${lastSyncTime}`, narrow: `Hoje, ${lastSyncTime}` };
  }
  const day = shortDay(lastSyncDay);
  return { wide: `Atualizado em ${day} às ${lastSyncTime}`, narrow: `${day}, ${lastSyncTime}` };
});
</script>

<template>
  <header class="header">
    <div class="month-picker">
      <button class="arrow" :disabled="!month.canGoBack" aria-label="Mês anterior" @click="month.shift(-1)">‹</button>
      <span class="month">{{ monthLabel(month.selected) }}</span>
      <button class="arrow" :disabled="!month.canGoForward" aria-label="Próximo mês" @click="month.shift(1)">›</button>
    </div>
    <span v-if="month.isClosed" class="closed-seal">Fechado</span>
    <span class="spacer" />
    <span v-if="sync.state === 'ok'" class="sync">
      <span class="sync-dot" />
      <span class="desktop-only">{{ syncText.wide }}</span>
      <span class="phone-only">{{ syncText.narrow }}</span>
    </span>
    <RouterLink v-else to="/config/dados" class="sync-warning">
      <span class="sync-dot" />
      <span class="sync-message">{{ sync.message }}</span>
    </RouterLink>
    <button class="lock" aria-label="Bloquear" title="Bloquear" @click="session.lock()">
      <span class="lock-icon">
        <span class="lock-shackle" />
        <span class="lock-body" />
      </span>
    </button>
  </header>
</template>

<style scoped>
.header {
  display: flex;
  align-items: center;
  gap: var(--space-10);
  padding: 0 clamp(var(--space-14), 3vw, var(--space-36));
  height: 56px;
  border-bottom: 1px solid var(--color-border-default);
  background: var(--color-surface-nav);
  position: sticky;
  top: 0;
  z-index: 5;
}

.month-picker {
  display: flex;
  align-items: center;
  border: 1px solid var(--color-border-input);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  height: 36px;
  flex-shrink: 0;
}

.arrow {
  border: 0;
  background: none;
  width: 34px;
  height: 34px;
  cursor: pointer;
  color: var(--color-text-ink-2);
  font-size: 16px; /* token-exempt: glyph size of the month arrows */
}

.arrow:disabled {
  opacity: 0.3;
}

.month {
  min-width: 112px;
  text-align: center;
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
  white-space: nowrap;
  padding: 0 var(--space-4);
  border-left: 1px solid var(--color-border-divider);
  border-right: 1px solid var(--color-border-divider);
  line-height: 34px;
}

.closed-seal {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  padding: var(--space-2) var(--space-8);
  border-radius: var(--radius-badge);
  border: 1px solid var(--color-border-button);
  color: var(--color-text-ink-2);
  background: var(--color-surface-card);
}

.spacer {
  flex: 1;
}

.sync {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  white-space: nowrap;
  min-width: 0;
}

.sync-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-accent-sync-dot);
  flex-shrink: 0;
}

.sync-warning {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  padding: 0 var(--space-10);
  border-radius: var(--radius-nav);
  border: 1px solid var(--color-warn-border);
  background: var(--color-warn-bg);
  color: var(--color-warn-text);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  min-width: 0;
  box-sizing: border-box;
}

.sync-warning:hover {
  color: var(--color-warn-text);
  text-decoration: none;
}

.sync-warning .sync-dot {
  background: var(--color-warn-text);
}

.sync-message {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lock {
  width: 44px;
  height: 44px;
  margin-right: calc(var(--space-8) * -1);
  border: 0;
  background: none;
  cursor: pointer;
  color: var(--color-text-ink-2);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-control);
  flex-shrink: 0;
}

.lock:hover {
  background: var(--color-surface-hover-nav);
  color: var(--color-text-ink);
}

.lock-icon {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.lock-shackle {
  width: 8px;
  height: 6px;
  border: 1.5px solid currentColor;
  border-bottom: 0;
  border-radius: 5px 5px 0 0; /* token-exempt: shape of the drawn padlock */
}

.lock-body {
  width: 14px;
  height: 10px;
  background: currentColor;
  border-radius: 2px; /* token-exempt: shape of the drawn padlock */
}
</style>
