import { prototypeState } from '@/api/fake/prototype-data';
import type { FakeState } from '@/api/fake/model';
import type { PendingReview } from '@/api/types';

/** The prototype's first review of the queue: a card purchase on 27 November that has a suggestion. */
export const FIRST_PENDING_ID = 't5';

/** A review of an establishment the app has never seen: it knows why it asks and has nothing to propose. */
export const NO_SUGGESTION: PendingReview = {
  reason: 'Estabelecimento novo',
  suggestion: null,
  confidence: null,
  justification: null,
};

/** The prototype's dataset with one transaction's pending review replaced by `pending`. */
export function stateWithPending(pending: PendingReview, id: string = FIRST_PENDING_ID): FakeState {
  const state = prototypeState();
  const transaction = state.transactions.find((candidate) => candidate.id === id);
  if (!transaction?.pending) throw new Error(`${id} is not a pending review of the prototype's dataset`);
  transaction.pending = pending;
  return state;
}

/** The prototype's dataset where the first review has nothing to suggest. */
export const stateWithoutSuggestion = (): FakeState => stateWithPending(NO_SUGGESTION);

/** The prototype's dataset where the first review keeps its suggestion and confidence and loses its justification. */
export function stateWithoutJustification(): FakeState {
  const state = prototypeState();
  const pending = state.transactions.find((candidate) => candidate.id === FIRST_PENDING_ID)?.pending;
  if (!pending) throw new Error(`${FIRST_PENDING_ID} is not a pending review of the prototype's dataset`);
  return stateWithPending({ ...pending, justification: null });
}
