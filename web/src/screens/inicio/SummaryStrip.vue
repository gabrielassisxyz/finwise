<script setup lang="ts">
import { computed } from 'vue';
import type { HomeSummary, YearMonth } from '@/api/types';
import { brl, brlWhole, monthLabel } from '@/lib/format';
import { useOverlayStore } from '@/stores/overlay';

const props = defineProps<{ summary: HomeSummary; month: YearMonth }>();

const overlay = useOverlayStore();

const limit = computed(() => {
  const { cap, spent } = props.summary.limit;
  const left = Math.round((cap - spent) * 100) / 100;
  return {
    over: left < 0,
    left: brl(left),
    spent: brl(spent),
    cap: brl(cap),
    fill: `${Math.min(100, cap ? (spent / cap) * 100 : 0)}%`,
  };
});

const currentWeek = computed(() => props.summary.weeks.find((week) => week.current) ?? null);

const netWorthTotal = computed(() => {
  const { available, reserved, invested } = props.summary.netWorth;
  return available + reserved + invested;
});

const pendingLabel = computed(() => {
  const count = props.summary.pendingCount;
  if (count === 0) return 'Tudo revisado';
  return count === 1 ? '1 para revisar' : `${count} para revisar`;
});

function openWeeks(event: MouseEvent): void {
  overlay.openMenu(event.currentTarget as HTMLElement, {
    title: `Semanas de ${monthLabel(props.month)} · guia e gasto`,
    items: props.summary.weeks.map((week) => {
      const spent = week.spent === null ? '—' : brlWhole(week.spent);
      const over = week.spent !== null && week.spent > week.guide ? ' · acima' : '';
      return {
        label: week.current ? `${week.label} · esta` : week.label,
        hint: `${brlWhole(week.guide)} · ${spent}${over}`,
      };
    }),
  });
}
</script>

<template>
  <div class="strip">
    <div class="card card-wide">
      <span class="caption">Limite do mês</span>
      <span class="figure" :class="{ over: limit.over }">{{ limit.over ? 'Passou' : 'Restam' }} {{ limit.left }}</span>
      <div class="bar"><div class="bar-fill" :class="{ over: limit.over }" :style="{ width: limit.fill }" /></div>
      <span class="detail">Gasto {{ limit.spent }} de {{ limit.cap }}</span>
    </div>

    <button v-if="currentWeek" class="card" @click="openWeeks">
      <span class="caption">Esta semana ▾</span>
      <span class="figure">{{ brl(currentWeek.spent ?? 0) }}</span>
      <span class="detail">Guia {{ brl(currentWeek.guide) }} · gasto {{ brl(currentWeek.spent ?? 0) }}</span>
    </button>

    <RouterLink to="/patrimonio" class="card">
      <span class="caption">Patrimônio</span>
      <span class="figure">{{ brl(netWorthTotal) }}</span>
      <span class="buckets">
        <span class="bucket"><span>Disponível</span><span>{{ brl(summary.netWorth.available) }}</span></span>
        <span class="bucket"><span>Reservado</span><span>{{ brl(summary.netWorth.reserved) }}</span></span>
        <span class="bucket"><span>Investido</span><span>{{ brl(summary.netWorth.invested) }}</span></span>
      </span>
    </RouterLink>

    <RouterLink to="/revisao" class="card">
      <span class="caption">Pendências</span>
      <span class="figure">{{ summary.pendingCount }}</span>
      <span class="action">{{ pendingLabel }} ›</span>
    </RouterLink>

    <RouterLink v-if="summary.payAhead" to="/contas/cartao" class="card card-accent">
      <span class="caption">Valor a adiantar da fatura</span>
      <span class="figure">{{ brl(summary.payAhead.amount) }}</span>
      <span class="action">{{ summary.payAhead.cardLabel }} ›</span>
    </RouterLink>
  </div>
</template>

<style scoped>
.strip {
  display: flex;
  flex-wrap: wrap;
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-card);
  overflow: hidden;
}

/* Each card draws its own left and top rule and is pulled one pixel over the
   strip's edge, so the rules only show between cards however they wrap. */
.card {
  flex: 1 1 180px;
  min-width: 0;
  box-sizing: border-box;
  padding: var(--space-16) var(--space-20);
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  border: 0;
  border-left: 1px solid var(--color-border-divider);
  border-top: 1px solid var(--color-border-divider);
  margin: -1px 0 0 -1px;
  background: var(--color-surface-card);
  text-align: left;
  color: inherit;
  text-decoration: none;
}

.card-wide {
  flex: 2 1 300px;
}

button.card,
a.card {
  cursor: pointer;
}

button.card:hover,
a.card:hover {
  background: var(--color-surface-hover-row);
  color: inherit;
  text-decoration: none;
}

.card-accent,
a.card-accent {
  background: var(--color-accent-card);
}

a.card-accent:hover {
  background: var(--color-accent-card-hover);
}

.caption {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.card-accent .caption {
  color: var(--color-accent-card-label);
}

.figure {
  font-size: var(--font-size-value);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
  font-variant-numeric: tabular-nums;
}

.figure.over {
  color: var(--color-bad-text);
}

.bar {
  height: 8px;
  border-radius: var(--radius-bar);
  background: var(--color-surface-track);
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background: var(--color-accent-base);
  border-radius: var(--radius-bar);
}

.bar-fill.over {
  background: var(--color-bad-text);
}

.detail {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
  font-variant-numeric: tabular-nums;
}

.buckets {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: var(--font-size-caption);
  color: var(--color-text-ink-2);
  font-variant-numeric: tabular-nums;
}

.bucket {
  display: flex;
  justify-content: space-between;
  gap: var(--space-8);
}

.action {
  font-size: var(--font-size-secondary);
  color: var(--color-accent-link);
  font-weight: var(--font-weight-medium);
}
</style>
