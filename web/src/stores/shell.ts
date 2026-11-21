import { defineStore } from 'pinia';
import { ref, watch } from 'vue';
import { useApi } from '@/api/context';
import type { ShellOverview } from '@/api/types';
import { useMonthStore } from './month';
import { useRevisionStore } from './revision';

/** What the frame around every screen shows: pending count, sync state, today. */
export const useShellStore = defineStore('shell', () => {
  const api = useApi();
  const month = useMonthStore();
  const revision = useRevisionStore();

  const overview = ref<ShellOverview | null>(null);

  async function load(): Promise<void> {
    const loaded = await api.shell.overview();
    month.configure(loaded);
    overview.value = loaded;
  }

  // The pending count and the closed months move with a write; the rest of the overview does not, so the month stays where it is.
  watch(
    () => revision.current,
    async () => {
      if (!overview.value) return;
      const loaded = await api.shell.overview();
      month.configure(loaded);
      overview.value = loaded;
    },
  );

  return { overview, load };
});
