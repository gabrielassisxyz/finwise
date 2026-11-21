<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { NAV_ITEMS, PHONE_BAR_COUNT } from '@/navigation';

defineProps<{ pendingCount: number }>();

const route = useRoute();

const barItems = NAV_ITEMS.slice(0, PHONE_BAR_COUNT);
const moreItems = NAV_ITEMS.slice(PHONE_BAR_COUNT);

const moreOpen = ref(false);
const moreActive = computed(() => moreItems.some((item) => item.id === route.meta.nav));

watch(
  () => route.fullPath,
  () => {
    moreOpen.value = false;
  },
);
</script>

<template>
  <nav class="bar">
    <RouterLink
      v-for="item in barItems"
      :key="item.id"
      :to="item.to"
      class="tab"
      :class="{ active: route.meta.nav === item.id }"
    >
      <span class="count-slot">
        <span v-if="item.id === 'revisao' && pendingCount > 0" class="count">{{ pendingCount }}</span>
      </span>
      <span class="label">{{ item.label }}</span>
    </RouterLink>
    <button class="tab" :class="{ active: moreActive }" :aria-expanded="moreOpen" @click="moreOpen = !moreOpen">
      <span class="count-slot" />
      <span class="label">Mais</span>
    </button>
  </nav>
  <template v-if="moreOpen">
    <div class="backdrop" @click="moreOpen = false" />
    <div class="sheet">
      <RouterLink
        v-for="item in moreItems"
        :key="item.id"
        :to="item.to"
        class="sheet-item"
        :class="{ active: route.meta.nav === item.id }"
      >
        {{ item.label }}
      </RouterLink>
    </div>
  </template>
</template>

<style scoped>
.bar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 15;
  display: flex;
  background: var(--color-surface-nav);
  border-top: 1px solid var(--color-border-default);
}

.tab {
  flex: 1;
  min-width: 0;
  min-height: 58px;
  border: 0;
  background: none;
  border-top: 2px solid transparent;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  font-size: var(--font-size-badge);
  font-weight: var(--font-weight-regular);
  cursor: pointer;
  color: var(--color-text-ink-2);
  padding: 0 var(--space-2);
  box-sizing: border-box;
  text-decoration: none;
}

.tab:hover {
  color: var(--color-text-ink-2);
  text-decoration: none;
}

.tab.active,
.tab.active:hover {
  border-top-color: var(--color-accent-base);
  color: var(--color-accent-positive);
  font-weight: var(--font-weight-semibold);
}

.count-slot {
  height: 16px;
  display: flex;
  align-items: center;
}

.count {
  font-size: var(--font-size-micro);
  font-weight: var(--font-weight-semibold);
  font-family: var(--font-family-mono);
  color: var(--color-text-on-accent);
  background: var(--color-accent-base);
  padding: 0 var(--space-6);
  border-radius: var(--radius-card);
  line-height: 16px;
}

.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.backdrop {
  position: fixed;
  inset: 0;
  z-index: 14;
  background: var(--color-overlay-sheet);
}

.sheet {
  position: fixed;
  left: 10px;
  right: 10px;
  bottom: 68px;
  z-index: 15;
  background: var(--color-surface-card);
  border: 1px solid var(--color-border-button);
  border-radius: var(--radius-dialog);
  padding: var(--space-6);
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-popover);
}

.sheet-item {
  min-height: 48px;
  display: flex;
  align-items: center;
  padding: 0 var(--space-14);
  font-size: var(--font-size-section);
  font-weight: var(--font-weight-regular);
  border-radius: var(--radius-control);
  color: var(--color-text-ink-2);
  text-decoration: none;
}

.sheet-item:hover {
  color: var(--color-text-ink-2);
  text-decoration: none;
}

.sheet-item.active,
.sheet-item.active:hover {
  color: var(--color-text-ink);
  font-weight: var(--font-weight-medium);
}
</style>
