<script setup lang="ts">
import { computed } from 'vue';
import type { TransactionDetail } from '@/api/types';
import SwitchToggle from '@/components/ui/SwitchToggle.vue';
import { saveStored, vStored } from '@/lib/storedField';
import { usePanelActions } from './panelActions';
import PanelSection from './PanelSection.vue';

const props = defineProps<{ detail: TransactionDetail }>();

const actions = usePanelActions();

const missingReason = computed(() => !!props.detail.exclusion && props.detail.exclusion.reason === '');
</script>

<template>
  <PanelSection>
    <div class="head">
      <span class="titles">
        <span class="title">Não conta</span>
        <span class="hint">Tira a transação de todos os totais</span>
      </span>
      <SwitchToggle :on="!!detail.exclusion" label="Não conta" @toggle="actions.setExcluded(!detail.exclusion)" />
    </div>
    <template v-if="detail.exclusion">
      <input
        class="reason"
        :class="{ missing: missingReason }"
        v-stored="detail.exclusion.reason"
        placeholder="Motivo (obrigatório)"
        @change="saveStored($event, actions.setExclusionReason)"
      />
      <span v-if="missingReason" class="warning">Só vale depois de informar o motivo.</span>
    </template>
  </PanelSection>
</template>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-12);
}

.titles {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.title {
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-medium);
}

.hint {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}

.reason {
  height: 36px;
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-body);
  outline: none;
  color: var(--color-text-ink);
}

.reason.missing {
  border-color: var(--color-warn-field-border);
}

.warning {
  font-size: var(--font-size-caption);
  color: var(--color-warn-text);
}
</style>
