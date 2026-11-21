import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useShellStore } from '@/stores/shell';
import { mountAt, prototypeOverview } from '@/test/mount';
import { marksOf } from './marks';
import TransactionList from './TransactionList.vue';

async function mountList() {
  const mounted = await mountAt(TransactionList, '/');
  useShellStore().overview = prototypeOverview();
  const month = useMonthStore();
  month.configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, month, overlay: useOverlayStore() };
}

const rowNames = (wrapper: Awaited<ReturnType<typeof mountList>>['wrapper']) =>
  wrapper.findAll('.row .name').map((name) => name.text());

const tabCounts = (wrapper: Awaited<ReturnType<typeof mountList>>['wrapper']) =>
  wrapper.findAll('.tab').map((tab) => tab.text().replace(/\s+/g, ''));

describe('marksOf', () => {
  it('orders the badges and ends with the tags', async () => {
    const list = await createFakeApi().transactions.list({ month: '2026-11', period: 'month' });
    const byName = (name: string) => marksOf(list.find((transaction) => transaction.originalName === name)!);

    expect(byName('Cinema Belas Artes')).toEqual([{ text: 'Pendente', tone: 'warning' }]);
    expect(byName('Bar do Zé')).toEqual([{ text: 'IA 58%', tone: 'warning' }]);
    expect(byName('Serviço de nuvem')).toEqual([{ text: 'Internacional', tone: 'neutral' }]);
    expect(byName('Estorno de anuidade')).toEqual([{ text: 'Estorno', tone: 'positive' }]);
    expect(byName('Pagamento da fatura')).toEqual([{ text: 'Movimentação', tone: 'neutral' }]);
    expect(byName('Sal & Brasa')).toEqual([{ text: '#com-ana', tone: 'tag' }]);
  });

  it('shows the model confidence only under the threshold', async () => {
    const list = await createFakeApi().transactions.list({ month: '2026-11', period: 'month' });
    const confident = { ...list.find((transaction) => transaction.id === 't4')!, belowThreshold: false };
    expect(marksOf(confident)).toEqual([]);
  });
});

