import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { createFakeApi } from '@/api/fake';
import { useRevisionStore } from '@/stores/revision';
import { holdFirstCall } from '@/test/holdCall';
import { lineNamed, recordLineMenu } from '@/test/lineMenu';
import { prototypeOverview } from '@/test/mount';
import { mountWithFailures } from '@/test/readOnly';
import { mountRouted } from '@/test/mountRouted';
import WeeklyTab from './WeeklyTab.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/revisao', component: Blank },
  { path: '/assinaturas', component: Blank },
  { path: '/assinaturas/:id', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountWeekly(state = prototypeState()) {
  const mounted = await mountRouted(WeeklyTab, routes, '/revisao', { state });
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountWeekly>>;

const button = (wrapper: Mounted['wrapper'], text: string) =>
  wrapper.findAll('button').find((candidate) => plain(candidate.text()) === text)!;
const counter = (wrapper: Mounted['wrapper']) => (wrapper.find('.aside').exists() ? plain(wrapper.find('.aside').text()) : '');
const nameShown = (wrapper: Mounted['wrapper']) => wrapper.find('.item .name').text();
const click = async (target: Pick<ReturnType<typeof button>, 'trigger'>) => {
  await target.trigger('click');
  await flushPromises();
};
const undo = async (overlay: Mounted['overlay']) => {
  overlay.toast!.actions.find((action) => action.label === 'Desfazer')!.run();
  await flushPromises();
};

describe('Semanal, the summary', () => {
  it('writes what was spent against the limit and what is left, and the week guide', async () => {
    const { wrapper } = await mountWeekly();
    const cells = wrapper.findAll('.cell').map((cell) => cell.findAll('span').map((span) => plain(span.text())).join(' '));
    expect(cells).toEqual(['Limite do mês Gasto R$ 1.460,00 de R$ 2.880,00 · Restam R$ 1.420,00', 'Esta semana Guia R$ 355,00 · gasto R$ 210,00']);
    expect(wrapper.find('.week .fill').attributes('style')).toContain('59.');
  });

  it('says "Passou" in red when the cap was passed', async () => {
    const state = prototypeState();
    state.spentBaseline['2026-11']!['Mercado'] = 2000;
    const { wrapper } = await mountWeekly(state);
    expect(plain(wrapper.find('.limit .detail').text())).toBe('Gasto R$ 3.152,00 de R$ 2.880,00 · Passou R$ 272,00');
    expect(wrapper.find('.limit .fill').classes()).toContain('over');
  });
});

describe('Semanal, the queue', () => {
  it('shows the first review: reason, names, amount, where, suggestion and its justification', async () => {
    const { wrapper } = await mountWeekly();
    expect(counter(wrapper)).toBe('1 de 7');
    const item = wrapper.find('.item');
    expect(item.find('.reason').text()).toBe('Baixa confiança');
    expect(item.find('.name').text()).toBe('Bar do Zé');
    expect(item.find('.raw').text()).toBe('BAR E LANCH DO ZE LTDA');
    expect(plain(item.find('.amount').text())).toBe('R$ 64,00');
    expect(plain(item.find('.where').text())).toBe('27/11/2026 · C6 Carbon · final 1234');
    expect(plain(item.find('.line').text())).toBe('Sugestão da IA: Restaurantes e bares · 58%');
    expect(item.find('.why').text()).toBe('O nome sugere bar, mas a compra foi no horário do almoço.');
    expect(button(wrapper, 'Aceitar')).toBeTruthy();
  });

  it('accepts: the item leaves, the counter moves on, a toast says where it went and Desfazer brings it back', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(button(wrapper, 'Aceitar'));
    expect(overlay.toast?.message).toBe('Bar do Zé foi para Restaurantes e bares.');
    expect(nameShown(wrapper)).toBe('Padaria Estrela');
    expect(counter(wrapper)).toBe('2 de 7');
    await undo(overlay);
    expect(nameShown(wrapper)).toBe('Bar do Zé');
    expect(counter(wrapper)).toBe('1 de 7');
  });

  it('skips to the next review and wraps around after the last', async () => {
    const { wrapper } = await mountWeekly();
    for (let skipped = 0; skipped < 6; skipped += 1) await click(button(wrapper, 'Pular'));
    expect(nameShown(wrapper)).toBe('Loja Verde Vida');
    expect(counter(wrapper)).toBe('7 de 7');
    await click(button(wrapper, 'Pular'));
    expect(nameShown(wrapper)).toBe('Bar do Zé');
    expect(counter(wrapper)).toBe('1 de 7');
  });

  it('forgets what was typed for a review once it is skipped', async () => {
    const { wrapper } = await mountWeekly();
    await wrapper.find('input[placeholder="Por quê?"]').setValue('depois');
    await click(button(wrapper, 'Pular'));
    expect((wrapper.find('input[placeholder="Por quê?"]').element as HTMLInputElement).value).toBe('');
  });

  it('offers every budget line to pick instead, and the pick turns Aceitar into Confirmar', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(button(wrapper, 'Escolher outra linha'));
    const items = overlay.menu!.items;
    expect(items.find((item) => item.label === 'Restaurantes e bares')?.current).toBe(true);
    items.find((item) => item.label === 'Delivery')!.onPick!();
    await flushPromises();
    expect(plain(wrapper.find('.item .line').text())).toBe('Linha escolhida: Delivery');
    expect(wrapper.find('.item .why').text()).toBe('A IA tinha sugerido Restaurantes e bares. Confirme para usar a sua escolha.');
    expect(button(wrapper, 'Confirmar')).toBeTruthy();
    expect(wrapper.findAll('button').some((candidate) => candidate.text() === 'Aceitar')).toBe(false);
  });

  it('puts the transaction on the line that was picked', async () => {
    const { wrapper, overlay, api } = await mountWeekly();
    await click(button(wrapper, 'Escolher outra linha'));
    overlay.menu!.items.find((item) => item.label === 'Delivery')!.onPick!();
    await flushPromises();
    await click(button(wrapper, 'Confirmar'));
    const moved = (await api.transactions.list({ month: '2026-11', period: 'month' })).find((transaction) => transaction.id === 't5');
    expect(moved?.line).toBe('Delivery');
    expect(overlay.toast?.message).toBe('Bar do Zé foi para Delivery.');
    expect(wrapper.findAll('button').some((candidate) => candidate.text() === 'Confirmar')).toBe(false);
  });

  it('offers a rule for an establishment, which the toast mentions and the history records', async () => {
    const { wrapper, overlay, api } = await mountWeekly();
    expect(wrapper.text()).not.toContain('Dar apelido a esta pessoa');
    await wrapper.find('input[type="checkbox"]').setValue(true);
    await click(button(wrapper, 'Aceitar'));
    expect(overlay.toast?.message).toBe('Bar do Zé foi para Restaurantes e bares. Vale para as próximas compras neste estabelecimento.');
    expect((await api.transactions.detail('t5'))?.counterparty?.defaultLine).toBe('Restaurantes e bares');
  });

  it('offers a nickname, and no rule, for a person', async () => {
    const { wrapper, overlay } = await mountWeekly();
    for (let skipped = 0; skipped < 2; skipped += 1) await click(button(wrapper, 'Pular'));
    expect(nameShown(wrapper)).toBe('Marcos A. Lima');
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false);
    await wrapper.find('input[placeholder="Aluguel"]').setValue('Aluguel');
    await click(button(wrapper, 'Aceitar'));
    expect(overlay.toast?.message).toBe('Aluguel foi para Lazer.');
  });

  it('shows "Tudo revisado." once the last review is answered', async () => {
    const { wrapper } = await mountWeekly();
    for (let answered = 0; answered < 7; answered += 1) await click(button(wrapper, 'Aceitar'));
    expect(wrapper.find('.empty .done').text()).toBe('Tudo revisado.');
    expect(wrapper.find('.empty .note').text()).toBe('Novas pendências aparecem aqui depois da sincronização da madrugada.');
    expect(counter(wrapper)).toBe('');
  });
});

