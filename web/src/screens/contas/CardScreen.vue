<script setup lang="ts">
import { ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { CardOverview } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import { useRevisionStore } from '@/stores/revision';
import CardBills from './CardBills.vue';
import CardFacts from './CardFacts.vue';
import CardPayAhead from './CardPayAhead.vue';

const api = useApi();
const router = useRouter();
const revision = useRevisionStore();

// Undefined until the answer arrives, null when the ledger has no card.
const card = ref<CardOverview | null | undefined>(undefined);

// A write elsewhere (a nickname, the closing day) refetches in place.
let latest = 0;
watch(
  () => revision.current,
  async () => {
    const request = ++latest;
    const loaded = await api.card.overview();
    if (request === latest) card.value = loaded;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen card-page" data-screen-label="Cartão">
    <button class="back" @click="router.push('/contas')">‹ Contas</button>
    <template v-if="card">
      <h1 class="screen-title">{{ card.title }}</h1>
      <CardFacts :card="card" />
      <div class="columns">
        <CardPayAhead :card="card" />
        <CardBills :bills="card.bills" />
      </div>
    </template>
    <EmptyBlock
      v-else-if="card === null"
      title="Nenhum cartão encontrado"
      hint="O cartão aparece aqui depois que o banco enviar a fatura."
    />
  </div>
</template>

<style scoped>
.card-page {
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

.columns {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--space-16);
  align-items: start;
}

@media (max-width: 820px) {
  .columns {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
