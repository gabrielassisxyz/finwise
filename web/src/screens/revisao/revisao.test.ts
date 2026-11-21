import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { describe, expect, it, vi } from 'vitest';
import { apiKey } from '@/api/context';
import { createFakeApi } from '@/api/fake';
import App from '@/App.vue';
import { createAppRouter } from '@/router';
import { useOverlayStore } from '@/stores/overlay';
import { useRevisionStore } from '@/stores/revision';

vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);

async function openApp(path: string) {
  const api = createFakeApi({ startLocked: false });
  const router = createAppRouter();
  const wrapper = mount(App, { global: { plugins: [createPinia(), router], provide: { [apiKey as symbol]: api } } });
  await router.push(path);
  await flushPromises();
  return { api, router, wrapper, overlay: useOverlayStore(), revision: useRevisionStore() };
}

type App = Awaited<ReturnType<typeof openApp>>;

const tab = (wrapper: VueWrapper, label: string) => wrapper.findAll('.tab').find((candidate) => candidate.text() === label)!;
const click = async (target: { trigger: (event: string) => Promise<void> }) => {
  await target.trigger('click');
  await flushPromises();
};
const stepHead = (wrapper: VueWrapper, title: string) =>
  wrapper.findAll('.step').find((step) => step.find('.title').text() === title)!.find('.head');

async function answerEverything({ api, revision }: App) {
  for (const item of (await api.review.weekly('2026-11')).queue.items) {
    await api.review.acceptPending(item.transactionId, { line: item.pending.suggestion, always: false, nickname: '', reason: '' });
  }
  revision.bump();
  await flushPromises();
}

describe('Revisão, the tabs', () => {
  it('shows one route per tab, and the tab bar moves between them', async () => {
    const { wrapper, router } = await openApp('/revisao');
    expect(tab(wrapper, 'Semanal').classes()).toContain('active');
    expect(wrapper.text()).toContain('Aguardando seu ok');

    await click(tab(wrapper, 'Fechamento do mês'));
    expect(router.currentRoute.value.path).toBe('/revisao/fechamento');
    expect(tab(wrapper, 'Fechamento do mês').classes()).toContain('active');
    expect(wrapper.text()).toContain('Meses fechados');

    await click(tab(wrapper, 'Dia do salário'));
    expect(router.currentRoute.value.path).toBe('/revisao/salario');
    expect(wrapper.text()).toContain('Transferências do dia do salário');

    await click(tab(wrapper, 'Semanal'));
    expect(router.currentRoute.value.path).toBe('/revisao');
  });

  it('opens each tab directly from its address, with Revisão marked in the sidebar', async () => {
    for (const [path, active] of [['/revisao/fechamento', 'Fechamento do mês'], ['/revisao/salario', 'Dia do salário']] as const) {
      const { wrapper } = await openApp(path);
      expect(tab(wrapper, active).classes()).toContain('active');
      expect(wrapper.find('.item.active').text()).toContain('Revisão');
    }
  });

  it('keeps a toast when the tab changes, and drops it when the screen does', async () => {
    const { wrapper, overlay, router } = await openApp('/revisao');
    await click(wrapper.findAll('.dismiss')[0]!);
    expect(overlay.toast?.message).toBe('Aviso dispensado.');
    await click(tab(wrapper, 'Dia do salário'));
    expect(overlay.toast?.message).toBe('Aviso dispensado.');
    await router.push('/budget');
    await flushPromises();
    expect(overlay.toast).toBeNull();
  });
});

describe('Revisão, a write reaches the rest of the app', () => {
  it('takes the pending counter down in the sidebar as reviews are answered', async () => {
    const { wrapper } = await openApp('/revisao');
    expect(wrapper.find('.item .count').text()).toBe('7');
    await click(wrapper.findAll('button').find((candidate) => candidate.text() === 'Aceitar')!);
    expect(wrapper.find('.item .count').text()).toBe('6');
  });

  it('shows the seal on the header and the closed list as soon as the month closes, with no reload', async () => {
    const app = await openApp('/revisao/fechamento');
    const { wrapper, overlay } = app;
    expect(wrapper.find('.closed-seal').exists()).toBe(false);
    await answerEverything(app);
    expect(wrapper.find('.item .count').exists()).toBe(false);

    await click(stepHead(wrapper, 'Fechar o mês'));
    await click(wrapper.find('.close'));
    overlay.confirmation!.onConfirm();
    await flushPromises();

    expect(wrapper.find('.closed-seal').text()).toBe('Fechado');
    expect(wrapper.findAll('.entry .label').map((label) => label.text())).toEqual(['Novembro de 2026', 'Outubro de 2026']);
  });

  it('shows the closed month read-only on the budget screen', async () => {
    const app = await openApp('/budget');
    expect(app.wrapper.text()).toContain('Editar budget');
    await app.router.push('/revisao/fechamento');
    await flushPromises();
    await answerEverything(app);
    await click(stepHead(app.wrapper, 'Fechar o mês'));
    await click(app.wrapper.find('.close'));
    app.overlay.confirmation!.onConfirm();
    await flushPromises();
    await app.router.push('/budget');
    await flushPromises();
    expect(app.wrapper.text()).not.toContain('Editar budget');
  });
});
