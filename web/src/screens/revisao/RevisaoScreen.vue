<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router';

const route = useRoute();
const router = useRouter();

const TABS = [
  { path: '/revisao', label: 'Semanal' },
  { path: '/revisao/fechamento', label: 'Fechamento do mês' },
  { path: '/revisao/salario', label: 'Dia do salário' },
];
</script>

<template>
  <div class="screen revisao" data-screen-label="Revisão">
    <h1 class="screen-title">Revisão</h1>
    <div class="tabs">
      <button
        v-for="tab in TABS"
        :key="tab.path"
        class="tab"
        :class="{ active: tab.path === route.path }"
        @click="router.push(tab.path)"
      >
        {{ tab.label }}
      </button>
    </div>
    <RouterView />
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  gap: var(--space-24);
  border-bottom: 1px solid var(--color-border-default);
  overflow-x: auto;
  overflow-y: hidden;
}

.tab {
  border: 0;
  background: none;
  padding: var(--space-10) 0 11px;
  margin-bottom: -1px;
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-regular);
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  color: var(--color-text-muted);
  border-bottom: 2px solid transparent;
}

.tab.active {
  color: var(--color-text-ink);
  font-weight: var(--font-weight-medium);
  border-bottom-color: var(--color-accent-base);
}
</style>
