import { flushPromises } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import type { FakeState } from '@/api/fake/model';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { useShellStore } from '@/stores/shell';
import { mountAt, prototypeOverview } from '@/test/mount';
import { stateWithoutJustification, stateWithoutSuggestion } from '@/test/pendingStates';
import TransactionList from './TransactionList.vue';

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

async function mountWith(state: FakeState) {
  const mounted = await mountAt(TransactionList, '/', {}, { state });
  useShellStore().overview = prototypeOverview();
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

type Mounted = Awaited<ReturnType<typeof mountWith>>;

// The wide row and the phone's card both draw the transaction; both are in the page.
const drawingsOf = (wrapper: Mounted['wrapper'], name: string) =>
  wrapper.findAll('.row, .card-row').filter((drawing) => drawing.text().includes(name));
const marksIn = (drawing: ReturnType<typeof drawingsOf>[number]) => drawing.findAll('.mark').map((mark) => mark.text());

describe('a pending review the app has nothing to suggest for, in the list', () => {
  it('shows "Pendente" in the pill and the reason as its mark, with no percentage', async () => {
    const { wrapper } = await mountWith(stateWithoutSuggestion());
    const drawings = drawingsOf(wrapper, 'Bar do Zé');
    expect(drawings).toHaveLength(2);
    for (const drawing of drawings) {
      expect(drawing.find('.pill .label').text()).toBe('Pendente');
      expect(marksIn(drawing)).toEqual(['Estabelecimento novo']);
      expect(drawing.text()).not.toContain('%');
    }
  });

  it('prints neither "null" nor "undefined" anywhere in the list', async () => {
    const { wrapper } = await mountWith(stateWithoutSuggestion());
    expect(wrapper.text()).not.toMatch(/null|undefined/);
  });

  it('still opens the menu of lines from the pill, with no line marked as the current one', async () => {
    const { wrapper, overlay } = await mountWith(stateWithoutSuggestion());
    await drawingsOf(wrapper, 'Bar do Zé')[0]!.find('.pill-button').trigger('click');
    expect(overlay.menu!.items.length).toBeGreaterThan(0);
    expect(overlay.menu!.items.some((item) => item.current)).toBe(false);
  });
});

describe('a pending review with a suggestion and no justification, in the list', () => {
  it('reads as it does with one: the suggestion as a question and its confidence', async () => {
    const { wrapper } = await mountWith(stateWithoutJustification());
    for (const drawing of drawingsOf(wrapper, 'Bar do Zé')) {
      expect(drawing.find('.pill .label').text()).toBe('Restaurantes e bares?');
      expect(marksIn(drawing)).toEqual(['IA 58%']);
    }
    expect(wrapper.text()).not.toMatch(/null|undefined/);
  });
});
