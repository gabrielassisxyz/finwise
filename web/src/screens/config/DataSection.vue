<script setup lang="ts">
import type { DataSettings, Moment } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { shortDay } from '@/lib/format';
import { useShellStore } from '@/stores/shell';

defineProps<{ data: DataSettings }>();

const shell = useShellStore();

/** "hoje às 02:00", or the date when it was not today. */
function when(moment: Moment): string {
  return moment.day === shell.overview?.today ? `hoje às ${moment.time}` : `em ${shortDay(moment.day)} às ${moment.time}`;
}
</script>

<template>
  <TitledCard title="Dados">
    <div v-for="connection in data.connections" :key="connection.bank" class="connection">
      <span class="bank">{{ connection.bank }}</span>
      <span class="status" :class="{ expired: connection.status === 'consent-expired' }">
        <span class="dot" />
        {{ connection.status === 'ok' ? 'Atualizado' : 'Consentimento expirado' }}
      </span>
    </div>
    <div class="when">
      <span>Última sincronização: {{ when(data.lastSync) }}</span>
      <span>Último backup: {{ when(data.lastBackup) }}</span>
    </div>
  </TitledCard>
</template>

<style scoped>
.connection {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 48px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  flex-wrap: wrap;
}

.bank {
  font-weight: var(--font-weight-medium);
}

.status {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  color: var(--color-accent-positive);
}

.status.expired {
  color: var(--color-warn-text);
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}

.when {
  padding: var(--space-12) var(--space-16);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
}
</style>
