<script setup lang="ts">
import { useSubscriptionsViewStore, type SubscriptionsTabId } from '@/stores/subscriptions-view';
import RecurringTab from './RecurringTab.vue';
import SubscriptionsTab from './SubscriptionsTab.vue';

const view = useSubscriptionsViewStore();

const TABS: { id: SubscriptionsTabId; label: string }[] = [
  { id: 'assinaturas', label: 'Assinaturas' },
  { id: 'recorrentes', label: 'Recorrentes' },
];
</script>

<template>
  <div class="screen assinaturas" data-screen-label="Assinaturas">
    <h1 class="screen-title">Assinaturas</h1>
    <div class="tabs">
      <button v-for="tab in TABS" :key="tab.id" class="tab" :class="{ active: tab.id === view.tab }" @click="view.tab = tab.id">
        {{ tab.label }}
      </button>
    </div>
    <SubscriptionsTab v-if="view.tab === 'assinaturas'" />
    <RecurringTab v-else />
  </div>
</template>

<style scoped>
.assinaturas {
  gap: var(--space-16);
}

.tabs {
  display: flex;
  gap: var(--space-24);
  border-bottom: 1px solid var(--color-border-default);
}

.tab {
  border: 0;
  background: none;
  padding: var(--space-10) 0 11px;
  margin-bottom: -1px;
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-regular);
  cursor: pointer;
  color: var(--color-text-muted);
  border-bottom: 2px solid transparent;
}

.tab.active {
  color: var(--color-text-ink);
  font-weight: var(--font-weight-medium);
  border-bottom-color: var(--color-accent-base);
}
</style>
