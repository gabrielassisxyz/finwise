import { useApi } from '@/api/context';
import type { ChangeResult, Transaction } from '@/api/types';
import { useOverlayStore, type ToastAction } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
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

  async function undo(changeId: string): Promise<void> {
    await api.changes.undo(changeId);
    revision.bump();
    overlay.dismissToast();
  }

  const undoAction = (changeId: string): ToastAction => ({ label: 'Desfazer', run: () => void undo(changeId) });

  function openLineMenu(anchor: HTMLElement, current: string | null, pick: (line: string) => void): void {
    overlay.openMenu(anchor, {
      search: true,
      placeholder: 'Buscar linha',
      items: list.options.value.lines.map((line) => ({
        label: line.name,
        hint: line.group,
        current: line.name === current,
        onPick: () => pick(line.name),
      })),
    });
  }

  async function applyToEstablishment(changeId: string, line: string, name: string): Promise<void> {
    const { changed } = await api.transactions.applyLineToEstablishment(changeId);
    revision.bump();
    const count = `${changed + 1} ${changed === 0 ? 'alterada' : 'alteradas'}`;
    overlay.showToast(`${line} agora vale para todas as compras em ${name}: ${count}.`, [undoAction(changeId)]);
  }

  async function changeLine(id: string, line: string): Promise<void> {
    const result = await api.transactions.setLine(id, line);
    if (!result) return;
    revision.bump();
    if (!result.establishment) {
      overlay.showToast(`Linha alterada para ${line}.`, [undoAction(result.changeId)]);
      return;
    }
    const { name } = result.establishment;
    overlay.showToast(`Linha alterada para ${line}. Aplicar a todas as compras em ${name}?`, [
      { label: 'Só esta', run: () => overlay.dismissToast() },
      { label: 'Todas', run: () => void applyToEstablishment(result.changeId, line, name) },
      undoAction(result.changeId),
    ]);
  }

  function pickLine(anchor: HTMLElement, transaction: Transaction): void {
    openLineMenu(anchor, transaction.line, (line) => void changeLine(transaction.id, line));
  }

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
