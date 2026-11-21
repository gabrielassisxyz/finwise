import { flushPromises, type VueWrapper } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { prototypeState } from '@/api/fake/prototype-data';
import { useOverlayStore } from '@/stores/overlay';
import { useShellStore } from '@/stores/shell';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import ConfigScreen from './ConfigScreen.vue';

const routes = [{ path: '/config/:section?', component: ConfigScreen }];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountConfig(path = '/config', state = prototypeState()) {
  const mounted = await mountRouted(ConfigScreen, routes, path, { state });
  useShellStore().overview = prototypeOverview();
  await flushPromises();
  return { ...mounted, state, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountConfig>>;

const section = (wrapper: VueWrapper, id: string) => wrapper.find(`[data-section="${id}"]`);

async function type(wrapper: VueWrapper, selector: string, value: string, event = 'change') {
  const input = wrapper.find(selector);
  (input.element as HTMLInputElement).value = value;
  await input.trigger(event);
  await flushPromises();
}

async function pickFromMenu({ overlay }: Mounted, label: string) {
  overlay.menu!.items.find((item) => item.label === label)!.onPick!();
  await flushPromises();
}


describe('Configuração layout', () => {
  it('has the six sections, in order, and a chip for each', async () => {
    const { wrapper } = await mountConfig();
    expect(wrapper.findAll('[data-section]').map((node) => node.attributes('data-section'))).toEqual([
      'cofrinhos',
      'apelidos',
      'categorizacao',
      'cartao',
      'pin',
      'dados',
    ]);
    expect(wrapper.findAll('.chip').map((chip) => chip.text())).toEqual([
      'Cofrinhos',
      'Apelidos',
      'Categorização',
      'Cartão',
      'PIN',
      'Dados',
    ]);
  });

  it('shows the first section on a phone when no section is named, and marks the others as hidden', async () => {
    const { wrapper } = await mountConfig('/config');
    const hidden = wrapper.findAll('.section.inactive').map((node) => node.attributes('data-section'));
    expect(hidden).toEqual(['apelidos', 'categorizacao', 'cartao', 'pin', 'dados']);
    expect(wrapper.find('.chip.active').text()).toBe('Cofrinhos');
  });

  it('opens the section named in the path, and falls back to the first for a name it does not know', async () => {
    const dados = await mountConfig('/config/dados');
    expect(dados.wrapper.find('.chip.active').text()).toBe('Dados');
    expect(dados.wrapper.find('.section:not(.inactive)').attributes('data-section')).toBe('dados');
    const unknown = await mountConfig('/config/nada');
    expect(unknown.wrapper.find('.chip.active').text()).toBe('Cofrinhos');
  });

  it('changes section from a chip, through the path', async () => {
    const { wrapper, router } = await mountConfig();
    await wrapper.findAll('.chip')[3]!.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/config/cartao');
    expect(wrapper.find('.chip.active').text()).toBe('Cartão');
  });
});

describe('Cofrinhos section', () => {
  it('lists each open pot with its bank, rate, bucket and linked lines', async () => {
    const { wrapper } = await mountConfig();
    const rows = section(wrapper, 'cofrinhos').findAll('.row');
    expect(rows).toHaveLength(13);
    expect(rows[0]!.find('.name').text()).toBe('RF Saúde');
    expect(rows[0]!.find('.bank').text()).toBe('Mercado Pago · 120% do CDI');
    expect(rows[0]!.find('.bucket').text()).toBe('Reservado ▾');
    expect(rows[0]!.find('.lines span').text()).toBe('Rainy Fund · Saúde');
    expect(rows[4]!.find('.lines span').text()).toBe('Gás · IPTU');
  });

  it('shows the pot without a bucket in amber, and the one without a name or lines as such', async () => {
    const { wrapper } = await mountConfig();
    const last = section(wrapper, 'cofrinhos').findAll('.row').at(-1)!;
    expect(last.find('.bucket').text()).toBe('Sem balde ▾');
    expect(last.find('.bucket').classes()).toContain('unset');
    expect(last.find('.name').text()).toBe('Sem nome');
    expect(last.find('.lines span').text()).toBe('Nenhuma linha');
    expect(section(wrapper, 'cofrinhos').findAll('.bucket.unset')).toHaveLength(1);
  });

  it('offers the three buckets with the current one marked, and writes the one picked', async () => {
    const mounted = await mountConfig();
    const last = section(mounted.wrapper, 'cofrinhos').findAll('.row').at(-1)!;
    await last.find('.bucket').trigger('click');
    expect(mounted.overlay.menu!.title).toBe('Balde');
    expect(mounted.overlay.menu!.items.map((item) => [item.label, item.current ?? false])).toEqual([
      ['Disponível', false],
      ['Reservado', false],
      ['Investido', false],
    ]);
    await pickFromMenu(mounted, 'Investido');
    await section(mounted.wrapper, 'cofrinhos').findAll('.bucket')[0]!.trigger('click');
    expect(mounted.overlay.menu!.items.filter((item) => item.current).map((item) => item.label)).toEqual(['Reservado']);
    const after = section(mounted.wrapper, 'cofrinhos').findAll('.row').at(-1)!;
    expect(after.find('.bucket').text()).toBe('Investido ▾');
    expect(after.find('.bucket').classes()).not.toContain('unset');
    expect(mounted.state.savingsPots.find((pot) => pot.id === 'sn')!.bucket).toBe('Investido');
  });

  it('moves the net worth the home screen shows when a bucket changes', async () => {
    const mounted = await mountConfig();
    const available = async () => (await mounted.api.home.summary('2026-11')).netWorth.available;
    const before = await available();
    await mounted.wrapper.findAll('.bucket').at(-1)!.trigger('click');
    await pickFromMenu(mounted, 'Disponível');
    expect(await available()).toBe(before + 500);
  });

  it('opens the menu of budget lines from Editar and links or unlinks the one picked', async () => {
    const mounted = await mountConfig();
    const remedios = section(mounted.wrapper, 'cofrinhos').findAll('.row')[6]!;
    await remedios.find('.edit').trigger('click');
    await flushPromises();
    expect(mounted.overlay.menu!.search).toBe(true);
    expect(mounted.overlay.menu!.items[0]).toMatchObject({ label: 'Mercado', hint: 'Variáveis' });
    await pickFromMenu(mounted, 'Remédios');
    expect(section(mounted.wrapper, 'cofrinhos').findAll('.row')[6]!.find('.lines span').text()).toBe('Nenhuma linha');
    await section(mounted.wrapper, 'cofrinhos').findAll('.row')[6]!.find('.edit').trigger('click');
    await flushPromises();
    await pickFromMenu(mounted, 'Saúde');
    expect(section(mounted.wrapper, 'cofrinhos').findAll('.row')[6]!.find('.lines span').text()).toBe('Saúde');
  });
});

describe('Apelidos section', () => {
  it('lists the establishments with their text, nickname, default line and fixed tags', async () => {
    const { wrapper } = await mountConfig();
    const rows = section(wrapper, 'apelidos').findAll('.row');
    expect(rows).toHaveLength(10);
    expect(rows[1]!.find('.raw').text()).toBe('POSTO SHELL AV PAULISTA');
    expect(rows[1]!.find('.line').text()).toBe('Transporte ▾');
    expect(rows[0]!.find('.line').text()).toBe('Nenhuma ▾');
    expect(rows[0]!.find('.tags').text()).toBe('—');
    expect((rows[1]!.find('input').element as HTMLInputElement).value).toBe('');
  });

  it('lists the people with their nicknames and the card with its label', async () => {
    const { wrapper } = await mountConfig();
    const people = section(wrapper, 'apelidos').findAll('.person');
    expect(people.map((row) => row.find('.full').text())).toEqual([
      'Marcos A. Lima',
      'Ana C. Ribeiro',
      'Carlos Mendes',
      'João P. Santos',
      'C6 Carbon · final 1234',
    ]);
    const values = people.map((row) => (row.find('input').element as HTMLInputElement).value);
    expect(values).toEqual(['', 'Ana', 'Carlos', 'Aluguel', 'Físico']);
  });

  it('renames an establishment in the list of transactions when its nickname is written', async () => {
    const mounted = await mountConfig();
    const input = section(mounted.wrapper, 'apelidos').findAll('.row')[1]!.find('input');
    (input.element as HTMLInputElement).value = 'Gasolina';
    await input.trigger('change');
    await flushPromises();
    const rows = await mounted.api.transactions.list({ month: '2026-11', period: 'month' });
    expect(rows.find((row) => row.id === 't2')!.name).toBe('Gasolina');
    expect((section(mounted.wrapper, 'apelidos').findAll('.row')[1]!.find('input').element as HTMLInputElement).value).toBe('Gasolina');
  });

  it('writes a person nickname and the card label', async () => {
    const mounted = await mountConfig();
    const people = () => section(mounted.wrapper, 'apelidos').findAll('.person');
    (people()[0]!.find('input').element as HTMLInputElement).value = 'Marcos';
    await people()[0]!.find('input').trigger('change');
    (people()[4]!.find('input').element as HTMLInputElement).value = 'Virtual';
    await people()[4]!.find('input').trigger('change');
    await flushPromises();
    const stored = await mounted.api.settings.overview();
    expect(stored.people[0]!.nickname).toBe('Marcos');
    expect(stored.cardLabel).toBe('Virtual');
  });

  it('sets the default line from the menu of lines', async () => {
    const mounted = await mountConfig();
    await section(mounted.wrapper, 'apelidos').findAll('.row')[2]!.find('.line').trigger('click');
    await flushPromises();
    await pickFromMenu(mounted, 'Delivery');
    expect(section(mounted.wrapper, 'apelidos').findAll('.row')[2]!.find('.line').text()).toBe('Delivery ▾');
  });
});

describe('Categorização section', () => {
  const slider = (wrapper: VueWrapper) => wrapper.find('input[type="range"]');

  it('states the threshold and the suggestion, with the slider between 50 and 99', async () => {
    const { wrapper } = await mountConfig();
    expect(wrapper.findAll('.slider > span').map((node) => node.text())).toEqual(['Aplicar sozinho a partir de', '85', '% de confiança']);
    expect(slider(wrapper).attributes()).toMatchObject({ min: '50', max: '99' });
    expect((slider(wrapper).element as HTMLInputElement).value).toBe('85');
    expect(wrapper.findAll('.legend .key').map((node) => plain(node.text()))).toEqual(['Limite atual:85%', 'Sugerido: 80%']);
    expect(wrapper.find('.use').text()).toBe('Usar o sugerido');
  });

  it('draws five bars with the calibration and the two markers where the thresholds are', async () => {
    const { wrapper } = await mountConfig();
    expect(wrapper.findAll('.band .value').map((node) => node.text())).toEqual(['46%', '31%', '18%', '7%', '2%']);
    expect(wrapper.findAll('.ranges span').map((node) => node.text())).toEqual(['50–59', '60–69', '70–79', '80–89', '90–100']);
    expect(wrapper.find('.marker.current').attributes('style')).toContain('left: 70%');
    expect(wrapper.find('.marker.suggested').attributes('style')).toContain('left: 60%');
  });

  it('draws each bar as the design scales it: 1.6 per percent corrected', async () => {
    const { wrapper } = await mountConfig();
    const heights = wrapper.findAll('.bar').map((bar) => bar.attributes('style'));
    expect(heights).toEqual(['height: 73.6%;', 'height: 49.6%;', 'height: 28.8%;', 'height: 11.2%;', 'height: 3.2%;']);
  });

  it('follows the slider while it moves and writes only when it is released', async () => {
    const mounted = await mountConfig();
    const input = slider(mounted.wrapper);
    (input.element as HTMLInputElement).value = '60';
    await input.trigger('input');
    expect(mounted.wrapper.find('.slider > span:nth-of-type(2)').text()).toBe('60');
    expect(mounted.wrapper.find('.marker.current').attributes('style')).toContain('left: 20%');
    expect(mounted.state.confidenceThreshold).toBe(85);
    await input.trigger('change');
    await flushPromises();
    expect(mounted.state.confidenceThreshold).toBe(60);
  });

  it('changes which transactions carry the unsure mark, for every screen', async () => {
    const mounted = await mountConfig();
    const marked = async () =>
      (await mounted.api.transactions.list({ month: '2026-11', period: 'month' })).filter((row) => row.belowThreshold).length;
    const before = await marked();
    const input = slider(mounted.wrapper);
    (input.element as HTMLInputElement).value = '60';
    await input.trigger('input');
    await input.trigger('change');
    await flushPromises();
    expect(await marked()).toBeLessThan(before);
  });

  it('uses the suggested threshold, says so with a toast, and takes it back from the toast', async () => {
    const mounted = await mountConfig();
    await mounted.wrapper.find('.use').trigger('click');
    await flushPromises();
    expect(mounted.state.confidenceThreshold).toBe(80);
    expect((slider(mounted.wrapper).element as HTMLInputElement).value).toBe('80');
    expect(mounted.overlay.toast!.message).toBe('Limite alterado para 80% de confiança.');
    expect(mounted.overlay.toast!.actions.map((action) => action.label)).toEqual(['Desfazer']);
    mounted.overlay.toast!.actions[0]!.run();
    await flushPromises();
    expect(mounted.state.confidenceThreshold).toBe(85);
    expect((slider(mounted.wrapper).element as HTMLInputElement).value).toBe('85');
    expect(mounted.overlay.toast).toBeNull();
  });

  it('writes the minimum of examples, and shows the bound the backend kept', async () => {
    const mounted = await mountConfig();
    await type(mounted.wrapper, 'input[type="number"]', '50');
    await type(mounted.wrapper, 'input[type="number"]', '80');
    expect(mounted.state.settings.minExamples).toBe(50);
    expect((mounted.wrapper.find('input[type="number"]').element as HTMLInputElement).value).toBe('50');
  });
});

describe('Cartão section', () => {
  it('starts automatic, with no day field', async () => {
    const { wrapper } = await mountConfig();
    const radios = section(wrapper, 'cartao').findAll('input[type="radio"]');
    expect(radios.map((radio) => (radio.element as HTMLInputElement).checked)).toEqual([true, false]);
    expect(section(wrapper, 'cartao').find('input[type="number"]').exists()).toBe(false);
    expect(section(wrapper, 'cartao').find('.caption').text()).toBe('Dia de fechamento · C6 Carbon · final 1234');
  });

  it('turns manual with a day field, and the card page follows the day', async () => {
    const mounted = await mountConfig();
    await section(mounted.wrapper, 'cartao').findAll('input[type="radio"]')[1]!.trigger('change');
    await flushPromises();
    const day = section(mounted.wrapper, 'cartao').find('input[type="number"]');
    expect(day.attributes()).toMatchObject({ min: '1', max: '28' });
    expect((day.element as HTMLInputElement).value).toBe('28');
    await type(mounted.wrapper, '[aria-label="Dia de fechamento"]', '20');
    expect((await mounted.api.card.overview()).closesOn).toBe('2026-12-20');
  });

  it('shows the day the backend kept when the one typed is out of bounds', async () => {
    const mounted = await mountConfig();
    await section(mounted.wrapper, 'cartao').findAll('input[type="radio"]')[1]!.trigger('change');
    await flushPromises();
    await type(mounted.wrapper, '[aria-label="Dia de fechamento"]', '40');
    expect(mounted.state.settings.manualClosingDay).toBe(28);
    expect((mounted.wrapper.find('[aria-label="Dia de fechamento"]').element as HTMLInputElement).value).toBe('28');
  });

  it('goes back to automatic and hides the day field', async () => {
    const mounted = await mountConfig();
    const radios = () => section(mounted.wrapper, 'cartao').findAll('input[type="radio"]');
    await radios()[1]!.trigger('change');
    await flushPromises();
    await radios()[0]!.trigger('change');
    await flushPromises();
    expect(section(mounted.wrapper, 'cartao').find('input[type="number"]').exists()).toBe(false);
    expect(mounted.state.settings.closingMode).toBe('auto');
  });
});

describe('PIN section', () => {
  async function openForm() {
    const mounted = await mountConfig();
    await section(mounted.wrapper, 'pin').find('.change').trigger('click');
    return mounted;
  }

  const fill = async ({ wrapper }: Mounted, values: string[]) => {
    const fields = section(wrapper, 'pin').findAll('input');
    for (const [index, value] of values.entries()) {
      (fields[index]!.element as HTMLInputElement).value = value;
      await fields[index]!.trigger('input');
    }
  };
  const save = async ({ wrapper }: Mounted) => {
    await wrapper.findAll('button').find((button) => button.text() === 'Salvar PIN')!.trigger('click');
    await flushPromises();
  };

  it('opens as a button and expands into three fields with Salvar PIN and Cancelar', async () => {
    const { wrapper } = await mountConfig();
    expect(section(wrapper, 'pin').find('.change').text()).toBe('Trocar PIN');
    expect(section(wrapper, 'pin').find('input').exists()).toBe(false);
    await section(wrapper, 'pin').find('.change').trigger('click');
    expect(section(wrapper, 'pin').findAll('label').map((label) => label.text())).toEqual(['PIN atual', 'Novo PIN', 'Repetir novo PIN']);
    expect(section(wrapper, 'pin').findAll('button').map((button) => button.text())).toEqual(['Salvar PIN', 'Cancelar']);
  });

  it('keeps only digits, six at most', async () => {
    const mounted = await openForm();
    await fill(mounted, ['12a3456789']);
    expect((section(mounted.wrapper, 'pin').find('input').element as HTMLInputElement).value).toBe('123456');
  });

  it.each([
    [['000000', '135790', '135790'], 'PIN atual incorreto'],
    [['123456', '1357', '1357'], 'O novo PIN precisa ter 6 dígitos'],
    [['123456', '135790', '135791'], 'Os dois campos do novo PIN não conferem'],
  ])('says what is wrong: %j', async (values, message) => {
    const mounted = await openForm();
    await fill(mounted, values);
    await save(mounted);
    expect(section(mounted.wrapper, 'pin').find('.error').text()).toBe(message);
  });

  it('clears the error as soon as a field is typed in', async () => {
    const mounted = await openForm();
    await save(mounted);
    expect(section(mounted.wrapper, 'pin').find('.error').text()).toBe('PIN atual incorreto');
    await fill(mounted, ['1']);
    expect(section(mounted.wrapper, 'pin').find('.error').text()).toBe('');
  });

  it('closes the form without changing anything on Cancelar', async () => {
    const mounted = await openForm();
    await fill(mounted, ['123456', '135790', '135790']);
    await section(mounted.wrapper, 'pin').findAll('button')[1]!.trigger('click');
    expect(section(mounted.wrapper, 'pin').find('.change').exists()).toBe(true);
    expect(await mounted.api.session.unlock('123456')).toEqual({ ok: true });
  });

  it('changes the PIN, says so, and the old one stops opening the app', async () => {
    const mounted = await openForm();
    await fill(mounted, ['123456', '135790', '135790']);
    await save(mounted);
    expect(section(mounted.wrapper, 'pin').find('.change').exists()).toBe(true);
    expect(mounted.overlay.toast).toMatchObject({ message: 'PIN alterado.', actions: [] });
    expect(await mounted.api.session.unlock('123456')).toEqual({ ok: false, reason: 'wrong-pin' });
    expect(await mounted.api.session.unlock('135790')).toEqual({ ok: true });
  });
});

describe('Dados section', () => {
  it('lists every connection as updated and when the data last came and was saved', async () => {
    const { wrapper } = await mountConfig();
    const rows = section(wrapper, 'dados').findAll('.connection').map((row) => [row.find('.bank').text(), plain(row.find('.status').text())]);
    expect(rows).toEqual([
      ['Itaú', 'Atualizado'],
      ['Mercado Pago', 'Atualizado'],
      ['C6', 'Atualizado'],
      ['Nubank', 'Atualizado'],
    ]);
    expect(section(wrapper, 'dados').findAll('.when span').map((node) => node.text())).toEqual([
      'Última sincronização: hoje às 02:00',
      'Último backup: hoje às 02:15',
    ]);
  });

  it('shows an expired consent in amber, and a date instead of "hoje" for an older sync', async () => {
    const state = prototypeState();
    state.settings.connections[0]!.consentExpired = true;
    state.settings.lastBackup = { day: '2026-11-26', time: '02:15' };
    const { wrapper } = await mountConfig('/config', state);
    const first = section(wrapper, 'dados').find('.connection');
    expect(plain(first.find('.status').text())).toBe('Consentimento expirado');
    expect(first.find('.status').classes()).toContain('expired');
    expect(section(wrapper, 'dados').findAll('.status.expired')).toHaveLength(1);
    expect(section(wrapper, 'dados').findAll('.when span')[1]!.text()).toBe('Último backup: em 26/11 às 02:15');
  });
});
