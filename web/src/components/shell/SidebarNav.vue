<script setup lang="ts">
import { useRoute } from 'vue-router';
import { NAV_ITEMS } from '@/navigation';

defineProps<{ pendingCount: number }>();

const route = useRoute();
</script>

<template>
  <aside class="sidebar">
    <div class="brand">
      <div class="logo">f</div>
      <div class="name">finwise</div>
    </div>
    <nav class="items">
      <RouterLink
        v-for="item in NAV_ITEMS"
        :key="item.id"
        :to="item.to"
        class="item"
        :class="{ active: route.meta.nav === item.id }"
      >
        <span>{{ item.label }}</span>
        <span v-if="item.id === 'revisao' && pendingCount > 0" class="count">{{ pendingCount }}</span>
      </RouterLink>
    </nav>
  </aside>
</template>

<style scoped>
.sidebar {
  width: clamp(168px, 16vw, 224px);
  flex-shrink: 0;
  border-right: 1px solid var(--color-border-default);
  background: var(--color-surface-nav);
  display: flex;
  flex-direction: column;
  padding: var(--space-20) var(--space-14);
  position: sticky;
  top: 0;
  height: 100vh;
  box-sizing: border-box;
}

.brand {
  display: flex;
  align-items: center;
  gap: var(--space-10);
  padding: var(--space-2) var(--space-8) 22px;
}

.logo {
  width: 26px;
  height: 26px;
  border-radius: var(--radius-nav);
  background: var(--color-accent-logo);
  color: var(--color-text-on-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: var(--font-weight-semibold);
  font-size: var(--font-size-body);
}

.name {
  font-weight: var(--font-weight-semibold);
  font-size: var(--font-size-section);
  letter-spacing: calc(var(--font-tracking-snug) * 1em);
}

.items {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-8);
  padding: var(--space-8) var(--space-10);
  border-radius: var(--radius-nav);
  font-size: var(--font-size-body);
  font-weight: var(--font-weight-regular);
  color: var(--color-text-ink-2);
  text-decoration: none;
}

.item.active {
  color: var(--color-text-ink);
  background: var(--color-accent-tint);
  font-weight: var(--font-weight-medium);
}

.item:hover {
  background: var(--color-surface-hover-nav);
  color: var(--color-text-ink);
  text-decoration: none;
}

.count {
  font-size: var(--font-size-badge);
  font-weight: var(--font-weight-semibold);
  font-family: var(--font-family-mono);
  color: var(--color-text-on-accent);
  background: var(--color-accent-base);
  padding: 1px 7px;
  border-radius: var(--radius-card);
}
</style>
