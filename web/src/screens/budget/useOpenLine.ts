import { ref, watch, type Ref } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetEstablishment, YearMonth } from '@/api/types';
import { useRevisionStore } from '@/stores/revision';

/** The line opened into its establishments, and what they are. */
export function useOpenLine(month: Ref<YearMonth>) {
  const api = useApi();
  const revision = useRevisionStore();

  const key = ref<string | null>(null);
  const establishments = ref<BudgetEstablishment[]>([]);

  function toggle(lineKey: string): void {
    key.value = key.value === lineKey ? null : lineKey;
  }

  watch(
    () => [key.value, month.value, revision.current] as const,
    async ([opened, selected]) => {
      if (!opened) return;
      const line = opened.slice(opened.indexOf(':') + 1);
      const loaded = await api.budget.establishments(selected, line);
      if (opened === key.value && selected === month.value) establishments.value = loaded;
    },
    { immediate: true },
  );

  return { key, establishments, toggle, close: () => (key.value = null) };
}
