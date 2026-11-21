import type { CardApi } from '../types';
import { cardOverview } from './card-rules';
import type { FakeState } from './model';

export function createFakeCard(state: FakeState): CardApi {
  return {
    async overview() {
      return cardOverview(state);
    },
  };
}
