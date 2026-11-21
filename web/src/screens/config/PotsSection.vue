<script setup lang="ts">
import type { Bucket, SettingsPot } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import TitledCard from '@/components/ui/TitledCard.vue';
import { useSettingsWrites } from './useSettingsWrites';

defineProps<{ pots: SettingsPot[] }>();

const BUCKETS: Bucket[] = ['Disponível', 'Reservado', 'Investido'];

const { api, overlay, write, openLineMenu } = useSettingsWrites();

function openBucketMenu(anchor: HTMLElement, pot: SettingsPot): void {
  overlay.openMenu(anchor, {
    title: 'Balde',
    items: BUCKETS.map((bucket) => ({
      label: bucket,
      current: pot.bucket === bucket,
      onPick: () => void write(() => api.settings.setPotBucket(pot.id, bucket)),
    })),
  });
}

function editLines(anchor: HTMLElement, pot: SettingsPot): void {
  void openLineMenu(anchor, (line) => void write(() => api.settings.toggleLinkedLine(pot.id, line)));
}
</script>

<template>
  <TitledCard title="Cofrinhos">
    <EmptyBlock
      v-if="pots.length === 0"
      title="Nenhum cofrinho ainda"
      hint="Os cofrinhos dos seus bancos aparecem aqui depois da sincronização."
    />
    <div v-for="pot in pots" :key="pot.id" class="row stack-row">
      <span class="who stack-full">
        <span class="name" :class="{ unnamed: pot.name === null }">{{ pot.name ?? 'Sem nome' }}</span>
        <span class="bank">{{ pot.cdiPercent === null ? pot.bank : `${pot.bank} · ${pot.cdiPercent}% do CDI` }}</span>
      </span>
      <span data-label="Balde">
        <button
          class="bucket"
          :class="{ unset: pot.bucket === null }"
          @click="openBucketMenu($event.currentTarget as HTMLElement, pot)"
        >
          {{ pot.bucket ?? 'Sem balde' }} ▾
        </button>
      </span>
      <span class="lines" data-label="Linhas do budget">
        <span>{{ pot.linkedLines.join(' · ') || 'Nenhuma linha' }}</span>
        <button class="edit" @click="editLines($event.currentTarget as HTMLElement, pot)">Editar</button>
      </span>
    </div>
  </TitledCard>
</template>

<style scoped>
.row {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) 150px minmax(0, 1.6fr);
  gap: var(--space-14);
  padding: var(--space-6) var(--space-16);
  min-height: 40px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
}

.who {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.name {
  font-weight: var(--font-weight-medium);
}

.name.unnamed {
  color: var(--color-text-muted);
}

.bank {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.bucket {
  height: 32px;
  padding: 0 var(--space-10);
  border-radius: var(--radius-nav);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  white-space: nowrap;
  color: var(--color-text-ink-2);
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-button);
}

.bucket.unset {
  color: var(--color-warn-text);
  background: var(--color-warn-bg);
  border-color: var(--color-warn-border);
}

.lines {
  display: flex;
  gap: var(--space-6);
  align-items: center;
  flex-wrap: wrap;
  font-size: var(--font-size-secondary);
  color: var(--color-text-pill);
}

.edit {
  border: 0;
  background: none;
  padding: 0 var(--space-4);
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}
</style>
