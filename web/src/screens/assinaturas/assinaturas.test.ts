import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { useOverlayStore } from '@/stores/overlay';
import { mountRouted } from '@/test/mountRouted';
import AssinaturasScreen from './AssinaturasScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/assinaturas', component: Blank },
  { path: '/assinaturas/:id', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountList() {
  const mounted = await mountRouted(AssinaturasScreen, routes, '/assinaturas');
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountList>>;

const rowOf = (wrapper: Mounted['wrapper'], name: string) =>
  wrapper.findAll('.row').find((row) => row.find('.name').text() === name)!;
const names = (wrapper: Mounted['wrapper']) => wrapper.findAll('.row .name').map((name) => name.text());
const button = (wrapper: Mounted['wrapper'], text: string) =>
  wrapper.findAll('button').find((candidate) => plain(candidate.text()) === text)!;

async function undo(overlay: Mounted['overlay']) {
  overlay.toast!.actions.find((action) => action.label === 'Desfazer')!.run();
  await flushPromises();
}

describe('Assinaturas, the list', () => {
  it('writes the total and the year above the table', async () => {
    const { wrapper } = await mountList();
    expect(plain(wrapper.find('.total').text())).toBe('Total mensal equivalente R$ 202,37 · Anual R$ 2.428,44');
  });

  it('lists the subscriptions in the order they were added, with their figures', async () => {
    const { wrapper } = await mountList();
    expect(names(wrapper)).toEqual(['Assistente de IA', 'Spotify', 'Amazon Prime', 'Hospedagem', 'Revista digital', 'Domínio']);
    const cells = rowOf(wrapper, 'Amazon Prime')
      .findAll('[data-label]')
      .map((cell) => `${cell.attributes('data-label')}=${plain(cell.text())}`);
    expect(cells).toEqual([
      'Valor=R$ 166,80',
      'Ciclo=Anual',
      'Próxima cobrança=07/12/2026',
      'Equivalente mensal=R$ 13,90',
      'Linha do budget=Assinaturas anuais',
      'Status=Analisar▾',
      'Revisar em=30/11/2026',
    ]);
  });

  it('shows the amber and red marks the backend sent, beside the name', async () => {
    const { wrapper } = await mountList();
    const marks = (name: string) => rowOf(wrapper, name).findAll('.mark').map((mark) => `${mark.text()}:${mark.classes()[1]}`);
    expect(marks('Assistente de IA')).toEqual(['Sobreposta:warning']);
    expect(marks('Spotify')).toEqual(['Preço subiu:warning']);
    expect(marks('Amazon Prime')).toEqual(['Renova em 9 dias:warning']);
    expect(marks('Domínio')).toEqual([]);
  });

  it('turns the review date amber only when it is overdue', async () => {
    const { wrapper } = await mountList();
    expect(rowOf(wrapper, 'Revista digital').find('.review').classes()).toContain('overdue');
    expect(rowOf(wrapper, 'Amazon Prime').find('.review').classes()).not.toContain('overdue');
  });

  it('colours the status pill: neutral for keeping, amber for looking at it, red for cancelling', async () => {
    const { wrapper } = await mountList();
    const tone = (name: string) => rowOf(wrapper, name).find('.pill').classes()[1];
    expect(tone('Spotify')).toBe('neutral');
    expect(tone('Amazon Prime')).toBe('warning');
    await rowOf(wrapper, 'Spotify').find('.pill').trigger('click');
    useOverlayStore().menu!.items.find((item) => item.label === 'Cancelar')!.onPick!();
    await flushPromises();
    expect(tone('Spotify')).toBe('bad');
  });

  it('carries the cycle, equivalent and line twice, as columns and as a subtitle, for the 1200px breakpoint to choose', async () => {
    const { wrapper } = await mountList();
    const row = rowOf(wrapper, 'Hospedagem');
    expect(plain(row.find('.subtitle.narrow-only').text())).toBe('Trimestral · R$ 20,00 por mês · Ferramentas de trabalho');
    expect(row.findAll('.wide-only').map((cell) => cell.attributes('data-label'))).toEqual([
      'Ciclo',
      'Equivalente mensal',
      'Linha do budget',
    ]);
    expect(wrapper.find('.head').findAll('.wide-only').map((cell) => cell.text())).toEqual([
      'Ciclo',
      'Equivalente mensal',
      'Linha do budget',
    ]);
    expect(row.classes()).toContain('stack-row');
    expect(row.find('.name-cell').classes()).toContain('stack-full');
  });

  it('opens the page of the row that was clicked', async () => {
    const { wrapper, router } = await mountList();
    await rowOf(wrapper, 'Spotify').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/assinaturas/sp');
  });
});

describe('Assinaturas, grouped by status', () => {
  it('lists a head per status that has any, with its count and monthly total', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.findAll('.group-head')).toHaveLength(0);
    await button(wrapper, 'Agrupar por status').trigger('click');
    await flushPromises();
    expect(wrapper.findAll('.group-head').map((head) => plain(head.text()))).toEqual([
      'Manter4 · R$ 158,57 por mês',
      'Analisar2 · R$ 43,80 por mês',
    ]);
    expect(names(wrapper)).toEqual(['Assistente de IA', 'Spotify', 'Hospedagem', 'Domínio', 'Amazon Prime', 'Revista digital']);
    expect(button(wrapper, 'Agrupar por status').attributes('aria-pressed')).toBe('true');
  });

  it('keeps the grouping while the user looks at the other tab and comes back', async () => {
    const { wrapper } = await mountList();
    await button(wrapper, 'Agrupar por status').trigger('click');
    await button(wrapper, 'Recorrentes').trigger('click');
    await button(wrapper, 'Assinaturas').trigger('click');
    await flushPromises();
    expect(wrapper.findAll('.group-head')).toHaveLength(2);
  });

  it('moves a subscription to its new group when its status changes', async () => {
    const { wrapper, overlay } = await mountList();
    await button(wrapper, 'Agrupar por status').trigger('click');
    await flushPromises();
    await rowOf(wrapper, 'Domínio').find('.pill').trigger('click');
    overlay.menu!.items.find((item) => item.label === 'Backup e cancelar')!.onPick!();
    await flushPromises();
    expect(wrapper.findAll('.group-head').map((head) => plain(head.text()))).toEqual([
      'Manter3 · R$ 151,90 por mês',
      'Analisar2 · R$ 43,80 por mês',
      'Backup e cancelar1 · R$ 6,67 por mês',
    ]);
  });
});

