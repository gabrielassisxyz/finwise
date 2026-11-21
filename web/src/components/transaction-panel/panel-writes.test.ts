import { flushPromises, type DOMWrapper } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent } from 'vue';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { useRevisionStore } from '@/stores/revision';
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

async function openOn(id: string) {
  const mounted = await mountAt(Host, '/', { attachTo: document.body });
  useShellStore().overview = prototypeOverview();
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  usePanelStore().open(id);
  await flushPromises();
  const { wrapper } = mounted;
  const panelText = (selector: string) => wrapper.find(`.panel ${selector}`).text();
  const click = async (selector: string, nth = 0) => {
    await wrapper.findAll(`.panel ${selector}`)[nth]!.trigger('click');
    await flushPromises();
  };
  const commit = async (field: DOMWrapper<Element>, value: string) => {
    (field.element as HTMLInputElement).value = value;
    await field.trigger('change');
    await flushPromises();
  };
  const pickFromMenu = async (label: string) => {
    useOverlayStore().menu!.items.find((item) => item.label === label)!.onPick!();
    await flushPromises();
  };
  const toggle = (name: string) => wrapper.find(`.panel [role="switch"][aria-label="${name}"]`);
  return { ...mounted, panelText, click, commit, pickFromMenu, toggle, overlay: useOverlayStore(), revision: useRevisionStore() };
}

const listRow = (wrapper: Awaited<ReturnType<typeof openOn>>['wrapper'], name: string) =>
  wrapper.findAll('.row').find((row) => row.find('.name').text() === name)!;

describe('confirming and correcting the model', () => {
  it('confirms a suggestion: the line is set, the doubt is gone and the list follows', async () => {
    const { wrapper, panelText, click, revision } = await openOn('t5');
    const before = revision.current;
    expect(listRow(wrapper, 'Bar do Zé').find('.label').text()).toBe('Restaurantes e bares?');

    await click('.confirm');
    expect(panelText('.line-button')).toBe('Restaurantes e bares· definida por Você▾');
    expect(wrapper.find('.panel .justification').exists()).toBe(false);
    expect(wrapper.find('.panel .step .text').text()).toMatch(/^Você confirmou Restaurantes e bares · 28\/11$/);
    expect(listRow(wrapper, 'Bar do Zé').find('.label').text()).toBe('Restaurantes e bares');
    expect(revision.current).toBeGreaterThan(before);
  });

  it('changes the line from the menu, offers the establishment in a toast and takes it back', async () => {
    const { wrapper, click, pickFromMenu, overlay } = await openOn('t4');
    await click('.change');
    expect(overlay.menu!.items.some((item) => item.label === 'Casa')).toBe(true);
    await pickFromMenu('Casa');

    expect(wrapper.find('.panel .line-button').text()).toContain('Casa');
    expect(overlay.toast!.message).toBe('Linha alterada para Casa. Aplicar a todas as compras em Pão de Açúcar?');
    expect(overlay.toast!.actions.map((action) => action.label)).toEqual(['Só esta', 'Todas', 'Desfazer']);

    overlay.toast!.actions[2]!.run();
    await flushPromises();
    expect(wrapper.find('.panel .line-button').text()).toContain('IA, 62% de confiança');
  });

  it('asks why after correcting the model and keeps the answer in the history', async () => {
    const { wrapper, click, pickFromMenu, commit } = await openOn('t4');
    expect(wrapper.find('.panel .reason').exists()).toBe(false);
    await click('.change');
    await pickFromMenu('Casa');

    await commit(wrapper.find('.panel .reason'), 'é do apartamento');
    expect(wrapper.find('.panel .step .text').text()).toBe('Você alterou para Casa · 28/11 · "é do apartamento"');
  });

  it('takes a corrected line back from the history', async () => {
    const { wrapper, click, pickFromMenu } = await openOn('t5');
    await click('.change');
    await pickFromMenu('Casa');
    await click('.step .undo');
    expect(wrapper.find('.panel .line-button').text()).toContain('sugestão da IA, 58% de confiança');
    expect(wrapper.find('.panel .reason').exists()).toBe(false);
    expect(wrapper.findAll('.panel .step')).toHaveLength(1);
  });
});