describe('Semanal, waiting for approval', () => {
  const titles = (wrapper: Mounted['wrapper']) => wrapper.findAll('.request .title').map((title) => title.text());

  it('lists the changes with their reason', async () => {
    const { wrapper } = await mountWeekly();
    expect(wrapper.findAll('.request .text').map((text) => plain(text.text()))).toEqual([
      'Subir o teto de Delivery de R$ 200 para R$ 250 · estourou três meses seguidos',
      'Marcar como "não conta": Estorno de anuidade R$ 120,00 · é a devolução de uma tarifa, não uma receita',
      'Mudar status de Revista digital para Cancelar · revisão vencida há dois meses',
    ]);
  });

  it('approves for real: the cap moves the limit above, a toast names the change and Desfazer reverts it', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(wrapper.findAll('.request .approve')[0]!);
    expect(overlay.toast?.message).toBe('Aprovado: subir o teto de delivery de r$ 200 para r$ 250.');
    expect(plain(wrapper.find('.limit .detail').text())).toContain('de R$ 2.930,00');
    expect(titles(wrapper)).toHaveLength(2);
    await undo(overlay);
    expect(plain(wrapper.find('.limit .detail').text())).toContain('de R$ 2.880,00');
    expect(titles(wrapper)).toHaveLength(3);
  });

  it('shows proposer when present and hides it when null', async () => {
    const state = prototypeState();
    state.review.proposals[0]!.proposer = 'revisor-semanal';
    const { wrapper } = await mountWeekly(state);
    const texts = wrapper.findAll('.request .text').map((text) => plain(text.text()));
    expect(texts[0]).toBe('Subir o teto de Delivery de R$ 200 para R$ 250 · estourou três meses seguidos · proposto por revisor-semanal');
    expect(texts[1]).not.toContain('proposto por');
  });

  it('refuses with an optional reason field, records no undo action, and lets the person cancel', async () => {
    const { wrapper, overlay, api } = await mountWeekly();
    await click(wrapper.findAll('.request .refuse')[0]!);
    const input = wrapper.find('.request .refuse-input');
    expect(input.attributes('placeholder')).toBe('Por quê? (opcional)');
    expect(input.attributes('maxlength')).toBe('500');

    // Cancelling closes the field without touching anything.
    await click(wrapper.find('.request .cancel'));
    expect(wrapper.find('.request .refuse-input').exists()).toBe(false);
    expect(titles(wrapper)).toHaveLength(3);
    expect(overlay.toast).toBeNull();

    // Typing a reason and confirming sends it and shows a toast with no Desfazer action.
    await click(wrapper.findAll('.request .refuse')[0]!);
    await wrapper.find('.request .refuse-input').setValue('Quero manter o teto atual');
    await click(wrapper.find('.request .confirm-refuse'));
    expect(overlay.toast?.message).toBe('Recusado. Nada foi alterado.');
    expect(overlay.toast?.actions).toEqual([]);
    expect(plain(wrapper.find('.limit .detail').text())).toContain('de R$ 2.880,00');
    expect(titles(wrapper)).toHaveLength(2);

    const refused = (await api.review.proposals()).find((item) => item.id === 'teto');
    expect(refused?.outcome).toBe('refused');
    expect(refused?.refusal).toBe('Quero manter o teto atual');
  });

  it('says nothing is waiting once the list is empty', async () => {
    const { wrapper } = await mountWeekly();
    for (let answered = 0; answered < 3; answered += 1) {
      await click(wrapper.find('.request .refuse'));
      await click(wrapper.find('.request .confirm-refuse'));
    }
    expect(plain(wrapper.find('.side .empty').text())).toBe(
      'Nada aguardando. Mudanças que precisam do seu ok aparecem aqui depois de cada sincronização ou do relatório do mês.',
    );
  });

  it('refetches silently when approve or refuse answers null because another tab answered first', async () => {
    const { wrapper, overlay, api } = await mountWeekly();
    // Answer the first approval directly on the API before clicking in the UI.
    await api.review.refuse('teto');
    const before = useRevisionStore().current;
    await click(wrapper.findAll('.request .approve')[0]!);
    expect(useRevisionStore().current).toBeGreaterThan(before);
    expect(overlay.toast).toBeNull();
    expect(titles(wrapper)).toHaveLength(2);
  });

  it('toasts proposal_stale with no action and removes the stale proposal when the underlying value already changed', async () => {
    const state = prototypeState();
    // Apply the exclusion already so approving the second proposal ("estorno") becomes stale.
    const effect = state.review.proposals[1]!.effect;
    if (effect.kind === 'exclude') {
      state.transactions.find((transaction) => transaction.id === effect.transactionId)!.excludedReason = effect.reason;
    }
    const api = createFakeApi({ startLocked: false, state });
    const mounted = await mountWithFailures(WeeklyTab, routes, '/revisao', { api });
    const before = mounted.revisionNow();

    await click(mounted.wrapper.findAll('.request .approve')[1]!);
    await mounted.settle();

    expect(mounted.toasts).toEqual(['Isso mudou depois da proposta. Nada foi alterado.']);
    expect(useOverlayStore(mounted.pinia).toast?.actions).toEqual([]);
    expect(mounted.revisionNow()).toBe(before + 1);
    expect(mounted.wrapper.findAll('.request .title')).toHaveLength(2);
  });
});

