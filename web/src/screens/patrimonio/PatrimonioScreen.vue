<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { Bucket, NetWorthOverview } from '@/api/types';
import { createLatestLoad } from '@/lib/latestLoad';
import { useRevisionStore } from '@/stores/revision';
import { useSavingsViewStore } from '@/stores/savings-view';
import BucketCards from './BucketCards.vue';
import BucketContents from './BucketContents.vue';
import NetWorthChart from './NetWorthChart.vue';
import PlanBlocks from './PlanBlocks.vue';
import YieldCard from './YieldCard.vue';

const api = useApi();
const view = useSavingsViewStore();
const revision = useRevisionStore();

const overview = ref<NetWorthOverview | null>(null);
const loads = createLatestLoad();

watch(
  () => revision.current,
  async () => {
    const isLatest = loads.start();
    const loaded = await api.netWorth.overview();
    if (isLatest()) overview.value = loaded;
  },
  { immediate: true },
);

const openContents = computed(() => overview.value?.buckets.find(({ bucket }) => bucket === view.openBucket) ?? null);

function toggle(bucket: Bucket): void {
  view.openBucket = view.openBucket === bucket ? null : bucket;
}
</script>

<template>
  <div class="screen patrimonio" data-screen-label="Patrimônio">
    <h1 class="screen-title">Patrimônio</h1>
    <template v-if="overview">
      <BucketCards :buckets="overview.buckets" :total="overview.total" :open="view.openBucket" @toggle="toggle" />
      <BucketContents v-if="openContents" :bucket="openContents" />
      <NetWorthChart v-model:stacked="view.stackedChart" :chart="overview.chart" :history-start="overview.historyStart" />
      <PlanBlocks :contribution="overview.contribution" :rate="overview.investmentRate" />
      <YieldCard :yields="overview.yields" />
    </template>
  </div>
</template>

<style scoped>
.patrimonio {
  gap: var(--space-16);
}
</style>
