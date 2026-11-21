import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useApi } from '@/api/context';
import type { ShellOverview } from '@/api/types';
import { useMonthStore } from './month';

/** What the frame around every screen shows: pending count, sync state, today. */
export const useShellStore = defineStore('shell', () => {
  const api = useApi();
  const month = useMonthStore();

  const overview = ref<ShellOverview | null>(null);

  async function load(): Promise<void> {
    const loaded = await api.shell.overview();
    month.configure(loaded);
    overview.value = loaded;
  }

  return { overview, load };
});