describe('Assinaturas, the status menu', () => {
  it('offers the four statuses with the current one marked, and does not open the page', async () => {
    const { wrapper, overlay, router } = await mountList();
    await rowOf(wrapper, 'Spotify').find('.pill').trigger('click');
    await flushPromises();
    expect(overlay.menu?.title).toBe('Status');
    expect(overlay.menu?.items.map((item) => [item.label, item.current])).toEqual([
      ['Manter', true],
      ['Analisar', false],
      ['Backup e cancelar', false],
      ['Cancelar', false],
    ]);
    expect(router.currentRoute.value.fullPath).toBe('/assinaturas');
  });

  it('changes the status, says so with a way back, and takes it back', async () => {
    const { wrapper, overlay } = await mountList();
    await rowOf(wrapper, 'Spotify').find('.pill').trigger('click');
    overlay.menu!.items.find((item) => item.label === 'Analisar')!.onPick!();
    await flushPromises();
    expect(plain(rowOf(wrapper, 'Spotify').find('.pill').text())).toBe('Analisar▾');
    expect(overlay.toast?.message).toBe('Spotify: status alterado para Analisar.');
    expect(overlay.toast?.actions.map((action) => action.label)).toEqual(['Desfazer']);

    await undo(overlay);
    expect(plain(rowOf(wrapper, 'Spotify').find('.pill').text())).toBe('Manter▾');
    expect(overlay.toast).toBeNull();
  });
});

