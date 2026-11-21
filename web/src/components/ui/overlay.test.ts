import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useOverlayStore } from '@/stores/overlay';
import { mountAt } from '@/test/mount';
import ConfirmDialog from './ConfirmDialog.vue';
import PopoverMenu from './PopoverMenu.vue';
import ToastHost from './ToastHost.vue';

function anchorAt(left: number, bottom: number): Element {
  const anchor = document.createElement('button');
  anchor.getBoundingClientRect = () => ({ left, bottom }) as DOMRect;
  return anchor;
}

describe('PopoverMenu', () => {
  const items = () => [
    { label: 'Mercado', hint: 'Variáveis', current: true, onPick: vi.fn() },
    { label: 'Saúde', hint: 'Fixos e essenciais', onPick: vi.fn() },
    { label: 'Educação', hint: 'Variáveis', onPick: vi.fn() },
  ];

  it('opens under its anchor and stays inside the window', async () => {
    const { wrapper } = await mountAt(PopoverMenu, '/');
    const overlay = useOverlayStore();

    overlay.openMenu(anchorAt(100, 200), { items: items() });
    expect(overlay.menu).toMatchObject({ x: 100, y: 204 });

    overlay.openMenu(anchorAt(window.innerWidth - 10, window.innerHeight - 10), { items: items() });
    expect(overlay.menu).toMatchObject({ x: window.innerWidth - 272, y: window.innerHeight - 340 });

    await wrapper.vm.$nextTick();
    expect(wrapper.find('.menu').exists()).toBe(true);
  });

  it('runs the picked item and closes', async () => {
    const { wrapper } = await mountAt(PopoverMenu, '/');
    const overlay = useOverlayStore();
    const offered = items();
    overlay.openMenu(anchorAt(0, 0), { title: 'Linha', items: offered });
    await wrapper.vm.$nextTick();

    expect(wrapper.find('.title').text()).toBe('Linha');
    expect(wrapper.find('.item.current').text()).toContain('Mercado');
    await wrapper.findAll('.item')[1]!.trigger('click');
    expect(offered[1]!.onPick).toHaveBeenCalledOnce();
    expect(overlay.menu).toBeNull();
  });

  it('narrows by a search that ignores accents and says when nothing is left', async () => {
    const { wrapper } = await mountAt(PopoverMenu, '/');
    const overlay = useOverlayStore();
    overlay.openMenu(anchorAt(0, 0), { search: true, placeholder: 'Buscar linha', items: items() });
    await wrapper.vm.$nextTick();

    const search = wrapper.find('.search');
    expect(search.attributes('placeholder')).toBe('Buscar linha');
    await search.setValue('saude');
    expect(wrapper.findAll('.item').map((item) => item.text())).toEqual(['SaúdeFixos e essenciais']);

    await search.setValue('xyz');
    expect(wrapper.find('.empty').text()).toBe('Nada encontrado');
  });

  it('closes on Escape and on a click outside', async () => {
    const { wrapper } = await mountAt(PopoverMenu, '/');
    const overlay = useOverlayStore();

    overlay.openMenu(anchorAt(0, 0), { items: items() });
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(overlay.menu).toBeNull();

    overlay.openMenu(anchorAt(0, 0), { items: items() });
    await wrapper.vm.$nextTick();
    await wrapper.find('.backdrop').trigger('click');
    expect(overlay.menu).toBeNull();
  });
});

describe('ToastHost', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('shows the message with its actions and leaves after eight seconds', async () => {
    const { wrapper } = await mountAt(ToastHost, '/');
    const overlay = useOverlayStore();
    const undo = vi.fn();

    overlay.showToast('Linha alterada para Mercado.', [{ label: 'Desfazer', run: undo }]);
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.message').text()).toBe('Linha alterada para Mercado.');
    await wrapper.find('.action').trigger('click');
    expect(undo).toHaveBeenCalledOnce();

    vi.advanceTimersByTime(7999);
    expect(overlay.toast).not.toBeNull();
    vi.advanceTimersByTime(1);
    expect(overlay.toast).toBeNull();
  });

  it('restarts the clock when a new message replaces the old one', async () => {
    await mountAt(ToastHost, '/');
    const overlay = useOverlayStore();
    overlay.showToast('primeira');
    vi.advanceTimersByTime(6000);
    overlay.showToast('segunda');
    vi.advanceTimersByTime(6000);
    expect(overlay.toast?.message).toBe('segunda');
  });
});

describe('ConfirmDialog', () => {
  it('runs the action only on confirmation', async () => {
    const { wrapper } = await mountAt(ConfirmDialog, '/');
    const overlay = useOverlayStore();
    const onConfirm = vi.fn();

    overlay.confirm({ message: 'Fechar novembro de 2026?', confirmLabel: 'Fechar o mês', onConfirm });
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.confirm').text()).toBe('Fechar o mês');
    await wrapper.find('.cancel').trigger('click');
    expect(onConfirm).not.toHaveBeenCalled();
    expect(overlay.confirmation).toBeNull();

    overlay.confirm({ message: 'Fechar novembro de 2026?', confirmLabel: 'Fechar o mês', onConfirm });
    await wrapper.vm.$nextTick();
    await wrapper.find('.confirm').trigger('click');
    expect(onConfirm).toHaveBeenCalledOnce();
    expect(overlay.confirmation).toBeNull();
  });
});
