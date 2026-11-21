<script setup lang="ts">
defineProps<{ count: number }>();

const emit = defineEmits<{
  'pick-line': [anchor: HTMLElement];
  'pick-tag': [anchor: HTMLElement];
  exclude: [];
  clear: [];
}>();

const anchorOf = (event: MouseEvent) => event.currentTarget as HTMLElement;
</script>

<template>
  <!-- The list lives deep in the page; the bar belongs in the floating stack, above the toast. -->
  <Teleport defer to="#float-above-toast">
    <div class="bar">
      <span class="count">{{ count }} {{ count === 1 ? 'selecionada' : 'selecionadas' }}</span>
      <button class="action" @click="emit('pick-line', anchorOf($event))">Alterar linha</button>
      <button class="action" @click="emit('pick-tag', anchorOf($event))">Adicionar tag</button>
      <button class="action" @click="emit('exclude')">Marcar como não conta</button>
      <button class="clear" aria-label="Limpar seleção" @click="emit('clear')">×</button>
    </div>
  </Teleport>
</template>

<style scoped>
.bar {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: var(--space-6);
  flex-wrap: wrap;
  justify-content: center;
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-card);
  padding: var(--space-6) var(--space-6) var(--space-6) var(--space-14);
  box-shadow: var(--shadow-popover);
  font-size: var(--font-size-secondary);
}

.count {
  font-weight: var(--font-weight-medium);
  margin-right: var(--space-6);
}

.action {
  height: 34px;
  padding: 0 var(--space-12);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  background: var(--color-surface-card);
  font-size: var(--font-size-secondary);
  cursor: pointer;
  color: var(--color-text-ink);
}

.clear {
  width: 34px;
  height: 34px;
  border: 0;
  background: none;
  font-size: 18px; /* token-exempt: glyph size of the close sign */
  color: var(--color-text-muted);
  cursor: pointer;
}
</style>
