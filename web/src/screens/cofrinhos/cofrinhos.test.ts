import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { mountRouted } from '@/test/mountRouted';
import CofrinhosScreen from './CofrinhosScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/cofrinhos', component: Blank },
  { path: '/cofrinhos/:id', component: Blank },
];

const plain = (text: string) => text.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim();

async function mountList() {
  const mounted = await mountRouted(CofrinhosScreen, routes, '/cofrinhos');
  await flushPromises();
  return mounted;
}

describe('Cofrinhos list', () => {
  it('groups the pots by bucket with the subtotal of each group', async () => {
    const { wrapper } = await mountList();
    const heads = wrapper.findAll('.group-head').map((head) => plain(head.text()));
    expect(heads).toEqual([
      'ReservadoSubtotal R$ 34.150,00',
      'InvestidoSubtotal R$ 37.800,00',
      'Sem baldeSubtotal R$ 500,00',
    ]);
    expect(wrapper.findAll('.row')).toHaveLength(13);
  });

  it('shows a pot nobody named as "Sem nome", greyed', async () => {
    const { wrapper } = await mountList();
    const unnamed = wrapper.find('.name.unnamed');
    expect(unnamed.text()).toBe('Sem nome');
    expect(wrapper.findAll('.name.unnamed')).toHaveLength(1);
  });

  it('writes the target with its bar, and a dash where there is none', async () => {
    const { wrapper } = await mountList();
    const rows = wrapper.findAll('.row');
    const pets = rows.find((row) => row.text().includes('RF Pets'))!;
    expect(plain(pets.find('.target').text())).toBe('R$ 4.200 de R$ 5.000');
    expect(pets.find('.bar-fill').attributes('style')).toContain('width: 84%');
    const gas = rows.find((row) => row.text().includes('Gás e IPTU'))!;
    expect(plain(gas.find('.target').text())).toBe('—');
    expect(gas.find('.bar').exists()).toBe(false);
  });

  it('writes the yield with a plus sign and the maturity as a date', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('Investimento 1'))!;
    expect(plain(row.find('.yield').text())).toBe('+ R$ 301,88');
    expect(row.text()).toContain('15/03/2028');
  });

  it('hides closed pots until the toggle is pressed, then adds a Status column and fades them', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.text()).not.toContain('Investimento 0');
    expect(wrapper.find('.head').text()).not.toContain('Status');

    const toggle = wrapper.find('button.toggle');
    expect(toggle.attributes('aria-pressed')).toBe('false');
    await toggle.trigger('click');
    await flushPromises();

    expect(toggle.attributes('aria-pressed')).toBe('true');
    expect(wrapper.find('.head').text()).toContain('Status');
    const closed = wrapper.findAll('.row').find((row) => row.text().includes('Investimento 0'))!;
    expect(closed.classes()).toContain('closed');
    expect(closed.find('.status').text()).toBe('Encerrado');
    expect(closed.find('.yield').text()).toBe('—');
    const open = wrapper.findAll('.row').find((row) => row.text().includes('Investimento 1'))!;
    expect(open.find('.status').text()).toBe('Ativo');
    expect(open.classes()).not.toContain('closed');
    expect(wrapper.find('.content').classes()).toContain('with-status');
  });

  it('opens the pot of the row that was clicked', async () => {
    const { wrapper, router } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('RF Pets'))!;
    await row.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/cofrinhos/rfp');
  });

  it('carries the bank twice, as a column and as a subtitle, for the 1200px breakpoint to choose', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('RF Pets'))!;
    expect(row.find('.subtitle.narrow-only').text()).toBe('Mercado Pago');
    expect(row.findAll('.wide-only').map((cell) => plain(cell.text()))).toEqual(['Mercado Pago', 'Reservado']);
    expect(wrapper.find('.head').findAll('.wide-only').map((cell) => cell.text())).toEqual(['Banco', 'Balde']);
  });

  it('labels every value of a row for the phone card, and lets the name span both columns', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.find('.row');
    expect(row.classes()).toContain('stack-row');
    expect(row.find('.name-cell').classes()).toContain('stack-full');
    expect(row.findAll('[data-label]').map((cell) => cell.attributes('data-label'))).toEqual([
      'Banco',
      'Balde',
      'Saldo',
      '% CDI',
      'Rendimento no mês',
      'Vencimento',
      'Alvo',
    ]);
  });
});
