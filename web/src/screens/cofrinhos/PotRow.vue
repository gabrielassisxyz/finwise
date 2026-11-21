<script setup lang="ts">
import type { SavingsPotRow } from '@/api/types';
import { brlWhole, longDay, signedBrl, brl } from '@/lib/format';

defineProps<{ pot: SavingsPotRow; showStatus: boolean }>();
defineEmits<{ open: [] }>();
</script>

<template>
  <div class="row stack-row" :class="{ closed: pot.closed }" @click="$emit('open')">
    <span class="name-cell stack-full">
      <span class="name" :class="{ unnamed: pot.name === null }">{{ pot.name ?? 'Sem nome' }}</span>
      <span class="subtitle narrow-only">{{ pot.bank }}</span>
    </span>
    <span class="secondary wide-only" data-label="Banco">{{ pot.bank }}</span>
    <span class="secondary wide-only" data-label="Balde">{{ pot.bucket ?? 'Sem balde' }}</span>
    <span class="balance" data-label="Saldo">{{ brl(pot.balance) }}</span>
    <span class="secondary right" data-label="% CDI">{{ pot.cdiPercent }}%</span>
    <span class="yield" data-label="Rendimento no mês">{{ pot.closed ? '—' : signedBrl(pot.monthlyYield) }}</span>
    <span class="secondary" data-label="Vencimento">{{ pot.maturity ? longDay(pot.maturity) : '—' }}</span>
    <span class="target" data-label="Alvo">
      <span>{{ pot.target ? `${brlWhole(pot.balance)} de ${brlWhole(pot.target.amount)}` : '—' }}</span>
      <span v-if="pot.target" class="bar"><span class="bar-fill" :style="{ width: `${pot.target.progress}%` }" /></span>
    </span>
    <span v-if="showStatus" class="status" data-label="Status">{{ pot.closed ? 'Encerrado' : 'Ativo' }}</span>
  </div>
</template>

<style scoped>
.row {
  display: grid;
  grid-template-columns: var(--cols) var(--status-width, );
  gap: var(--space-14);
  padding: 0 var(--space-16);
  min-height: 50px;
  align-items: center;
  font-size: var(--font-size-body);
  border-top: 1px solid var(--color-border-row);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}

.row:hover {
  background: var(--color-surface-hover-row);
}

.row.closed {
  opacity: 0.55;
}

.name-cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.name {
  font-weight: var(--font-weight-medium);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.name.unnamed {
  color: var(--color-text-muted);
}

.subtitle {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.secondary {
  color: var(--color-text-ink-2);
}

.right,
.balance,
.yield {
  text-align: right;
}

.balance {
  font-weight: var(--font-weight-medium);
}

.yield {
  color: var(--color-accent-positive);
}

.target {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-2);
}

.bar {
  display: block;
  height: 5px;
  border-radius: var(--radius-bar);
  background: var(--color-surface-track);
  overflow: hidden;
}

.bar-fill {
  display: block;
  height: 100%;
  background: var(--color-accent-base);
}

.status {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
}
</style>