describe('the establishment and the person', () => {
  it('renames the establishment everywhere it appears', async () => {
    const { wrapper, click, panelText } = await openOn('t2');
    await click('.link');
    await wrapper.find('.panel .nickname-field').setValue('Shell');
    await click('.save');
    expect(panelText('.nickname .name')).toBe('Shell');
    expect(panelText('.header .name')).toBe('Shell');
    expect(listRow(wrapper, 'Shell').exists()).toBe(true);
  });

  it('sets the default line from the menu', async () => {
    const { panelText, click, pickFromMenu } = await openOn('t16');
    await click('.select');
    await pickFromMenu('Lazer');
    expect(panelText('.select')).toBe('Lazer ▾');
  });

  it('adds a fixed tag from the menu, without offering one it has, and removes it', async () => {
    const { wrapper, click, pickFromMenu, overlay } = await openOn('t16');
    await click('.add');
    expect(overlay.menu!.title).toBe('Tag fixa para Pousada Serra Azul');
    expect(overlay.menu!.items.map((item) => item.label)).toEqual(['#com-ana', '#trabalho', '#presente', '#casa-nova']);
    await pickFromMenu('#trabalho');
    expect(wrapper.findAll('.panel .row .chip').map((chip) => chip.text())).toEqual(['#viagem ×', '#trabalho ×']);
    await click('.row .chip', 1);
    expect(wrapper.findAll('.panel .row .chip').map((chip) => chip.text())).toEqual(['#viagem ×']);
  });

  it('gives a person a nickname', async () => {
    const { wrapper, commit, panelText } = await openOn('t31');
    await commit(wrapper.find('.panel .field'), 'Marcos');
    expect(panelText('.header .name')).toBe('Marcos');
    expect(listRow(wrapper, 'Marcos').exists()).toBe(true);
  });
});

describe('tags and the note', () => {
  it('adds a typed tag on Enter with its # and hyphens, and clears the field', async () => {
    const { wrapper } = await openOn('t2');
    const field = wrapper.find('.panel .field');
    await field.setValue('viagem de trabalho');
    await field.trigger('keydown', { key: 'Enter' });
    await flushPromises();
    expect(wrapper.findAll('.panel .chips .chip').map((chip) => chip.text())).toEqual(['#viagem-de-trabalho ×']);
    expect((wrapper.find('.panel .field').element as HTMLInputElement).value).toBe('');
    expect(listRow(wrapper, 'Posto Shell').text()).toContain('#viagem-de-trabalho');
  });

  it('ignores Enter on an empty field', async () => {
    const { wrapper, revision } = await openOn('t2');
    const before = revision.current;
    await wrapper.find('.panel .field').trigger('keydown', { key: 'Enter' });
    expect(revision.current).toBe(before);
  });

  it('adds a suggestion, narrows the suggestions as you type, and removes a chip', async () => {
    const { wrapper, click } = await openOn('t2');
    await wrapper.find('.panel .field').setValue('pres');
    expect(wrapper.findAll('.panel .suggestion').map((tag) => tag.text())).toEqual(['#presente']);
    await click('.suggestion');
    expect(wrapper.findAll('.panel .chips .chip').map((chip) => chip.text())).toEqual(['#presente ×']);
    await click('.chips .chip');
    expect(wrapper.findAll('.panel .chips .chip')).toHaveLength(0);
  });

  it('keeps the note', async () => {
    const { wrapper, commit } = await openOn('t2');
    await commit(wrapper.find('.panel .note'), 'abastecer na sexta');
    expect((wrapper.find('.panel .note').element as HTMLTextAreaElement).value).toBe('abastecer na sexta');
  });
});

describe('splitting', () => {
  it('opens with two parts that close with the total, and shows what is missing as it is edited', async () => {
    const { wrapper, click, commit, panelText } = await openOn('t2');
    await click('.toggle');
    expect(wrapper.findAll('.panel .part')).toHaveLength(2);
    expect(panelText('.remaining')).toBe('Fecha com o total');
    expect(panelText('.pick')).toBe('Transporte ▾');

    await commit(wrapper.findAll('.panel .value')[0]!, '100');
    expect(panelText('.remaining')).toBe('Faltam R$ 80,00');
    expect(wrapper.find('.panel .remaining').classes()).not.toContain('closed');
    await commit(wrapper.findAll('.panel .value')[1]!, '90');
    expect(panelText('.remaining')).toBe('Passou R$ 10,00');
    expect(listRow(wrapper, 'Posto Shell').text()).toContain('Dividida');
  });

  it('picks the line of a part, adds a part with what is left, and removes parts down to none', async () => {
    const { wrapper, click, commit, pickFromMenu } = await openOn('t2');
    await click('.toggle');
    await click('.pick', 1);
    await pickFromMenu('Lazer');
    expect(wrapper.findAll('.panel .pick')[1]!.text()).toBe('Lazer ▾');

    await commit(wrapper.findAll('.panel .value')[0]!, '100');
    await click('.foot .add');
    expect(wrapper.findAll('.panel .part')).toHaveLength(3);
    expect((wrapper.findAll('.panel .value')[2]!.element as HTMLInputElement).value).toBe('80');

    await click('.remove', 2);
    await click('.remove', 1);
    expect(wrapper.find('.panel .part').exists()).toBe(false);
    expect(wrapper.find('.panel .toggle').text()).toBe('Dividir em linhas');
  });
});