describe('Semanal, the warnings', () => {
  const texts = (wrapper: Mounted['wrapper']) => wrapper.findAll('.warning .text').map((text) => text.text());

  it('lists five, some with their own buttons, each with a cross', async () => {
    const { wrapper } = await mountWeekly();
    expect(texts(wrapper)).toHaveLength(5);
    expect(wrapper.findAll('.warning').map((row) => row.findAll('.action').map((action) => action.text()))).toEqual([
      [],
      ['Sim', 'Não'],
      ['Silenciar este par'],
      [],
      [],
    ]);
    expect(wrapper.findAll('.dismiss')).toHaveLength(5);
  });

  it('opens the subscription a warning is about, or the list when it names none', async () => {
    const { wrapper, router } = await mountWeekly();
    await click(wrapper.findAll('.warning .text')[0]!);
    expect(router.currentRoute.value.path).toBe('/assinaturas/sp');
    await router.push('/revisao');
    await click(wrapper.findAll('.warning .text')[4]!);
    expect(router.currentRoute.value.path).toBe('/assinaturas');
  });

  it('dismisses one with a toast that brings it back', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(wrapper.findAll('.dismiss')[0]!);
    expect(overlay.toast?.message).toBe('Aviso dispensado.');
    expect(texts(wrapper)).toHaveLength(4);
    await undo(overlay);
    expect(texts(wrapper)).toHaveLength(5);
  });

  it('turns a new recurring charge into a subscription with "Sim"', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(button(wrapper, 'Sim'));
    expect(overlay.toast?.message).toBe('Serviço de nuvem entrou nas assinaturas.');
    expect(texts(wrapper)).toHaveLength(4);
  });

  it('refuses it with "Não"', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(button(wrapper, 'Não'));
    expect(overlay.toast?.message).toBe('Serviço de nuvem não será tratado como assinatura.');
    expect(texts(wrapper)).toHaveLength(4);
  });

  it('silences an overlapping pair', async () => {
    const { wrapper, overlay } = await mountWeekly();
    await click(button(wrapper, 'Silenciar este par'));
    expect(overlay.toast?.message).toBe('Par silenciado. Não avisaremos mais sobre esta sobreposição.');
    expect(texts(wrapper)).toHaveLength(4);
  });

  it('says there are none once they are all dismissed', async () => {
    const { wrapper } = await mountWeekly();
    for (let dismissed = 0; dismissed < 5; dismissed += 1) await click(wrapper.find('.dismiss'));
    expect(plain(wrapper.find('.side section:last-child .empty').text())).toBe(
      'Sem avisos. Aumentos de preço, renovações e cobranças estranhas aparecem aqui quando forem detectados.',
    );
  });
});

