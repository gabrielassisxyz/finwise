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

  let latest = 0;

  async function load(): Promise<void> {
    const request = ++latest;
    const loaded = await api.shell.overview();
    if (request !== latest) return;
    month.configure(loaded);
    overview.value = loaded;
  }

  // The pending count and the closed months move with a write; the rest of the overview does not, so the month stays where it is.
  watch(
    () => revision.current,
    () => {
      if (!overview.value) return;
      void load();
    },
  );

  return { overview, load };
});
