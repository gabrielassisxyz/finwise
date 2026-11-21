import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { useOverlayStore } from '@/stores/overlay';
import { mountAt } from '@/test/mount';
import ToastHost from './ToastHost.vue';

describe('ToastHost', () => {
  it('keeps the place for the selection bar above the toast', async () => {
    const { wrapper } = await mountAt(ToastHost, '/', { attachTo: document.body });
    useOverlayStore().showToast('Pronto.');
    await flushPromises();

    const stack = [...wrapper.find('.float').element.children];
    const slotIndex = stack.findIndex((child) => child.id === 'float-above-toast');
    const toastIndex = stack.findIndex((child) => child.classList.contains('toast'));
    expect(slotIndex).toBeGreaterThanOrEqual(0);
    expect(slotIndex).toBeLessThan(toastIndex);
    wrapper.unmount();
  });
});
