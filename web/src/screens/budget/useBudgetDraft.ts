import { ref } from 'vue';
import type { BudgetDoc, BudgetLineDoc, EditableGroupKey } from '@/api/types';

/**
 * A budget being edited: reordering, renaming and adding lines. What the
 * numbers come to is asked of the backend, which is given the draft.
 */
export function useBudgetDraft() {
  const draft = ref<BudgetDoc | null>(null);

  function start(budget: BudgetDoc): void {
    draft.value = structuredClone(budget);
  }

  function discard(): void {
    draft.value = null;
  }

  function change(key: EditableGroupKey, edit: (lines: BudgetLineDoc[]) => BudgetLineDoc[]): void {
    if (!draft.value) return;
    draft.value = draft.value.map((group) => (group.key === key ? { ...group, lines: edit(group.lines) } : group));
  }

  const patch = (index: number, fields: Partial<BudgetLineDoc>) => (lines: BudgetLineDoc[]) =>
    lines.map((line, at) => (at === index ? { ...line, ...fields } : line));

  const rename = (key: EditableGroupKey, index: number, name: string) => change(key, patch(index, { name }));
  const setAmount = (key: EditableGroupKey, index: number, amount: number) => change(key, patch(index, { amount }));
  const add = (key: EditableGroupKey) => change(key, (lines) => [...lines, { name: 'Nova linha', amount: 0 }]);
  const remove = (key: EditableGroupKey, index: number) => change(key, (lines) => lines.filter((_, at) => at !== index));

  function move(key: EditableGroupKey, index: number, by: -1 | 1): void {
    change(key, (lines) => {
      const target = index + by;
      if (target < 0 || target >= lines.length) return lines;
      const moved = [...lines];
      [moved[index], moved[target]] = [moved[target]!, moved[index]!];
      return moved;
    });
  }

  return { draft, start, discard, rename, setAmount, add, remove, move };
}
