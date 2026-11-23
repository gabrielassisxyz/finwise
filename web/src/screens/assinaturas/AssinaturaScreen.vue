<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { BudgetLineOption, SubscriptionDetail } from '@/api/types';
import MarkBadge from '@/components/transactions/MarkBadge.vue';
import TitledCard from '@/components/ui/TitledCard.vue';
import { brl } from '@/lib/format';
import { useRevisionStore } from '@/stores/revision';
import { useShellStore } from '@/stores/shell';
import SubscriptionCharges from './SubscriptionCharges.vue';
import SubscriptionEditor from './SubscriptionEditor.vue';

const api = useApi();
const route = useRoute();
const router = useRouter();
const revision = useRevisionStore();
const shell = useShellStore();

const subscription = ref<SubscriptionDetail | null>(null);
const lines = ref<BudgetLineOption[]>([]);
const missing = ref(false);

const id = computed(() => String(route.params.id));

let latestDetail = 0;
watch(
  [id, () => revision.current],
  async ([requested], previous) => {
    const request = ++latestDetail;
    if (requested !== previous?.[0]) subscription.value = null;
    const loaded = await api.subscriptions.detail(requested);
    if (request !== latestDetail || requested !== id.value) return;
    subscription.value = loaded;
    missing.value = loaded === null;
  },
  { immediate: true },
);

// A subscription's line belongs to no month, so it is resolved in the current one.
const currentMonth = computed(() => shell.overview?.today.slice(0, 7) ?? null);
let latestLines = 0;
watch(
  [currentMonth, () => revision.current],
  async ([current]) => {
    if (!current) return;
    const request = ++latestLines;
    const options = await api.transactions.filterOptions(current);
    if (request === latestLines) lines.value = options.lines;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen assinatura" data-screen-label="Assinatura">
    <button class="back" @click="router.push('/assinaturas')">‹ Assinaturas</button>
    <p v-if="missing" class="missing">Assinatura não encontrada.</p>
    <template v-if="subscription">
      <div class="heading">
        <div class="title-block">
          <h1 class="screen-title">{{ subscription.name }}</h1>
          <div class="marks">
            <MarkBadge v-for="mark in subscription.marks" :key="mark.text" :mark="mark" large />
          </div>
        </div>
        <div class="price-block">
          <span class="cycle">{{ subscription.cycle }}</span>
          <span class="price">{{ brl(subscription.amount) }}</span>
        </div>
      </div>
      <div class="split-columns">
        <SubscriptionEditor :subscription="subscription" :lines="lines" />
        <div class="side">
          <SubscriptionCharges :charges="subscription.charges" />
          <TitledCard title="Alternativas" aside="Escritas pela IA">
            <div v-for="text in subscription.alternatives" :key="text" class="alternative">{{ text }}</div>
            <div v-if="subscription.alternatives.length === 0" class="alternative">
              Nenhuma alternativa encontrada ainda. A IA procura planos mais baratos a cada renovação.
            </div>
          </TitledCard>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.assinatura {
  gap: var(--space-16);
}

.back {
  align-self: flex-start;
  border: 0;
  background: none;
  padding: 0;
  min-height: 32px;
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  cursor: pointer;
}

.missing {
  margin: 0;
  font-size: var(--font-size-body);
  color: var(--color-text-muted);
}

.heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: var(--space-16);
  flex-wrap: wrap;
}

.title-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
}

.marks {
  display: flex;
  gap: var(--space-6);
  flex-wrap: wrap;
}

.price-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-end;
}

.cycle {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.price {
  font-size: var(--font-size-title);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.side {
  display: flex;
  flex-direction: column;
  gap: var(--space-16);
  min-width: 0;
}

.alternative {
  padding: var(--space-12) var(--space-16);
  font-size: var(--font-size-body);
  line-height: 1.45;
  border-bottom: 1px solid var(--color-border-row);
  text-wrap: pretty;
}
</style>
