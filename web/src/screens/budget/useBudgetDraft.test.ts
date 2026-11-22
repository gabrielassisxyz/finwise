import { describe, expect, it } from 'vitest';
import type { BudgetDoc } from '@/api/types';
import { useBudgetDraft } from './useBudgetDraft';

// A line that lost its id would read to the server as removed and added again,
// clearing its transactions; one that took another's id would take its history.

function invented(): BudgetDoc {
  return [
    { key: 'rec', name: 'Receitas', lines: [{ id: '1', name: 'Salário', amount: 9000 }] },
    {
      key: 'var',
      name: 'Variáveis',
      lines: [
        { id: '2', name: 'Feira', amount: 600 },
        { id: '3', name: 'Padaria', amount: 150, flag: 'Não garantida' },
        { id: '4', name: 'Cinema', amount: 120 },
      ],
    },
  ];
}

function started() {
  const editor = useBudgetDraft();
  editor.start(invented());
  return editor;
}

const linesOf = (editor: ReturnType<typeof useBudgetDraft>, key: string) =>
  editor.draft.value!.find((group) => group.key === key)!.lines;

const idsAndNames = (editor: ReturnType<typeof useBudgetDraft>) => linesOf(editor, 'var').map((line) => [line.id, line.name]);

describe('the budget draft keeps each line its own id', () => {
  it('copies every id on start, without sharing the lines it was given', () => {
    const source = invented();
    const editor = useBudgetDraft();
    editor.start(source);
    expect(editor.draft.value).toEqual(source);
    linesOf(editor, 'var')[0]!.name = 'Outra';
    expect(source[1]!.lines[0]!.name).toBe('Feira');
  });

  it('keeps the id through a rename', () => {
    const editor = started();
    editor.rename('var', 1, 'Pão');
    expect(linesOf(editor, 'var')[1]).toEqual({ id: '3', name: 'Pão', amount: 150, flag: 'Não garantida' });
    expect(idsAndNames(editor)).toEqual([['2', 'Feira'], ['3', 'Pão'], ['4', 'Cinema']]);
  });

  it('keeps the id through a change of amount', () => {
    const editor = started();
    editor.setAmount('var', 2, 90);
    expect(linesOf(editor, 'var')[2]).toEqual({ id: '4', name: 'Cinema', amount: 90 });
    expect(linesOf(editor, 'rec')[0]!.id).toBe('1');
  });

  it('moves the id with its line, up and down', () => {
    const editor = started();
    editor.move('var', 2, -1);
    expect(idsAndNames(editor)).toEqual([['2', 'Feira'], ['4', 'Cinema'], ['3', 'Padaria']]);
    editor.move('var', 0, 1);
    expect(idsAndNames(editor)).toEqual([['4', 'Cinema'], ['2', 'Feira'], ['3', 'Padaria']]);
  });

  it('adds a line with no id at all', () => {
    const editor = started();
    editor.add('var');
    const added = linesOf(editor, 'var').at(-1)!;
    expect('id' in added).toBe(false);
    expect(idsAndNames(editor).slice(0, 3)).toEqual([['2', 'Feira'], ['3', 'Padaria'], ['4', 'Cinema']]);
  });

  it('removes a line with its id, leaving every other line its own, and a line added afterwards has none', () => {
    const editor = started();
    editor.remove('var', 1);
    expect(idsAndNames(editor)).toEqual([['2', 'Feira'], ['4', 'Cinema']]);
    expect(editor.draft.value!.flatMap((group) => group.lines).some((line) => line.id === '3')).toBe(false);
    editor.add('var');
    const added = linesOf(editor, 'var').at(-1)!;
    expect('id' in added).toBe(false);
    expect(linesOf(editor, 'var').map((line) => line.id)).toEqual(['2', '4', undefined]);
  });
});
