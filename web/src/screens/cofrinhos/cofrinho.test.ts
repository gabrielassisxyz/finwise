import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { defineComponent, h } from 'vue';
import { RouterView } from 'vue-router';
import { prototypeState } from '@/api/fake/prototype-data';
import { useRevisionStore } from '@/stores/revision';
import { mountRouted } from '@/test/mountRouted';
import CofrinhoScreen from './CofrinhoScreen.vue';
import CofrinhosScreen from './CofrinhosScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/cofrinhos', component: Blank },
  { path: '/cofrinhos/:id', component: Blank },
];

const plain = (text: string) => text.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim();

async function mountPot(id: string, state = prototypeState()) {
  const mounted = await mountRouted(CofrinhoScreen, routes, `/cofrinhos/${id}`, { state });
  await flushPromises();
  return mounted;
}

describe('Cofrinho page', () => {
  it('names the pot, where it is and what it holds', async () => {
    const { wrapper } = await mountPot('rfp');
    expect(wrapper.find('h1').text()).toBe('RF Pets');
    expect(wrapper.find('.subtitle').text()).toBe('Mercado Pago · Reservado · 120% do CDI');
    expect(plain(wrapper.find('.balance').text())).toBe('R$ 4.200,00');
  });

  it('calls an unnamed pot "Sem nome" and a pot with no bucket "Sem balde"', async () => {
    const { wrapper } = await mountPot('sn');
    expect(wrapper.find('h1').text()).toBe('Sem nome');
    expect(wrapper.find('.subtitle').text()).toBe('C6 · Sem balde · 100% do CDI');
  });

  it('warns how much is missing when the pot is under its target', async () => {
    const { wrapper } = await mountPot('rfp');
    expect(plain(wrapper.find('.notice').text())).toBe(
      'Abaixo do alvo: faltam R$ 800,00. Aporte religado até voltar ao alvo.',
    );
  });

  it('has no warning at the target or without one', async () => {
    expect((await mountPot('rfs')).wrapper.find('.notice').exists()).toBe(false);
    expect((await mountPot('gas')).wrapper.find('.notice').exists()).toBe(false);
  });

  it.each([
    ['i0', 'Encerrado', false],
    ['rfs', 'Desligado: está no alvo', false],
    ['emg', 'Sem aporte planejado', false],
    ['gas', 'Feito em 05/11', false],
    ['rfp', 'Ainda não feito', true],
  ])('says the contribution of %s is "%s"', async (id, text, warned) => {
    const { wrapper } = await mountPot(id);
    const value = wrapper.find('.contribution .value');
    expect(value.text()).toBe(text);
    expect(value.classes().includes('pending')).toBe(warned);
  });

  it('lists the lines a pot feeds, with the share of each when the pot is shared', async () => {
    expect(plain((await mountPot('gas')).wrapper.find('.lines .value').text())).toBe(
      'Gás R$ 240,00 · IPTU R$ 360,00',
    );
    expect((await mountPot('rem')).wrapper.find('.lines .value').text()).toBe('Remédios');
    expect((await mountPot('emg')).wrapper.find('.lines .value').text()).toBe('Nenhuma linha ligada');
  });

  it('shows the investment figures for an investment only', async () => {
    const { wrapper } = await mountPot('i1');
    const facts = wrapper.findAll('.investment').map((cell) => plain(cell.text()));
    expect(facts).toEqual([
      'Valor aplicadoR$ 22.400,00',
      'Valor brutoR$ 25.000,00',
      'ImpostoR$ 455,00',
      'Valor líquidoR$ 24.545,00',
      'Pode resgatar em15/03/2028',
    ]);
    expect((await mountPot('rfs')).wrapper.findAll('.investment')).toHaveLength(0);
  });

  it('lists the movements, with the year only on a day outside the current one', async () => {
    const investment = await mountPot('i1');
    expect(plain(investment.wrapper.find('.movement').text())).toBe('Aplicação · 15/03/2025+ R$ 22.400,00');
    const withdrawal = await mountPot('rfp');
    expect(plain(withdrawal.wrapper.find('.movement').text())).toBe('Retirada · veterinário · 14/10− R$ 800,00');
  });

  it('explains the empty history when nothing was deposited or withdrawn', async () => {
    const { wrapper } = await mountPot('com');
    expect(wrapper.findAll('.movement')).toHaveLength(0);
    expect(plain(wrapper.find('.empty').text())).toBe(
      'Nenhum depósito ou retirada desde novembro de 2026. Os próximos aparecem aqui no dia seguinte.',
    );
  });

  it('keeps the empty-history note off a pot that has movements', async () => {
    expect((await mountPot('gas')).wrapper.find('.empty').exists()).toBe(false);
  });

  it('unfolds the month of yield into its days, and folds it again', async () => {
    const { wrapper } = await mountPot('i1');
    expect(wrapper.findAll('.yield-day')).toHaveLength(0);
    const month = wrapper.find('.month');
    expect(plain(month.text())).toBe('▸Novembro+ R$ 301,88');

    await month.trigger('click');
    expect(wrapper.findAll('.yield-day')).toHaveLength(18);
    expect(plain(wrapper.find('.yield-day').text())).toMatch(/^27\/11\+ R\$ \d/);
    expect(month.text()).toContain('▾');

    await month.trigger('click');
    expect(wrapper.findAll('.yield-day')).toHaveLength(0);
  });

  it('has nothing to unfold for a closed pot', async () => {
    const { wrapper } = await mountPot('i0');
    await wrapper.find('.month').trigger('click');
    expect(wrapper.findAll('.yield-day')).toHaveLength(0);
    expect(wrapper.find('.month').text()).toContain('▸');
    expect(plain(wrapper.find('.month .total').text())).toBe('R$ 0,00');
  });

  it('starts another pot with the month folded', async () => {
    const { wrapper, router } = await mountPot('i1');
    await wrapper.find('.month').trigger('click');
    expect(wrapper.findAll('.yield-day')).toHaveLength(18);
    await router.push('/cofrinhos/i2');
    await flushPromises();
    expect(wrapper.find('h1').text()).toBe('Investimento 2');
    expect(wrapper.findAll('.yield-day')).toHaveLength(0);
  });

  it('goes back to the list', async () => {
    const { wrapper, router } = await mountPot('rfp');
    await wrapper.find('button.back').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/cofrinhos');
  });

  it('says so when no pot has the id', async () => {
    const { wrapper } = await mountPot('nope');
    expect(wrapper.find('.missing').text()).toBe('Cofrinho não encontrado.');
    expect(wrapper.find('h1').exists()).toBe(false);
  });
});

