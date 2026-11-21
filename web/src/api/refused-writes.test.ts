import type { DOMWrapper, VueWrapper } from '@vue/test-utils';
import { beforeEach, describe, expect, it } from 'vitest';
import { defineComponent, type Component } from 'vue';
import type { FakeState } from '@/api/fake/model';
import { createFakeApi } from '@/api/fake';
import { prototypeState } from '@/api/fake/prototype-data';
import TransactionPanel from '@/components/transaction-panel/TransactionPanel.vue';
import TransactionList from '@/components/transactions/TransactionList.vue';
import AssinaturasScreen from '@/screens/assinaturas/AssinaturasScreen.vue';
import BudgetScreen from '@/screens/budget/BudgetScreen.vue';
import ConfigScreen from '@/screens/config/ConfigScreen.vue';
import ClosingTab from '@/screens/revisao/ClosingTab.vue';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { mountWithFailures, type FailureMount } from '@/test/readOnly';
import { READ_ONLY_MESSAGE } from './failures';

// One refused write through each of the seven ways the interface makes one. The
// API refuses everything that writes, as the read-only backend does; what the
// person must see is the same sentence whichever way the write was made.

const Blank = { template: '<div />' };
const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

/** What the person is left with after the write was refused: the toast, how often the screens were told, what escaped. */
function afterRefusal(mounted: FailureMount, revisionBefore: number) {
  const overlay = useOverlayStore(mounted.pinia);
  return {
    toasts: mounted.toasts,
    toastActions: overlay.toast?.actions.map((action) => action.label),
    revisionBumps: mounted.revisionNow() - revisionBefore,
    unclaimed: mounted.unclaimed,
  };
}

const refusedOnce = { toasts: [READ_ONLY_MESSAGE], toastActions: [], revisionBumps: 1, unclaimed: [] };

async function pickFromMenu(mounted: FailureMount, label: string): Promise<void> {
  useOverlayStore(mounted.pinia).menu!.items.find((item) => item.label === label)!.onPick!();
  await mounted.settle();
}

async function click(mounted: FailureMount, target: DOMWrapper<Element>): Promise<void> {
  await target.trigger('click');
  await mounted.settle();
}

describe('the transaction panel\'s write', () => {
  const Host = defineComponent({ components: { TransactionList, TransactionPanel }, template: '<TransactionList /><TransactionPanel />' });

  it('shows the read-only message once and nothing else', async () => {
    const mounted = await mountWithFailures(Host, [{ path: '/', component: Blank }], '/', {
      mounting: { attachTo: document.body },
    });
    usePanelStore(mounted.pinia).open('t5');
    await mounted.settle();
    const before = mounted.revisionNow();

    await click(mounted, mounted.wrapper.find('.panel .confirm'));

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(mounted.wrapper.find('.panel .line-button').text()).not.toContain('definida por Você');
  });
});

describe('the list\'s change of line (useLineChange)', () => {
  it('shows the read-only message once, with no way back, and leaves the line as it was', async () => {
    const mounted = await mountWithFailures(TransactionList, [{ path: '/', component: Blank }], '/', );
    const row = () => mounted.wrapper.findAll('.row').find((candidate) => candidate.find('.name').text() === 'Posto Shell')!;
    const labelBefore = row().find('.pill .label').text();
    const before = mounted.revisionNow();

    await click(mounted, row().find('.pill-button'));
    await pickFromMenu(mounted, 'Lazer');

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(row().find('.pill .label').text()).toBe(labelBefore);
  });
});

describe('the selection\'s write (actOnSelection)', () => {
  it('shows the read-only message once, with no way back, and no "marked" confirmation', async () => {
    const mounted = await mountWithFailures(TransactionList, [{ path: '/', component: Blank }], '/', );
    for (const name of ['Aluguel', 'Posto Shell']) {
      const row = mounted.wrapper.findAll('.row').find((candidate) => candidate.find('.name').text() === name)!;
      await row.find('.check').setValue(true);
    }
    const before = mounted.revisionNow();
    const exclude = [...document.querySelectorAll<HTMLButtonElement>('#float-above-toast .bar button')].find(
      (button) => button.textContent === 'Marcar como não conta',
    )!;

    exclude.click();
    await mounted.settle();

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(mounted.wrapper.findAll('.row.faded')).toHaveLength(0);
  });
});

describe('the settings screen\'s writes (useSettingsWrites)', () => {
  it('shows the read-only message once and keeps the bucket the pot had', async () => {
    const mounted = await mountWithFailures(ConfigScreen, [{ path: '/config/:section?', component: ConfigScreen }], '/config');
    const lastRow = () => mounted.wrapper.find('[data-section="cofrinhos"]').findAll('.row').at(-1)!;
    const bucketBefore = lastRow().find('.bucket').text();
    const before = mounted.revisionNow();

    await click(mounted, lastRow().find('.bucket'));
    await pickFromMenu(mounted, 'Investido');

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(lastRow().find('.bucket').text()).toBe(bucketBefore);
  });
});

