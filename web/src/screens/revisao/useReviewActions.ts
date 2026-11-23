import { useApi } from '@/api/context';
import { useOverlayStore, type ToastAction } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';

/** What every write of the review screens ends with: the screens refetch, and a toast offers the way back. */
export function useReviewActions() {
  const api = useApi();
  const overlay = useOverlayStore();
  const revision = useRevisionStore();

  async function undo(changeId: string): Promise<void> {
    await api.changes.undo(changeId);
    revision.bump();
    overlay.dismissToast();
  }

  function announce(message: string, changeId: string): void {
    revision.bump();
    const undoAction: ToastAction = { label: 'Desfazer', run: () => void undo(changeId) };
    overlay.showToast(message, [undoAction]);
  }

  function notify(message: string): void {
    revision.bump();
    overlay.showToast(message);
  }

  function refresh(): void {
    revision.bump();
  }

  return { announce, notify, refresh };
}
