<script setup lang="ts">
import { watch } from 'vue';
import { useRoute } from 'vue-router';
import type { ShellOverview } from '@/api/types';
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue';
import PopoverMenu from '@/components/ui/PopoverMenu.vue';
import ToastHost from '@/components/ui/ToastHost.vue';
import { useOverlayStore } from '@/stores/overlay';
import AppHeader from './AppHeader.vue';
import PhoneNav from './PhoneNav.vue';
import SidebarNav from './SidebarNav.vue';

defineProps<{ overview: ShellOverview }>();

const route = useRoute();
const overlay = useOverlayStore();

// Whatever was floating belongs to the screen that was left.
watch(
  () => route.fullPath,
  () => overlay.closeAll(),
);
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
    <ToastHost />
    <PopoverMenu />
    <ConfirmDialog />
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