describe('Semanal, the lines each review offers', () => {
  const NOV = '2026-11';
  const OCT = '2026-10';

  /** One pending purchase moved to the month before, so the queue spans two months. */
  function queueOverTwoMonths() {
    const state = prototypeState();
    state.transactions.find((transaction) => transaction.pending && transaction.name === 'Padaria Estrela')!.day = '2026-10-15';
    return state;
  }

  async function mountRecorded(state = queueOverTwoMonths()) {
    const api = createFakeApi({ startLocked: false, state });
    const recorded = recordLineMenu(api, { named: true });
    const mounted = await mountWithFailures(WeeklyTab, routes, '/revisao', { api });
    return { ...mounted, ...recorded, overlay: useOverlayStore(mounted.pinia) };
  }

  const monthOfItem = (wrapper: Mounted['wrapper']) => {
    const [, mm, yyyy] = /^(\d{2})\/(\d{2})\/(\d{4})/.exec(plain(wrapper.find('.item .where').text()))!.slice(1);
    return `${yyyy}-${mm}`;
  };

  it('asks for each month its items fall in, once, and offers each item the lines of its own month', async () => {
    const m = await mountRecorded();
    expect([...m.months].sort()).toEqual([OCT, NOV]);
    const seen: string[] = [];
    for (let shown = 0; shown < 7; shown += 1) {
      const month = monthOfItem(m.wrapper);
      seen.push(month);
      await click(button(m.wrapper, 'Escolher outra linha'));
      expect(m.overlay.menu!.items.map((item) => item.label), nameShown(m.wrapper)).toEqual([lineNamed(month, 1)]);
      await click(button(m.wrapper, 'Pular'));
    }
    expect(new Set(seen)).toEqual(new Set([NOV, OCT]));
    expect([...m.months].sort()).toEqual([OCT, NOV]);
  });

  it('asks again for the months still in the queue after a write', async () => {
    const m = await mountRecorded();
    useRevisionStore(m.pinia).bump();
    await m.settle();
    expect([...m.months].sort()).toEqual([OCT, OCT, NOV, NOV]);
    await click(button(m.wrapper, 'Escolher outra linha'));
    expect(m.overlay.menu!.items.map((item) => item.label)).toEqual([lineNamed(monthOfItem(m.wrapper), 2)]);
  });

  it('draws the queue only once the lines of its months arrived', async () => {
    const api = createFakeApi({ startLocked: false, state: queueOverTwoMonths() });
    const { hold } = recordLineMenu(api, { named: true });
    const release = hold(OCT);
    const mounted = await mountWithFailures(WeeklyTab, routes, '/revisao', { api });
    expect(mounted.wrapper.find('.item').exists()).toBe(false);
    release();
    await mounted.settle();
    expect(mounted.wrapper.find('.item').exists()).toBe(true);
  });

  it('asks nothing when the queue is empty', async () => {
    const state = prototypeState();
    for (const transaction of state.transactions) transaction.pending = null;
    const m = await mountRecorded(state);
    expect(m.wrapper.text()).toContain('Tudo revisado.');
    expect(m.months).toEqual([]);
  });
});

