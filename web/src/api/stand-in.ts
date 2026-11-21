import { createFakeApi } from './fake';
import { startingState } from './fake/starting-state';
import type { FinwiseApi } from './types';

/**
 * The only door to the in-memory stand-in and the prototype's dataset. The
 * entry point reaches it through a dynamic import that is dropped from the
 * build for real use, so nothing here may be imported from anywhere else.
 *
 * Only the dev server reads the starting state. The prototype build, which the
 * parity check measures, always serves the prototype's dataset: there
 * `import.meta.env.DEV` is false and the variable is never looked at.
 */
export function createStandIn(): FinwiseApi {
  return createFakeApi({ state: import.meta.env.DEV ? startingState(import.meta.env.VITE_FINWISE_STATE) : undefined });
}
