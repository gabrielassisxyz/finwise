<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { SavingsPotDetail } from '@/api/types';
import { brl } from '@/lib/format';
import PotFacts from './PotFacts.vue';
import PotMovements from './PotMovements.vue';
import PotYield from './PotYield.vue';
import { potSubtitle } from './potText';

const api = useApi();
const route = useRoute();
const router = useRouter();

const pot = ref<SavingsPotDetail | null>(null);
const missing = ref(false);

const id = computed(() => String(route.params.id));

watch(
  id,
  async (requested) => {
    pot.value = null;
    missing.value = false;
    const loaded = await api.savingsPots.detail(requested);
    if (requested !== id.value) return;
    pot.value = loaded;
    missing.value = loaded === null;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen cofrinho" data-screen-label="Cofrinho">
    <button class="back" @click="router.push('/cofrinhos')">‹ Cofrinhos</button>
    <p v-if="missing" class="missing">Cofrinho não encontrado.</p>
    <template v-if="pot">
      <div class="heading">
        <div class="title-block">
          <h1 class="screen-title">{{ pot.name ?? 'Sem nome' }}</h1>
          <span class="subtitle">{{ potSubtitle(pot) }}</span>
        </div>
        <div class="balance-block">
          <span class="balance-label">Saldo</span>
          <span class="balance">{{ brl(pot.balance) }}</span>
        </div>
      </div>
      <div v-if="pot.belowTarget" class="notice">
        Abaixo do alvo: faltam {{ brl(pot.belowTarget.shortfall) }}. Aporte religado até voltar ao alvo.
      </div>
      <PotFacts :pot="pot" />
      <div class="split-columns">
        <PotMovements :pot="pot" />
        <PotYield :pot="pot" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.cofrinho {
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
  gap: var(--space-4);
}

.subtitle {
  font-size: var(--font-size-body);
  color: var(--color-text-muted);
}

.balance-block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-end;
}

.balance-label {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.balance {
  font-size: var(--font-size-title);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.notice {
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-control);
  border: 1px solid var(--color-warn-border);
  background: var(--color-warn-bg);
  color: var(--color-warn-notice-text);
  font-size: var(--font-size-body);
  text-wrap: pretty;
}
</style>