describe('Assinaturas, Nova assinatura', () => {
  it('opens a searchable menu of this month card purchases, with the amount as a hint', async () => {
    const { wrapper, overlay } = await mountList();
    await button(wrapper, 'Nova assinatura').trigger('click');
    await flushPromises();
    expect(overlay.menu).toMatchObject({ title: 'Criar a partir de uma transação', search: true, placeholder: 'Buscar transação' });
    expect(overlay.menu?.items).toHaveLength(12);
    expect(overlay.menu?.items[0]).toMatchObject({ label: 'Cinema Belas Artes' });
    expect(plain(overlay.menu!.items[0]!.hint!)).toBe('R$ 65,10');
  });

  it('adds the picked purchase as a subscription, updates the total, and takes it back', async () => {
    const { wrapper, overlay } = await mountList();
    await button(wrapper, 'Nova assinatura').trigger('click');
    await flushPromises();
    overlay.menu!.items.find((item) => item.label === 'Bar do Zé')!.onPick!();
    await flushPromises();

    expect(names(wrapper).at(-1)).toBe('Bar do Zé');
    expect(plain(wrapper.find('.total').text())).toContain('R$ 266,37');
    expect(plain(rowOf(wrapper, 'Bar do Zé').find('.pill').text())).toBe('Analisar▾');
    expect(overlay.toast?.message).toBe('Bar do Zé entrou nas assinaturas.');

    await undo(overlay);
    expect(names(wrapper)).not.toContain('Bar do Zé');
    expect(plain(wrapper.find('.total').text())).toContain('R$ 202,37');
  });
});

describe('Assinaturas, the suggestion', () => {
  it('asks whether the purchase that looks like a subscription is one', async () => {
    const { wrapper } = await mountList();
    expect(wrapper.find('.strip').text()).toContain('Serviço de nuvem parece assinatura. Confirmar?');
    expect(wrapper.find('.strip').findAll('button').map((candidate) => candidate.text())).toEqual(['Sim', 'Não é assinatura']);
  });

  it('turns it into a subscription on "Sim", and the strip goes away; undo brings both back', async () => {
    const { wrapper, overlay } = await mountList();
    await button(wrapper, 'Sim').trigger('click');
    await flushPromises();
    expect(wrapper.find('.strip').exists()).toBe(false);
    expect(names(wrapper).at(-1)).toBe('Serviço de nuvem');
    expect(overlay.toast?.message).toBe('Serviço de nuvem entrou nas assinaturas.');

    await undo(overlay);
    expect(wrapper.find('.strip').exists()).toBe(true);
    expect(names(wrapper)).not.toContain('Serviço de nuvem');
  });

  it('drops the strip on "Não é assinatura" without adding a row; undo brings it back', async () => {
    const { wrapper, overlay } = await mountList();
    await button(wrapper, 'Não é assinatura').trigger('click');
    await flushPromises();
    expect(wrapper.find('.strip').exists()).toBe(false);
    expect(names(wrapper)).toHaveLength(6);
    expect(overlay.toast?.message).toBe('Serviço de nuvem não será tratado como assinatura.');

    await undo(overlay);
    expect(wrapper.find('.strip').exists()).toBe(true);
  });
});

describe('Assinaturas, Recorrentes', () => {
  async function openTab() {
    const mounted = await mountList();
    await button(mounted.wrapper, 'Recorrentes').trigger('click');
    await flushPromises();
    return mounted;
  }

  it('lists the purchases with the expected cadence, the last purchase, the next one and the average', async () => {
    const { wrapper } = await openTab();
    const rows = wrapper.findAll('.row').map((row) => [...row.element.children].map((cell) => plain(cell.textContent ?? '')));
    expect(rows).toEqual([
      ['Areia do gato', 'A cada 3 semanas', '08/11/2026', '29/11/2026', 'R$ 54,90'],
      ['Filtro de águaEstá na hora', 'A cada 6 meses', '14/05/2026', '14/11/2026', 'R$ 89,00'],
      ['Remédio de uso contínuoEstá na hora', 'A cada 30 dias', '27/10/2026', '26/11/2026', 'R$ 112,40'],
      ['Ração', 'A cada 5 semanas', '02/11/2026', '07/12/2026', 'R$ 139,90'],
    ]);
  });

  it('replaces the subscription table, and the header of the first tab is gone', async () => {
    const { wrapper } = await openTab();
    expect(wrapper.find('.total').exists()).toBe(false);
    expect(wrapper.find('.head').text()).toContain('Cadência esperada');
    expect(wrapper.find('.tab.active').text()).toBe('Recorrentes');
  });

  it('labels every value of a row for the phone card', async () => {
    const { wrapper } = await openTab();
    expect(wrapper.find('.row').findAll('[data-label]').map((cell) => cell.attributes('data-label'))).toEqual([
      'Cadência esperada',
      'Última compra',
      'Próxima esperada',
      'Valor médio',
    ]);
  });
});
