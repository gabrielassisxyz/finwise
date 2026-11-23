<script setup lang="ts">
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { ClosingStep as ClosingStepModel, ClosingView } from '@/api/types';
import { monthInSentence } from '@/lib/format';
import { createLatestLoad } from '@/lib/latestLoad';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useReviewViewStore } from '@/stores/review-view';
import { useRevisionStore } from '@/stores/revision';
import ClosedMonthsList from './ClosedMonthsList.vue';
import ClosingStep from './ClosingStep.vue';
import { STEP_TITLES } from './closingText';
import StepCardBill from './StepCardBill.vue';
import StepClose from './StepClose.vue';
import StepContributions from './StepContributions.vue';
import StepLeftover from './StepLeftover.vue';
import StepPending from './StepPending.vue';
import StepPlannedActual from './StepPlannedActual.vue';
import StepReport from './StepReport.vue';
import StepSubscriptions from './StepSubscriptions.vue';
import { useClosingActions } from './useClosingActions';

const api = useApi();
const month = useMonthStore();
const overlay = useOverlayStore();
const revision = useRevisionStore();
const view = useReviewViewStore();
const actions = useClosingActions();

const closing = ref<ClosingView | null>(null);

const loads = createLatestLoad();
watch(
  () => [month.selected, revision.current] as const,
  async ([selected]) => {
    const isLatest = loads.start();
    const loaded = await api.review.closing(selected);
    if (isLatest() && selected === month.selected) closing.value = loaded;
  },
  { immediate: true },
);

function toggle(index: number): void {
  view.openStep = view.openStep === index ? -1 : index;
}

async function markDone(step: ClosingStepModel, index: number): Promise<void> {
  await actions.markStepDone(month.selected, step.id);
  view.openStep = index + 1;
}

function askToClose(): void {
  overlay.confirm({
    message: `Fechar ${monthInSentence(month.selected)}? O budget deste mês fica guardado e não muda mais.`,
    confirmLabel: 'Fechar o mês',
    onConfirm: async () => {
      await actions.closeMonth(month.selected);
      view.openStep = -1;
    },
  });
}

function openClosedMonth(selected: string): void {
  month.selected = selected;
  view.openStep = 1;
  window.scrollTo(0, 0);
}
</script>

<template>
  <template v-if="closing">
    <div class="steps">
      <ClosingStep
        v-for="(step, index) in closing.steps"
        :key="step.id"
        :number="index + 1"
        :title="STEP_TITLES[step.id]"
        :state="step.state"
        :open="view.openStep === index"
        :can-mark-done="step.markable && step.state !== 'done'"
        @toggle="toggle(index)"
        @done="markDone(step, index)"
      >
        <StepPending v-if="step.id === 'pending'" :view="closing" />
        <StepPlannedActual v-else-if="step.id === 'planned-actual'" :groups="closing.plannedActual" />
        <StepLeftover v-else-if="step.id === 'leftover'" :leftover="closing.leftover" />
        <StepContributions v-else-if="step.id === 'contributions'" :contributions="closing.contributions" />
        <StepCardBill v-else-if="step.id === 'card-bill' && closing.cardBill" :bill="closing.cardBill" />
        <StepSubscriptions
          v-else-if="step.id === 'subscriptions'"
          :subscriptions="closing.subscriptionsToReview"
          :read-only="closing.closed"
          @keep="actions.keepSubscription(month.selected, $event)"
          @cancel="actions.cancelSubscription($event)"
        />
        <StepReport
          v-else-if="step.id === 'report'"
          :report="closing.report"
          :read-only="closing.closed"
          @send="actions.sendSuggestion($event)"
        />
        <StepClose v-else-if="step.id === 'close'" :view="closing" @close="askToClose" />
      </ClosingStep>
    </div>
    <ClosedMonthsList :months="closing.closedMonths" @open="openClosedMonth" />
  </template>
</template>

<style scoped>
.steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-10);
}
</style>
