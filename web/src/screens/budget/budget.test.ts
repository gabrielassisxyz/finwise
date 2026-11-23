import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { createFakeApi } from '@/api/fake';
import type { BudgetDoc, FinwiseApi } from '@/api/types';
import { useRevisionStore } from '@/stores/revision';
import { holdFirstCall } from '@/test/holdCall';
import BudgetScreen from './BudgetScreen.vue';
import { mountBudgetAt, plain, typeInto } from './budgetTestSupport';

const mountBudget = () => mountBudgetAt('/budget', BudgetScreen);

type Mounted = Awaited<ReturnType<typeof mountBudget>>;
const groupNamed = (m: Mounted, name: string) => m.wrapper.findAll('section.group').find((g) => plain(g.find('.title').text()) === `▾ ${name}` || plain(g.find('.title').text()) === `▸ ${name}`)!;
const rowOf = (m: Mounted, group: string, line: string) =>
  groupNamed(m, group)
    .findAll('.row')
    .find((row) => plain(row.find('.name-cell').text()).replace(/^[▸▾]\s*/, '').startsWith(line))!;
const editRow = (m: Mounted, group: string, name: string) =>
  groupNamed(m, group)
    .findAll('.row')
    .find((row) => (row.find('.name-input').element as HTMLInputElement).value === name)!;
const cells = (row: ReturnType<typeof rowOf>) =>
  plain(row.findAll(':scope > span').map((cell) => plain(cell.text())).join(' '));
const button = (m: Mounted, label: string) => m.wrapper.findAll('button').find((b) => b.text() === label)!;

