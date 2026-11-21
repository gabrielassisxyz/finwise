import { computed } from 'vue';
import { useApi } from '@/api/context';
import type { ChangeResult } from '@/api/types';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useLineChange } from './useLineChange';
import type { TransactionListState } from './useTransactionList';

const BULK_EXCLUDE_REASON = 'Marcada em lote';

/**
 * What a user can do to the transactions of a list: change a line, alone or
 * for the whole establishment, and act on the selection. Each write answers
 * with a toast that takes it back.
 */
export function useTransactionActions(list: TransactionListState, selected: Set<string>) {
  const api = useApi();
  const overlay = useOverlayStore();
  const revision = useRevisionStore();

  const { undoAction, openLineMenu, pickLine } = useLineChange(computed(() => list.options.value.lines));

  /** One write over the selection: one change, one toast, and the selection is spent. */
  async function actOnSelection(write: (ids: string[]) => Promise<ChangeResult>, message: (count: number) => string) {
    const ids = [...selected];
    const { changeId } = await write(ids);
    selected.clear();
    revision.bump();
    overlay.showToast(message(ids.length), [undoAction(changeId)]);
  }

  function pickLineForSelection(anchor: HTMLElement): void {
    openLineMenu(anchor, null, (line) => {
      void actOnSelection(
        (ids) => api.transactions.setLineForMany(ids, line),
        (count) => `${count} transações agora estão em ${line}.`,
      );
    });
  }

  function pickTagForSelection(anchor: HTMLElement): void {
    overlay.openMenu(anchor, {
      title: 'Adicionar tag',
      items: list.options.value.tags.map((tag) => ({
        label: tag,
        onPick: () => {
          void actOnSelection(
            (ids) => api.transactions.addTagToMany(ids, tag),
            (count) => `Tag ${tag} adicionada a ${count} transações.`,
          );
        },
      })),
    });
  }

  function excludeSelection(): void {
    void actOnSelection(
      (ids) => api.transactions.excludeMany(ids, BULK_EXCLUDE_REASON),
      (count) => `${count} transações marcadas como não conta.`,
    );
  }

  return { pickLine, pickLineForSelection, pickTagForSelection, excludeSelection };
}