describe('reimbursement', () => {
  it('expects half back, lets the amount change and links a received PIX', async () => {
    const { wrapper, click, commit, toggle, panelText } = await openOn('t2');
    await toggle('Reembolsável').trigger('click');
    await flushPromises();
    const expected = () => wrapper.find('.panel .expected input').element as HTMLInputElement;
    expect(expected().value).toBe('90');
    await commit(wrapper.find('.panel .expected input'), '120');
    expect(expected().value).toBe('120');

    await click('.link-button');
    expect(wrapper.findAll('.panel .candidate').map((row) => row.text())).toEqual([
      'Ana · 12/11+ R$ 150,00',
      'Carlos · 11/11+ R$ 200,00',
    ]);
    await click('.candidate');
    expect(panelText('.linked-title')).toBe('Reembolsado: R$ 150,00 de Ana em 12/11');
    expect(panelText('.linked-net')).toBe('Esta compra conta por R$ 30,00.');
    expect(panelText('.header .amount')).toBe('R$ 30,00');
    expect(listRow(wrapper, 'Posto Shell').find('.amount-before').text()).toBe('R$ 180,00');
    expect(wrapper.find('.panel .linking, .panel .candidates').exists()).toBe(false);
  });

  it('undoes the link and drops everything when switched off', async () => {
    const { wrapper, click, toggle, panelText } = await openOn('t2');
    await toggle('Reembolsável').trigger('click');
    await flushPromises();
    await click('.link-button');
    await click('.candidate');
    await click('.unlink');
    expect(panelText('.header .amount')).toBe('R$ 180,00');
    expect(wrapper.find('.panel .expected').exists()).toBe(true);

    await toggle('Reembolsável').trigger('click');
    await flushPromises();
    expect(wrapper.find('.panel .expected').exists()).toBe(false);
    expect(toggle('Reembolsável').attributes('aria-checked')).toBe('false');
  });
});

describe('"não conta"', () => {
  it('shows the amber warning and still counts until a reason is given', async () => {
    const { wrapper, toggle, commit } = await openOn('t2');
    await toggle('Não conta').trigger('click');
    await flushPromises();
    expect(wrapper.find('.panel .reason').classes()).toContain('missing');
    expect(wrapper.find('.panel .warning').text()).toBe('Só vale depois de informar o motivo.');
    expect(listRow(wrapper, 'Posto Shell').classes()).not.toContain('faded');
    expect(wrapper.find('.panel .header .mark').exists()).toBe(false);

    await commit(wrapper.find('.panel .reason'), 'Compra da empresa');
    expect(wrapper.find('.panel .warning').exists()).toBe(false);
    expect(listRow(wrapper, 'Posto Shell').classes()).toContain('faded');
    expect(wrapper.find('.panel .header .mark').text()).toBe('Não conta');
  });

  it('counts again when switched off, and says so in the history', async () => {
    const { wrapper, toggle, commit } = await openOn('t2');
    await toggle('Não conta').trigger('click');
    await flushPromises();
    await commit(wrapper.find('.panel .reason'), 'Compra da empresa');
    await toggle('Não conta').trigger('click');
    await flushPromises();
    expect(listRow(wrapper, 'Posto Shell').classes()).not.toContain('faded');
    expect(wrapper.find('.panel .reason').exists()).toBe(false);
    expect(wrapper.find('.panel .step .text').text()).toBe('Você voltou a contar esta transação · 28/11');
  });
});

describe('a refund', () => {
  it('marks a credit as a refund and picks the line it reduces', async () => {
    const { wrapper, toggle, click, pickFromMenu, panelText } = await openOn('t33');
    expect(wrapper.find('.panel .offsets').exists()).toBe(false);
    await toggle('Marcar como estorno').trigger('click');
    await flushPromises();
    expect(panelText('.offsets')).toBe('Abate a linha:Nenhuma ▾');
    expect(panelText('.header .mark')).toBe('Estorno');

    await click('.offsets .select');
    await pickFromMenu('Mercado');
    expect(panelText('.offsets')).toBe('Abate a linha:Mercado ▾');
    expect(wrapper.findAll('.panel .step .text').map((step) => step.text())).toEqual([
      'Você definiu que o estorno abate Mercado · 28/11',
      'Você marcou como estorno · 28/11',
      'Regra classificou como Renda extra · 11/11',
    ]);
  });
});

describe('taking a step back from the history', () => {
  it('restores the state before that step alone', async () => {
    const { wrapper, click } = await openOn('t36');
    await click('.step .undo');
    expect(wrapper.find('.panel .line-button').text()).toContain('Lazer');
    expect(wrapper.find('.panel .line-button').text()).toContain('IA, 54% de confiança');
    expect(wrapper.findAll('.panel .step .text').map((step) => step.text())).toEqual(['IA classificou como Lazer · 03/11']);
    expect(wrapper.findAll('.panel .step .undo')).toHaveLength(0);
  });
});
