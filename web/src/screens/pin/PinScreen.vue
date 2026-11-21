<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue';
import { useSessionStore } from '@/stores/session';

const ERASE = '⌫';
const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '', '0', ERASE];

const session = useSessionStore();

function press(key: string): void {
  if (key === ERASE) session.eraseDigit();
  else session.pressDigit(key);
}

function onKeydown(event: KeyboardEvent): void {
  if (/^\d$/.test(event.key)) session.pressDigit(event.key);
  else if (event.key === 'Backspace') session.eraseDigit();
}

onMounted(() => window.addEventListener('keydown', onKeydown));
onUnmounted(() => window.removeEventListener('keydown', onKeydown));
</script>

<template>
  <div class="pin-screen" data-screen-label="PIN">
    <div class="logo">f</div>
    <!-- Keyed by the shake count so each wrong PIN restarts the animation. -->
    <div :key="session.shakeCount" class="dots" :class="{ shaking: session.shakeCount > 0 }">
      <span v-for="position in 6" :key="position" class="dot" :class="{ filled: position <= session.pin.length }" />
    </div>
    <div class="message" role="alert">{{ session.message }}</div>
    <div class="keypad" :class="{ 'on-hold': session.keypadOnHold }">
      <button
        v-for="key in KEYS"
        :key="key"
        class="key"
        :class="{ blank: key === '' }"
        :disabled="session.keypadOnHold"
        :aria-label="key === ERASE ? 'Apagar' : key"
        @click="press(key)"
      >
        {{ key }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.pin-screen {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 26px;
  padding: var(--space-24);
  box-sizing: border-box;
}

.logo {
  width: 40px;
  height: 40px;
  border-radius: var(--radius-card);
  background: var(--color-accent-logo);
  color: var(--color-text-on-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: var(--font-weight-semibold);
  font-size: 21px; /* token-exempt: glyph size of the large logo */
}

.dots {
  display: flex;
  gap: var(--space-14);
}

.dots.shaking {
  animation: pin-shake var(--duration-shake);
}

.dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  box-sizing: border-box;
  border: 1.5px solid var(--color-border-dot);
  background: transparent;
}

.dot.filled {
  border-color: var(--color-text-ink);
  background: var(--color-text-ink);
}

.message {
  min-height: 40px;
  max-width: 260px;
  font-size: var(--font-size-body);
  color: var(--color-bad-text);
  text-align: center;
  text-wrap: pretty;
}

.keypad {
  display: grid;
  grid-template-columns: repeat(3, 72px);
  gap: var(--space-14);
}

.keypad.on-hold {
  opacity: 0.35;
}

.key {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  border: 1px solid var(--color-border-input);
  background: var(--color-surface-card);
  font-size: var(--font-size-title);
  color: var(--color-text-ink);
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}

.key:hover {
  background: var(--color-surface-hover-item);
}

.key:active {
  background: var(--color-surface-pressed);
}

.key.blank {
  visibility: hidden;
}

@keyframes pin-shake {
  0%,
  100% {
    transform: translateX(0);
  }
  20% {
    transform: translateX(-9px);
  }
  40% {
    transform: translateX(9px);
  }
  60% {
    transform: translateX(-6px);
  }
  80% {
    transform: translateX(6px);
  }
}
</style>
