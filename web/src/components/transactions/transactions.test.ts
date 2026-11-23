import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createFakeApi } from '@/api/fake';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { useShellStore } from '@/stores/shell';
import { prototypeState } from '@/api/fake/prototype-data';
import type { PendingReview } from '@/api/types';
import { holdFirstCall } from '@/test/holdCall';
import { recordLineMenu } from '@/test/lineMenu';
import { mountAt, prototypeOverview } from '@/test/mount';
import { FIRST_PENDING_ID, NO_SUGGESTION, stateWithPending } from '@/test/pendingStates';
import { mountWithFailures } from '@/test/readOnly';
import { marksOf } from './marks';
import TransactionList from './TransactionList.vue';

// Where the app's floating stack would receive the selection bar.
beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

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

describe('marksOf for a pending review', () => {
  const marksOver = async (pending: PendingReview) => {
    const list = await createFakeApi({ state: stateWithPending(pending) }).transactions.list({ month: '2026-11', period: 'month' });
    return marksOf(list.find((transaction) => transaction.id === FIRST_PENDING_ID)!);
  };

  it('names the reason where the confidence would be when there is none', async () => {
    expect(await marksOver(NO_SUGGESTION)).toEqual([{ text: 'Estabelecimento novo', tone: 'warning' }]);
    expect(await marksOver({ reason: 'Baixa confiança', suggestion: 'Casa', confidence: null, justification: null })).toEqual([
      { text: 'Baixa confiança', tone: 'warning' },
    ]);
  });

  it('keeps the confidence of a suggestion, with or without its justification', async () => {
    const pending = { reason: 'Baixa confiança', suggestion: 'Restaurantes e bares', confidence: 58 };
    expect(await marksOver({ ...pending, justification: null })).toEqual([{ text: 'IA 58%', tone: 'warning' }]);
    expect(await marksOver({ ...pending, justification: 'Porque sim.' })).toEqual([{ text: 'IA 58%', tone: 'warning' }]);
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
  const select = async (wrapper: Mounted['wrapper'], ...names: string[]) => {
    for (const name of names) await rowsNamed(wrapper, name)[0]!.find('.check').setValue(true);
  };
  const bar = () => document.querySelector('#float-above-toast .bar');
  const barButton = (label: string) =>
    [...document.querySelectorAll<HTMLButtonElement>('#float-above-toast .bar button')].find((button) => button.textContent === label)!;
  const click = async (button: HTMLElement) => {
    button.click();
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

  it('shows the bar for the selection, in the floating stack, and clears it with ×', async () => {
    const { wrapper } = await mountList();
    expect(bar()).toBeNull();

    await select(wrapper, 'Aluguel');
    expect(bar()!.querySelector('.count')!.textContent).toBe('1 selecionada');
    await select(wrapper, 'Posto Shell');
    expect(bar()!.querySelector('.count')!.textContent).toBe('2 selecionadas');
    expect([...bar()!.querySelectorAll('button')].map((button) => button.textContent)).toEqual([
      'Alterar linha',
      'Adicionar tag',
      'Marcar como não conta',
      '×',
    ]);

    await click(document.querySelector<HTMLElement>('[aria-label="Limpar seleção"]')!);
    expect(bar()).toBeNull();
    expect(wrapper.findAll<HTMLInputElement>('.row .check').filter((box) => box.element.checked)).toHaveLength(0);
  });

  it('changes the line of the selection as one change with one Desfazer', async () => {
    const { wrapper, overlay } = await mountList();
    await select(wrapper, 'Bar do Zé', 'Posto Shell');
    await click(barButton('Alterar linha'));
    expect(overlay.menu).toMatchObject({ search: true, placeholder: 'Buscar linha' });
    expect(overlay.menu!.items.some((item) => item.current)).toBe(false);

    overlay.menu!.items.find((item) => item.label === 'Lazer')!.onPick!();
    await flushPromises();

    expect(overlay.toast!.message).toBe('2 transações agora estão em Lazer.');
    expect(toastActions(overlay)).toEqual(['Desfazer']);
    expect(bar()).toBeNull();
    expect(pillOf(wrapper, 'Bar do Zé')).toBe('Lazer');
    expect(useShellStore().overview!.pendingCount).toBe(6);

    await runToastAction(overlay, 'Desfazer');
    expect(pillOf(wrapper, 'Posto Shell')).toBe('Transporte');
    expect(pillOf(wrapper, 'Bar do Zé')).toBe('Restaurantes e bares?');
    expect(useShellStore().overview!.pendingCount).toBe(7);
  });

  it('adds a tag to the selection from a titled menu of the known tags', async () => {
    const { wrapper, overlay, api } = await mountList();
    await select(wrapper, 'Aluguel', 'Posto Shell');
    await click(barButton('Adicionar tag'));

    expect(overlay.menu!.title).toBe('Adicionar tag');
    expect(overlay.menu!.search).toBeUndefined();
    expect(overlay.menu!.items.map((item) => item.label)).toEqual((await api.transactions.filterOptions('2026-11')).tags);

    overlay.menu!.items.find((item) => item.label === '#viagem')!.onPick!();
    await flushPromises();
    expect(overlay.toast!.message).toBe('Tag #viagem adicionada a 2 transações.');
    expect(rowsNamed(wrapper, 'Posto Shell')[0]!.text()).toContain('#viagem');
    expect(bar()).toBeNull();
  });

  it('marks the selection as not counting', async () => {
    const { wrapper, overlay, api } = await mountList();
    const excludeMany = vi.spyOn(api.transactions, 'excludeMany');
    await select(wrapper, 'Aluguel', 'Posto Shell');
    await click(barButton('Marcar como não conta'));
    expect(excludeMany).toHaveBeenCalledWith(['t34', 't2'], 'Marcada em lote');

    expect(overlay.toast!.message).toBe('2 transações marcadas como não conta.');
    expect(toastActions(overlay)).toEqual(['Desfazer']);
    expect(wrapper.findAll('.row.faded')).toHaveLength(2);

    await runToastAction(overlay, 'Desfazer');
    expect(wrapper.findAll('.row.faded')).toHaveLength(0);
  });
});

describe('the menu of lines the list draws from', () => {
  const NOV = '2026-11';

  it('is asked for the month of the list\'s query, whatever the period, and for the new month after a change', async () => {
    const api = createFakeApi({ startLocked: false });
    const { months } = recordLineMenu(api);
    const mounted = await mountWithFailures(TransactionList, [{ path: '/', component: { template: '<div />' } }], '/', { api });
    const overlay = useOverlayStore(mounted.pinia);
    const pickPeriod = async (label: string) => {
      await mounted.wrapper.findAll('.filter')[0]!.trigger('click');
      overlay.menu!.items.find((item) => item.label === label)!.onPick!();
      await mounted.settle();
    };
    expect(months).toEqual([NOV]);
    await pickPeriod('Últimos 3 meses');
    await pickPeriod('Este ano');
    await pickPeriod('Mês selecionado');
    expect(months).toEqual([NOV, NOV, NOV, NOV]);
    useMonthStore(mounted.pinia).shift(-1);
    await mounted.settle();
    expect(months.at(-1)).toBe('2026-10');
  });
});

describe('the filter by line', () => {
  const OFF_BUDGET = 'Combustível de antes';

  /** Two of the month's rows on a line that a later budget renamed: the month's budget no longer has it. */
  function withRenamedLine() {
    const state = prototypeState();
    for (const name of ['Posto Shell', 'Cinema Belas Artes']) {
      state.transactions.find((transaction) => transaction.name === name && transaction.day.startsWith('2026-11'))!.line = OFF_BUDGET;
    }
    return state;
  }

  async function mountWith(state = prototypeState()) {
    const mounted = await mountAt(TransactionList, '/', {}, { state });
    useShellStore().overview = prototypeOverview();
    useMonthStore().configure(prototypeOverview());
    await flushPromises();
    const overlay = useOverlayStore();
    const openFilter = async () => {
      await mounted.wrapper.findAll('.filter')[2]!.trigger('click');
      return overlay.menu!.items;
    };
    const menuLines = async () => (await mounted.api.transactions.filterOptions('2026-11')).lines.map((line) => line.name);
    return { ...mounted, overlay, openFilter, menuLines };
  }

  it('offers a line a row is on and the menu lacks, once, after every line of the menu in its order', async () => {
    const m = await mountWith(withRenamedLine());
    const menu = await m.menuLines();
    expect(menu).not.toContain(OFF_BUDGET);
    expect((await m.openFilter()).map((item) => item.label)).toEqual([...menu, OFF_BUDGET]);
  });

  it('leaves exactly the rows on that line when it is picked, and keeps offering it while the filter is on', async () => {
    const m = await mountWith(withRenamedLine());
    (await m.openFilter()).find((item) => item.label === OFF_BUDGET)!.onPick!();
    await flushPromises();
    expect(rowNames(m.wrapper).sort()).toEqual(['Cinema Belas Artes', 'Posto Shell']);
    const again = await m.openFilter();
    expect(again.filter((item) => item.label === OFF_BUDGET)).toHaveLength(1);
    expect(again.find((item) => item.current)?.label).toBe(OFF_BUDGET);

    again.find((item) => item.label === 'Delivery')!.onPick!();
    await flushPromises();
    expect(rowNames(m.wrapper)).toEqual(['iFood', 'Rappi', 'iFood']);
    expect((await m.openFilter()).map((item) => item.label)).toContain(OFF_BUDGET);
  });

  it('lists the menu and nothing more when every row is on a line of the menu', async () => {
    const m = await mountWith();
    expect((await m.openFilter()).map((item) => item.label)).toEqual(await m.menuLines());
  });
});

describe('TransactionList out-of-order loads', () => {
  it('keeps the later rows and filter options together and clears loading when an earlier load answers late', async () => {
    const { wrapper, overlay, api } = await mountList();
    const first = holdFirstCall(api.transactions, 'list');

    await wrapper.findAll('.filter')[0]!.trigger('click');
    overlay.menu!.items.find((item) => item.label === 'Últimos 3 meses')!.onPick!();
    await flushPromises();
    expect(wrapper.find('.skeleton').exists()).toBe(true);

    await api.transactions.setLine('t2', 'Lazer');
    const doc = await api.budget.document('2026-11');
    doc.find((group) => group.key === 'var')!.lines.push({ name: 'Linha nova', amount: 100 });
    await api.budget.save('2026-11', doc);
    useRevisionStore().bump();
    await flushPromises();

    expect(wrapper.find('.skeleton').exists()).toBe(false);
    const postoPill = () =>
      wrapper
        .findAll('.row')
        .find((row) => row.find('.name').text() === 'Posto Shell')!
        .find('.pill .label')
        .text();
    const lineLabels = async () => {
      await wrapper.findAll('.filter')[2]!.trigger('click');
      return overlay.menu!.items.map((item) => item.label);
    };
    expect(postoPill()).toBe('Lazer');
    expect(await lineLabels()).toContain('Linha nova');

    first.release();
    await flushPromises();
    expect(wrapper.find('.skeleton').exists()).toBe(false);
    expect(postoPill()).toBe('Lazer');
    expect(await lineLabels()).toContain('Linha nova');
  });
});
