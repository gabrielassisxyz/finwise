import type { Pinia } from 'pinia';
import type { App } from 'vue';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useSessionStore } from '@/stores/session';
import { useShellStore } from '@/stores/shell';
import { isApiFailure } from './http/failure';

export const READ_ONLY_MESSAGE = 'Somente leitura por enquanto. Nada foi alterado.';
export const LEDGER_UNAVAILABLE_MESSAGE = 'Dados do banco indisponíveis no momento.';
export const LOAD_FAILED_MESSAGE = 'Não foi possível carregar os dados.';
export const RETRY_LABEL = 'Tentar de novo';

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

  return (error) => {
    if (!isApiFailure(error)) return false;
    switch (error.kind) {
      case 'read_only':
        // The screens fetch again so none keeps showing an optimistic state the server refused.
        useOverlayStore(pinia).showToast(READ_ONLY_MESSAGE);
        useRevisionStore(pinia).bump();
        break;
      case 'locked':
        useSessionStore(pinia).markLocked();
        break;
      case 'ledger_unavailable':
        offerRetry(LEDGER_UNAVAILABLE_MESSAGE);
        break;
      default:
        offerRetry(LOAD_FAILED_MESSAGE);
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
