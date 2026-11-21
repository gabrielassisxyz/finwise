<script setup lang="ts">
import type { NetWorthBucket } from '@/api/types';
import TitledCard from '@/components/ui/TitledCard.vue';
import { brl } from '@/lib/format';

defineProps<{ bucket: NetWorthBucket }>();
</script>

<template>
  <TitledCard :title="`O que está em ${bucket.bucket}`">
    <div v-for="item in bucket.items" :key="`${item.name}-${item.detail}`" class="item">
      <span>{{ item.name ?? 'Sem nome' }} <span class="detail">· {{ item.detail }}</span></span>
      <span class="amount">{{ brl(item.amount) }}</span>
    </div>
  </TitledCard>
</template>

<style scoped>
.item {
  display: flex;
  justify-content: space-between;
  gap: var(--space-12);
  padding: 0 var(--space-16);
  min-height: 44px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.detail {
  color: var(--color-text-muted);
}

.amount {
  font-weight: var(--font-weight-medium);
}
</style>
