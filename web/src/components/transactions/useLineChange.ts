import type { Ref } from 'vue';
import { useApi } from '@/api/context';
import type { BudgetLineOption } from '@/api/types';
import { useOverlayStore, type ToastAction } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';

/**
 * Changing a budget line, shared by the list and the transaction panel: the
 * menu of lines, the write, and the toast that offers "all purchases here"
 * and takes the change back.
 */
export function useLineChange(lines: Ref<BudgetLineOption[]>) {
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
      items: lines.value.map((line) => ({
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

  function pickLine(anchor: HTMLElement, transaction: { id: string; line: string | null }): void {
    openLineMenu(anchor, transaction.line, (line) => void changeLine(transaction.id, line));
  }

  return { undoAction, openLineMenu, pickLine };
}
