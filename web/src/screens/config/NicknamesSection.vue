<script setup lang="ts">
import type { SettingsEstablishment, SettingsOverview } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import NicknameInput from './NicknameInput.vue';
import { useSettingsWrites } from './useSettingsWrites';

defineProps<{ settings: SettingsOverview }>();

const { api, write, openLineMenu } = useSettingsWrites();

function pickLine(anchor: HTMLElement, establishment: SettingsEstablishment): void {
  void openLineMenu(anchor, (line) => void write(() => api.settings.setDefaultLine(establishment.key, line)));
}
</script>

<template>
  <TitledCard title="Apelidos">
    <div class="subhead first">Estabelecimentos</div>
    <div v-for="row in settings.establishments" :key="row.key" class="row stack-row">
      <span class="raw stack-full">{{ row.rawText }}</span>
      <span data-label="Apelido">
        <NicknameInput
          :value="row.nickname ?? ''"
          label="Apelido"
          @commit="(text) => write(() => api.settings.setNickname(row.key, text))"
        />
      </span>
      <span data-label="Linha padrão">
        <button class="line" @click="pickLine($event.currentTarget as HTMLElement, row)">
          {{ row.defaultLine ?? 'Nenhuma' }} ▾
        </button>
      </span>
      <span class="tags" data-label="Tags fixas">{{ row.fixedTags.join(' ') || '—' }}</span>
    </div>

    <div class="subhead">Pessoas</div>
    <div v-for="person in settings.people" :key="person.key" class="person">
      <span class="full">{{ person.fullName }}</span>
      <NicknameInput
        class="short"
        :value="person.nickname ?? ''"
        label="Apelido da pessoa"
        placeholder="Sem apelido"
        @commit="(text) => write(() => api.settings.setNickname(person.key, text))"
      />
    </div>

    <div class="subhead">Cartões</div>
    <div class="person last">
      <span class="full">{{ settings.cardTitle }}</span>
      <NicknameInput
        class="short"
        :value="settings.cardLabel"
        label="Rótulo do cartão"
        placeholder="Físico ou Virtual"
        @commit="(text) => write(() => api.settings.setCardLabel(text))"
      />
    </div>
  </TitledCard>
</template>

<style scoped>
.subhead {
  padding: var(--space-14) var(--space-16) var(--space-4);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-ink-2);
}

.subhead.first {
  padding-top: var(--space-12);
}

.row {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1.2fr) minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-14);
  padding: var(--space-6) var(--space-16);
  min-height: 40px;
  align-items: center;
  font-size: var(--font-size-secondary);
  border-bottom: 1px solid var(--color-border-row);
}

.raw {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.line {
  max-width: 100%;
  height: 32px;
  padding: 0 var(--space-10);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-item);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tags {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-3);
}

.person {
  display: flex;
  justify-content: space-between;
  gap: var(--space-14);
  padding: var(--space-6) var(--space-16);
  min-height: 40px;
  align-items: center;
  font-size: var(--font-size-secondary);
  border-bottom: 1px solid var(--color-border-row);
}

.person.last {
  padding-bottom: var(--space-12);
  border-bottom: 0;
}

.full {
  min-width: 0;
}

.person .short {
  width: 50%;
  max-width: 220px;
}
</style>
