<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue';
import { normalize } from '@/lib/text';
import { useOverlayStore, type MenuItem } from '@/stores/overlay';

const overlay = useOverlayStore();

const query = ref('');
const searchInput = useTemplateRef<HTMLInputElement>('searchInput');

const visibleItems = computed(() => {
  const wanted = normalize(query.value);
  const items = overlay.menu?.items ?? [];
  return wanted ? items.filter((item) => normalize(item.label).includes(wanted)) : items;
});

function pick(item: MenuItem): void {
  overlay.closeMenu();
  item.onPick?.();
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') overlay.closeMenu();
}

watch(
  () => overlay.menu,
  async (menu) => {
    query.value = '';
    if (!menu?.search) return;
    await nextTick();
    searchInput.value?.focus();
  },
);

onMounted(() => window.addEventListener('keydown', onKeydown));
onUnmounted(() => window.removeEventListener('keydown', onKeydown));
</script>

<template>
  <template v-if="overlay.menu">
    <div class="backdrop" @click="overlay.closeMenu()" />
    <div class="menu" role="menu" :style="{ left: `${overlay.menu.x}px`, top: `${overlay.menu.y}px` }">
      <span v-if="overlay.menu.title" class="title">{{ overlay.menu.title }}</span>
      <input
        v-if="overlay.menu.search"
        ref="searchInput"
        v-model="query"
        class="search"
        :placeholder="overlay.menu.placeholder ?? 'Buscar'"
      />
      <button
        v-for="item in visibleItems"
        :key="item.label"
        class="item"
        :class="{ current: item.current }"
        role="menuitem"
        @click="pick(item)"
      >
        <span>{{ item.label }}</span>
        <span class="hint">{{ item.hint }}</span>
      </button>
      <span v-if="visibleItems.length === 0" class="empty">Nada encontrado</span>
    </div>
  </template>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 60;
}

.menu {
  position: fixed;
  z-index: 61;
  width: 264px;
  max-height: 330px;
  overflow-y: auto;
  box-sizing: border-box;
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-card);
  box-shadow: var(--shadow-popover);
  padding: var(--space-6);
  display: flex;
  flex-direction: column;
}

.title {
  font-size: var(--font-size-caption);
  font-weight: var(--font-weight-medium);
  color: var(--color-text-muted);
  padding: var(--space-6) var(--space-10);
}

.search {
  height: 34px;
  margin: var(--space-2) var(--space-2) var(--space-6);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-nav);
  padding: 0 var(--space-10);
  font-size: var(--font-size-secondary);
  outline: none;
  flex-shrink: 0;
}

.item {
  border: 0;
  background: none;
  text-align: left;
  padding: var(--space-8) var(--space-10);
  border-radius: var(--radius-item);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-regular);
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-10);
  color: var(--color-text-ink);
  flex-shrink: 0;
}

.item.current {
  font-weight: var(--font-weight-semibold);
}

.item:hover {
  background: var(--color-surface-hover-item);
}

.hint {
  font-size: var(--font-size-caption);
  color: var(--color-text-muted);
  font-weight: var(--font-weight-regular);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.empty {
  font-size: var(--font-size-secondary);
  color: var(--color-text-muted);
  padding: var(--space-8) var(--space-10);
}
</style>
