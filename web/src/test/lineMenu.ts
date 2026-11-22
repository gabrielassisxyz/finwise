import type { FinwiseApi, TransactionFilterOptions, YearMonth } from '@/api/types';

/** The one line a named answer holds: its month and how many times that month had been asked. */
export const lineNamed = (month: YearMonth, call: number): string => `Linha de ${month} #${call}`;
export const tagNamed = (month: YearMonth, call: number): string => `tag de ${month} #${call}`;

/**
 * Wraps the line menu of a stand-in so a test sees which month each call asked for. The stand-in answers the same
 * menu whatever the month, so with `named` each answer holds one line and one tag named after its month and its
 * call instead, which tells two months or two revisions apart on screen. `hold` keeps a month's answers back until
 * the function it returns is called.
 */
export function recordLineMenu(api: FinwiseApi, options: { named?: boolean } = {}) {
  const months: YearMonth[] = [];
  const held = new Map<YearMonth, Promise<void>>();
  const answer = api.transactions.filterOptions.bind(api.transactions);

  api.transactions.filterOptions = async (month: YearMonth): Promise<TransactionFilterOptions> => {
    months.push(month);
    const call = months.filter((asked) => asked === month).length;
    const gate = held.get(month);
    const answered = await answer(month);
    await gate;
    if (!options.named) return answered;
    return { ...answered, tags: [tagNamed(month, call)], lines: [{ name: lineNamed(month, call), group: 'Variáveis' }] };
  };

  function hold(month: YearMonth): () => void {
    let release = (): void => {};
    held.set(month, new Promise<void>((done) => (release = done)));
    return () => {
      held.delete(month);
      release();
    };
  }

  return { months, hold };
}
