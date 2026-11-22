import { useApi } from '@/api/context';
import type { BudgetLineOption, ChangeResult } from '@/api/types';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useShellStore } from '@/stores/shell';

/**
 * The writes of the settings screen. Each one makes every screen fetch again,
 * since a threshold, a bucket or a nickname changes what others show, and
 * offers the way back only where the design shows a toast.
 */
export function useSettingsWrites() {
  const api = useApi();
  const overlay = useOverlayStore();
  const revision = useRevisionStore();
  const shell = useShellStore();

  async function undo(changeId: string): Promise<void> {
    await api.changes.undo(changeId);
    revision.bump();
    overlay.dismissToast();
  }

  async function write(run: () => Promise<ChangeResult>, toast?: string): Promise<void> {
    const { changeId } = await run();
    revision.bump();
    if (toast) overlay.showToast(toast, [{ label: 'Desfazer', run: () => void undo(changeId) }]);
  }

  /**
   * The menu of budget lines of the current month: a default line and a pot's link belong to no month, so they are
   * resolved in the current one, never in the month the screen shows.
   */
  async function openLineMenu(anchor: HTMLElement, pick: (line: string) => void): Promise<void> {
    const current = shell.overview?.today.slice(0, 7);
    if (!current) return;
    const { lines }: { lines: BudgetLineOption[] } = await api.transactions.filterOptions(current);
    overlay.openMenu(anchor, {
      search: true,
      placeholder: 'Buscar linha',
      items: lines.map((line) => ({ label: line.name, hint: line.group, onPick: () => pick(line.name) })),
    });
  }

  return { api, overlay, write, openLineMenu };
}
