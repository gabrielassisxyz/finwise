import { createHttpApi } from './http/client';
import type { FinwiseApi } from './types';

// The stand-in sits behind a dynamic import under a build-time constant, so the
// build for real use carries neither it nor the prototype's dataset.
export async function provisionApi(): Promise<FinwiseApi> {
  if (!__FINWISE_STAND_IN__) return createHttpApi();
  return (await import('./stand-in')).createStandIn();
}