describe('TransactionList', () => {
  it('lists the month and counts each kind', async () => {
    const { wrapper } = await mountList();
    expect(rowNames(wrapper)).toHaveLength(40);
    expect(tabCounts(wrapper)).toEqual(['Todas40', 'PIX7', 'Boletos3', 'Cartão26', 'Outros4']);
    expect(wrapper.find('.foot').text()).toContain('40 transações');
    expect(wrapper.find('.total').exists()).toBe(false);
  });

  it('shows the nickname in place of the name', async () => {
    const { wrapper } = await mountList();
    expect(rowNames(wrapper)).toContain('Aluguel');
    expect(rowNames(wrapper)).not.toContain('João P. Santos');
  });

  it('narrows to a kind and renames the columns for it', async () => {
    const { wrapper } = await mountList();
    const [, , boletos, cartao, outros] = wrapper.findAll('.tab');

    await boletos!.trigger('click');
    expect(rowNames(wrapper)).toEqual(['Internet', 'Condomínio', 'Plano de saúde']);
    expect(wrapper.find('.col-description').text()).toBe('Beneficiário');

    await cartao!.trigger('click');
    expect(wrapper.find('.head').text()).toContain('Valor original');
    expect(wrapper.find('.col-account').exists()).toBe(false);
    expect(wrapper.find('.content').classes()).toContain('wide');

    await outros!.trigger('click');
    expect(wrapper.find('.head .col-kind').text()).toBe('Tipo');
    expect(wrapper.findAll('.kind-badge').map((badge) => badge.text())).toEqual(['Débito', 'TEF', 'TED', 'Tarifa']);
  });

  it('searches without accents, by the bank text, by tag and by amount', async () => {
    const { wrapper } = await mountList();
    const search = wrapper.find('.search');

    await search.setValue('acucar');
    expect(rowNames(wrapper)).toHaveLength(6);
    expect(wrapper.find('.total').text()).toBe('6 transações · R$ 212,00');

    await search.setValue('velox');
    expect(rowNames(wrapper)).toEqual(['Internet']);

    await search.setValue('#viagem');
    expect(rowNames(wrapper)).toEqual(['Pousada Serra Azul']);

    await search.setValue('86,40');
    expect(rowNames(wrapper)).toEqual(['Pão de Açúcar']);

    await search.setValue('joao p. santos');
    expect(rowNames(wrapper)).toEqual(['Aluguel']);
  });

  it('totals what is shown, leaving movements out and marking a net inflow', async () => {
    const { wrapper } = await mountList();
    await wrapper.findAll('.tab')[4]!.trigger('click');
    await wrapper.find('.search').setValue('t');
    expect(rowNames(wrapper)).toEqual(['Pet Shop Bicho Bom', 'Pagamento da fatura', 'Transferência para C6', 'Tarifa de pacote']);
    expect(wrapper.find('.total').text()).toBe('4 transações · R$ 121,00');

    await wrapper.findAll('.tab')[1]!.trigger('click');
    await wrapper.find('.search').setValue('recebido');
    expect(wrapper.find('.total').text()).toBe('3 transações · + R$ 10.350,00');
  });

  it('keeps only what waits for review, and only what has no line', async () => {
    const { wrapper } = await mountList();
    const toggles = wrapper.findAll('.filter').filter((button) => button.attributes('aria-pressed') !== undefined);

    await toggles[0]!.trigger('click');
    expect(rowNames(wrapper)).toHaveLength(7);
    expect(wrapper.findAll('.chip').map((chip) => chip.text())).toEqual(['Só pendentes×']);

    await wrapper.find('.chip-remove').trigger('click');
    expect(rowNames(wrapper)).toHaveLength(40);

    await toggles[1]!.trigger('click');
    expect(rowNames(wrapper)).toHaveLength(7);
    expect(rowNames(wrapper)).not.toContain('Pagamento da fatura');
  });

  it('filters by a line picked from the menu', async () => {
    const { wrapper, overlay } = await mountList();
    await wrapper.findAll('.filter')[2]!.trigger('click');
    expect(overlay.menu?.search).toBe(true);

    overlay.menu!.items.find((item) => item.label === 'Delivery')!.onPick!();
    await flushPromises();
    expect(rowNames(wrapper)).toEqual(['iFood', 'Rappi', 'iFood']);
    expect(wrapper.find('.chip').text()).toContain('Linha: Delivery');
  });

  it('fetches a wider period when one is picked', async () => {
    const { wrapper, overlay } = await mountList();
    await wrapper.findAll('.filter')[0]!.trigger('click');
    overlay.menu!.items.find((item) => item.label === 'Este ano')!.onPick!();
    await flushPromises();
    expect(rowNames(wrapper)).toHaveLength(45);
    expect(wrapper.find('.chip').text()).toContain('Período: este ano');
  });

  it('says why the list is empty', async () => {
    const { wrapper, month } = await mountList();
    await wrapper.find('.search').setValue('zzzz');
    expect(wrapper.find('.empty-title').text()).toBe('Nenhuma transação com esses filtros');

    await wrapper.find('.search').setValue('');
    month.shift(1);
    await flushPromises();
    expect(wrapper.find('.empty-title').text()).toBe('Nenhuma transação em dezembro de 2026 ainda');
    expect(wrapper.find('.empty-hint').text()).toContain('na manhã seguinte');
  });

  it('opens the tax line of a foreign purchase', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.find('.iof-row').exists()).toBe(false);
    await wrapper.find('.iof-toggle').trigger('click');
    expect(wrapper.find('.iof-row').text()).toContain('IOF · R$ 3,85');
  });

  it('selects every visible row and drops the selection when the view changes', async () => {
    const { wrapper } = await mountList();
    const checked = () => wrapper.findAll<HTMLInputElement>('.row .check').filter((box) => box.element.checked).length;

    await wrapper.find('.head .check').setValue(true);
    expect(checked()).toBe(40);

    await wrapper.findAll('.tab')[2]!.trigger('click');
    expect(checked()).toBe(0);
  });
});

