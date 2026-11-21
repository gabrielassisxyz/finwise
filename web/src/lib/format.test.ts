import { describe, expect, it } from 'vitest';
import { brl, longDay, monthInSentence, monthLabel, shiftMonth, shortDay, signedBrl } from './format';

describe('months', () => {
  it('labels a month for the selector and for a sentence', () => {
    expect(monthLabel('2026-11')).toBe('novembro 2026');
    expect(monthInSentence('2026-03')).toBe('março de 2026');
  });

  it('shifts across a year boundary in both directions', () => {
    expect(shiftMonth('2026-11', 1)).toBe('2026-12');
    expect(shiftMonth('2026-12', 1)).toBe('2027-01');
    expect(shiftMonth('2026-01', -1)).toBe('2025-12');
    expect(shiftMonth('2026-03', -14)).toBe('2025-01');
  });
});

describe('days', () => {
  it('formats short and long', () => {
    expect(shortDay('2026-11-05')).toBe('05/11');
    expect(longDay('2026-11-05')).toBe('05/11/2026');
  });
});

describe('money', () => {
  it('formats with thousands, two decimals and a non-breaking space', () => {
    expect(brl(1234.5)).toBe('R$ 1.234,50');
    expect(brl(-80)).toBe('R$ 80,00');
  });

  it('marks money in with a plus and money out with a true minus', () => {
    expect(signedBrl(150)).toBe('+ R$ 150,00');
    expect(signedBrl(-64)).toBe('− R$ 64,00');
    expect(signedBrl(0)).toBe('R$ 0,00');
  });
});
