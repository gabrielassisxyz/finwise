<script setup lang="ts">
import { ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { SubscriptionList } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import ToggleButton from '@/components/ui/ToggleButton.vue';
import { brl } from '@/lib/format';
import { useRevisionStore } from '@/stores/revision';
import { useSubscriptionsViewStore } from '@/stores/subscriptions-view';
import SubscriptionRow from './SubscriptionRow.vue';
import SuggestionStrip from './SuggestionStrip.vue';
import { useSubscriptionActions } from './useSubscriptionActions';

const api = useApi();
const router = useRouter();
const view = useSubscriptionsViewStore();
const revision = useRevisionStore();
const actions = useSubscriptionActions();

const list = ref<SubscriptionList | null>(null);

watch(
  [() => view.grouped, () => revision.current],
  async ([grouped]) => {
    const loaded = await api.subscriptions.list({ grouped });
    if (grouped === view.grouped) list.value = loaded;
  },
  { immediate: true },
);
</script>

<template>
  <template v-if="list">
    <div class="top">
      <span class="total">
        Total mensal equivalente <strong>{{ brl(list.monthlyTotal) }}</strong> <span class="year">· Anual {{ brl(list.annualTotal) }}</span>
      </span>
      <div class="buttons">
        <ToggleButton :pressed="view.grouped" @click="view.grouped = !view.grouped">Agrupar por status</ToggleButton>
        <button class="add" @click="actions.openCandidateMenu($event.currentTarget as HTMLElement)">Nova assinatura</button>
      </div>
    </div>
    <SuggestionStrip
      v-if="list.suggestion"
      :name="list.suggestion.name"
      @accept="actions.acceptSuggestion()"
      @reject="actions.rejectSuggestion(list.suggestion.name)"
    />
    <div class="table">
      <div class="scroller">
        <div class="content stack-table">
          <div class="head desktop-only">
            <span>Nome</span>
            <span class="right">Valor</span>
            <span class="wide-only">Ciclo</span>
            <span>Próxima cobrança</span>
            <span class="right wide-only">Equivalente mensal</span>
            <span class="wide-only">Linha do budget</span>
            <span>Status</span>
            <span>Revisar em</span>
          </div>
          <EmptyBlock
            v-if="list.groups.every((group) => group.subscriptions.length === 0)"
            title="Nenhuma assinatura ainda"
            hint="Use Nova assinatura para marcar uma compra do cartão como assinatura."
          />
          <template v-for="group in list.groups" :key="group.status ?? 'all'">
            <div v-if="group.status" class="group-head">
              <span class="group-name">{{ group.status }}</span>
              <span class="group-meta">{{ group.subscriptions.length }} · {{ brl(group.monthlyTotal) }} por mês</span>
            </div>
            <SubscriptionRow
              v-for="row in group.subscriptions"
              :key="row.id"
              :row="row"
              @open="router.push(`/assinaturas/${row.id}`)"
              @pick-status="actions.openStatusMenu($event, row)"
            />
          </template>
        </div>
      </div>
    </div>
  </template>
</template>

<style scoped>
.top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
  flex-wrap: wrap;
}

.total {
  font-size: var(--font-size-section);
  font-variant-numeric: tabular-nums;
}

.total strong {
  font-weight: var(--font-weight-semibold);
}

.year {
  color: var(--color-text-muted);
}

.buttons {
  display: flex;
  gap: var(--space-8);
  flex-wrap: wrap;
}

.add {
  height: 36px;
  padding: 0 var(--space-14);
  border-radius: var(--radius-control);
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
  cursor: pointer;
}

.add:hover {
  background: var(--color-accent-hover);
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

/* Below 1200px the cycle, equivalent and line columns are display: none, so they leave the grid and the name carries them. */
.content {
  --cols: minmax(0, 1fr) 84px 92px 150px 86px;
  --table-min-width: 600px;
}

@media (min-width: 1200px) {
  .content {
    --cols: minmax(0, 1.7fr) 88px 86px 104px 100px minmax(0, 1.1fr) 158px 96px;
    --table-min-width: 920px;
  }
}

.head {
  display: grid;
  grid-template-columns: var(--cols);
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
  gap: var(--space-12);
  padding: var(--space-12) var(--space-16) var(--space-8);
  border-top: 1px solid var(--color-border-divider);
  background: var(--color-surface-nav);
  font-size: var(--font-size-secondary);
}

.group-name {
  font-weight: var(--font-weight-semibold);
}

.group-meta {
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
