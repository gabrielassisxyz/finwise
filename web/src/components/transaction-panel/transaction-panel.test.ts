import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent } from 'vue';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { useShellStore } from '@/stores/shell';
import { createFakeApi } from '@/api/fake';
import { prototypeState } from '@/api/fake/prototype-data';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';
import { holdFirstCall } from '@/test/holdCall';
import { lineNamed, recordLineMenu, tagNamed } from '@/test/lineMenu';
import { mountAt, prototypeOverview } from '@/test/mount';
import { mountWithFailures } from '@/test/readOnly';
import TransactionPanel from './TransactionPanel.vue';

const Host = defineComponent({
  components: { TransactionList, TransactionPanel },
  template: '<TransactionList /><TransactionPanel />',
});

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

async function mountHost() {
  const mounted = await mountAt(Host, '/', { attachTo: document.body });
  useShellStore().overview = prototypeOverview();
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, panel: usePanelStore() };
}

async function openOn(id: string) {
  const mounted = await mountHost();
  mounted.panel.open(id);
  await flushPromises();
  return mounted;
}

const labels = (wrapper: Awaited<ReturnType<typeof openOn>>['wrapper']) =>
  wrapper.findAll('.panel .label, .panel .title, .panel .kicker').map((node) => node.text());

describe('opening and closing', () => {
  it('is closed until a transaction is chosen', async () => {
    const { wrapper } = await mountHost();
    expect(wrapper.find('.panel').exists()).toBe(false);
  });

  it('opens from a wide row and from a phone card', async () => {
    const { wrapper, panel } = await mountHost();
    await wrapper.findAll('.row')[1]!.trigger('click');
    await flushPromises();
    expect(panel.openId).toBe('t2');
    expect(wrapper.find('.panel .name').text()).toBe('Posto Shell');

    panel.close();
    await flushPromises();
    await wrapper.findAll('.card-row')[2]!.trigger('click');
    await flushPromises();
    expect(wrapper.find('.panel .name').text()).toBe('iFood');
  });

  it('closes on its cross, on a click outside and when the screen changes', async () => {
    const { wrapper, router } = await openOn('t2');
    await wrapper.find('.close').trigger('click');
    expect(wrapper.find('.panel').exists()).toBe(false);

    usePanelStore().open('t2');
    await flushPromises();
    await wrapper.find('.scrim').trigger('click');
    expect(wrapper.find('.panel').exists()).toBe(false);

    usePanelStore().open('t2');
    await flushPromises();
    await router.push('/revisao');
    await flushPromises();
    expect(wrapper.find('.panel').exists()).toBe(false);
  });

  it('closes when the shell goes away, as it does when the app is locked', async () => {
    const { wrapper, panel } = await openOn('t2');
    wrapper.unmount();
    expect(panel.openId).toBeNull();
  });

  it('shows another transaction without keeping the previous one on screen', async () => {
    const { wrapper, panel } = await openOn('t2');
    panel.open('t34');
    await flushPromises();
    expect(wrapper.find('.panel .name').text()).toBe('Aluguel');
  });
});

describe('the header', () => {
  it('names the transaction, its amount, date, account and badges', async () => {
    const { wrapper } = await openOn('t7');
    const header = wrapper.find('.panel .header');
    expect(header.find('.name').text()).toBe('Serviço de nuvem');
    expect(header.find('.amount').text()).toBe('R$\u00a0113,85');
    expect(header.find('.subtitle').text()).toBe('24/11/2026 · C6 Carbon · final 1234');
    expect(header.findAll('.mark').map((mark) => mark.text())).toEqual(['Internacional']);
  });

  it('signs an incoming amount', async () => {
    const { wrapper } = await openOn('t32');
    expect(wrapper.find('.panel .amount').text()).toBe('+\u00a0R$\u00a0150,00');
    expect(wrapper.find('.panel .name').text()).toBe('Ana');
  });
});

describe('which sections each kind of transaction shows', () => {
  const COMMON = ['Linha do budget', 'Tags', 'Nota', 'Não conta', 'Detalhes', 'Histórico'];

  it('shows a card purchase the establishment, the split and the reimbursement', async () => {
    const { wrapper } = await openOn('t2');
    expect(labels(wrapper)).toEqual([
      'Transação',
      'Linha do budget',
      'Estabelecimento',
      'Tags',
      'Nota',
      'Dividir',
      'Reembolsável',
      'Não conta',
      'Detalhes',
      'Histórico',
    ]);
  });

  it('shows a PIX to a person the nickname instead of the establishment', async () => {
    const { wrapper } = await openOn('t31');
    expect(labels(wrapper)).toContain('Apelido da pessoa');
    expect(labels(wrapper)).not.toContain('Estabelecimento');
    expect(wrapper.find('.panel .hint').text()).toBe('Vale para todo PIX com Marcos A. Lima.');
    expect(labels(wrapper)).toContain('Dividir');
  });

  it('shows a refund the switch and the line it reduces, and neither split nor reimbursement', async () => {
    const { wrapper } = await openOn('t10');
    expect(labels(wrapper)).toContain('Marcar como estorno');
    expect(labels(wrapper)).not.toContain('Dividir');
    expect(labels(wrapper)).not.toContain('Reembolsável');
    expect(wrapper.find('.panel .offsets').text()).toBe('Abate a linha:Nenhuma ▾');
  });

  it('shows a movement only what every transaction has', async () => {
    const { wrapper } = await openOn('t52');
    expect(labels(wrapper)).toEqual(['Transação', ...COMMON]);
  });

  it('hides the refund line while the switch is off', async () => {
    const { wrapper } = await openOn('t11');
    expect(wrapper.find('.panel .offsets').exists()).toBe(false);
  });
});

