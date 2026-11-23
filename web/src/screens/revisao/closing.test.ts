import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useReviewViewStore } from '@/stores/review-view';
import { useRevisionStore } from '@/stores/revision';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import ClosingTab from './ClosingTab.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/revisao', component: Blank },
  { path: '/revisao/fechamento', component: Blank },
  { path: '/contas/cartao', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

function answered(transaction: ReturnType<typeof prototypeState>['transactions'][number]): void {
  Object.assign(transaction, { line: transaction.pending?.suggestion ?? null, by: 'Você', pending: null });
}

function stateWithoutPending() {
  const state = prototypeState();
  state.transactions.filter((transaction) => transaction.pending).forEach(answered);
  return state;
}

async function mountClosing(state = prototypeState()) {
  const mounted = await mountRouted(ClosingTab, routes, '/revisao/fechamento', { state });
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore(), view: useReviewViewStore(), month: useMonthStore() };
}

type Mounted = Awaited<ReturnType<typeof mountClosing>>;

const stepOf = (wrapper: Mounted['wrapper'], title: string) =>
  wrapper.findAll('.step').find((step) => step.find('.title').text() === title)!;
const open = async (wrapper: Mounted['wrapper'], title: string) => {
  await stepOf(wrapper, title).find('.head').trigger('click');
  await flushPromises();
};
const buttonIn = (root: { findAll: Mounted['wrapper']['findAll'] }, text: string) =>
  root.findAll('button').find((candidate) => plain(candidate.text()) === text)!;
const click = async (target: { trigger: (event: string) => Promise<void> }) => {
  await target.trigger('click');
  await flushPromises();
};
const undo = async (overlay: Mounted['overlay']) => {
  overlay.toast!.actions.find((action) => action.label === 'Desfazer')!.run();
  await flushPromises();
};

describe('Fechamento, the accordion', () => {
  it('lists the eight steps with the first one open and pending', async () => {
    const { wrapper } = await mountClosing();
    const heads = wrapper.findAll('.step').map((step) => `${step.find('.number').text()} ${step.find('.title').text()} ${step.find('.status').text()}`.trim());
    expect(heads).toEqual([
      '1 Pendências Pendente',
      '2 Orçado × realizado',
      '3 Sobra do mês',
      '4 Aportes',
      '5 Fatura',
      '6 Assinaturas a revisar',
      '7 Relatório do mês',
      '8 Fechar o mês',
    ]);
    expect(wrapper.findAll('.body')).toHaveLength(1);
    expect(stepOf(wrapper, 'Pendências').find('.body').text()).toContain('7 pendências · resolver antes de fechar');
  });

  it('opens one step at a time and folds the open one when it is clicked again', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Sobra do mês');
    expect(wrapper.findAll('.body')).toHaveLength(1);
    expect(stepOf(wrapper, 'Sobra do mês').find('.chevron').text()).toBe('▾');
    expect(stepOf(wrapper, 'Pendências').find('.chevron').text()).toBe('▸');
    await open(wrapper, 'Sobra do mês');
    expect(wrapper.findAll('.body')).toHaveLength(0);
  });

  it('offers "Marcar como feito e seguir" on steps 2 to 7 only, and opens the next step with it', async () => {
    const { wrapper } = await mountClosing();
    expect(wrapper.text()).not.toContain('Marcar como feito e seguir');
    await open(wrapper, 'Orçado × realizado');
    await click(buttonIn(stepOf(wrapper, 'Orçado × realizado'), 'Marcar como feito e seguir'));
    expect(stepOf(wrapper, 'Orçado × realizado').find('.status').text()).toBe('Feito');
    expect(stepOf(wrapper, 'Orçado × realizado').find('.number').text()).toBe('✓');
    expect(stepOf(wrapper, 'Orçado × realizado').find('.body').exists()).toBe(false);
    expect(stepOf(wrapper, 'Sobra do mês').find('.body').exists()).toBe(true);
    await open(wrapper, 'Orçado × realizado');
    expect(stepOf(wrapper, 'Orçado × realizado').text()).not.toContain('Marcar como feito e seguir');
    await open(wrapper, 'Fechar o mês');
    expect(stepOf(wrapper, 'Fechar o mês').text()).not.toContain('Marcar como feito e seguir');
  });
});

