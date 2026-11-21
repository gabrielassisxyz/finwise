<script setup lang="ts">
import { computed, onUnmounted, watch } from 'vue';
import { useRoute } from 'vue-router';
import { usePanelStore } from '@/stores/panel';
import DetailsSection from './DetailsSection.vue';
import EstablishmentSection from './EstablishmentSection.vue';
import ExclusionSection from './ExclusionSection.vue';
import HistorySection from './HistorySection.vue';
import LineSection from './LineSection.vue';
import NoteSection from './NoteSection.vue';
import PanelHeader from './PanelHeader.vue';
import PersonSection from './PersonSection.vue';
import ReimbursementSection from './ReimbursementSection.vue';
import RefundSection from './RefundSection.vue';
import SplitSection from './SplitSection.vue';
import TagsSection from './TagsSection.vue';
import { providePanelActions } from './panelActions';
import { useTransactionDetail } from './useTransactionDetail';

const panel = usePanelStore();
const route = useRoute();
providePanelActions();
const { detail } = useTransactionDetail(computed(() => panel.openId));

// Leaving the screen, or the app being locked (which unmounts the shell), closes the panel.
watch(() => route.fullPath, () => panel.close());
onUnmounted(() => panel.close());
</script>

<template>
  <template v-if="detail">
    <div class="scrim" @click="panel.close()" />
    <aside class="panel" data-screen-label="Painel da transação">
      <PanelHeader :detail="detail" @close="panel.close()" />
      <LineSection :detail="detail" />
      <EstablishmentSection v-if="detail.sections.establishment" :detail="detail" />
      <PersonSection v-if="detail.sections.person" :detail="detail" />
      <TagsSection :detail="detail" />
      <NoteSection :detail="detail" />
      <SplitSection v-if="detail.sections.split" :detail="detail" />
      <ReimbursementSection v-if="detail.sections.reimbursement" :detail="detail" />
      <ExclusionSection :detail="detail" />
      <RefundSection v-if="detail.sections.refund" :detail="detail" />
      <DetailsSection :detail="detail" />
      <HistorySection :detail="detail" />
    </aside>
  </template>
</template>

<style scoped>
.scrim {
  position: fixed;
  inset: 0;
  background: var(--color-overlay-panel);
  z-index: 20;
}

.panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: 460px;
  max-width: 100vw;
  box-sizing: border-box;
  background: var(--color-surface-card);
  border-left: 1px solid var(--color-border-button);
  z-index: 21;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-panel);
}
</style>
