import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { useRevisionStore } from '@/stores/revision';
import { mountRouted } from '@/test/mountRouted';
import CofrinhosScreen from './CofrinhosScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/cofrinhos', component: Blank },
  { path: '/cofrinhos/:id', component: Blank },
];

const plain = (text: string) => text.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim();

async function mountList() {
  const mounted = await mountRouted(CofrinhosScreen, routes, '/cofrinhos');
  await flushPromises();
  return mounted;
}

describe('Cofrinhos list', () => {
  it('groups the pots by bucket with the subtotal of each group', async () => {
    const { wrapper } = await mountList();
    const heads = wrapper.findAll('.group-head').map((head) => plain(head.text()));
    expect(heads).toEqual([
      'ReservadoSubtotal R$ 34.150,00',
      'InvestidoSubtotal R$ 37.800,00',
      'Sem baldeSubtotal R$ 500,00',
    ]);
    expect(wrapper.findAll('.row')).toHaveLength(13);
  });

  it('shows a pot nobody named as "Sem nome", greyed', async () => {
    const { wrapper } = await mountList();
    const unnamed = wrapper.find('.name.unnamed');
    expect(unnamed.text()).toBe('Sem nome');
    expect(wrapper.findAll('.name.unnamed')).toHaveLength(1);
  });

  it('writes the target with its bar, and a dash where there is none', async () => {
    const { wrapper } = await mountList();
    const rows = wrapper.findAll('.row');
    const pets = rows.find((row) => row.text().includes('RF Pets'))!;
    expect(plain(pets.find('.target').text())).toBe('R$ 4.200 de R$ 5.000');
    expect(pets.find('.bar-fill').attributes('style')).toContain('width: 84%');
    const gas = rows.find((row) => row.text().includes('Gás e IPTU'))!;
    expect(plain(gas.find('.target').text())).toBe('—');
    expect(gas.find('.bar').exists()).toBe(false);
  });

  it('writes the yield with a plus sign and the maturity as a date', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('Investimento 1'))!;
    expect(plain(row.find('.yield').text())).toBe('+ R$ 301,88');
    expect(row.text()).toContain('15/03/2028');
  });

  it('hides closed pots until the toggle is pressed, then adds a Status column and fades them', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.text()).not.toContain('Investimento 0');
    expect(wrapper.find('.head').text()).not.toContain('Status');

    const toggle = wrapper.find('button.toggle');
    expect(toggle.attributes('aria-pressed')).toBe('false');
    await toggle.trigger('click');
    await flushPromises();

    expect(toggle.attributes('aria-pressed')).toBe('true');
    expect(wrapper.find('.head').text()).toContain('Status');
    const closed = wrapper.findAll('.row').find((row) => row.text().includes('Investimento 0'))!;
    expect(closed.classes()).toContain('closed');
    expect(closed.find('.status').text()).toBe('Encerrado');
    expect(closed.find('.yield').text()).toBe('—');
    const open = wrapper.findAll('.row').find((row) => row.text().includes('Investimento 1'))!;
    expect(open.find('.status').text()).toBe('Ativo');
    expect(open.classes()).not.toContain('closed');
    expect(wrapper.find('.content').classes()).toContain('with-status');
  });

  it('opens the pot of the row that was clicked', async () => {
    const { wrapper, router } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('RF Pets'))!;
    await row.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/cofrinhos/rfp');
  });

  it('carries the bank twice, as a column and as a subtitle, for the 1200px breakpoint to choose', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes('RF Pets'))!;
    expect(row.find('.subtitle.narrow-only').text()).toBe('Mercado Pago');
    expect(row.findAll('.wide-only').map((cell) => plain(cell.text()))).toEqual(['Mercado Pago', 'Reservado']);
    expect(wrapper.find('.head').findAll('.wide-only').map((cell) => cell.text())).toEqual(['Banco', 'Balde']);
  });

  it('labels every value of a row for the phone card, and lets the name span both columns', async () => {
    const { wrapper } = await mountList();
    const row = wrapper.find('.row');
    expect(row.classes()).toContain('stack-row');
    expect(row.find('.name-cell').classes()).toContain('stack-full');
    expect(row.findAll('[data-label]').map((cell) => cell.attributes('data-label'))).toEqual([
      'Banco',
      'Balde',
      'Saldo',
      '% CDI',
      'Rendimento no mês',
      'Vencimento',
      'Alvo',
    ]);
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

describe('Cofrinhos list after a change', () => {
  const heads = (wrapper: Awaited<ReturnType<typeof mountList>>['wrapper']) =>
    wrapper.findAll('.group-name').map((head) => plain(head.text()));

  it('regroups after a write and a bump, and again after its undo, without a remount', async () => {
    const { wrapper, api } = await mountList();
    const { changeId } = (await api.settings.setPotBucket('sn', 'Disponível'))!;
    useRevisionStore().bump();
    await flushPromises();
    expect(heads(wrapper)).toEqual(['Reservado', 'Investido', 'Disponível']);
    await api.changes.undo(changeId);
    useRevisionStore().bump();
    await flushPromises();
    expect(heads(wrapper)).toEqual(['Reservado', 'Investido', 'Sem balde']);
  });

  it('asks once more per bump, and keeps the list while it waits', async () => {
    const { wrapper, api } = await mountList();
    const read = vi.spyOn(api.savingsPots, 'list').mockImplementationOnce(() => new Promise(() => {}));
    useRevisionStore().bump();
    await flushPromises();
    expect(wrapper.findAll('.row')).toHaveLength(13);
    read.mockRestore();
    const counted = vi.spyOn(api.savingsPots, 'list');
    useRevisionStore().bump();
    await flushPromises();
    expect(counted).toHaveBeenCalledTimes(1);
    useRevisionStore().bump();
    await flushPromises();
    expect(counted).toHaveBeenCalledTimes(2);
  });

  it('does not show an answer for the option that is no longer selected', async () => {
    const { wrapper, api } = await mountList();
    const original = api.savingsPots.list.bind(api.savingsPots);
    const late = holdNext(() => original({ includeClosed: false }));
    let calls = 0;
    vi.spyOn(api.savingsPots, 'list').mockImplementation((...args: Parameters<typeof original>) => (++calls === 1 ? late.held : (original as (...a: typeof args) => ReturnType<typeof original>)(...args)));
    useRevisionStore().bump();
    await flushPromises();
    await wrapper.find('button.toggle').trigger('click');
    await flushPromises();
    late.release();
    await flushPromises();
    expect(wrapper.find('.head').text()).toContain('Status');
    expect(wrapper.findAll('.row').length).toBeGreaterThan(13);
  });

  it('does not let an earlier answer replace a later one for the same option', async () => {
    const { wrapper, api } = await mountList();
    const original = api.savingsPots.list.bind(api.savingsPots);
    const early = holdNext(() => original({ includeClosed: false }));
    let calls = 0;
    vi.spyOn(api.savingsPots, 'list').mockImplementation((...args: Parameters<typeof original>) => (++calls === 1 ? early.held : (original as (...a: typeof args) => ReturnType<typeof original>)(...args)));
    useRevisionStore().bump();
    await api.settings.setPotBucket('sn', 'Disponível');
    useRevisionStore().bump();
    await flushPromises();
    early.release();
    await flushPromises();
    expect(heads(wrapper)).toEqual(['Reservado', 'Investido', 'Disponível']);
  });
});