describe('the "announce" helpers of the subscriptions and of the review', () => {
  it('shows the read-only message once for a subscription\'s new status, with no way back', async () => {
    const routes = [{ path: '/assinaturas', component: Blank }];
    const mounted = await mountWithFailures(AssinaturasScreen, routes, '/assinaturas');
    const pill = () => mounted.wrapper.findAll('.row').find((row) => row.find('.name').text() === 'Spotify')!.find('.pill');
    const pillBefore = plain(pill().text());
    const before = mounted.revisionNow();

    await click(mounted, pill());
    await pickFromMenu(mounted, 'Analisar');

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(plain(pill().text())).toBe(pillBefore);
  });

  it('shows the read-only message once for keeping a subscription in the closing, with no way back', async () => {
    const mounted = await mountClosing();
    const step = () => mounted.wrapper.findAll('.step').find((candidate) => candidate.find('.title').text() === 'Assinaturas a revisar')!;
    await click(mounted, step().find('.head'));
    const before = mounted.revisionNow();

    await click(mounted, step().find('.keep'));

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(step().findAll('.keep').length).toBeGreaterThan(0);
  });
});

async function mountClosing(state?: FakeState): Promise<FailureMount> {
  const routes = [
    { path: '/revisao', component: Blank },
    { path: '/revisao/fechamento', component: Blank },
    { path: '/contas/cartao', component: Blank },
  ];
  return mountWithFailures(ClosingTab, routes, '/revisao/fechamento', { state });
}

describe('the closing\'s writes and the budget\'s save', () => {
  const stepOf = (wrapper: VueWrapper, title: string) => wrapper.findAll('.step').find((step) => step.find('.title').text() === title)!;

  it('shows the read-only message once for marking a step done, and the step stays open', async () => {
    const mounted = await mountClosing();
    await click(mounted, stepOf(mounted.wrapper, 'Orçado × realizado').find('.head'));
    const before = mounted.revisionNow();
    const mark = stepOf(mounted.wrapper, 'Orçado × realizado')
      .findAll('button')
      .find((button) => plain(button.text()) === 'Marcar como feito e seguir')!;

    await click(mounted, mark);

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(stepOf(mounted.wrapper, 'Orçado × realizado').find('.status').text()).not.toBe('Feito');
  });

  it('shows the read-only message once for closing the month', async () => {
    const state = prototypeState();
    for (const transaction of state.transactions.filter((candidate) => candidate.pending)) {
      Object.assign(transaction, { line: transaction.pending?.suggestion ?? null, by: 'Você', pending: null });
    }
    const mounted = await mountClosing(state);
    await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.head'));
    await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.close'));
    const before = mounted.revisionNow();

    useOverlayStore(mounted.pinia).confirmation!.onConfirm();
    await mounted.settle();

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect((await mounted.api.shell.overview()).closedMonths.map((closed) => closed.month)).toEqual(['2026-10']);
  });

  it('leaves the "Fechar o mês" step unfolded when the close is refused, and folds it when the close goes through', async () => {
    const withNothingPending = () => {
      const state = prototypeState();
      for (const transaction of state.transactions.filter((candidate) => candidate.pending)) {
        Object.assign(transaction, { line: transaction.pending?.suggestion ?? null, by: 'Você', pending: null });
      }
      return state;
    };
    const isOpen = (mounted: FailureMount) => stepOf(mounted.wrapper, 'Fechar o mês').find('.body').exists();
    const confirmClose = async (mounted: FailureMount) => {
      await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.head'));
      await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.close'));
      useOverlayStore(mounted.pinia).confirmation!.onConfirm();
      await mounted.settle();
    };

    const refused = await mountClosing(withNothingPending());
    await confirmClose(refused);
    expect(refused.toasts).toEqual([READ_ONLY_MESSAGE]);
    expect(isOpen(refused)).toBe(true);

    const state = withNothingPending();
    const accepted = await mountWithFailures(ClosingTab, [{ path: '/revisao/fechamento', component: Blank }], '/revisao/fechamento', {
      api: createFakeApi({ startLocked: false, state }),
    });
    await confirmClose(accepted);
    expect(accepted.toasts).toEqual([]);
    expect(isOpen(accepted)).toBe(false);
  });

  it('shows the read-only message once for saving the budget, and keeps the edit', async () => {
    const routes = [{ path: '/budget', component: BudgetScreen as Component }];
    const mounted = await mountWithFailures(BudgetScreen, routes, '/budget');
    const button = (label: string) => mounted.wrapper.findAll('button').find((candidate) => candidate.text() === label)!;
    await click(mounted, button('Editar budget'));
    const before = mounted.revisionNow();

    await click(mounted, button('Salvar'));
    useOverlayStore(mounted.pinia).confirmation!.onConfirm();
    await mounted.settle();

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(button('Salvar')).toBeDefined();
  });
});

describe('the PIN form', () => {
  it('shows the read-only message once, and not "PIN alterado."', async () => {
    const mounted = await mountWithFailures(ConfigScreen, [{ path: '/config/:section?', component: ConfigScreen }], '/config');
    const section = () => mounted.wrapper.find('[data-section="pin"]');
    await click(mounted, section().find('.change'));
    const fields = section().findAll('input');
    for (const [index, value] of ['123456', '135790', '135790'].entries()) {
      (fields[index]!.element as HTMLInputElement).value = value;
      await fields[index]!.trigger('input');
    }
    const before = mounted.revisionNow();

    await click(mounted, mounted.wrapper.findAll('button').find((candidate) => candidate.text() === 'Salvar PIN')!);

    expect(afterRefusal(mounted, before)).toEqual(refusedOnce);
    expect(mounted.toasts).not.toContain('PIN alterado.');
    expect(section().find('.save').exists()).toBe(true);
  });
});
