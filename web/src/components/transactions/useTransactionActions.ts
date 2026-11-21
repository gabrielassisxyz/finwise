import { useApi } from '@/api/context';
import type { Transaction } from '@/api/types';
import { useOverlayStore, type ToastAction } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import type { TransactionListState } from './useTransactionList';

/**
 * What a user can do to the transactions of a list: change a line, alone or
 * for the whole establishment. Each write answers with a toast that takes it
 * back.
 */
export function useTransactionActions(list: TransactionListState) {
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

  return { pickLine };
}
