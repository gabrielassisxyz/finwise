import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { useMonthStore } from '@/stores/month';
import { useShellStore } from '@/stores/shell';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import AccountScreen from './AccountScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/contas', component: Blank },
  { path: '/contas/:id', component: AccountScreen },
];

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountAccount(id: string) {
  const mounted = await mountRouted(AccountScreen, routes, `/contas/${id}`);
  useShellStore().overview = prototypeOverview();
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return mounted;
}

const rowNames = (wrapper: Awaited<ReturnType<typeof mountAccount>>['wrapper']) =>
  wrapper.findAll('.row .name').map((name) => name.text());

describe('Account page', () => {
  it('heads the page with the bank and name, the bucket and the balance', async () => {
    const { wrapper } = await mountAccount('itau');
    expect(wrapper.find('h1').text()).toBe('Itaú · Conta corrente');
    expect(wrapper.find('.bucket').text()).toBe('Balde: Disponível');
    expect(plain(wrapper.find('.balance').text())).toBe('R$ 7.020,00');
  });

  it('lists only the movements of that account', async () => {
    const { wrapper } = await mountAccount('itau');
    const names = rowNames(wrapper);
    expect(names).toContain('Salário');
    expect(names).toContain('Condomínio');
    expect(names).not.toContain('Posto Shell');
    expect(names).not.toContain('Pet Shop Bicho Bom');
    expect(wrapper.findAll('.row .account').every((cell) => cell.text() === 'Itaú')).toBe(true);
  });

  it('lists the movements of another account on its own page', async () => {
    const { wrapper } = await mountAccount('c6');
    expect(rowNames(wrapper)).toEqual(['Pet Shop Bicho Bom']);
  });

  it('drops the Conta filter and keeps the others, without showing the account as a filter chip', async () => {
    const { wrapper } = await mountAccount('itau');
    const buttons = wrapper.findAll('.filter').map((button) => button.text());
    expect(buttons).not.toContain('Conta ▾');
    expect(buttons).toEqual(['Período ▾', 'Linha do budget ▾', 'Tag ▾', 'Só pendentes', 'Sem categoria']);
    expect(wrapper.find('.chips').exists()).toBe(false);
  });

  it('counts the tabs over that account only', async () => {
    const { wrapper } = await mountAccount('itau');
    const counts = wrapper.findAll('.tab').map((tab) => tab.text().replace(/\s+/g, ''));
    expect(counts).toEqual(['Todas12', 'PIX6', 'Boletos3', 'Cartão0', 'Outros3']);
  });

  it('says there is nothing yet for the idle account, rather than showing other accounts', async () => {
    const { wrapper } = await mountAccount('nu');
    expect(rowNames(wrapper)).toEqual([]);
    expect(wrapper.find('.empty-title').text()).toContain('Nenhuma transação');
  });

  it('goes back to the list of accounts', async () => {
    const { wrapper, router } = await mountAccount('itau');
    await wrapper.find('.back').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/contas');
  });

  it('says so when no account has that id, and for the card, which has a page of its own', async () => {
    expect((await mountAccount('nope')).wrapper.text()).toContain('Conta não encontrada.');
    expect((await mountAccount('card')).wrapper.text()).toContain('Conta não encontrada.');
  });
});
