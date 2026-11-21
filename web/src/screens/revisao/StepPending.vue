<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import type { ClosingView } from '@/api/types';

const props = defineProps<{ view: ClosingView }>();

const router = useRouter();

const waiting = computed(() => (props.view.closed ? 0 : props.view.pendingCount));
const text = computed(() => {
  if (!waiting.value) return 'Nenhuma pendência';
  return `${waiting.value} ${waiting.value === 1 ? 'pendência' : 'pendências'} · resolver antes de fechar`;
});
</script>

<template>
  <span>{{ text }}</span>
  <button v-if="waiting" class="resolve" @click="router.push('/revisao')">Resolver na revisão semanal</button>
</template>

<style scoped>
.resolve {
  align-self: flex-start;
  height: 36px;
  padding: 0 var(--space-14);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-control);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
}
</style>