describe('moving between the list and a pot', () => {
  it('keeps the toggle where it was when the user comes back from a pot', async () => {
    const App = defineComponent({ render: () => h(RouterView) });
    const { wrapper, router } = await mountRouted(
      App,
      [
        { path: '/cofrinhos', component: CofrinhosScreen },
        { path: '/cofrinhos/:id', component: CofrinhoScreen },
      ],
      '/cofrinhos',
    );
    await flushPromises();
    await wrapper.find('button.toggle').trigger('click');
    await flushPromises();
    await wrapper.findAll('.row')[0]!.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/cofrinhos/rfs');

    await wrapper.find('button.back').trigger('click');
    await flushPromises();
    expect(wrapper.find('button.toggle').attributes('aria-pressed')).toBe('true');
    expect(wrapper.text()).toContain('Investimento 0');
  });
});

/** Lets a read run now but hands its answer over only when released, so an answer can arrive late. */
function holdNext<T>(read: () => Promise<T>) {
  let release = () => {};
  const held = read().then(
    (answer) =>
      new Promise<T>((resolve) => {
        release = () => resolve(answer);
      }),
  );
  return { held, release: () => release() };
}

describe('Cofrinho page after a change', () => {
  it('shows the new bucket after a write and a bump, without a remount', async () => {
    const { wrapper, api } = await mountPot('rfp');
    await api.settings.setPotBucket('rfp', 'Investido');
    useRevisionStore().bump();
    await flushPromises();
    expect(wrapper.find('.subtitle').text()).toBe('Mercado Pago · Investido · 120% do CDI');
  });

  it('keeps the pot on screen while it asks again, and asks once per bump', async () => {
    const { wrapper, api } = await mountPot('rfp');
    const read = vi.spyOn(api.savingsPots, 'detail').mockImplementationOnce(() => new Promise(() => {}));
    useRevisionStore().bump();
    await flushPromises();
    expect(read).toHaveBeenCalledTimes(1);
    expect(wrapper.find('h1').text()).toBe('RF Pets');
    expect(wrapper.find('.missing').exists()).toBe(false);
    read.mockRestore();
    const counted = vi.spyOn(api.savingsPots, 'detail');
    useRevisionStore().bump();
    await flushPromises();
    expect(counted).toHaveBeenCalledTimes(1);
    useRevisionStore().bump();
    await flushPromises();
    expect(counted).toHaveBeenCalledTimes(2);
  });

  it('does not show an answer for a pot the route has left', async () => {
    const { wrapper, api, router } = await mountPot('rfp');
    const original = api.savingsPots.detail.bind(api.savingsPots);
    const late = holdNext(() => original('rfp'));
    let calls = 0;
    vi.spyOn(api.savingsPots, 'detail').mockImplementation((...args: Parameters<typeof original>) => (++calls === 1 ? late.held : (original as (...a: typeof args) => ReturnType<typeof original>)(...args)));
    useRevisionStore().bump();
    await flushPromises();
    await router.push('/cofrinhos/rfs');
    await flushPromises();
    late.release();
    await flushPromises();
    expect(wrapper.find('h1').text()).not.toBe('RF Pets');
    expect(wrapper.find('h1').text()).toBe('RF Saúde');
  });
});
