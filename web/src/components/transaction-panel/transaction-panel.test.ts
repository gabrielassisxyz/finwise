import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent } from 'vue';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { useMonthStore } from '@/stores/month';
import { usePanelStore } from '@/stores/panel';
import { useShellStore } from '@/stores/shell';
import { mountAt, prototypeOverview } from '@/test/mount';
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
