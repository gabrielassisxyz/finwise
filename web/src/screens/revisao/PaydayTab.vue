<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { PaydayTransfer, PaydayView, YearMonth } from '@/api/types';
import EmptyBlock from '@/components/ui/EmptyBlock.vue';
import { brl, orUnknown, shortDay, UNKNOWN } from '@/lib/format';
import { createLatestLoad } from '@/lib/latestLoad';
import { useMonthStore } from '@/stores/month';
import { useRevisionStore } from '@/stores/revision';
import { useReviewActions } from './useReviewActions';

const api = useApi();
const month = useMonthStore();
const revision = useRevisionStore();
const actions = useReviewActions();

// A transfer's key belongs to the month its list was loaded for, which the
// selected month may already have left.
const shown = ref<{ month: YearMonth; view: PaydayView } | null>(null);
const view = computed(() => shown.value?.view ?? null);

const loads = createLatestLoad();
watch(
  () => [month.selected, revision.current] as const,
  async ([selected]) => {
    const isLatest = loads.start();
    const loaded = await api.review.payday(selected);
    if (isLatest() && selected === month.selected) shown.value = { month: selected, view: loaded };
  },
  { immediate: true },
);

const figures = computed(() => {
  if (!view.value) return [];
  const { cushion, balance, balanceLabel, difference, covered } = view.value;
  return [
    { key: 'Colchão necessário', value: brl(cushion), hint: 'Fixos e essenciais + variáveis do mês', tone: '' },
    { key: 'Saldo em conta', value: orUnknown(balance, brl), hint: balanceLabel ?? 'Nenhuma conta recebe o salário', tone: '' },
    covered === null
      ? { key: 'Diferença para o colchão', value: UNKNOWN, hint: 'Sem saldo para comparar', tone: '' }
      : {
          key: covered ? 'Acima do colchão' : 'Faltam para o colchão',
          value: orUnknown(difference, brl),
          hint: covered ? 'O salário pode ser guardado no dia em que chegar' : 'Complete o colchão antes de guardar',
          tone: covered ? 'positive' : 'bad',
        },
  ];
});

function stateText(transfer: PaydayTransfer): string {
  if (transfer.state === 'pending') return 'Pendente';
  if (transfer.state === 'on-target') return 'no alvo';
  const day = transfer.doneOn ? shortDay(transfer.doneOn) : '';
  return transfer.markedByHand ? `Feita em ${day} · marcada por você` : `Feita em ${day}`;
}

async function markDone(transfer: PaydayTransfer): Promise<void> {
  if (!shown.value) return;
  const result = await api.review.markTransferDone(shown.value.month, transfer.key);
  if (result) actions.announce(`${transfer.name}: transferência marcada como feita.`, result.changeId);
}
</script>

<template>
  <template v-if="view">
    <div class="cell-strip figures">
      <div v-for="figure in figures" :key="figure.key" class="cell">
        <span class="caption">{{ figure.key }}</span>
        <span class="value" :class="figure.tone">{{ figure.value }}</span>
        <span class="hint">{{ figure.hint }}</span>
      </div>
    </div>
    <section class="card">
      <div class="head">
        <span class="title">Transferências do dia do salário</span>
        <span class="hint">O app marca como feita quando encontra a transferência. Você também pode marcar à mão.</span>
      </div>
      <EmptyBlock
        v-if="view.transfers.length === 0"
        title="Nenhuma transferência planejada"
        hint="Elas aparecem aqui quando o budget tiver aportes para fazer no dia do salário."
      />
      <div v-for="transfer in view.transfers" :key="transfer.key" class="row stack-row">
        <span class="name stack-full">{{ transfer.name }}</span>
        <span class="amount" data-label="Valor">{{ brl(transfer.amount) }}</span>
        <span class="state" :class="transfer.state" data-label="Estado">{{ stateText(transfer) }}</span>
        <span class="action">
          <button v-if="transfer.state === 'pending'" class="mark" @click="markDone(transfer)">Marcar como feita</button>
        </span>
      </div>
    </section>
  </template>
</template>

<style scoped>
.figures {
  --cell-flex: 1 1 200px;
}

.caption,
.hint {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.hint {
  font-size: var(--font-size-caption);
}

.value {
  font-size: var(--font-size-value);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.value.positive {
  color: var(--color-accent-positive);
}

.value.bad {
  color: var(--color-bad-text);
}

.card {
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

.head {
  padding: var(--space-12) var(--space-16);
  background: var(--color-surface-head);
  border-bottom: 1px solid var(--color-border-divider);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-semibold);
}

.row {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1.4fr) 150px;
  gap: var(--space-16);
  padding: 0 var(--space-16);
  min-height: 52px;
  align-items: center;
  font-size: var(--font-size-body);
  border-bottom: 1px solid var(--color-border-row);
  font-variant-numeric: tabular-nums;
}

.name {
  font-weight: var(--font-weight-medium);
}

.amount {
  text-align: right;
}

.state {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-medium);
  color: var(--color-accent-positive);
}

.state.pending {
  color: var(--color-warn-text);
}

.state.on-target {
  color: var(--color-text-ink-2);
}

.action {
  display: flex;
  justify-content: flex-end;
}

.mark {
  height: 36px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
  white-space: nowrap;
}

.mark:hover {
  background: var(--color-surface-hover-item);
}
</style>
