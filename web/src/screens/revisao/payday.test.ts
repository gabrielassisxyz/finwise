import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { holdFirstCall } from '@/test/holdCall';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import PaydayTab from './PaydayTab.vue';

const routes = [{ path: '/revisao/salario', component: { template: '<div />' } }];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountPayday(state = prototypeState()) {
  const mounted = await mountRouted(PaydayTab, routes, '/revisao/salario', { state });
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore(), revision: useRevisionStore() };
}

const figures = (wrapper: Awaited<ReturnType<typeof mountPayday>>['wrapper']) =>
  wrapper.findAll('.cell').map((cell) => cell.findAll('span').map((span) => plain(span.text())).join(' | '));
const rowOf = (wrapper: Awaited<ReturnType<typeof mountPayday>>['wrapper'], name: string) =>
  wrapper.findAll('.row').find((row) => row.find('.name').text() === name)!;

describe('Dia do salário', () => {
  it('shows the cushion, the balance and what is above it, with the advice', async () => {
    const { wrapper } = await mountPayday();
    expect(figures(wrapper)).toEqual([
      'Colchão necessário | R$ 6.780,00 | Fixos e essenciais + variáveis do mês',
      'Saldo em conta | R$ 7.020,00 | Itaú · Conta corrente',
      'Acima do colchão | R$ 240,00 | O salário pode ser guardado no dia em que chegar',
    ]);
    expect(wrapper.findAll('.value')[2]!.classes()).toContain('positive');
  });

  it('says what is missing, in red, when the balance falls short', async () => {
    const state = prototypeState();
    state.accounts.find((account) => account.id === 'itau')!.balance = 6000;
    const { wrapper } = await mountPayday(state);
    expect(figures(wrapper)[2]).toBe('Faltam para o colchão | R$ 780,00 | Complete o colchão antes de guardar');
    expect(wrapper.findAll('.value')[2]!.classes()).toContain('bad');
  });

  it('lists the transfers with their state, and offers the button only on a pending one', async () => {
    const { wrapper } = await mountPayday();
    const states = wrapper.findAll('.row').map((row) => [row.find('.name').text(), row.find('.state').text(), row.find('.mark').exists()]);
    expect(states).toEqual([
      ['RF Casa', 'no alvo', false],
      ['Gás e IPTU', 'Feita em 05/11', false],
      ['Assinaturas anuais', 'Feita em 05/11', false],
      ['Remédios', 'Feita em 05/11', false],
      ['Compras', 'Pendente', true],
      ['Viagens', 'Pendente', true],
      ['Investimento', 'Pendente', true],
    ]);
  });

  it('marks a transfer as done, says so in a toast and takes it back with Desfazer', async () => {
    const { wrapper, overlay, revision } = await mountPayday();
    const before = revision.current;
    await rowOf(wrapper, 'Compras').find('.mark').trigger('click');
    await flushPromises();
    expect(rowOf(wrapper, 'Compras').find('.state').text()).toBe('Feita em 28/11 · marcada por você');
    expect(rowOf(wrapper, 'Compras').find('.mark').exists()).toBe(false);
    expect(overlay.toast?.message).toBe('Compras: transferência marcada como feita.');
    expect(revision.current).toBeGreaterThan(before);

    overlay.toast!.actions.find((action) => action.label === 'Desfazer')!.run();
    await flushPromises();
    expect(rowOf(wrapper, 'Compras').find('.state').text()).toBe('Pendente');
    expect(overlay.toast).toBeNull();
  });

  it('tells two transfers of one name apart by their key, and marks only the one pressed', async () => {
    const state = prototypeState();
    state.plannedContributions = [
      { key: 'pot:reserve:nu/1', name: 'Reserva', amount: 60, state: 'pending', doneOn: null },
      { key: 'pot:reserve:mp/1', name: 'Reserva', amount: 240, state: 'pending', doneOn: null },
    ];
    const { wrapper, overlay, api } = await mountPayday(state);
    const mark = vi.spyOn(api.review, 'markTransferDone');
    const rows = () =>
      wrapper.findAll('.row').map((row) => [row.find('.name').text(), plain(row.find('.amount').text()), row.find('.state').text()]);
    expect(rows()).toEqual([
      ['Reserva', 'R$ 60,00', 'Pendente'],
      ['Reserva', 'R$ 240,00', 'Pendente'],
    ]);

    await wrapper.findAll('.row')[1]!.find('.mark').trigger('click');
    await flushPromises();
    expect(useMonthStore().selected).toBe('2026-11');
    expect(mark).toHaveBeenCalledExactlyOnceWith('2026-11', 'pot:reserve:mp/1');
    expect(rows()).toEqual([
      ['Reserva', 'R$ 60,00', 'Pendente'],
      ['Reserva', 'R$ 240,00', 'Feita em 28/11 · marcada por você'],
    ]);

    overlay.toast!.actions.find((action) => action.label === 'Desfazer')!.run();
    await flushPromises();
    expect(rows()).toEqual([
      ['Reserva', 'R$ 60,00', 'Pendente'],
      ['Reserva', 'R$ 240,00', 'Pendente'],
    ]);
  });

  it('keeps a transfer on its own row when another of the same name leaves the list', async () => {
    const state = prototypeState();
    state.plannedContributions = [
      { key: 'pot:reserve:nu/1', name: 'Reserva', amount: 60, state: 'pending', doneOn: null },
      { key: 'pot:reserve:mp/1', name: 'Reserva', amount: 240, state: 'pending', doneOn: null },
    ];
    const { wrapper, revision } = await mountPayday(state);
    const second = wrapper.findAll('.row')[1]!.element;

    state.plannedContributions.splice(0, 1);
    revision.bump();
    await flushPromises();
    const rows = wrapper.findAll('.row');
    expect(rows.map((row) => plain(row.find('.amount').text()))).toEqual(['R$ 240,00']);
    expect(rows[0]!.element).toBe(second);
  });

  it('does not let an earlier payday answer replace a later one for the same month', async () => {
    const { wrapper, api, revision } = await mountPayday();
    const comprasKey = (await api.review.payday('2026-11')).transfers.find((transfer) => transfer.name === 'Compras')!.key;

    const first = holdFirstCall(api.review, 'payday');
    revision.bump();
    await flushPromises();

    await api.review.markTransferDone('2026-11', comprasKey);
    revision.bump();
    await flushPromises();
    expect(rowOf(wrapper, 'Compras').find('.state').text()).toBe('Feita em 28/11 · marcada por você');

    first.release();
    await flushPromises();
    expect(rowOf(wrapper, 'Compras').find('.state').text()).toBe('Feita em 28/11 · marcada por você');
  });
});
