import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';
import { prototypeState } from './prototype-data';

const ids = (purchases: { transactionId: string }[]) => purchases.map((purchase) => purchase.transactionId);

describe('card overview', () => {
  it('opens on the prototype figures', async () => {
    const card = (await createFakeApi().card.overview())!;
    expect(card).toMatchObject({
      title: 'C6 Carbon · final 1234',
      openBill: 2310,
      closesOn: '2026-11-28',
      dueOn: '2026-12-07',
      bestPurchaseDay: '2026-11-28',
      limit: 12000,
      availableLimit: 9303.6,
    });
  });

  it('adds up what was bought since the closing, newest first, and leaves the older purchases out', async () => {
    const { payAhead } = (await createFakeApi().card.overview())!;
    expect(payAhead.amount).toBe(386.4);
    expect(ids(payAhead.purchases)).toEqual(['t1', 't2', 't3', 't4']);
  });

  it('lists the purchases of the two days before the closing as the ones that may still move', async () => {
    const { mayFallOnNextBill } = (await createFakeApi().card.overview())!;
    expect(ids(mayFallOnNextBill)).toEqual(['t5', 't6']);
  });

  it('takes the pay-ahead off the available limit as well as the open bill', async () => {
    const state = prototypeState();
    state.card!.limit = 5000;
    const card = (await createFakeApi({ state }).card.overview())!;
    expect(card.availableLimit).toBe(5000 - 2310 - 386.4);
  });

  it('has nothing to pay ahead before the month reaches the closing day', async () => {
    const state = prototypeState();
    state.shell.today = '2026-11-10';
    const card = (await createFakeApi({ state }).card.overview())!;
    expect(card.payAhead).toEqual({ amount: 0, purchases: [] });
    expect(card.closesOn).toBe('2026-11-28');
  });

  it('closes on the next month once this month closing has passed, and rolls the year over for the due date', async () => {
    const state = prototypeState();
    state.shell.today = '2026-12-29';
    state.currentMonth = '2026-12';
    const card = (await createFakeApi({ state }).card.overview())!;
    expect(card.closesOn).toBe('2027-01-28');
    expect(card.dueOn).toBe('2027-02-07');
  });

  it('follows a manual closing day: the closing moves, and so does what falls after it', async () => {
    const api = createFakeApi();
    await api.settings.setClosing('manual', 20);
    const card = (await api.card.overview())!;
    expect(card.closesOn).toBe('2026-12-20');
    expect(card.dueOn).toBe('2027-01-07');
    // From the 20th on, purchases only: the refund of the 20th is a credit and is not one.
    expect(ids(card.payAhead.purchases)).toEqual(['t1', 't2', 't3', 't4', 't5', 't6', 't26', 't7', 't8', 't9']);
    expect(card.payAhead.amount).toBe(762.35);
    expect(card.availableLimit).toBe(8927.65);
    expect(card.mayFallOnNextBill).toEqual([]);
  });

  it('goes back to the bank rule when the mode is automatic, whatever manual day is kept', async () => {
    const api = createFakeApi();
    await api.settings.setClosing('manual', 20);
    await api.settings.setClosing('auto', 20);
    expect((await api.card.overview())!.closesOn).toBe('2026-11-28');
  });

  it('shows a purchase under its nickname', async () => {
    const api = createFakeApi();
    await api.settings.setNickname('Posto Shell', 'Gasolina');
    const { payAhead } = (await api.card.overview())!;
    expect(payAhead.purchases.find((purchase) => purchase.transactionId === 't2')!.name).toBe('Gasolina');
  });
});

describe('previous bills', () => {
  it('come newest first, each with a total that is the sum of its items', async () => {
    const { bills } = (await createFakeApi().card.overview())!;
    expect(bills.map((bill) => [bill.month, bill.closedOn, bill.dueOn, bill.total])).toEqual([
      ['2026-11', '2026-10-28', '2026-11-07', 2140],
      ['2026-10', '2026-09-28', '2026-10-07', 1985.3],
      ['2026-09', '2026-08-28', '2026-09-07', 2412.75],
    ]);
  });

  it('total what the items say, not a stored figure', async () => {
    const state = prototypeState();
    state.card!.bills[0]!.items.push({ name: 'Extra', day: null, amount: 10.1 });
    const { bills } = (await createFakeApi({ state }).card.overview())!;
    expect(bills[0]!.total).toBe(2150.1);
  });

  it('keep the items with a null day for a line that sums several purchases', async () => {
    const { bills } = (await createFakeApi().card.overview())!;
    expect(bills[0]!.items.at(-1)).toEqual({ name: 'Outras 31 compras', day: null, amount: 1790.8 });
  });
});

describe('home pay-ahead', () => {
  it('is the card page figure', async () => {
    const api = createFakeApi();
    await api.settings.setClosing('manual', 20);
    expect((await api.home.summary('2026-11')).payAhead).toEqual({ amount: 762.35, cardLabel: 'C6 Carbon · final 1234' });
  });

  it('disappears while there is nothing to pay ahead', async () => {
    const state = prototypeState();
    state.shell.today = '2026-11-10';
    expect((await createFakeApi({ state }).home.summary('2026-11')).payAhead).toBeNull();
  });
});

describe('a card whose closing is unknown', () => {
  const unknownClosing = async () => {
    const state = prototypeState();
    state.card!.bankClosingDay = null;
    return (await createFakeApi({ state }).card.overview())!;
  };

  it('answers null for the closing, the best purchase day and the next-bill amount, and the rest as known', async () => {
    const card = await unknownClosing();
    expect(card.closesOn).toBeNull();
    expect(card.bestPurchaseDay).toBeNull();
    expect(card.payAhead).toEqual({ amount: null, purchases: [] });
    expect(card.mayFallOnNextBill).toEqual([]);
    expect(card).toMatchObject({ dueOn: '2026-12-07', limit: 12000, availableLimit: 9690 });
    expect(card.bills).toHaveLength(3);
  });

  it('offers no pay-ahead on the home summary and no card bill on the closing', async () => {
    const state = prototypeState();
    state.card!.bankClosingDay = null;
    const api = createFakeApi({ state });
    expect((await api.home.summary('2026-11')).payAhead).toBeNull();
    expect((await api.review.closing('2026-11')).cardBill).toBeNull();
  });
});
