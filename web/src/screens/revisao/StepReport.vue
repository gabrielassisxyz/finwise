<script setup lang="ts">
import type { ReportSection } from '@/api/types';

defineProps<{ report: ReportSection[]; readOnly: boolean }>();
defineEmits<{ send: [proposalId: string] }>();
</script>

<template>
  <div v-for="section in report" :key="section.heading" class="section">
    <span class="heading">{{ section.heading }}</span>
    <template v-for="paragraph in section.paragraphs" :key="paragraph.text">
      <span class="text">{{ paragraph.text }}</span>
      <button
        v-if="paragraph.proposalId"
        class="send"
        :disabled="paragraph.sent || readOnly"
        @click="$emit('send', paragraph.proposalId)"
      >
        {{ paragraph.sent ? 'Enviado para Aguardando seu ok' : 'Enviar para Aguardando seu ok' }}
      </button>
    </template>
  </div>
  <span class="footer">Escrito pela IA a partir dos dados deste mês.</span>
</template>

<style scoped>
.section {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  max-width: 680px;
}

.heading {
  font-size: var(--font-size-secondary);
  font-weight: var(--font-weight-semibold);
}

.text {
  line-height: 1.55;
  color: var(--color-text-pill);
  text-wrap: pretty;
}

.send {
  align-self: flex-start;
  height: 32px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
}

.send:disabled {
  opacity: 0.55; /* token-exempt: the dimming of a disabled control */
  cursor: default;
}

.footer {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
}
</style>
