import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent } from 'vue';
import type { FakeState } from '@/api/fake/model';
import TransactionList from '@/components/transactions/TransactionList.vue';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { useShellStore } from '@/stores/shell';
import { mountAt, prototypeOverview } from '@/test/mount';
import { FIRST_PENDING_ID, stateWithoutJustification, stateWithoutSuggestion } from '@/test/pendingStates';
import TransactionPanel from './TransactionPanel.vue';

const Host = defineComponent({
  components: { TransactionList, TransactionPanel },
  template: '<TransactionList /><TransactionPanel />',
});

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

async function openWith(state: FakeState) {
  const mounted = await mountAt(Host, '/', { attachTo: document.body }, { state });
  useShellStore().overview = prototypeOverview();
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  usePanelStore().open(FIRST_PENDING_ID);
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

describe('the panel of a pending review the app has nothing to suggest for', () => {
  it('names no line, gives the reason as the origin and offers nothing to confirm', async () => {
    const { wrapper } = await openWith(stateWithoutSuggestion());
    expect(wrapper.find('.panel .line-button').text()).toBe('Sem linha· Estabelecimento novo▾');
    expect(wrapper.find('.panel .confirm').exists()).toBe(false);
    expect(wrapper.find('.panel .change').exists()).toBe(false);
    expect(wrapper.find('.panel .justification').exists()).toBe(false);
  });

  it('prints neither "null" nor "undefined"', async () => {
    const { wrapper } = await openWith(stateWithoutSuggestion());
    expect(wrapper.find('.panel').text()).not.toMatch(/null|undefined/);
  });

  it('still opens the menu of lines from the line button', async () => {
    const { wrapper, overlay } = await openWith(stateWithoutSuggestion());
    await wrapper.find('.panel .line-button').trigger('click');
    expect(overlay.menu!.items.length).toBeGreaterThan(0);
  });
});

describe('the panel of a pending review with a suggestion and no justification', () => {
  it('offers to confirm or change the suggestion, with no justification text', async () => {
    const { wrapper } = await openWith(stateWithoutJustification());
    expect(wrapper.find('.panel .line-button').text()).toBe('Restaurantes e bares· sugestão da IA, 58% de confiança▾');
    expect(wrapper.findAll('.panel .actions button').map((button) => button.text())).toEqual(['Confirmar', 'Alterar']);
    expect(wrapper.find('.panel .confirm').exists()).toBe(true);
    expect(wrapper.find('.panel .justification').exists()).toBe(false);
    expect(wrapper.find('.panel').text()).not.toMatch(/null|undefined/);
  });
});