describe('Budget screen', () => {
  it('shows the three figures, then the groups in order', async () => {
    const m = await mountBudget();
    expect(m.wrapper.findAll('.fig').map((cell) => plain(cell.text()))).toEqual([
      'ReceitaR$ 12.000,00',
      'DestinadoR$ 12.000,00',
      'Taxa de investimento32%',
    ]);
    expect(m.wrapper.findAll('section.group .title').map((t) => plain(t.text()))).toEqual([
      '▾ Receita',
      '▾ Fixos e essenciais',
      '▾ Variáveis',
      '▾ Provisões',
      '▾ Reservas de desejo',
      '▾ Rainy Funds',
      '▾ Tarifas e juros',
      '▾ Investimento',
    ]);
  });

  it('writes each group with its own columns and subtotal', async () => {
    const m = await mountBudget();
    const columns = (name: string) =>
      groupNamed(m, name)
        .find('.columns')
        .findAll('span')
        .map((s) => s.text())
        .filter(Boolean);
    expect(columns('Receita')).toEqual(['Linha', 'Esperado', 'Recebido']);
    expect(columns('Variáveis')).toEqual(['Linha', 'Teto', 'Gasto', 'Resta']);
    expect(columns('Provisões')).toEqual(['Linha', 'Aporte mensal', 'Saldo', 'Próxima cobrança']);
    expect(columns('Rainy Funds')).toEqual(['Linha', 'Alvo', 'Saldo', 'Aporte']);
    expect(plain(groupNamed(m, 'Variáveis').find('.subtotal').text())).toBe('Subtotal R$ 1.460,00 de R$ 2.880,00');
    expect(plain(groupNamed(m, 'Provisões').find('.subtotal').text())).toBe('Aporte mensal R$ 320,00');
    expect(plain(groupNamed(m, 'Rainy Funds').find('.subtotal').text())).toBe('Saldo R$ 11.200,00');
    expect(plain(groupNamed(m, 'Investimento').find('.subtotal').text())).toBe('O que sobra da receita R$ 3.800,00');
  });

  it('marks the variable line that went over, in red, with a full bar', async () => {
    const m = await mountBudget();
    const delivery = rowOf(m, 'Variáveis', 'Delivery');
    expect(plain(delivery.find('.name-cell').text())).toBe('▸DeliveryEstourou');
    expect(cells(delivery)).toContain('R$ 200,00 R$ 206,00 − R$ 6,00');
    expect(delivery.find('.mark.bad').exists()).toBe(true);
    expect(delivery.find('.fill').classes()).toContain('over');
    expect(delivery.find('.fill').attributes('style')).toContain('width: 100%');
    expect(rowOf(m, 'Variáveis', 'Mercado').find('.fill').classes()).not.toContain('over');
  });

  it('colours what is over in a fixed line red and heavy, and what is not quiet', async () => {
    const m = await mountBudget();
    const over = rowOf(m, 'Fixos e essenciais', 'Contas da casa').findAll('.value')[2]!;
    expect(over.classes()).toEqual(expect.arrayContaining(['bad', 'strong']));
    expect(rowOf(m, 'Fixos e essenciais', 'Saúde').findAll('.value')[2]!.classes()).toContain('quiet');
  });

  it('turns the subtotal of a variable group red once it spent more than its caps', async () => {
    const m = await mountBudget();
    expect(groupNamed(m, 'Variáveis').find('strong').classes()).not.toContain('bad');
    await button(m, 'Editar budget').trigger('click');
    await flushPromises();
    for (const name of ['Mercado', 'Restaurantes e bares']) {
      await typeInto(editRow(m, 'Variáveis', name).findAll('input')[1]!, '10');
    }
    for (const name of ['Delivery', 'Transporte', 'Casa', 'Assinaturas', 'Ferramentas de trabalho', 'Lazer', 'Educação']) {
      await typeInto(editRow(m, 'Variáveis', name).findAll('input')[1]!, '10');
    }
    expect(groupNamed(m, 'Variáveis').find('strong').classes()).toContain('bad');
  });

  it('says a rainy fund is back on below its target and off at it', async () => {
    const m = await mountBudget();
    expect(cells(rowOf(m, 'Rainy Funds', 'Pets'))).toBe('Pets R$ 5.000,00 R$ 4.200,00 Religado');
    expect(cells(rowOf(m, 'Rainy Funds', 'Saúde'))).toContain('Desligado');
    expect(rowOf(m, 'Rainy Funds', 'Pets').findAll('.value')[2]!.classes()).toContain('warning');
    expect(rowOf(m, 'Rainy Funds', 'Saúde').findAll('.value')[2]!.classes()).not.toContain('warning');
  });

  it('labels every value of a row for the phone card', async () => {
    const m = await mountBudget();
    const row = rowOf(m, 'Fixos e essenciais', 'Saúde');
    expect(row.classes()).toContain('stack-row');
    expect(row.findAll('[data-label]').map((cell) => cell.attributes('data-label'))).toEqual(['Esperado', 'Gasto', 'Diferença']);
  });

  it('collapses a group on its title and keeps it collapsed', async () => {
    const m = await mountBudget();
    await groupNamed(m, 'Variáveis').find('.head').trigger('click');
    expect(groupNamed(m, 'Variáveis').findAll('.row')).toHaveLength(0);
    expect(plain(groupNamed(m, 'Variáveis').find('.title').text())).toBe('▸ Variáveis');
    expect(groupNamed(m, 'Fixos e essenciais').findAll('.row')).toHaveLength(4);
    await groupNamed(m, 'Variáveis').find('.head').trigger('click');
    expect(groupNamed(m, 'Variáveis').findAll('.row')).toHaveLength(9);
  });

  it('opens a line into its establishments, and closes it on a second click', async () => {
    const m = await mountBudget();
    await rowOf(m, 'Variáveis', 'Delivery').trigger('click');
    await flushPromises();
    expect(groupNamed(m, 'Variáveis').findAll('.establishment').map((e) => plain(e.text()))).toEqual([
      'iFood ›R$ 133,30 (2)',
      'Rappi ›R$ 72,70 (1)',
    ]);
    await rowOf(m, 'Variáveis', 'Delivery').trigger('click');
    expect(m.wrapper.findAll('.establishment')).toHaveLength(0);
  });

  it('does not open a provision, which has no establishments of its own', async () => {
    const m = await mountBudget();
    const row = rowOf(m, 'Provisões', 'Gás e IPTU');
    expect(row.attributes('role')).toBeUndefined();
    await row.trigger('click');
    expect(m.wrapper.findAll('.establishment')).toHaveLength(0);
  });

  it('says so when a line has nothing in the month', async () => {
    const m = await mountBudget();
    m.month.shift(1);
    await flushPromises();
    await rowOf(m, 'Variáveis', 'Delivery').trigger('click');
    await flushPromises();
    expect(plain(m.wrapper.find('.empty').text())).toBe(
      'Nenhuma transação nesta linha em dezembro de 2026. Elas aparecem aqui assim que forem classificadas.',
    );
  });

  it('goes to the home screen filtered by the establishment that was clicked', async () => {
    const m = await mountBudget();
    await rowOf(m, 'Variáveis', 'Delivery').trigger('click');
    await flushPromises();
    await m.wrapper.find('.establishment').trigger('click');
    await flushPromises();
    expect(m.router.currentRoute.value.path).toBe('/');
    expect(m.router.currentRoute.value.query).toEqual({ establishment: 'iFood' });
  });

  it('on a closed month hides the buttons and says the budget is closed', async () => {
    const m = await mountBudget();
    expect(button(m, 'Editar budget')).toBeDefined();
    m.month.shift(-1);
    await flushPromises();
    expect(button(m, 'Editar budget')).toBeUndefined();
    expect(button(m, 'Cenários')).toBeUndefined();
    expect(m.wrapper.find('.notice').text()).toBe('Budget de outubro de 2026 · fechado');
  });
});

