<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { HomeSummary } from '@/api/types';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { createLatestLoad } from '@/lib/latestLoad';
import { useMonthStore } from '@/stores/month';
import { useRevisionStore } from '@/stores/revision';
import SummaryStrip from './SummaryStrip.vue';

const api = useApi();
const month = useMonthStore();
const revision = useRevisionStore();

const summary = ref<HomeSummary | null>(null);

// A write refetches the summary in place: the strip keeps showing the old figures until the new ones land.
const loads = createLatestLoad();
watch(
  () => [month.selected, revision.current] as const,
  async ([selected]) => {
    const isLatest = loads.start();
    const loaded = await api.home.summary(selected);
    if (isLatest() && selected === month.selected) summary.value = loaded;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen" data-screen-label="Início">
    <h1 class="screen-title">Início</h1>
    <SummaryStrip v-if="summary" :summary="summary" :month="month.selected" />
    <TransactionList />
  </div>
</template>
