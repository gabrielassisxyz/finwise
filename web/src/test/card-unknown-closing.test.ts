import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import CardScreen from '@/screens/contas/CardScreen.vue';
import { mountRouted } from './mountRouted';

const Blank = { template: '<div />' };
const routes = ['/contas', '/contas/cartao'].map((path) => ({ path, component: Blank }));
const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountCard(unknownClosing: boolean) {
  const state = prototypeState();
  if (unknownClosing) state.card!.bankClosingDay = null;
  const mounted = await mountRouted(CardScreen, routes, '/contas/cartao', { state });
  await flushPromises();
  return mounted;
}

describe('a card whose closing is unknown', () => {
  it('shows a dash for the closing, the best day to buy and the next-bill amount, and keeps what is known', async () => {
    const { wrapper } = await mountCard(true);
    expect(plain(wrapper.find('.dates').text())).toBe('Fecha em — · Vence em — · Melhor dia de compra: —');
    expect(plain(wrapper.find('.title').text())).toBe('Valor a adiantar: —');
    expect(plain(wrapper.text())).not.toContain('R$ 0,00');
    const cells = wrapper.findAll('.limit').map((cell) => plain(Array.from(cell.element.children, (child) => child.textContent ?? '').join(' ')));
    expect(cells).toEqual(['Limite R$ 12.000,00', 'Limite disponível R$ 9.690,00']);
    expect(wrapper.findAll('.purchase').length).toBe(0);
  });

  it('still lists the past bills', async () => {
    const { wrapper } = await mountCard(true);
    expect(plain(wrapper.text())).toContain('R$ 2.140,00');
  });

  it('keeps R$ 0,00 for a closing that is known and has nothing after it yet', async () => {
    const state = prototypeState();
    state.shell.today = '2026-11-10';
    const { wrapper } = await mountRouted(CardScreen, routes, '/contas/cartao', { state });
    await flushPromises();
    expect(plain(wrapper.find('.title').text())).toBe('Valor a adiantar: R$ 0,00');
  });
});
