<script setup lang="ts">
defineProps<{ name: string }>();
defineEmits<{ accept: []; reject: [] }>();
</script>

<template>
  <div class="strip">
    <span>{{ name }} parece assinatura. Confirmar?</span>
    <span class="buttons">
      <button class="yes" @click="$emit('accept')">Sim</button>
      <button class="no" @click="$emit('reject')">Não é assinatura</button>
    </span>
  </div>
</template>

<style scoped>
.strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-10);
  flex-wrap: wrap;
  padding: var(--space-10) var(--space-12) var(--space-10) var(--space-16);
  border-radius: var(--radius-card);
  border: 1px solid var(--color-accent-badge-border);
  background: var(--color-accent-card);
  font-size: var(--font-size-body);
}

.buttons {
  display: flex;
  gap: var(--space-8);
}

button {
  height: 34px;
  border-radius: var(--radius-nav);
  font-size: var(--font-size-secondary);
  cursor: pointer;
}

.yes {
  padding: 0 var(--space-14);
  border: 0;
  background: var(--color-text-ink);
  color: var(--color-text-on-accent);
  font-weight: var(--font-weight-medium);
}

.no {
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-strong);
  background: var(--color-surface-card);
  color: var(--color-text-ink);
}
</style>