describe('Budget editing', () => {
  async function editing() {
    const m = await mountBudget();
    await button(m, 'Editar budget').trigger('click');
    await flushPromises();
    return m;
  }

  it('turns names and amounts into inputs, with the buttons of a line and one to add', async () => {
    const m = await editing();
    expect(button(m, 'Cenários')).toBeUndefined();
    expect(button(m, 'Cancelar')).toBeDefined();
    const row = rowOf(m, 'Fixos e essenciais', '');
    expect(row.findAll('input')).toHaveLength(2);
    expect(row.findAll('button').map((b) => b.attributes('aria-label'))).toEqual(['Subir', 'Descer', 'Remover linha']);
    expect(groupNamed(m, 'Fixos e essenciais').find('.add').text()).toBe('+ Nova linha');
    expect(groupNamed(m, 'Rainy Funds').find('.add').exists()).toBe(false);
    expect(rowOf(m, 'Rainy Funds', 'Pets').find('input').exists()).toBe(false);
  });

  it('shows the figures the draft would have, without saving it', async () => {
    const m = await editing();
    const extra = editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!;
    const before = await m.api.budget.document('2026-11');
    await typeInto(extra, '0');
    expect(m.wrapper.find('.fig .positive').text()).toBe('22%');
    expect(await m.api.budget.document('2026-11')).toEqual(before);
  });

  it('shows the rate in red when the draft outgrows the income', async () => {
    const m = await editing();
    await typeInto(editRow(m, 'Receita', 'Salário').findAll('input')[1]!, '100');
    expect(m.wrapper.find('.fig .value.bad').exists()).toBe(true);
    expect(m.wrapper.text()).toContain('Budget maior que a receita');
  });

  it('adds a line, and moves one up and down', async () => {
    const m = await editing();
    await groupNamed(m, 'Reservas de desejo').find('.add').trigger('click');
    const names = () => groupNamed(m, 'Reservas de desejo').findAll('.name-input').map((i) => (i.element as HTMLInputElement).value);
    expect(names()).toEqual(['Compras', 'Viagens', 'Nova linha']);
    await groupNamed(m, 'Reservas de desejo').findAll('button[aria-label="Subir"]')[1]!.trigger('click');
    expect(names()).toEqual(['Viagens', 'Compras', 'Nova linha']);
    await groupNamed(m, 'Reservas de desejo').findAll('button[aria-label="Descer"]')[2]!.trigger('click');
    expect(names()).toEqual(['Viagens', 'Compras', 'Nova linha']);
    await groupNamed(m, 'Reservas de desejo').findAll('button[aria-label="Descer"]')[0]!.trigger('click');
    expect(names()).toEqual(['Compras', 'Viagens', 'Nova linha']);
  });

  it('asks before removing a line, and removes it only once confirmed', async () => {
    const m = await editing();
    await groupNamed(m, 'Variáveis').findAll('button[aria-label="Remover linha"]')[0]!.trigger('click');
    expect(m.overlay.confirmation?.message).toBe(
      'Remover a linha Mercado? As transações dela ficam sem categoria a partir de novembro de 2026.',
    );
    expect(m.overlay.confirmation?.confirmLabel).toBe('Remover linha');
    expect(groupNamed(m, 'Variáveis').findAll('.row')).toHaveLength(9);
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect(groupNamed(m, 'Variáveis').findAll('.row')).toHaveLength(8);
  });

  it('cancels without saving anything', async () => {
    const m = await editing();
    await typeInto(editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!, '0');
    await button(m, 'Cancelar').trigger('click');
    await flushPromises();
    expect(button(m, 'Editar budget')).toBeDefined();
    expect(m.wrapper.find('.fig .positive').text()).toBe('32%');
  });

  it('asks before saving, then writes from the month on and leaves edit mode', async () => {
    const m = await editing();
    const revision = useRevisionStore();
    const before = revision.current;
    await typeInto(editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!, '0');
    await button(m, 'Salvar').trigger('click');
    expect(m.overlay.confirmation?.message).toBe('Vale a partir de novembro de 2026. Meses fechados não mudam.');
    expect(m.overlay.confirmation?.confirmLabel).toBe('Salvar');
    expect((await m.api.budget.document('2026-11'))[0]!.lines[1]!.amount).toBe(1500);
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect((await m.api.budget.document('2026-11'))[0]!.lines[1]!.amount).toBe(0);
    expect((await m.api.budget.document('2026-10'))[0]!.lines[1]!.amount).toBe(1500);
    expect(revision.current).toBe(before + 1);
    expect(button(m, 'Editar budget')).toBeDefined();
    expect(m.wrapper.find('.fig .positive').text()).toBe('22%');
  });

  it('leaves edit mode when the month changes', async () => {
    const m = await editing();
    m.month.shift(1);
    await flushPromises();
    expect(button(m, 'Editar budget')).toBeDefined();
    expect(m.wrapper.findAll('.name-input')).toHaveLength(0);
  });

  it('leaves the screen with the edit gone: a new visit starts from the saved budget', async () => {
    const m = await editing();
    await typeInto(editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!, '0');
    m.wrapper.unmount();
    const again = await mountBudget();
    expect(again.wrapper.find('.fig .positive').text()).toBe('32%');
  });
});

