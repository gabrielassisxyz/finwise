/** A budget amount is never negative: blank, text that is not a number and a negative all become 0. */
export function typedAmount(text: string): number {
  const amount = Number(text);
  return amount >= 0 ? amount : 0;
}
