import { createFakeApi } from './fake';
import type { FinwiseApi } from './types';

/**
 * The only door to the in-memory stand-in and the prototype's dataset. The
 * entry point reaches it through a dynamic import that is dropped from the
 * build for real use, so nothing here may be imported from anywhere else.
 */
export function createStandIn(): FinwiseApi {
  return createFakeApi();
}
