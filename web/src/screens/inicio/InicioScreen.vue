<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { HomeSummary } from '@/api/types';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { useMonthStore } from '@/stores/month';
import SummaryStrip from './SummaryStrip.vue';

const api = useApi();
const month = useMonthStore();

const summary = ref<HomeSummary | null>(null);

watch(
  () => month.selected,
  async (selected) => {
    const loaded = await api.home.summary(selected);
    if (selected === month.selected) summary.value = loaded;
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
