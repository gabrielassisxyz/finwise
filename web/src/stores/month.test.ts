import { describe, expect, it } from 'vitest';
import { prototypeOverview, withStores } from '@/test/mount';
import { useMonthStore } from './month';

function setup() {
  const month = withStores(() => useMonthStore()).result;
  month.configure(prototypeOverview());
  return month;
}

describe('month store', () => {
  it('starts on the month of today', () => {
    expect(setup().selected).toBe('2026-11');
  });

  it('keeps the selection when the overview is loaded again', () => {
    const month = setup();
    month.shift(-1);
    month.configure(prototypeOverview());
    expect(month.selected).toBe('2026-10');
  });

  it('does not move past either end of the range', () => {
    const month = setup();
    month.shift(1);
    month.shift(1);
    expect(month.selected).toBe('2026-12');
    expect(month.canGoForward).toBe(false);

    month.shift(-1);
    month.shift(-1);
    month.shift(-1);
    expect(month.selected).toBe('2026-10');
    expect(month.canGoBack).toBe(false);
  });

  it('knows which months are closed', () => {
    const month = setup();
    expect(month.isClosed).toBe(false);
    month.shift(-1);
    expect(month.isClosed).toBe(true);
  });
});
