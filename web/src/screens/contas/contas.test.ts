import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { mountRouted } from '@/test/mountRouted';
import ContasScreen from './ContasScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/contas', component: Blank },
  { path: '/contas/cartao', component: Blank },
  { path: '/contas/:id', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountList() {
  const mounted = await mountRouted(ContasScreen, routes, '/contas');
  await flushPromises();
  return mounted;
}

describe('Contas', () => {
  it('lists the accounts with the balance, or the open bill for the card', async () => {
    const { wrapper } = await mountList();
    const rows = wrapper.findAll('.row').map((row) => row.findAll(':scope > span').map((cell) => plain(cell.text())).join(' | '));
    expect(rows).toEqual([
      'Itaú | Conta corrente | Conta | R$ 7.020,00',
      'Mercado Pago | Conta | Conta | R$ 312,40',
      'C6 | Conta corrente | Conta | R$ 1.180,25',
      'Nubank | ContaSem movimento recente | Conta | R$ 96,10',
      'C6 | C6 Carbon · final 1234 | Cartão | R$ 2.310,00',
    ]);
    const labels = wrapper.findAll('.amount').map((cell) => cell.attributes('data-label'));
    expect(labels).toEqual(['Saldo', 'Saldo', 'Saldo', 'Saldo', 'Fatura atual']);
  });

  it('labels the figure of the card as the current bill', async () => {
    const { wrapper } = await mountList();
    const card = wrapper.findAll('.row').find((row) => row.text().includes('Cartão'))!;
    expect(card.find('.amount').attributes('data-label')).toBe('Fatura atual');
  });

  it('fades the idle account and notes it, and no other', async () => {
    const { wrapper } = await mountList();
    const idle = wrapper.findAll('.row.idle');
    expect(idle).toHaveLength(1);
    expect(idle[0]!.text()).toContain('Nubank');
    expect(wrapper.findAll('.note')).toHaveLength(1);
  });

  it('closes with the note about what the banks send', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.find('.footer').text()).toBe(
      'Só aparece o que os bancos enviam. Contas novas entram sozinhas depois da próxima sincronização.',
    );
  });

  it('opens an account on its own page and the card on the card page', async () => {
    const { wrapper, router } = await mountList();
    await wrapper.findAll('.row')[0]!.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/contas/itau');
    await router.push('/contas');
    await wrapper.findAll('.row')[4]!.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/contas/cartao');
  });
});
