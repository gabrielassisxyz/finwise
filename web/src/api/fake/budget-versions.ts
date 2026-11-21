import type { BudgetDoc, YearMonth } from '../types';
import { shiftMonth } from '@/lib/format';
import type { FakeBudgetGroup, FakeState } from './model';

// The budget is versioned by month: a save holds from its month on, and a
// closed month keeps the budget it was closed with.

const isClosed = (state: FakeState, month: YearMonth): boolean =>
  state.shell.closedMonths.some((closed) => closed.month === month);

/** The budget in force in a month: the latest version that starts on or before it. */
export function budgetFor(state: FakeState, month: YearMonth): FakeBudgetGroup[] {
  const earlier = state.budgetVersions.filter((version) => version.from <= month);
  const version = earlier.at(-1) ?? state.budgetVersions[0];
  return structuredClone(version?.groups ?? []);
}

/** A draft as the budget stores it: amounts are numbers, whatever was typed. */
export function cleanDraft(draft: BudgetDoc): FakeBudgetGroup[] {
  return draft.map((group) => ({
    key: group.key,
    name: group.name,
    lines: group.lines.map((line) => ({
      name: line.name,
      amount: Number.isFinite(line.amount) ? line.amount : 0,
      ...(line.flag ? { flag: line.flag } : {}),
    })),
  }));
}

/** What the other screens read is the budget of the current month. */
export function syncCurrentBudget(state: FakeState): void {
  state.budget = budgetFor(state, state.currentMonth);
}

/**
 * Makes `draft` the budget from `month` on. A closed month after it keeps what
 * it had, and the budget resumes the month after it.
 */
export function saveFrom(state: FakeState, month: YearMonth, draft: BudgetDoc): void {
  const closedAfter = state.shell.closedMonths.map((closed) => closed.month).filter((closed) => closed > month);
  const kept = closedAfter.map((closed) => ({ from: closed, groups: budgetFor(state, closed) }));
  const resumed = closedAfter
    .map((closed) => shiftMonth(closed, 1))
    .filter((next) => !isClosed(state, next))
    .map((next) => ({ from: next, groups: cleanDraft(draft) }));
  const before = state.budgetVersions.filter((version) => version.from < month);
  state.budgetVersions = [...before, { from: month, groups: cleanDraft(draft) }, ...kept, ...resumed].sort((a, b) =>
    a.from.localeCompare(b.from),
  );
  syncCurrentBudget(state);
}

export function assertOpen(state: FakeState, month: YearMonth): void {
  if (isClosed(state, month)) throw new Error(`The budget of ${month} is closed and does not change`);
}
