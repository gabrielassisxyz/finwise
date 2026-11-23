import { ref, watch, type Ref } from 'vue';
import { useApi } from '@/api/context';
import type { TransactionDetail } from '@/api/types';
import { createLatestLoad } from '@/lib/latestLoad';
import { useRevisionStore } from '@/stores/revision';

/**
 * The detail of the open transaction. A write bumps the revision and the
 * detail is fetched again in place, so the panel never blanks between edits;
 * only opening another transaction does.
 */
export function useTransactionDetail(id: Ref<string | null>) {
  const api = useApi();
  const revision = useRevisionStore();
  const detail = ref<TransactionDetail | null>(null);

  const loads = createLatestLoad();

  async function load(): Promise<void> {
    const wanted = id.value;
    if (wanted === null) {
      loads.cancel();
      return;
    }
    const isLatest = loads.start();
    const loaded = await api.transactions.detail(wanted);
    // A slower answer for a transaction or a revision the panel already left must not land.
    if (isLatest() && wanted === id.value) detail.value = loaded;
  }

  watch(id, (next) => {
    loads.cancel();
    detail.value = null;
    if (next !== null) void load();
  }, { immediate: true });
  watch(() => revision.current, () => void load());

  return { detail };
}
