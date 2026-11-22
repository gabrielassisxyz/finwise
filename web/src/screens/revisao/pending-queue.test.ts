import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import type { FakeState } from '@/api/fake/model';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import { stateWithoutJustification, stateWithoutSuggestion, stateWithPending } from '@/test/pendingStates';
import WeeklyTab from './WeeklyTab.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/revisao', component: Blank },
  { path: '/assinaturas', component: Blank },
  { path: '/assinaturas/:id', component: Blank },
];

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountQueue(state: FakeState) {
  const mounted = await mountRouted(WeeklyTab, routes, '/revisao', { state });
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  const accepted = vi.spyOn(mounted.api.review, 'acceptPending');
  return { ...mounted, accepted, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountQueue>>;

const buttonNamed = (wrapper: Mounted['wrapper'], text: string) =>
  wrapper.findAll('.item button').find((candidate) => plain(candidate.text()) === text);
const press = async (button: ReturnType<typeof buttonNamed>) => {
  await button!.trigger('click');
  await flushPromises();
};
const pickLine = async ({ wrapper, overlay }: Mounted, line: string) => {
  await press(buttonNamed(wrapper, 'Escolher outra linha'));
  overlay.menu!.items.find((item) => item.label === line)!.onPick!();
  await flushPromises();
};

describe('the queue item of a review the app has nothing to suggest for', () => {
  it('shows its reason and no suggestion, no percentage and no earlier suggestion', async () => {
    const { wrapper } = await mountQueue(stateWithoutSuggestion());
    const item = wrapper.find('.item');
    expect(item.find('.reason').text()).toBe('Estabelecimento novo');
    expect(item.find('.name').text()).toBe('Bar do Zé');
    expect(item.text()).not.toContain('Sugestão da IA');
    expect(item.text()).not.toContain('%');
    expect(item.text()).not.toContain('A IA tinha sugerido');
    expect(item.text()).not.toMatch(/null|undefined/);
    expect(item.find('.suggestion').exists()).toBe(false);
  });

  it('keeps "Aceitar" disabled and sends nothing for a click on it', async () => {
    const { wrapper, accepted } = await mountQueue(stateWithoutSuggestion());
    const accept = buttonNamed(wrapper, 'Aceitar')!;
    expect(accept.attributes('disabled')).toBeDefined();
    await press(accept);
    expect(accepted).not.toHaveBeenCalled();
  });

  it('opens the menu of lines with none marked as the current one', async () => {
    const { wrapper, overlay } = await mountQueue(stateWithoutSuggestion());
    await press(buttonNamed(wrapper, 'Escolher outra linha'));
    expect(overlay.menu!.items.length).toBeGreaterThan(0);
    expect(overlay.menu!.items.some((item) => item.current)).toBe(false);
  });

  it('turns into "Confirmar" once a line is picked, and accepting sends that line', async () => {
    const mounted = await mountQueue(stateWithoutSuggestion());
    await pickLine(mounted, 'Mercado');
    const item = mounted.wrapper.find('.item');
    expect(plain(item.find('.line').text())).toBe('Linha escolhida: Mercado');
    expect(item.find('.why').exists()).toBe(false);
    expect(item.text()).not.toContain('A IA tinha sugerido');
    expect(item.text()).not.toMatch(/null|undefined/);
    const confirm = buttonNamed(mounted.wrapper, 'Confirmar')!;
    expect(confirm.attributes('disabled')).toBeUndefined();
    expect(buttonNamed(mounted.wrapper, 'Aceitar')).toBeUndefined();

    await press(confirm);
    expect(mounted.accepted).toHaveBeenCalledTimes(1);
    expect(mounted.accepted).toHaveBeenCalledWith('t5', { line: 'Mercado', always: false, nickname: '', reason: '' });
  });
});

describe('the queue item of a review with a suggestion and no justification', () => {
  it('shows the suggestion and its confidence and no explanation', async () => {
    const { wrapper } = await mountQueue(stateWithoutJustification());
    const item = wrapper.find('.item');
    expect(plain(item.find('.line').text())).toBe('Sugestão da IA: Restaurantes e bares · 58%');
    expect(item.find('.why').exists()).toBe(false);
    expect(item.text()).not.toMatch(/null|undefined/);
    expect(buttonNamed(wrapper, 'Aceitar')!.attributes('disabled')).toBeUndefined();
  });

  it('accepts the suggestion', async () => {
    const { wrapper, accepted } = await mountQueue(stateWithoutJustification());
    await press(buttonNamed(wrapper, 'Aceitar'));
    expect(accepted).toHaveBeenCalledWith('t5', expect.objectContaining({ line: 'Restaurantes e bares' }));
  });

  it('still says what the model had suggested once another line is picked', async () => {
    const mounted = await mountQueue(stateWithoutJustification());
    await pickLine(mounted, 'Delivery');
    expect(mounted.wrapper.find('.item .why').text()).toBe('A IA tinha sugerido Restaurantes e bares. Confirme para usar a sua escolha.');
  });
});

describe('the queue item of a review with a suggestion and no confidence', () => {
  it('shows the suggestion with no percentage', async () => {
    const { wrapper } = await mountQueue(stateWithPending({ reason: 'Baixa confiança', suggestion: 'Casa', confidence: null, justification: null }));
    const item = wrapper.find('.item');
    expect(plain(item.find('.line').text())).toBe('Sugestão da IA: Casa');
    expect(item.text()).not.toMatch(/%|null|undefined/);
  });
});