describe('Budget editing keeps each line its id', () => {
  const NOV = '2026-11';
  const plainCopy = (doc: BudgetDoc): BudgetDoc => JSON.parse(JSON.stringify(doc)) as BudgetDoc;

  /** A stand-in whose budget of the month answers an id on every line, and that records the drafts it is given. */
  async function withIds() {
    const api: FinwiseApi = createFakeApi();
    const doc = await api.budget.document(NOV);
    let next = 100;
    for (const group of doc) for (const line of group.lines) line.id = String(next++);
    await api.budget.save(NOV, doc);
    const viewed: BudgetDoc[] = [];
    const saved: BudgetDoc[] = [];
    const { view, save } = api.budget;
    api.budget.view = (month, draft) => (draft && viewed.push(plainCopy(draft)), view(month, draft));
    api.budget.save = (month, draft) => (saved.push(plainCopy(draft)), save(month, draft));
    return { api, doc, viewed, saved };
  }

  it('sends the view and the save each original line with its id and the added line with none', async () => {
    const { api, doc, viewed, saved } = await withIds();
    const m = await mountBudgetAt('/budget', BudgetScreen, { api });
    await button(m, 'Editar budget').trigger('click');
    await flushPromises();
    const wishes = () => groupNamed(m, 'Reservas de desejo');
    await typeInto(editRow(m, 'Reservas de desejo', 'Compras').findAll('input')[0]!, 'Compras online');
    await wishes().findAll('button[aria-label="Subir"]')[1]!.trigger('click');
    await typeInto(editRow(m, 'Reservas de desejo', 'Viagens').findAll('input')[1]!, '900');
    await wishes().find('.add').trigger('click');
    await flushPromises();

    const original = doc.find((group) => group.key === 'res')!.lines;
    const idOf = (name: string) => original.find((line) => line.name === name)!.id;
    const expectEdited = (draft: BudgetDoc) => {
      const lines = draft.find((group) => group.key === 'res')!.lines;
      expect(lines.map((line) => [line.id, line.name, line.amount])).toEqual([
        [idOf('Viagens'), 'Viagens', 900],
        [idOf('Compras'), 'Compras online', original.find((line) => line.name === 'Compras')!.amount],
        [undefined, 'Nova linha', 0],
      ]);
      expect('id' in lines[2]!).toBe(false);
      const others = (budget: BudgetDoc) => budget.filter((group) => group.key !== 'res');
      expect(others(draft)).toEqual(others(doc));
    };
    expectEdited(viewed.at(-1)!);

    await button(m, 'Salvar').trigger('click');
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect(saved).toHaveLength(1);
    expectEdited(saved[0]!);
  });
});