describe('acting on the list', () => {
  type Mounted = Awaited<ReturnType<typeof mountList>>;

  const rowsNamed = (wrapper: Mounted['wrapper'], name: string) =>
    wrapper.findAll('.row').filter((row) => row.find('.name').text() === name);
  const pillOf = (wrapper: Mounted['wrapper'], name: string, nth = 0) => rowsNamed(wrapper, name)[nth]!.find('.pill .label').text();
  const toastActions = (overlay: Mounted['overlay']) => overlay.toast!.actions.map((action) => action.label);
  const runToastAction = async (overlay: Mounted['overlay'], label: string) => {
    overlay.toast!.actions.find((action) => action.label === label)!.run();
    await flushPromises();
  };
  const pickLine = async (mounted: Mounted, name: string, line: string, nth = 0) => {
    await rowsNamed(mounted.wrapper, name)[nth]!.find('.pill-button').trigger('click');
    mounted.overlay.menu!.items.find((item) => item.label === line)!.onPick!();
    await flushPromises();
  };

  it('opens a searchable menu of the budget lines from the pill, with the current one marked', async () => {
    const { wrapper, overlay } = await mountList();
    await rowsNamed(wrapper, 'Posto Shell')[0]!.find('.pill-button').trigger('click');

    expect(overlay.menu).toMatchObject({ search: true, placeholder: 'Buscar linha' });
    expect(overlay.menu!.items[0]).toMatchObject({ label: 'Mercado', hint: 'Variáveis', current: false });
    expect(overlay.menu!.items.filter((item) => item.current).map((item) => item.label)).toEqual(['Transporte']);
  });

  it('changes the line, asks about the establishment and keeps the rows on screen meanwhile', async () => {
    const mounted = await mountList();
    const { wrapper, overlay, api } = mounted;
    const list = api.transactions.list.bind(api.transactions);
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    vi.spyOn(api.transactions, 'list').mockImplementation(async (query) => (await held, list(query)));

    await pickLine(mounted, 'Posto Shell', 'Lazer');

    expect(wrapper.find('.skeleton').exists()).toBe(false);
    expect(rowNames(wrapper)).toHaveLength(40);
    expect(overlay.toast!.message).toBe('Linha alterada para Lazer. Aplicar a todas as compras em Posto Shell?');
    expect(toastActions(overlay)).toEqual(['Só esta', 'Todas', 'Desfazer']);

    release();
    await flushPromises();
    expect(pillOf(wrapper, 'Posto Shell')).toBe('Lazer');
  });

  it('does nothing when the same line is picked again', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'Posto Shell', 'Transporte');
    expect(mounted.overlay.toast).toBeNull();
  });

  it('dismisses the toast on "Só esta" and leaves the other purchases alone', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'iFood', 'Lazer');
    await runToastAction(mounted.overlay, 'Só esta');
    expect(mounted.overlay.toast).toBeNull();
    expect(pillOf(mounted.wrapper, 'iFood', 1)).toBe('Restaurantes e bares');
  });

  it('applies the line to the whole establishment and says how many changed', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'iFood', 'Lazer');
    await runToastAction(mounted.overlay, 'Todas');

    expect(mounted.overlay.toast!.message).toBe('Lazer agora vale para todas as compras em iFood: 3 alteradas.');
    expect(toastActions(mounted.overlay)).toEqual(['Desfazer']);
    expect(rowsNamed(mounted.wrapper, 'iFood').map((row) => row.find('.pill .label').text())).toEqual(['Lazer', 'Lazer', 'Lazer']);
  });

  it('says "alterada" when the establishment has no other purchase', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'Pousada Serra Azul', 'Lazer');
    await runToastAction(mounted.overlay, 'Todas');
    expect(mounted.overlay.toast!.message).toBe('Lazer agora vale para todas as compras em Pousada Serra Azul: 1 alterada.');
  });

  it('takes back both steps with one Desfazer', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'iFood', 'Lazer');
    await runToastAction(mounted.overlay, 'Todas');
    await runToastAction(mounted.overlay, 'Desfazer');

    expect(mounted.overlay.toast).toBeNull();
    expect(rowsNamed(mounted.wrapper, 'iFood').map((row) => row.find('.pill .label').text())).toEqual([
      'Delivery',
      'Restaurantes e bares',
      'Delivery',
    ]);
  });

  it('skips the question for a person and offers only Desfazer', async () => {
    const mounted = await mountList();
    await pickLine(mounted, 'Aluguel', 'Casa');
    expect(mounted.overlay.toast!.message).toBe('Linha alterada para Casa.');
    expect(toastActions(mounted.overlay)).toEqual(['Desfazer']);
  });

  it('lowers the pending count of the frame when a pending purchase gets a line', async () => {
    const mounted = await mountList();
    const shell = useShellStore();
    expect(shell.overview!.pendingCount).toBe(7);

    await pickLine(mounted, 'Bar do Zé', 'Restaurantes e bares');
    expect(shell.overview!.pendingCount).toBe(6);
    expect(mounted.wrapper.findAll('.row').filter((row) => row.text().includes('Pendente'))).toHaveLength(1);

    await runToastAction(mounted.overlay, 'Desfazer');
    expect(shell.overview!.pendingCount).toBe(7);
  });
});
