<script setup lang="ts">
import { computed } from 'vue';
import type { ReviewQueueItem } from '@/api/types';
import { brl, longDay } from '@/lib/format';
import { useReviewViewStore } from '@/stores/review-view';

const props = defineProps<{ item: ReviewQueueItem }>();
defineEmits<{ accept: []; pick: [anchor: HTMLElement]; skip: [] }>();

const { draft } = useReviewViewStore();

const picked = computed(() => draft.line);
const suggestion = computed(() => {
  const { pending } = props.item;
  return picked.value ? `Linha escolhida: ${picked.value}` : `Sugestão da IA: ${pending.suggestion} · ${pending.confidence}%`;
});
const justification = computed(() =>
  picked.value
    ? `A IA tinha sugerido ${props.item.pending.suggestion}. Confirme para usar a sua escolha.`
    : props.item.pending.justification,
);
</script>

<template>
  <div class="item">
    <span class="reason">{{ item.pending.reason }}</span>
    <div class="who">
      <span class="names">
        <span class="name">{{ item.name }}</span>
        <span class="raw">{{ item.rawDescription }}</span>
      </span>
      <span class="amount">{{ brl(item.amount) }}</span>
    </div>
    <span class="where">{{ longDay(item.day) }} · {{ item.accountLabel }}</span>
    <div class="suggestion">
      <span class="line">{{ suggestion }}</span>
      <span class="why">{{ justification }}</span>
    </div>
    <div class="buttons">
      <button class="primary" @click="$emit('accept')">{{ picked ? 'Confirmar' : 'Aceitar' }}</button>
      <button class="secondary" @click="$emit('pick', $event.currentTarget as HTMLElement)">Escolher outra linha</button>
      <button class="quiet" @click="$emit('skip')">Pular</button>
    </div>
    <div class="extras">
      <label v-if="!item.person" class="always">
        <input v-model="draft.always" type="checkbox" />Aplicar sempre a este estabelecimento
      </label>
      <label v-if="item.person" class="nickname">
        Dar apelido a esta pessoa
        <input v-model="draft.nickname" class="field" placeholder="Aluguel" />
      </label>
      <input v-model="draft.reason" class="field" placeholder="Por quê?" aria-label="Por quê?" />
    </div>
  </div>
</template>

<style scoped>
.item {
  padding: var(--space-20) 18px; /* token-exempt: between two steps of the spacing scale */
  display: flex;
  flex-direction: column;
  gap: var(--space-14);
}

.reason {
  align-self: flex-start;
  font-size: var(--font-size-caption);
  line-height: 1.6;
  padding: 0 var(--space-8);
  border-radius: var(--radius-badge);
  color: var(--color-warn-text);
  background: var(--color-warn-bg);
  border: 1px solid var(--color-warn-border);
}

.who {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-16);
}

.names {
  display: flex;
  flex-direction: column;
  gap: 3px; /* token-exempt: between two steps of the spacing scale */
  min-width: 0;
}

.name,
.amount {
  font-size: var(--font-size-panel-title);
  font-weight: var(--font-weight-semibold);
  letter-spacing: calc(var(--font-tracking-tight) * 1em);
}

.amount {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.raw {
  font-size: var(--font-size-badge);
  font-family: var(--font-family-mono);
  color: var(--color-text-muted-2);
}

.where {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
}

.suggestion {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-12) var(--space-14);
  border-radius: var(--radius-control);
  background: var(--color-accent-card);
}

.line {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.why {
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-3);
  text-wrap: pretty;
}

.buttons {
  display: flex;
  gap: var(--space-8);
  flex-wrap: wrap;
}

.buttons button {
  height: 40px;
  border-radius: var(--radius-control);
  font-size: var(--font-size-body);
  cursor: pointer;
}

.primary {
  padding: 0 18px; /* token-exempt: between two steps of the spacing scale */
  border: 0;
  background: var(--color-accent-base);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.primary:hover {
  background: var(--color-accent-hover);
}

.secondary {
  padding: 0 var(--space-14);
  border: 1px solid var(--color-border-button);
  background: var(--color-surface-card);
  color: var(--color-text-ink);
}

.secondary:hover {
  background: var(--color-surface-hover-item);
}

.quiet {
  padding: 0 var(--space-14);
  border: 0;
  background: none;
  color: var(--color-text-ink-2);
}

.quiet:hover {
  background: var(--color-surface-hover-item);
}

.extras {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
  padding-top: var(--space-12);
  border-top: 1px solid var(--color-border-divider);
}

.always {
  display: flex;
  align-items: center;
  gap: var(--space-8);
  font-size: var(--font-size-body);
  cursor: pointer;
  min-height: 28px;
}

.always input {
  width: 16px;
  height: 16px;
  margin: 0;
  accent-color: var(--color-accent-base);
}

.nickname {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink-2);
}

.field {
  height: 36px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  color: var(--color-text-ink);
}
</style>