describe('Budget amounts in edit mode', () => {
  async function viewCallsAfterTyping(value: string) {
    const m = await mountBudget();
    await button(m, 'Editar budget').trigger('click');
    await flushPromises();
    const view = vi.spyOn(m.api.budget, 'view');
    await typeInto(editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!, value);
    const sent = view.mock.calls.map(([, draft]) => draft!.flatMap((group) => group.lines.map((line) => line.amount)));
    return { m, sent };
  }

  it.each([
    ['-50', 0],
    ['0', 0],
    ['150', 150],
    ['12.5', 12.5],
    ['', 0],
  ])('puts %j in the draft as %d', async (typed, amount) => {
    const { sent } = await viewCallsAfterTyping(typed);
    expect(sent.length).toBeGreaterThan(0);
    expect(sent.at(-1)).toContain(amount);
    expect(sent.flat().every((value) => value >= 0)).toBe(true);
  });

  it('carries 0 for the line that was given a negative', async () => {
    const { m } = await viewCallsAfterTyping('-50');
    expect((editRow(m, 'Receita', 'Renda extra').findAll('input')[1]!.element as HTMLInputElement).value).toBe('0');
  });
});

describe('Open line establishments out-of-order loads', () => {
  it('does not let an earlier establishments answer replace a later one for the same line', async () => {
    const m = await mountBudget();
    await rowOf(m, 'Variáveis', 'Delivery').trigger('click');
    await flushPromises();

    const first = holdFirstCall(m.api.budget, 'establishments');
    useRevisionStore().bump();
    await flushPromises();

    await m.api.transactions.setLine('t2', 'Delivery');
    useRevisionStore().bump();
    await flushPromises();
    const shown = () => groupNamed(m, 'Variáveis').findAll('.establishment').map((e) => plain(e.text()));
    expect(shown().some((entry) => entry.startsWith('Posto Shell'))).toBe(true);

    first.release();
    await flushPromises();
    expect(shown().some((entry) => entry.startsWith('Posto Shell'))).toBe(true);
  });
});
