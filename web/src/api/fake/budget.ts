import type { BudgetApi } from '../types';
import { assertOpen, budgetFor, cleanDraft, saveFrom } from './budget-versions';
import { buildView, establishmentsOf, scenarioImpact } from './budget-view';
import type { ChangeLog } from './changes';
import type { FakeState, FakeTransaction } from './model';

const UNNAMED_SCENARIO = 'Cenário sem nome';

export function createFakeBudget(state: FakeState, original: FakeTransaction[], log: ChangeLog): BudgetApi {
  async function write(month: string, draft: Parameters<BudgetApi['save']>[1]) {
    assertOpen(state, month);
    const changeId = log.beginBudget();
    saveFrom(state, month, draft);
    return { changeId };
  }

  return {
    async document(month) {
      return budgetFor(state, month);
    },
    async view(month, draft) {
      return buildView(state, original, month, draft && cleanDraft(draft));
    },
    async establishments(month, line) {
      return establishmentsOf(state, month, line);
    },
    save: write,
    async scenarios() {
      return structuredClone(state.scenarios).map(({ name, groups }) => ({ name, budget: groups }));
    },
    async evaluateScenario(month, draft) {
      return scenarioImpact(state, month, cleanDraft(draft));
    },
    async saveScenario(name, draft) {
      const kept = name.trim() || UNNAMED_SCENARIO;
      const changeId = log.beginBudget();
      state.scenarios = [...state.scenarios.filter((scenario) => scenario.name !== kept), { name: kept, groups: cleanDraft(draft) }];
      return { changeId, name: kept };
    },
    applyScenario: write,
  };
}
