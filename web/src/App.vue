<script setup lang="ts">
import { onMounted, watch } from 'vue';
import AppShell from './components/shell/AppShell.vue';
import PinScreen from './screens/pin/PinScreen.vue';
import { useSessionStore } from './stores/session';
import { useShellStore } from './stores/shell';

const session = useSessionStore();
const shell = useShellStore();

onMounted(() => session.load());

// Nothing behind the PIN is fetched while the app is locked.
watch(
  () => session.ready && !session.locked,
  (unlocked) => {
    if (unlocked) void shell.load();
  },
);
</script>

<template>
  <template v-if="session.ready">
    <PinScreen v-if="session.locked" />
    <AppShell v-else-if="shell.overview" :overview="shell.overview" />
  </template>
</template>
