<script setup lang="ts">
import { ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { SavingsPotGroup } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import ToggleButton from '@/components/ui/ToggleButton.vue';
import { brl } from '@/lib/format';
import { useSavingsViewStore } from '@/stores/savings-view';
import PotRow from './PotRow.vue';

const api = useApi();
const router = useRouter();
const view = useSavingsViewStore();

const groups = ref<SavingsPotGroup[] | null>(null);

watch(
  () => view.showClosed,
  async (includeClosed) => {
    const loaded = await api.savingsPots.list({ includeClosed });
    if (includeClosed === view.showClosed) groups.value = loaded;
  },
  { immediate: true },
);
</script>

<template>
  <div class="screen cofrinhos" data-screen-label="Cofrinhos">
    <div class="top">
      <h1 class="screen-title">Cofrinhos</h1>
      <ToggleButton :pressed="view.showClosed" @click="view.showClosed = !view.showClosed">
        Mostrar encerrados
      </ToggleButton>
    </div>
    <div class="table">
      <div class="scroller">
        <div class="content stack-table" :class="{ 'with-status': view.showClosed }">
          <div class="head desktop-only">
            <span>Nome</span>
            <span class="wide-only">Banco</span>
            <span class="wide-only">Balde</span>
            <span class="right">Saldo</span>
            <span class="right">% CDI</span>
            <span class="right">Rendimento no mês</span>
            <span>Vencimento</span>
            <span>Alvo</span>
            <span v-if="view.showClosed">Status</span>
          </div>
          <EmptyBlock
            v-if="groups && groups.length === 0"
            title="Nenhum cofrinho ainda"
            hint="Os cofrinhos e investimentos dos seus bancos aparecem aqui depois da sincronização."
          />
          <template v-for="group in groups" :key="group.bucket ?? 'none'">
            <div class="group-head">
              <span class="group-name">{{ group.bucket ?? 'Sem balde' }}</span>
              <span class="subtotal">Subtotal <strong>{{ brl(group.subtotal) }}</strong></span>
            </div>
            <PotRow
              v-for="pot in group.pots"
              :key="pot.id"
              :pot="pot"
              :show-status="view.showClosed"
              @open="router.push(`/cofrinhos/${encodeURIComponent(pot.id)}`)"
            />
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cofrinhos {
  gap: var(--space-16);
}

.top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-12);
  flex-wrap: wrap;
}

.table {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.scroller {
  overflow-x: auto;
}

/* Below 1200px the bank and bucket columns are display: none, so they leave the grid and the name carries the bank. */
.content {
  --cols: minmax(0, 1.6fr) 92px 46px 96px 78px minmax(0, 1.2fr);
  --table-min-width: 600px;
}

.content.with-status {
  --status-width: 84px;
}

@media (min-width: 1200px) {
  .content {
    --cols: minmax(0, 1.5fr) minmax(0, 1fr) 84px 104px 56px 124px 92px minmax(0, 1.3fr);
    --table-min-width: 900px;
  }
}

.head {
  display: grid;
  grid-template-columns: var(--cols) var(--status-width, );
  gap: var(--space-14);
  padding: var(--space-10) var(--space-16);
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  background: var(--color-surface-head);
}

.right {
  text-align: right;
}

.group-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16) var(--space-8);
  border-top: 1px solid var(--color-border-divider);
  background: var(--color-surface-nav);
}

.group-name {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-semibold);
}

.subtotal {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.subtotal strong {
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-ink);
}
</style>
