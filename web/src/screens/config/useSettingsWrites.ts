import { useApi } from '@/api/context';
import type { BudgetLineOption, ChangeResult, SettingsOverview } from '@/api/types';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';

/**
 * The writes of the settings screen. Each one makes every screen fetch again,
 * since a threshold, a bucket or a nickname changes what others show, and
 * offers the way back only where the design shows a toast.
 */
export function useSettingsWrites() {
  const api = useApi();
  const overlay = useOverlayStore();
  const revision = useRevisionStore();

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

  /** What the backend holds now, for a field whose typed value it may have bounded. */
  async function stored(): Promise<SettingsOverview> {
    return api.settings.overview();
  }

  /** The menu of budget lines, the same one the transaction list offers. */
  async function openLineMenu(anchor: HTMLElement, pick: (line: string) => void): Promise<void> {
    const { lines }: { lines: BudgetLineOption[] } = await api.transactions.filterOptions();
    overlay.openMenu(anchor, {
      search: true,
      placeholder: 'Buscar linha',
      items: lines.map((line) => ({ label: line.name, hint: line.group, onPick: () => pick(line.name) })),
    });
  }

  return { api, overlay, write, stored, openLineMenu };
}
