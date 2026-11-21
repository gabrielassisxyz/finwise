import { fileURLToPath, URL } from 'node:url';
import vue from '@vitejs/plugin-vue';
import { defineConfig } from 'vitest/config';

const BACKEND_ADDRESS = 'http://127.0.0.1:8080';

// Real use talks to the backend: the stand-in is the default only for the dev
// server and for `vite build --mode prototype`, which the parity check builds.
function providesStandIn(command: string, mode: string): boolean {
  if (command === 'build') return mode === 'prototype';
  return mode !== 'backend';
}

export default defineConfig(({ command, mode }) => ({
  plugins: [vue()],
  define: {
    __FINWISE_STAND_IN__: JSON.stringify(providesStandIn(command, mode)),
  },
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    // The contract lives outside web/; the client reads its manifest from there.
    fs: { allow: ['..'] },
    proxy: mode === 'backend' ? { '/api': BACKEND_ADDRESS } : undefined,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts', 'scripts/**/*.test.mjs'],
  },
}));