describe('Semanal, out-of-order loads', () => {
  it('does not let an earlier weekly answer replace a later one for the same month', async () => {
    const { wrapper, api } = await mountWeekly();
    const first = holdFirstCall(api.review, 'weekly');
    useRevisionStore().bump();
    await flushPromises();

    await api.review.acceptPending('t5', { line: 'Restaurantes e bares', reason: '', always: false, nickname: '' });
    useRevisionStore().bump();
    await flushPromises();
    expect(nameShown(wrapper)).toBe('Padaria Estrela');

    first.release();
    await flushPromises();
    expect(nameShown(wrapper)).toBe('Padaria Estrela');
  });

  it('does not let the chained line menus of an earlier load land after a newer load', async () => {
    const api = createFakeApi({ startLocked: false });
    recordLineMenu(api, { named: true });
    const mounted = await mountWithFailures(WeeklyTab, routes, '/revisao', { api });
    const overlay = useOverlayStore(mounted.pinia);

    const firstMenu = holdFirstCall(api.transactions, 'filterOptions');
    useRevisionStore(mounted.pinia).bump();
    await mounted.settle();

    await api.review.acceptPending('t5', { line: 'Restaurantes e bares', reason: '', always: false, nickname: '' });
    useRevisionStore(mounted.pinia).bump();
    await mounted.settle();
    expect(nameShown(mounted.wrapper)).toBe('Padaria Estrela');

    firstMenu.release();
    await mounted.settle();
    expect(nameShown(mounted.wrapper)).toBe('Padaria Estrela');
    await click(button(mounted.wrapper, 'Escolher outra linha'));
    expect(overlay.menu!.items.map((item) => item.label)).toEqual([lineNamed('2026-11', 3)]);
  });
});
