<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import AppShell from './components/shell/AppShell.vue';
import ToastHost from './components/ui/ToastHost.vue';
import PinScreen from './screens/pin/PinScreen.vue';
import { useSessionStore } from './stores/session';
import { useShellStore } from './stores/shell';

const session = useSessionStore();
const shell = useShellStore();

onMounted(() => session.load());

// The frame carries the toasts; until it is up (and on the PIN screen) a failed first read would have nowhere to say so.
const frameIsUp = computed(() => session.ready && !session.locked && shell.overview !== null);

// Nothing behind the PIN is fetched while the app is locked.
watch(
  () => session.ready && !session.locked,
  (unlocked) => {
    if (unlocked) void shell.load();
  },
);
</script>

<template>
  <ToastHost v-if="!frameIsUp" />
  <template v-if="session.ready">
    <PinScreen v-if="session.locked" />
    <AppShell v-else-if="shell.overview" :overview="shell.overview" />
  </template>
</template>
