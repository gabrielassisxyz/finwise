<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { AccountDetail } from '@/api/types';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { brl } from '@/lib/format';

const api = useApi();
const route = useRoute();
const router = useRouter();

const account = ref<AccountDetail | null>(null);
const missing = ref(false);

const id = computed(() => String(route.params.id));

let latest = 0;
watch(
  id,
  async (requested) => {
    const request = ++latest;
    account.value = null;
    missing.value = false;
    const loaded = await api.accounts.detail(requested);
    if (request !== latest || requested !== id.value) return;
    account.value = loaded;
    missing.value = loaded === null;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen conta" data-screen-label="Conta">
    <div class="header">
      <button class="back" @click="router.push('/contas')">‹ Contas</button>
      <p v-if="missing" class="missing">Conta não encontrada.</p>
      <div v-if="account" class="heading">
        <div class="title-block">
          <h1 class="screen-title">{{ account.bank }} · {{ account.name }}</h1>
          <span class="bucket">Balde: {{ account.bucket }}</span>
        </div>
        <div class="balance-block">
          <span class="balance-label">Saldo</span>
          <span class="balance">{{ brl(account.balance) }}</span>
        </div>
      </div>
    </div>
    <TransactionList v-if="account" :key="account.id" :account="account.ledgerAccount" />
  </div>
</template>

<style scoped>
.header {
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
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

.bucket {
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
</style>