describe('Fechamento, what each step shows', () => {
  it('says how many reviews wait, with a way to the weekly tab', async () => {
    const { wrapper, router } = await mountClosing();
    const step = stepOf(wrapper, 'Pendências');
    await click(buttonIn(step, 'Resolver na revisão semanal'));
    expect(router.currentRoute.value.path).toBe('/revisao');
  });

  it('says "1 pendência" for one, and finishes by itself with a "Feito" and no button when none waits', async () => {
    const one = prototypeState();
    for (const transaction of one.transactions.filter((candidate) => candidate.pending).slice(1)) answered(transaction);
    const single = await mountClosing(one);
    expect(stepOf(single.wrapper, 'Pendências').find('.body').text()).toContain('1 pendência · resolver antes de fechar');

    const none = await mountClosing(stateWithoutPending());
    const step = stepOf(none.wrapper, 'Pendências');
    expect(step.find('.status').text()).toBe('Feito');
    expect(step.find('.body').text()).toBe('Nenhuma pendência');
  });

  it('writes planned, actual and difference per group and line, the negative ones in red', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Orçado × realizado');
    const rows = wrapper.findAll('.table .row').map((row) => row.findAll('span').map((span) => plain(span.text())).join(' | '));
    expect(rows[0]).toBe('Receita | R$ 12.000,00 | R$ 10.350,00 | − R$ 1.650,00');
    expect(rows[1]).toBe('Salário | R$ 10.000,00 | R$ 10.000,00 | R$ 0,00');
    expect(rows.find((row) => row.startsWith('Contas da casa'))).toBe('Contas da casa | R$ 2.000,00 | R$ 2.059,90 | − R$ 59,90');
    expect(rows.find((row) => row.startsWith('Provisões'))).toBe('Provisões (aportes) | R$ 320,00 | R$ 320,00 | R$ 0,00');
    expect(rows.find((row) => row.startsWith('Reservas de desejo'))).toBe('Reservas de desejo (aportes) | R$ 1.100,00 | R$ 0,00 | + R$ 1.100,00');
    const bad = wrapper.findAll('.table .row').filter((row) => row.find('.right.bad').exists()).length;
    expect(bad).toBeGreaterThan(3);
    const contas = wrapper.findAll('.table .row').find((row) => row.text().startsWith('Contas da casa'))!;
    expect(contas.findAll('.right')[2]!.classes()).toContain('bad');
    expect(wrapper.findAll('.table .row').find((row) => row.text().startsWith('Pets'))!.findAll('.right')[2]!.classes()).not.toContain('bad');
  });

  it('tells what is left, or by how much the variable lines went over', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Sobra do mês');
    expect(plain(stepOf(wrapper, 'Sobra do mês').find('.text').text())).toBe('Sobraram R$ 1.420,00 nas linhas variáveis. Destino: Investimento.');

    const state = prototypeState();
    state.spentBaseline['2026-11']!['Mercado'] = 2000;
    const over = await mountClosing(state);
    await open(over.wrapper, 'Sobra do mês');
    expect(plain(stepOf(over.wrapper, 'Sobra do mês').find('.text').text())).toBe(
      'As linhas variáveis passaram R$ 272,00 do total. O valor sai do Investimento do mês.',
    );
  });

  it('says "Sobraram R$ 0,00" when the variable lines spent exactly the cap', async () => {
    const state = prototypeState();
    state.spentBaseline['2026-11']!['Mercado'] = 1728;
    const { wrapper } = await mountClosing(state);
    await open(wrapper, 'Sobra do mês');
    expect(plain(stepOf(wrapper, 'Sobra do mês').find('.text').text())).toBe('Sobraram R$ 0,00 nas linhas variáveis. Destino: Investimento.');
  });

  it('lists each planned transfer as done, on target or not found', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Aportes');
    const rows = wrapper.findAll('.step .list .row').map((row) => `${row.find('span').text()}=${row.find('.state').text()}`);
    expect(rows).toEqual([
      'RF Casa=No alvo',
      'Gás e IPTU=Feito',
      'Assinaturas anuais=Feito',
      'Remédios=Feito',
      'Compras=Não encontrado',
      'Viagens=Não encontrado',
      'Investimento=Não encontrado',
    ]);
    expect(wrapper.findAll('.state.missing')).toHaveLength(3);
  });

  it('draws two transfers of one name as two rows, each with its own state, also once the list is reordered', async () => {
    const state = prototypeState();
    state.plannedContributions = [
      { key: 'pot:reserva-casa', name: 'Reserva', amount: 240, state: 'done', doneOn: '2026-11-05' },
      { key: 'pot:reserva-viagem', name: 'Reserva', amount: 60, state: 'pending', doneOn: null },
      { key: 'line:viagens', name: 'Viagens', amount: 600, state: 'pending', doneOn: null },
    ];
    const { wrapper } = await mountClosing(state);
    await open(wrapper, 'Aportes');
    const rows = () => stepOf(wrapper, 'Aportes').findAll('.list .row').map((row) => `${row.find('span').text()}=${row.find('.state').text()}`);
    expect(rows()).toEqual(['Reserva=Feito', 'Reserva=Não encontrado', 'Viagens=Não encontrado']);

    const [house, trip, travels] = state.plannedContributions;
    state.plannedContributions = [travels!, trip!, house!];
    useRevisionStore().bump();
    await flushPromises();
    expect(rows()).toEqual(['Viagens=Não encontrado', 'Reserva=Não encontrado', 'Reserva=Feito']);
  });

  it('shows the amount to pay ahead, the purchases behind it and a link to the card', async () => {
    const { wrapper, router } = await mountClosing();
    await open(wrapper, 'Fatura');
    const step = stepOf(wrapper, 'Fatura');
    expect(plain(step.find('.body > span').text())).toBe('Valor a adiantar: R$ 386,40');
    expect(step.findAll('.list .row').map((row) => plain(row.text()))).toEqual([
      'Cinema Belas Artes · 28/11R$ 65,10',
      'Posto Shell · 28/11R$ 180,00',
      'iFood · 28/11R$ 54,90',
      'Pão de Açúcar · 28/11R$ 86,40',
    ]);
    await click(buttonIn(step, 'Abrir C6 Carbon · final 1234 ›'));
    expect(router.currentRoute.value.path).toBe('/contas/cartao');
  });
});

