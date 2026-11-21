<script setup lang="ts">
import type { ShellOverview } from '@/api/types';
import AppHeader from './AppHeader.vue';
import PhoneNav from './PhoneNav.vue';
import SidebarNav from './SidebarNav.vue';

defineProps<{ overview: ShellOverview }>();
</script>

<template>
  <div class="shell">
    <SidebarNav class="desktop-only" :pending-count="overview.pendingCount" />
    <div class="column">
      <AppHeader :sync="overview.sync" :today="overview.today" />
      <main class="main">
        <RouterView />
      </main>
    </div>
    <div class="phone-only">
      <PhoneNav :pending-count="overview.pendingCount" />
    </div>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  min-height: 100vh;
  background: var(--color-surface-page);
}

.column {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.main {
  padding: var(--space-24) clamp(var(--space-16), 3vw, var(--space-36)) 96px;
  max-width: 1280px;
  width: 100%;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--space-20);
}

@media (max-width: 820px) {
  .main {
    padding: var(--space-16) var(--space-14) 150px;
  }
}
</style>
