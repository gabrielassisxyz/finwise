import type { Pinia } from 'pinia';
import type { App } from 'vue';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useSessionStore } from '@/stores/session';
import { useShellStore } from '@/stores/shell';
import type { RefusalReason } from './contract/errors';
import { isApiFailure, type ApiFailure } from './http/failure';

export const READ_ONLY_MESSAGE = 'Somente leitura por enquanto. Nada foi alterado.';
export const LEDGER_UNAVAILABLE_MESSAGE = 'Dados do banco indisponíveis no momento.';
export const LOAD_FAILED_MESSAGE = 'Não foi possível carregar os dados.';
export const RETRY_LABEL = 'Tentar de novo';

/** What the person reads when the server refused a call and said why. */
export const REFUSAL_SENTENCES: Record<RefusalReason, string> = {
  month_closed: 'Esse mês já está fechado.',
  future_month: 'Esse mês ainda não chegou.',
  pending_reviews: 'Resolva as pendências do mês antes de fechar.',
  stale_change: 'Isso já foi alterado depois e não pode mais ser desfeito.',
  not_found: 'Esse item não existe mais.',
  unknown_line: 'Essa linha não existe no budget desse mês.',
  invalid_budget: 'O budget tem linha sem nome ou repetida.',
  unstable_pot: 'O banco não identifica esse cofrinho de forma estável.',
  split_with_reimbursement: 'Uma compra dividida não pode ter reembolso ligado.',
};
/** A refusal whose reason is newer than this interface, or missing. */
export const UNKNOWN_REFUSAL_MESSAGE = 'Não foi possível concluir.';
/** A write the server answered and did not carry out: it rolled back whole, or never ran. */
export const SAVE_FAILED_MESSAGE = 'Não foi possível salvar.';
/** A write whose answer never arrived may have been stored after all, so it is never called lost. */
export const SAVE_UNCONFIRMED_MESSAGE = 'Não deu para confirmar se foi salvo. Atualizando a tela.';

/**
 * What a failed call becomes on screen. Call sites do not catch: every
 * rejection of the API lands here, from Vue's handler or from the window, so a
 * refused write and a failed read look the same wherever they were made.
 * Answers whether the error was the API's; anything else is a bug and is left
 * for the caller to report.
 */
export function createFailureHandler(pinia: Pinia): (error: unknown) => boolean {
  function tryAgain(): void {
    useOverlayStore(pinia).dismissToast();
    resumeStartup(pinia);
    useRevisionStore(pinia).bump();
  }

  function offerRetry(message: string): void {
    useOverlayStore(pinia).showToast(message, [{ label: RETRY_LABEL, run: tryAgain }]);
  }

  // Nothing to retry: a write is never sent again, and the screens fetch again at once so none keeps showing what
  // the server does not hold.
  function showWhatIsStored(message: string): void {
    useOverlayStore(pinia).showToast(message);
    useRevisionStore(pinia).bump();
  }

  function showFailedCall(error: ApiFailure): void {
    if (error.methodClass !== 'write') offerRetry(LOAD_FAILED_MESSAGE);
    else showWhatIsStored(error.kind === 'network' ? SAVE_UNCONFIRMED_MESSAGE : SAVE_FAILED_MESSAGE);
  }

  return (error) => {
    if (!isApiFailure(error)) return false;
    switch (error.kind) {
      case 'read_only':
        showWhatIsStored(READ_ONLY_MESSAGE);
        break;
      case 'refused':
        showWhatIsStored(error.reason ? REFUSAL_SENTENCES[error.reason] : UNKNOWN_REFUSAL_MESSAGE);
        break;
      case 'locked':
        useSessionStore(pinia).markLocked();
        break;
      case 'ledger_unavailable':
        offerRetry(LEDGER_UNAVAILABLE_MESSAGE);
        break;
      default:
        showFailedCall(error);
    }
    return true;
  };
}

/** The first reads of the app are not watched by the revision counter, so a bump alone would leave a blank page. */
function resumeStartup(pinia: Pinia): void {
  const session = useSessionStore(pinia);
  if (!session.ready) {
    void session.load();
    return;
  }
  const shell = useShellStore(pinia);
  if (!session.locked && !shell.overview) void shell.load();
}

/** Both doors a rejection can leave through: a handler Vue awaited, and a promise nobody did. */
export function failureListeners(pinia: Pinia) {
  const handle = createFailureHandler(pinia);
  return {
    onVueError(error: unknown): void {
      if (!handle(error)) console.error(error);
    },
    onUnhandledRejection(event: PromiseRejectionEvent): void {
      if (handle(event.reason)) event.preventDefault();
    },
  };
}

export function installFailureHandling(app: App, pinia: Pinia, target: Window = window): () => void {
  const { onVueError, onUnhandledRejection } = failureListeners(pinia);
  app.config.errorHandler = onVueError;
  target.addEventListener('unhandledrejection', onUnhandledRejection);
  return () => target.removeEventListener('unhandledrejection', onUnhandledRejection);
}
