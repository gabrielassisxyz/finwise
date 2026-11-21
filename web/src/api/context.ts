import { inject, type InjectionKey } from 'vue';
import type { FinwiseApi } from './types';

export const apiKey: InjectionKey<FinwiseApi> = Symbol('finwise-api');

export function useApi(): FinwiseApi {
  const api = inject(apiKey);
  if (!api) throw new Error('No FinwiseApi was provided to the app');
  return api;
}
