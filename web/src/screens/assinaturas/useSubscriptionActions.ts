import { useApi } from '@/api/context';
import type { SubscriptionCandidate, SubscriptionStatus } from '@/api/types';
import { brl } from '@/lib/format';
import { useOverlayStore, type ToastAction } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { STATUSES } from './subscriptionText';

/** The writes of the subscription screens with their menus and the toast that takes each back. */
export function useSubscriptionActions() {
  const api = useApi();
  const overlay = useOverlayStore();
  const revision = useRevisionStore();

  async function undo(changeId: string): Promise<void> {
    await api.changes.undo(changeId);
    revision.bump();
    overlay.dismissToast();
  }

  /** Announces a write that was made, with the way back. */
  function announce(message: string, changeId: string): void {
    revision.bump();
    const undoAction: ToastAction = { label: 'Desfazer', run: () => void undo(changeId) };
    overlay.showToast(message, [undoAction]);
  }

  async function changeStatus(subscription: { id: string; name: string }, status: SubscriptionStatus): Promise<void> {
    const { changeId } = await api.subscriptions.setStatus(subscription.id, status);
    announce(`${subscription.name}: status alterado para ${status}.`, changeId);
  }

  function openStatusMenu(anchor: HTMLElement, subscription: { id: string; name: string; status: SubscriptionStatus }): void {
    overlay.openMenu(anchor, {
      title: 'Status',
      items: STATUSES.map((status) => ({
        label: status,
        current: status === subscription.status,
        onPick: () => void changeStatus(subscription, status),
      })),
    });
  }

  async function createFrom(candidate: SubscriptionCandidate): Promise<void> {
    const created = await api.subscriptions.create(candidate.transactionId);
    if (created) announce(`${created.name} entrou nas assinaturas.`, created.changeId);
  }

  async function openCandidateMenu(anchor: HTMLElement): Promise<void> {
    const candidates = await api.subscriptions.candidates();
    overlay.openMenu(anchor, {
      title: 'Criar a partir de uma transação',
      search: true,
      placeholder: 'Buscar transação',
      items: candidates.map((candidate) => ({
        label: candidate.name,
        hint: brl(candidate.amount),
        onPick: () => void createFrom(candidate),
      })),
    });
  }

  async function acceptSuggestion(): Promise<void> {
    const created = await api.subscriptions.acceptSuggestion();
    if (created) announce(`${created.name} entrou nas assinaturas.`, created.changeId);
  }

  async function rejectSuggestion(name: string): Promise<void> {
    const result = await api.subscriptions.rejectSuggestion();
    if (result) announce(`${name} não será tratado como assinatura.`, result.changeId);
  }

  return { openStatusMenu, openCandidateMenu, acceptSuggestion, rejectSuggestion };
}
