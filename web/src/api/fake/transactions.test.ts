import { describe, expect, it } from 'vitest';
import { createFakeApi } from './index';

const api = () => createFakeApi().transactions;

describe('fake transactions', () => {
  it('lists one month, newest first, in entry order within a day', async () => {
    const list = await api().list({ month: '2026-11', period: 'month' });
    expect(list).toHaveLength(40);
    expect(list.slice(0, 4).map((transaction) => transaction.name)).toEqual([
      'Cinema Belas Artes',
      'Posto Shell',
      'iFood',
      'Pão de Açúcar',
    ]);
    expect(list.every((transaction) => transaction.day.startsWith('2026-11'))).toBe(true);
  });

  it('widens to the last three months and to the year', async () => {
    const quarter = await api().list({ month: '2026-12', period: 'last-3-months' });
    expect(quarter).toHaveLength(45);

    const january = await api().list({ month: '2027-01', period: 'last-3-months' });
    expect(january).toHaveLength(40);

    const year = await api().list({ month: '2026-03', period: 'year' });
    expect(year).toHaveLength(45);
  });

  it('answers an empty month with nothing', async () => {
    expect(await api().list({ month: '2026-12', period: 'month' })).toEqual([]);
  });

  it('offers the budget lines with spending first, fees after the provisions and income last', async () => {
    const { lines } = await api().filterOptions();
    const names = lines.map((line) => line.name);
    expect(names[0]).toBe('Mercado');
    expect(names.at(-1)).toBe('Reembolsos');
    expect(names.indexOf('Tarifas e juros')).toBe(names.indexOf('Remédios') + 1);
    expect(lines.find((line) => line.name === 'Viagens')?.group).toBe('Reservas de desejo');
  });

  it('offers every account, with the card under its label', async () => {
    const { accounts } = await api().filterOptions();
    expect(accounts.map((account) => account.label)).toEqual(['Itaú', 'Mercado Pago', 'C6', 'Nubank', 'C6 Carbon · 1234']);
    expect(accounts.at(-1)?.value).toBe('C6 Carbon');
  });
});
