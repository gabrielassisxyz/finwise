<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { AccountRow } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import { brl } from '@/lib/format';

const api = useApi();
const router = useRouter();

const accounts = ref<AccountRow[] | null>(null);

void api.accounts.list().then((loaded) => (accounts.value = loaded));

function open(account: AccountRow): void {
  void router.push(account.kind === 'Cartão' ? '/contas/cartao' : `/contas/${account.id}`);
}
</script>

<template>
  <div class="screen contas" data-screen-label="Contas">
    <h1 class="screen-title">Contas</h1>
    <div class="table">
      <div class="head desktop-only">
        <span>Banco</span>
        <span>Nome</span>
        <span>Tipo</span>
        <span class="right">Saldo ou fatura atual</span>
      </div>
      <EmptyBlock
        v-if="accounts && accounts.length === 0"
        title="Nenhuma conta ainda"
        hint="As contas dos seus bancos aparecem aqui depois da primeira sincronização."
      />
      <div
        v-for="account in accounts"
        :key="account.id"
        class="row stack-row"
        :class="{ idle: account.idle }"
        @click="open(account)"
      >
        <span class="bank" data-label="Banco">{{ account.bank }}</span>
        <span class="name" data-label="Nome">
          <span>{{ account.name }}</span>
          <span v-if="account.idle" class="note">Sem movimento recente</span>
        </span>
        <span class="kind" data-label="Tipo">{{ account.kind }}</span>
        <span class="amount" :data-label="account.kind === 'Cartão' ? 'Fatura atual' : 'Saldo'">{{ brl(account.amount) }}</span>
      </div>
    </div>
    <span class="footer">
      Só aparece o que os bancos enviam. Contas novas entram sozinhas depois da próxima sincronização.
    </span>
  </div>
</template>

<style scoped>
.contas {
  gap: var(--space-16);
}

.table {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head,
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.6fr) 100px minmax(0, 1fr);
  gap: var(--space-16);
  padding: 0 var(--space-16);
}

.head {
  padding-top: var(--space-10);
  padding-bottom: var(--space-10);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  background: var(--color-surface-head);
}

.row {
  min-height: 54px;
  align-items: center;
  font-size: var(--font-size-body);
  border-top: 1px solid var(--color-border-row);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}

.row:hover {
  background: var(--color-surface-hover-row);
}

.row.idle {
  opacity: 0.55;
}

.right {
  text-align: right;
}

.bank {
  font-weight: var(--font-weight-medium);
}

.name {
  display: flex;
  gap: var(--space-8);
  align-items: center;
  flex-wrap: wrap;
}

.note {
  font-size: var(--font-size-badge);
  line-height: 1.5;
  padding: 0 var(--space-6);
  border-radius: var(--radius-badge);
  border: 1px solid var(--color-border-button);
  color: var(--color-text-ink-2);
}

.kind {
  color: var(--color-text-ink-2);
}

.amount {
  text-align: right;
  font-weight: var(--font-weight-medium);
}

.footer {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}
</style>