describe('the line of the budget', () => {
  it('says where the line came from', async () => {
    const { wrapper } = await openOn('t2');
    expect(wrapper.find('.panel .line-button').text()).toBe('Transporte· definida por Regra▾');
    expect(wrapper.find('.panel .justification').exists()).toBe(false);
  });

  it('asks to confirm or change what the model chose, with its reason', async () => {
    const { wrapper } = await openOn('t4');
    expect(wrapper.find('.panel .line-button').text()).toContain('IA, 62% de confiança');
    expect(wrapper.find('.panel .justification').text()).toBe('Compras anteriores neste estabelecimento foram todas em Mercado.');
    expect(wrapper.findAll('.panel .actions button').map((button) => button.text())).toEqual(['Confirmar', 'Alterar']);
  });

  it('does the same for a pending suggestion, in the suggestion\'s name', async () => {
    const { wrapper } = await openOn('t5');
    expect(wrapper.find('.panel .line-button').text()).toBe('Restaurantes e bares· sugestão da IA, 58% de confiança▾');
    expect(wrapper.findAll('.panel .actions button')).toHaveLength(2);
  });

  it('does not ask why before the model was corrected', async () => {
    const { wrapper } = await openOn('t4');
    expect(wrapper.find('.panel .reason').exists()).toBe(false);
  });
});

describe('the other sections, untouched', () => {
  it('shows the establishment\'s default line and fixed tags', async () => {
    const { wrapper } = await openOn('t16');
    expect(wrapper.find('.panel .select').text()).toBe('Nenhuma ▾');
    expect(wrapper.findAll('.panel .row .chip').map((chip) => chip.text())).toEqual(['#viagem ×']);
  });

  it('keeps the exclusion and the split closed', async () => {
    const { wrapper } = await openOn('t2');
    expect(wrapper.findAll('.panel [role="switch"]').map((node) => node.attributes('aria-checked'))).toEqual(['false', 'false']);
    expect(wrapper.find('.panel .part').exists()).toBe(false);
    expect(wrapper.find('.panel .toggle').text()).toBe('Dividir em linhas');
  });

  it('lists the history newest first and offers to undo only the steps that changed something', async () => {
    const { wrapper } = await openOn('t36');
    const steps = wrapper.findAll('.panel .step');
    expect(steps.map((step) => step.find('.text').text())).toEqual([
      'Você alterou para Mercado · 04/11 · "é o mercado do bairro"',
      'IA classificou como Lazer · 03/11',
    ]);
    expect(steps.map((step) => step.find('.undo').exists())).toEqual([true, false]);
  });
});