describe('Fechamento, the subscriptions to review', () => {
  const names = (wrapper: Mounted['wrapper']) => stepOf(wrapper, 'Assinaturas a revisar').findAll('.row .name').map((name) => name.text());

  it('lists those whose review came due, with amount, cycle and date', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Assinaturas a revisar');
    expect(names(wrapper)).toEqual(['Amazon Prime', 'Revista digital']);
    expect(plain(stepOf(wrapper, 'Assinaturas a revisar').find('.sub').text())).toBe('R$ 166,80 · anual · revisar em 30/11/2026');
  });

  it('keeps one, schedules the next review and takes it back with Desfazer', async () => {
    const { wrapper, overlay } = await mountClosing();
    await open(wrapper, 'Assinaturas a revisar');
    await click(stepOf(wrapper, 'Assinaturas a revisar').find('.keep'));
    expect(overlay.toast?.message).toBe('Amazon Prime: mantida. Nova revisão em 30/05/2027.');
    expect(names(wrapper)).toEqual(['Revista digital']);
    await undo(overlay);
    expect(names(wrapper)).toEqual(['Amazon Prime', 'Revista digital']);
  });

  it('marks one for cancelling, and says none are due once all were decided', async () => {
    const { wrapper, overlay } = await mountClosing();
    await open(wrapper, 'Assinaturas a revisar');
    await click(stepOf(wrapper, 'Assinaturas a revisar').find('.cancel'));
    expect(overlay.toast?.message).toBe('Amazon Prime: marcada para cancelar.');
    await click(stepOf(wrapper, 'Assinaturas a revisar').find('.cancel'));
    expect(stepOf(wrapper, 'Assinaturas a revisar').find('.none').text()).toBe(
      'Nenhuma assinatura com revisão vencida. Elas entram aqui quando a data de “Revisar em” chega.',
    );
  });
});

describe('Fechamento, the report', () => {
  it('has the five blocks, the footer, and two buttons that send a cap for approval', async () => {
    const { wrapper } = await mountClosing();
    await open(wrapper, 'Relatório do mês');
    const step = stepOf(wrapper, 'Relatório do mês');
    expect(step.findAll('.heading').map((heading) => heading.text())).toEqual([
      'Orçado × realizado',
      'Onde dá para cortar',
      'Fora do padrão',
      'Sugestões de teto',
      'Comparado aos meses anteriores',
    ]);
    expect(step.find('.footer').text()).toBe('Escrito pela IA a partir dos dados deste mês.');
    expect(step.findAll('.send').map((button) => button.text())).toEqual([
      'Enviar para Aguardando seu ok',
      'Enviado para Aguardando seu ok',
    ]);
    expect(step.findAll('.send').map((button) => button.attributes('disabled') !== undefined)).toEqual([false, true]);
  });

  it('sends a suggestion: the button turns into "Enviado" and disables, and the change is waiting for approval', async () => {
    const { wrapper, overlay, api } = await mountClosing();
    await open(wrapper, 'Relatório do mês');
    await click(stepOf(wrapper, 'Relatório do mês').findAll('.send')[0]!);
    expect(overlay.toast?.message).toBe('Sugestão enviada para Aguardando seu ok.');
    const sends = stepOf(wrapper, 'Relatório do mês').findAll('.send');
    expect(sends.map((button) => button.text())).toEqual(['Enviado para Aguardando seu ok', 'Enviado para Aguardando seu ok']);
    expect((await api.review.weekly('2026-11')).approvals.map((approval) => approval.id)).toContain('lazer');
    await undo(overlay);
    expect(stepOf(wrapper, 'Relatório do mês').findAll('.send')[0]!.text()).toBe('Enviar para Aguardando seu ok');
  });
});

