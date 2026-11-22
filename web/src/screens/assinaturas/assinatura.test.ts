import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { useOverlayStore } from '@/stores/overlay';
import { useShellStore } from '@/stores/shell';
import { createFakeApi } from '@/api/fake';
import { useMonthStore } from '@/stores/month';
import { useRevisionStore } from '@/stores/revision';
import { lineNamed, recordLineMenu } from '@/test/lineMenu';
import { prototypeOverview } from '@/test/mount';
import { mountWithFailures } from '@/test/readOnly';
import { mountRouted } from '@/test/mountRouted';
import AssinaturaScreen from './AssinaturaScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/assinaturas', component: Blank },
  { path: '/assinaturas/:id', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountPage(id: string) {
  const mounted = await mountRouted(AssinaturaScreen, routes, `/assinaturas/${id}`);
  useShellStore().overview = prototypeOverview();
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountPage>>;

const facts = (wrapper: Mounted['wrapper']) =>
  wrapper.findAll('.fact').map((fact) => [fact.find('.label').text(), plain(fact.findAll('span')[1]!.text())]);

describe('Assinatura, the page', () => {
  it('names the subscription, its marks, its cycle and its amount', async () => {
    const { wrapper } = await mountPage('sp');
    expect(wrapper.find('h1').text()).toBe('Spotify');
    expect(wrapper.findAll('.marks .mark').map((mark) => mark.text())).toEqual(['Preço subiu']);
    expect(wrapper.find('.marks .mark').classes()).toContain('large');
    expect(wrapper.find('.cycle').text()).toBe('Mensal');
    expect(plain(wrapper.find('.price').text())).toBe('R$ 21,90');
  });

  it('lists the next charge and the monthly and yearly equivalents', async () => {
    expect(facts((await mountPage('sp')).wrapper)).toEqual([
      ['Próxima cobrança', '22/12/2026'],
      ['Equivalente mensal', 'R$ 21,90'],
      ['Equivalente anual', 'R$ 262,80'],
    ]);
    expect(facts((await mountPage('ho')).wrapper)).toEqual([
      ['Próxima cobrança', '10/01/2027'],
      ['Equivalente mensal', 'R$ 20,00'],
      ['Equivalente anual', 'R$ 240,00'],
    ]);
  });

  it('shows the line, the status, the review date and the note', async () => {
    const { wrapper } = await mountPage('ho');
    expect(plain(wrapper.find('.line-button').text())).toBe('Ferramentas de trabalho ▾');
    expect(plain(wrapper.find('.pill').text())).toBe('Manter▾');
    expect(wrapper.find<HTMLInputElement>('input[type="date"]').element.value).toBe('2027-06-01');
    expect(wrapper.find<HTMLTextAreaElement>('textarea').element.value).toBe('');
    expect(wrapper.find('textarea').attributes('placeholder')).toBe('Por que manter, o que conferir antes de cancelar');
  });

  it('goes back to the list from the link', async () => {
    const { wrapper, router } = await mountPage('sp');
    expect(wrapper.find('.back').text()).toBe('‹ Assinaturas');
    await wrapper.find('.back').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/assinaturas');
  });

  it('says so for a subscription that does not exist', async () => {
    const { wrapper } = await mountPage('missing');
    expect(wrapper.find('.missing').text()).toBe('Assinatura não encontrada.');
    expect(wrapper.find('h1').exists()).toBe(false);
  });
});

describe('Assinatura, editing', () => {
  it('changes the budget line from a searchable menu of lines', async () => {
    const { wrapper, overlay } = await mountPage('sp');
    await wrapper.find('.line-button').trigger('click');
    expect(overlay.menu).toMatchObject({ search: true, placeholder: 'Buscar linha' });
    expect(overlay.menu?.items.find((item) => item.current)?.label).toBe('Assinaturas');

    overlay.menu!.items.find((item) => item.label === 'Lazer')!.onPick!();
    await flushPromises();
    expect(plain(wrapper.find('.line-button').text())).toBe('Lazer ▾');
  });

  it('changes the status with a toast that takes it back', async () => {
    const { wrapper, overlay } = await mountPage('sp');
    await wrapper.find('.pill').trigger('click');
    expect(overlay.menu?.items.find((item) => item.current)?.label).toBe('Manter');

    overlay.menu!.items.find((item) => item.label === 'Cancelar')!.onPick!();
    await flushPromises();
    expect(plain(wrapper.find('.pill').text())).toBe('Cancelar▾');
    expect(wrapper.find('.pill').classes()).toContain('bad');
    expect(overlay.toast?.message).toBe('Spotify: status alterado para Cancelar.');

    overlay.toast!.actions[0]!.run();
    await flushPromises();
    expect(plain(wrapper.find('.pill').text())).toBe('Manter▾');
  });

  it('saves a new review date, and ignores a date that was cleared', async () => {
    const { wrapper, api } = await mountPage('sp');
    const input = wrapper.find('input[type="date"]');
    await input.setValue('2027-05-20');
    await flushPromises();
    expect((await api.subscriptions.detail('sp'))?.reviewOn).toBe('2027-05-20');

    await input.setValue('');
    await flushPromises();
    expect((await api.subscriptions.detail('sp'))?.reviewOn).toBe('2027-05-20');
  });

  it('saves the note when the field is left', async () => {
    const { wrapper, api } = await mountPage('sp');
    await wrapper.find('textarea').setValue('Conferir o plano anual');
    await wrapper.find('textarea').trigger('change');
    await flushPromises();
    expect((await api.subscriptions.detail('sp'))?.note).toBe('Conferir o plano anual');
  });
});

describe('Assinatura, the charge history', () => {
  const charges = (wrapper: Mounted['wrapper']) => wrapper.findAll('.charge').map((charge) => plain(charge.text()));

  it('lists the last three charges, newest first', async () => {
    const { wrapper } = await mountPage('sp');
    expect(charges(wrapper)).toEqual(['22/11/2026R$ 21,90', '22/10/2026R$ 19,90', '22/09/2026R$ 19,90']);
  });

  it('expands a foreign charge to its IOF and folds it again; a domestic charge does not expand', async () => {
    const { wrapper } = await mountPage('ho');
    expect(charges(wrapper)[0]).toBe('▸10/10/2026· US$ 10,50R$ 60,00');
    expect(wrapper.find('.iof').exists()).toBe(false);

    await wrapper.findAll('.charge')[0]!.trigger('click');
    expect(plain(wrapper.find('.iof').text())).toBe('IOF · R$ 2,03já incluído no valor acima');
    expect(charges(wrapper)[0]).toContain('▾');

    await wrapper.findAll('.charge')[1]!.trigger('click');
    expect(wrapper.findAll('.iof')).toHaveLength(1);

    await wrapper.findAll('.charge')[1]!.trigger('click');
    expect(wrapper.find('.iof').exists()).toBe(false);

    const domestic = await mountPage('sp');
    await domestic.wrapper.findAll('.charge')[0]!.trigger('click');
    expect(domestic.wrapper.find('.iof').exists()).toBe(false);
  });
});

describe('Assinatura, the alternatives', () => {
  it('lists what the model wrote', async () => {
    const { wrapper } = await mountPage('sp');
    expect(wrapper.findAll('.alternative').map((item) => item.text())).toEqual([
      'Plano anual: R$ 219,00, economia de R$ 43,80 por ano.',
      'Plano para duas pessoas: R$ 27,90, se for dividir com alguém.',
    ]);
    expect(wrapper.text()).toContain('Escritas pela IA');
  });

  it('says none was found when the model found none', async () => {
    const { wrapper } = await mountPage('do');
    expect(wrapper.findAll('.alternative').map((item) => item.text())).toEqual([
      'Nenhuma alternativa encontrada ainda. A IA procura planos mais baratos a cada renovação.',
    ]);
  });
});

describe('Assinatura, the menu of lines', () => {
  it('asks for the current month while the screen shows another, and again after a write', async () => {
    const api = createFakeApi({ startLocked: false });
    const { months } = recordLineMenu(api, { named: true });
    const mounted = await mountWithFailures(AssinaturaScreen, routes, '/assinaturas/sp', { api, frame: false });
    const month = useMonthStore(mounted.pinia);
    month.configure(prototypeOverview());
    month.shift(-1);
    useShellStore(mounted.pinia).overview = prototypeOverview();
    await mounted.settle();
    expect(month.selected).toBe('2026-10');
    expect(months).toEqual(['2026-11']);
    const overlay = useOverlayStore(mounted.pinia);
    const menu = async () => {
      await mounted.wrapper.find('.line-button').trigger('click');
      return overlay.menu!.items.map((item) => item.label);
    };
    expect(await menu()).toEqual([lineNamed('2026-11', 1)]);

    useRevisionStore(mounted.pinia).bump();
    await mounted.settle();
    expect(months).toEqual(['2026-11', '2026-11']);
    expect(await menu()).toEqual([lineNamed('2026-11', 2)]);
  });
});
