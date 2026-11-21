<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useApi } from '@/api/context';
import type { ReviewWarning, WarningActionId, WarningOutcome } from '@/api/types';
import ReviewCard from './ReviewCard.vue';
import { useReviewActions } from './useReviewActions';

defineProps<{ warnings: ReviewWarning[] }>();

const api = useApi();
const router = useRouter();
const actions = useReviewActions();

const OUTCOME_MESSAGE: Record<WarningOutcome, (name: string) => string> = {
  'subscription-created': (name) => `${name} entrou nas assinaturas.`,
  'suggestion-rejected': (name) => `${name} não será tratado como assinatura.`,
  'pair-silenced': () => 'Par silenciado. Não avisaremos mais sobre esta sobreposição.',
};

function open(warning: ReviewWarning): void {
  void router.push(warning.subscriptionId ? `/assinaturas/${warning.subscriptionId}` : '/assinaturas');
}

async function dismiss(warning: ReviewWarning): Promise<void> {
  const result = await api.review.dismissWarning(warning.id);
  if (result) actions.announce('Aviso dispensado.', result.changeId);
}

async function run(warning: ReviewWarning, action: WarningActionId): Promise<void> {
  const result = await api.review.runWarningAction(warning.id, action);
  if (result) actions.announce(OUTCOME_MESSAGE[result.outcome](result.name), result.changeId);
}
</script>

<template>
  <ReviewCard title="Avisos">
    <div v-for="warning in warnings" :key="warning.id" class="warning">
      <button class="text" @click="open(warning)">{{ warning.text }}</button>
      <button v-for="action in warning.actions" :key="action.id" class="action" @click="run(warning, action.id)">
        {{ action.label }}
      </button>
      <button class="dismiss" aria-label="Dispensar aviso" title="Dispensar" @click="dismiss(warning)">×</button>
    </div>
    <div v-if="!warnings.length" class="empty">
      Sem avisos. Aumentos de preço, renovações e cobranças estranhas aparecem aqui quando forem detectados.
    </div>
  </ReviewCard>
</template>

<style scoped>
.warning {
  padding: var(--space-10) var(--space-8) var(--space-10) 18px; /* token-exempt: between two steps of the spacing scale */
  display: flex;
  align-items: center;
  gap: var(--space-8);
  flex-wrap: wrap;
  border-bottom: 1px solid var(--color-border-row);
}

.text {
  flex: 1 1 200px;
  min-width: 0;
  border: 0;
  background: none;
  padding: var(--space-4) 0;
  text-align: left;
  font-size: var(--font-size-body);
  line-height: 1.45;
  cursor: pointer;
  color: var(--color-text-ink);
  text-wrap: pretty;
}

.text:hover {
  color: var(--color-accent-link);
}

.action {
  height: 32px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  color: var(--color-text-ink);
  cursor: pointer;
  white-space: nowrap;
}

.dismiss {
  width: 36px;
  height: 36px;
  border: 0;
  background: none;
  font-size: 18px; /* token-exempt: glyph size of the dismiss cross */
  color: var(--color-text-muted);
  cursor: pointer;
  border-radius: var(--radius-nav);
}

.dismiss:hover {
  background: var(--color-surface-hover-item);
}

.empty {
  padding: 22px 18px; /* token-exempt: between two steps of the spacing scale */
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  text-wrap: pretty;
}
</style>