describe('Fechamento, closing the month', () => {
  it('blocks the button while reviews wait and says how many', async () => {
    const { wrapper, overlay } = await mountClosing();
    await open(wrapper, 'Fechar o mês');
    const step = stepOf(wrapper, 'Fechar o mês');
    expect(step.find('.text').text()).toBe('Resolva as 7 pendências antes de fechar o mês.');
    expect(step.find('.close').attributes('disabled')).toBeDefined();
    await step.find('.close').trigger('click');
    expect(overlay.confirmation).toBeNull();
  });

  it('asks before closing, and nothing changes when the question is refused', async () => {
    const { wrapper, overlay, api } = await mountClosing(stateWithoutPending());
    await open(wrapper, 'Fechar o mês');
    expect(stepOf(wrapper, 'Fechar o mês').find('.text').text()).toBe('Tudo pronto. Depois de fechado, o mês fica só para leitura.');
    await click(stepOf(wrapper, 'Fechar o mês').find('.close'));
    expect(overlay.confirmation).toMatchObject({
      message: 'Fechar novembro de 2026? O budget deste mês fica guardado e não muda mais.',
      confirmLabel: 'Fechar o mês',
    });
    overlay.closeConfirmation();
    expect((await api.shell.overview()).closedMonths.map((closed) => closed.month)).toEqual(['2026-10']);
  });

  it('closes the month once confirmed: every step turns done and the list gains the month', async () => {
    const { wrapper, overlay, api, view } = await mountClosing(stateWithoutPending());
    await open(wrapper, 'Fechar o mês');
    await click(stepOf(wrapper, 'Fechar o mês').find('.close'));
    overlay.confirmation!.onConfirm();
    await flushPromises();
    expect((await api.shell.overview()).closedMonths.map((closed) => closed.month)).toEqual(['2026-10', '2026-11']);
    expect(view.openStep).toBe(-1);
    expect(wrapper.findAll('.step .status').map((status) => status.text())).toEqual(Array(8).fill('Feito'));
    expect(wrapper.findAll('.entry .label').map((label) => label.text())).toEqual(['Novembro de 2026', 'Outubro de 2026']);
    expect(wrapper.findAll('.entry .sub').map((sub) => plain(sub.text()))[0]).toBe('Fechado em 28/11/2026 ›');
    await open(wrapper, 'Fechar o mês');
    const step = stepOf(wrapper, 'Fechar o mês');
    expect(step.find('.text').text()).toBe('Novembro de 2026 foi fechado em 28/11/2026. O budget deste mês está guardado e não muda mais.');
    expect(step.find('.close').exists()).toBe(false);
  });
});

describe('Fechamento, the closed months', () => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);

  it('lists October, closed on 02/11/2026', async () => {
    const { wrapper } = await mountClosing();
    expect(wrapper.find('.heading').text()).toBe('Meses fechados');
    expect(wrapper.findAll('.entry').map((entry) => plain(entry.text()))).toEqual(['Outubro de 2026Fechado em 02/11/2026 ›']);
  });

  it('switches the selected month on click and opens the step with the table', async () => {
    const { wrapper, month, view } = await mountClosing();
    await click(wrapper.find('.entry'));
    expect(month.selected).toBe('2026-10');
    expect(view.openStep).toBe(1);
    expect(stepOf(wrapper, 'Orçado × realizado').find('.body').exists()).toBe(true);
  });

  it('is read-only: every step done, no button to mark or to close, no decision to make', async () => {
    const { wrapper, month } = await mountClosing();
    month.selected = '2026-10';
    await flushPromises();
    expect(wrapper.findAll('.step .status').map((status) => status.text())).toEqual(Array(8).fill('Feito'));
    expect(wrapper.text()).not.toContain('Marcar como feito e seguir');
    await open(wrapper, 'Fechar o mês');
    expect(stepOf(wrapper, 'Fechar o mês').find('.text').text()).toBe(
      'Outubro de 2026 foi fechado em 02/11/2026. O budget deste mês está guardado e não muda mais.',
    );
    expect(wrapper.find('.close').exists()).toBe(false);
    await open(wrapper, 'Assinaturas a revisar');
    expect(wrapper.find('.keep').exists()).toBe(false);
    await open(wrapper, 'Relatório do mês');
    expect(wrapper.findAll('.send').every((button) => button.attributes('disabled') !== undefined)).toBe(true);
    await open(wrapper, 'Pendências');
    expect(wrapper.find('.resolve').exists()).toBe(false);
  });
});
