<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { SettingsOverview } from '@/api/types';
import { useRevisionStore } from '@/stores/revision';
import CardSection from './CardSection.vue';
import CategorizationSection from './CategorizationSection.vue';
import DataSection from './DataSection.vue';
import NicknamesSection from './NicknamesSection.vue';
import PinSection from './PinSection.vue';
import PotsSection from './PotsSection.vue';
import SectionChips from './SectionChips.vue';
import { sectionOf, type ConfigSectionId } from './sections';

const api = useApi();
const route = useRoute();
const router = useRouter();
const revision = useRevisionStore();

const settings = ref<SettingsOverview | null>(null);

// Every write refetches, so a bound the backend applied shows as it was kept.
let latest = 0;
watch(
  () => revision.current,
  async () => {
    const request = ++latest;
    const loaded = await api.settings.overview();
    if (request === latest) settings.value = loaded;
  },
  { immediate: true },
);

const active = computed(() => sectionOf(route.params.section));

function select(id: ConfigSectionId): void {
  void router.push(`/config/${id}`);
}
</script>

<template>
  <div class="screen config" data-screen-label="Configuração">
    <h1 class="screen-title">Configuração</h1>
    <SectionChips :active="active" @select="select" />
    <template v-if="settings">
      <div class="section" :class="{ inactive: active !== 'cofrinhos' }" data-section="cofrinhos">
        <PotsSection :pots="settings.pots" />
      </div>
      <div class="section" :class="{ inactive: active !== 'apelidos' }" data-section="apelidos">
        <NicknamesSection :settings="settings" />
      </div>
      <div class="section" :class="{ inactive: active !== 'categorizacao' }" data-section="categorizacao">
        <CategorizationSection :categorization="settings.categorization" />
      </div>
      <div class="section" :class="{ inactive: active !== 'cartao' }" data-section="cartao">
        <CardSection :title="settings.cardTitle" :closing="settings.closing" />
      </div>
      <div class="section" :class="{ inactive: active !== 'pin' }" data-section="pin">
        <PinSection />
      </div>
      <div class="section" :class="{ inactive: active !== 'dados' }" data-section="dados">
        <DataSection :data="settings.data" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.config {
  gap: var(--space-16);
  max-width: 920px;
}

/* A phone shows one section, chosen from the chips; a wide screen shows all of them. */
@media (max-width: 820px) {
  .section.inactive {
    display: none;
  }
}
</style>
