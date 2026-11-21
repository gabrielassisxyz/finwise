import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { usePanelStore } from '@/stores/panel';
import { useRevisionStore } from '@/stores/revision';
import { mountRouted } from '@/test/mountRouted';
import CardScreen from './CardScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/contas', component: Blank },
  { path: '/contas/cartao', component: Blank },
  { path: '/config/:section', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** The text of an element's children set apart by a space, since flex children carry no whitespace between them. */
const spaced = (element: { element: Element }) =>
  plain(Array.from(element.element.children, (child) => child.textContent ?? '').join(' '));

async function mountCard() {
  const mounted = await mountRouted(CardScreen, routes, '/contas/cartao');
  await flushPromises();
  return mounted;
}

describe('Card page', () => {
  it('names the card and states the open bill, its dates and the limits', async () => {
    const { wrapper } = await mountCard();
    expect(wrapper.find('h1').text()).toBe('C6 Carbon · final 1234');
    const cells = wrapper.findAll('.cell').map(spaced);
    expect(cells).toEqual([
      'Fatura aberta R$ 2.310,00 Fecha em 28/11 · Vence em 07/12 · Melhor dia de compra: 28/11',
      'Limite R$ 12.000,00',
      'Limite disponível R$ 9.303,60',
    ]);
  });

  it('heads the first column with the pay-ahead figure and its explanation', async () => {
    const { wrapper } = await mountCard();
    expect(plain(wrapper.find('.title').text())).toBe('Valor a adiantar: R$ 386,40');
    expect(wrapper.find('.explanation').text()).toBe(
      'Compras feitas a partir do fechamento. Pagando adiantado, o gasto do cartão fica igual ao do mês.',
    );
  });

  it('lists the purchases since the closing, then the ones that may still move', async () => {
    const { wrapper } = await mountCard();
    const rows = wrapper.findAll('.purchase').map(spaced);
    expect(rows).toEqual([
      'Cinema Belas Artes · 28/11 R$ 65,10',
      'Posto Shell · 28/11 R$ 180,00',
      'iFood · 28/11 R$ 54,90',
      'Pão de Açúcar · 28/11 R$ 86,40',
      'Bar do Zé · 27/11 R$ 64,00',
      'Amazon · 26/11 R$ 129,00',
    ]);
    expect(wrapper.find('.maybe-title').text()).toBe('Podem entrar na próxima fatura');
    expect(wrapper.find('.maybe-note').text()).toBe('Compras dos dois dias antes do fechamento. O banco ainda pode lançá-las depois.');
  });

  it('opens the transaction panel on the purchase pressed', async () => {
    const { wrapper } = await mountCard();
    await wrapper.findAll('.purchase')[1]!.trigger('click');
    expect(usePanelStore().openId).toBe('t2');
    await wrapper.findAll('.purchase')[5]!.trigger('click');
    expect(usePanelStore().openId).toBe('t6');
  });

  it('lists the previous bills closed, newest first, with the dates and the total', async () => {
    const { wrapper } = await mountCard();
    const bills = wrapper.findAll('.bill').map((bill) => `${bill.find('.chevron').text()} ${spaced(bill.find('.text'))} ${bill.find('.total').text()}`.replace(/\u00a0/g, ' '));
    expect(bills).toEqual([
      '▸ Novembro de 2026 Fechou em 28/10 · Venceu em 07/11 R$ 2.140,00',
      '▸ Outubro de 2026 Fechou em 28/09 · Venceu em 07/10 R$ 1.985,30',
      '▸ Setembro de 2026 Fechou em 28/08 · Venceu em 07/09 R$ 2.412,75',
    ]);
    expect(wrapper.findAll('.item')).toHaveLength(0);
  });

  it('opens one bill at a time to show its items, and closes it when pressed again', async () => {
    const { wrapper } = await mountCard();
    await wrapper.findAll('.bill')[0]!.trigger('click');
    expect(wrapper.findAll('.item').map(spaced)).toEqual([
      'Pão de Açúcar · 28/10 R$ 64,20',
      'Posto Shell · 20/10 R$ 175,00',
      'Assistente de IA · 05/10 R$ 110,00',
      'Outras 31 compras · — R$ 1.790,80',
    ]);
    expect(wrapper.findAll('.bill')[0]!.text()).toContain('▾');
    await wrapper.findAll('.bill')[1]!.trigger('click');
    expect(wrapper.findAll('.item')).toHaveLength(3);
    expect(wrapper.findAll('.bill')[0]!.text()).toContain('▸');
    await wrapper.findAll('.bill')[1]!.trigger('click');
    expect(wrapper.findAll('.item')).toHaveLength(0);
  });

  it('leads to the card section of the settings', async () => {
    const { wrapper, router } = await mountCard();
    await wrapper.find('.adjust').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/config/cartao');
  });

  it('goes back to the accounts', async () => {
    const { wrapper, router } = await mountCard();
    await wrapper.find('.back').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/contas');
  });

  it('draws again from the backend after a write, so a new closing day shows', async () => {
    const { wrapper, api } = await mountCard();
    await api.settings.setClosing('manual', 20);
    useRevisionStore().bump();
    await flushPromises();
    expect(plain(wrapper.find('.title').text())).toBe('Valor a adiantar: R$ 762,35');
    expect(wrapper.find('.cell').text()).toContain('Fecha em 20/12');
  });
});