describe('the line menus ask for the month they are for', () => {
  const NOV = '2026-11';
  const OCT = '2026-10';
  const PanelOnly = defineComponent({ components: { TransactionPanel }, template: '<TransactionPanel />' });

  /** t60 is an October card purchase, split; t64 an October income marked as a refund; t2 a purchase of November. */
  function octoberWithSplitAndRefund() {
    const state = prototypeState();
    state.transactions.find((transaction) => transaction.id === 't60')!.split = [
      { line: 'Mercado', amount: 40 },
      { line: null, amount: 24.2 },
    ];
    state.transactions.find((transaction) => transaction.id === 't64')!.refund = true;
    return state;
  }

  /** `frame: false` mounts the panel before the shell has its overview, so the current month is not known yet. */
  async function mountPanel(options: { named?: boolean; frame?: boolean } = { named: true }) {
    const api = createFakeApi({ startLocked: false, state: octoberWithSplitAndRefund() });
    const recorded = recordLineMenu(api, options);
    const mounted = await mountWithFailures(PanelOnly, [{ path: '/', component: { template: '<div />' } }], '/', {
      api,
      frame: options.frame ?? true,
      mounting: { attachTo: document.body },
    });
    const overlay = useOverlayStore(mounted.pinia);
    const open = async (id: string) => {
      usePanelStore(mounted.pinia).open(id);
      await mounted.settle();
    };
    const menuOf = async (selector: string) => {
      await mounted.wrapper.find(selector).trigger('click');
      return overlay.menu!.items.map((item) => item.label);
    };
    const defaultLineMenu = () => menuOf('.panel .section:has(.nickname) .select');
    return { ...mounted, ...recorded, open, menuOf, defaultLineMenu };
  }

  it('asks nothing while no transaction is open, not even after a write elsewhere', async () => {
    const m = await mountPanel();
    useRevisionStore(m.pinia).bump();
    await m.settle();
    expect(m.months).toEqual([]);
  });

  it('waits for the current month when the shell has no overview yet, then asks for both months and shows', async () => {
    const m = await mountPanel({ named: true, frame: false });
    await m.open('t60');
    expect(m.months).toEqual([]);
    expect(m.wrapper.find('.panel').exists()).toBe(false);

    useShellStore(m.pinia).overview = prototypeOverview();
    await m.settle();
    expect([...m.months].sort()).toEqual([OCT, NOV]);
    expect(m.wrapper.find('.panel .name').text()).toBe('Pão de Açúcar');
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 1)]);
    expect(await m.defaultLineMenu()).toEqual([lineNamed(NOV, 1)]);
  });

  it('asks once for a transaction of the current month, whose answer serves every menu', async () => {
    const m = await mountPanel();
    await m.open('t2');
    expect(m.months).toEqual([NOV]);
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(NOV, 1)]);
    expect(await m.defaultLineMenu()).toEqual([lineNamed(NOV, 1)]);
  });

  it('lists the lines of the transaction\'s month for its line, a split part\'s and the refund\'s', async () => {
    const m = await mountPanel();
    await m.open('t60');
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 1)]);
    expect(await m.menuOf('.panel .part .pick')).toEqual([lineNamed(OCT, 1)]);
    await m.open('t64');
    expect(await m.menuOf('.panel .offsets .select')).toEqual([lineNamed(OCT, 2)]);
    expect(new Set(m.months)).toEqual(new Set([OCT, NOV]));
  });

  it('lists the current month\'s lines for the establishment\'s default line of an earlier transaction', async () => {
    const m = await mountPanel();
    await m.open('t60');
    expect(await m.defaultLineMenu()).toEqual([lineNamed(NOV, 1)]);
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 1)]);
    expect([...m.months].sort()).toEqual([OCT, NOV]);
  });

  it('shows the month of the transaction opened last, also when the one before answers after it', async () => {
    const m = await mountPanel();
    const release = m.hold(OCT);
    usePanelStore(m.pinia).open('t60');
    await m.settle();
    await m.open('t2');
    release();
    await m.settle();
    expect(m.months).toContain(OCT);
    expect(m.months.at(-1)).toBe(NOV);
    expect(m.wrapper.find('.panel .name').text()).toBe('Posto Shell');
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(NOV, 2)]);
  });

  it('shows nothing to pick from until the menus of the transaction\'s month arrived', async () => {
    const m = await mountPanel();
    const release = m.hold(OCT);
    await m.open('t60');
    expect(m.wrapper.find('.panel .line-button').exists()).toBe(false);
    release();
    await m.settle();
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 1)]);
  });

  it('opens its line menu on the month\'s lines right away, never empty, over the prototype\'s dataset', async () => {
    const m = await mountPanel({ named: false });
    await m.open('t2');
    const labels = await m.menuOf('.panel .line-button');
    expect(labels.length).toBeGreaterThan(0);
    expect(labels).toEqual((await createFakeApi().transactions.filterOptions(NOV)).lines.map((line) => line.name));
  });

  it('asks again for the months it holds after a write, and its menus show the new answer without reopening', async () => {
    const m = await mountPanel();
    await m.open('t60');
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 1)]);
    useRevisionStore(m.pinia).bump();
    await m.settle();
    expect([...m.months].sort()).toEqual([OCT, OCT, NOV, NOV]);
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 2)]);
    expect(await m.menuOf('.panel .part .pick')).toEqual([lineNamed(OCT, 2)]);
    expect(await m.defaultLineMenu()).toEqual([lineNamed(NOV, 2)]);
    expect(await m.menuOf('.panel .section:has(.nickname) .add')).toEqual([tagNamed(OCT, 2)]);
  });

  it('drops an in-flight menu answer when the panel closes before it arrives', async () => {
    const m = await mountPanel();
    const releaseFirst = m.hold(OCT);
    await m.open('t60');
    expect(m.wrapper.find('.panel .line-button').exists()).toBe(false);

    usePanelStore(m.pinia).close();
    await m.settle();
    releaseFirst();
    await m.settle();

    const releaseSecond = m.hold(OCT);
    await m.open('t60');
    expect(m.wrapper.find('.panel .line-button').exists()).toBe(false);

    releaseSecond();
    await m.settle();
    expect(await m.menuOf('.panel .line-button')).toEqual([lineNamed(OCT, 2)]);
  });
});

describe('TransactionDetail out-of-order loads', () => {
  it('does not let an earlier detail answer replace a later one for the same transaction', async () => {
    const { wrapper, api } = await openOn('t2');
    const first = holdFirstCall(api.transactions, 'detail');
    useRevisionStore().bump();
    await flushPromises();

    await api.transactions.setLine('t2', 'Lazer');
    useRevisionStore().bump();
    await flushPromises();
    expect(wrapper.find('.panel .line-button').text()).toContain('Lazer');

    first.release();
    await flushPromises();
    expect(wrapper.find('.panel .line-button').text()).toContain('Lazer');
  });
});
