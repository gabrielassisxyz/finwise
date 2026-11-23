import type { DOMWrapper, VueWrapper } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, type Component } from 'vue';
import { createFakeApi } from '@/api/fake';
import type { FakeState } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import TransactionPanel from '@/components/transaction-panel/TransactionPanel.vue';
import TransactionList from '@/components/transactions/TransactionList.vue';
import BudgetScreen from '@/screens/budget/BudgetScreen.vue';
import ConfigScreen from '@/screens/config/ConfigScreen.vue';
import ClosingTab from '@/screens/revisao/ClosingTab.vue';
import { useOverlayStore } from '@/stores/overlay';
import { usePanelStore } from '@/stores/panel';
import { failingOne, mountWithFailures, type ApiMethodName, type FailureMount } from '@/test/readOnly';
import type { ApiErrorKind, RefusalReason } from './contract/errors';
import { LEDGER_UNAVAILABLE_MESSAGE, RETRY_LABEL, SAVE_FAILED_MESSAGE, SAVE_UNCONFIRMED_MESSAGE } from './failures';
import type { FinwiseApi } from './types';

// One refusal per reason, each through a write that can really be answered with it. Whatever the write, the person
// reads the reason's sentence, has nothing to retry, and the screen fetches again once to show what is stored.

const MONTH = '2026-11';
const Blank = { template: '<div />' };
const plain = (text: string) => text.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim();

beforeEach(() => {
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});

function failing(method: ApiMethodName, kind: ApiErrorKind, reason?: RefusalReason, state: FakeState = prototypeState()): FinwiseApi {
  return failingOne(createFakeApi({ startLocked: false, state }), method, kind, reason);
}

interface Mark {
  revision: number;
  toasts: number;
}

const mark = (mounted: FailureMount): Mark => ({ revision: mounted.revisionNow(), toasts: mounted.toasts.length });

/** What the write left: the toasts shown since the mark, the actions of the one showing, the bumps, what escaped. */
function since(mounted: FailureMount, at: Mark) {
  return {
    toasts: mounted.toasts.slice(at.toasts),
    actions: useOverlayStore(mounted.pinia).toast?.actions.map((action) => action.label),
    bumps: mounted.revisionNow() - at.revision,
    unclaimed: mounted.unclaimed,
  };
}

const shownOnce = (message: string) => ({ toasts: [message], actions: [], bumps: 1, unclaimed: [] });

async function click(mounted: FailureMount, target: DOMWrapper<Element>): Promise<void> {
  await target.trigger('click');
  await mounted.settle();
}

async function pickFromMenu(mounted: FailureMount, label: string): Promise<void> {
  useOverlayStore(mounted.pinia).menu!.items.find((item) => item.label === label)!.onPick!();
  await mounted.settle();
}

async function confirm(mounted: FailureMount): Promise<void> {
  void useOverlayStore(mounted.pinia).confirmation!.onConfirm();
  await mounted.settle();
}

const buttonLabelled = (mounted: FailureMount, label: string) =>
  mounted.wrapper.findAll('button').find((candidate) => plain(candidate.text()) === label);

// --- the budget's save ---

async function mountBudget(api: FinwiseApi): Promise<FailureMount> {
  return mountWithFailures(BudgetScreen, [{ path: '/budget', component: BudgetScreen as Component }], '/budget', { api });
}

async function saveTheBudget(mounted: FailureMount): Promise<Mark> {
  const at = mark(mounted);
  await click(mounted, buttonLabelled(mounted, 'Salvar')!);
  await confirm(mounted);
  return at;
}

// --- the closing ---

function nothingPending(): FakeState {
  const state = prototypeState();
  for (const transaction of state.transactions.filter((candidate) => candidate.pending)) {
    Object.assign(transaction, { line: transaction.pending?.suggestion ?? null, by: 'Você', pending: null });
  }
  return state;
}

const stepOf = (wrapper: VueWrapper, title: string) => wrapper.findAll('.step').find((step) => step.find('.title').text() === title)!;

async function closeTheMonth(reason: RefusalReason): Promise<void> {
  const routes = [
    { path: '/revisao', component: Blank },
    { path: '/revisao/fechamento', component: Blank },
    { path: '/contas/cartao', component: Blank },
  ];
  const api = failing('review.closeMonth', 'refused', reason, nothingPending());
  const mounted = await mountWithFailures(ClosingTab, routes, '/revisao/fechamento', { api });
  await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.head'));
  await click(mounted, stepOf(mounted.wrapper, 'Fechar o mês').find('.close'));
  const at = mark(mounted);

  await confirm(mounted);

  expect(since(mounted, at)).toEqual(shownOnce(reasonSentence[reason]));
  expect(stepOf(mounted.wrapper, 'Fechar o mês').find('.body').exists()).toBe(true);
  expect(stepOf(mounted.wrapper, 'Fechar o mês').find('.close').exists()).toBe(true);
  expect((await api.shell.overview()).closedMonths.map((closed) => closed.month)).toEqual(['2026-10']);
}

// --- the list and the panel ---

const ListAndPanel = defineComponent({ components: { TransactionList, TransactionPanel }, template: '<TransactionList /><TransactionPanel />' });

async function mountList(api: FinwiseApi): Promise<FailureMount> {
  return mountWithFailures(ListAndPanel, [{ path: '/', component: Blank }], '/', { api, mounting: { attachTo: document.body } });
}

const listRow = (mounted: FailureMount, name: string) => mounted.wrapper.findAll('.row').find((row) => row.find('.name').text() === name)!;

async function changeLineInList(mounted: FailureMount, name: string, line: string): Promise<void> {
  await click(mounted, listRow(mounted, name).find('.pill-button'));
  await pickFromMenu(mounted, line);
}

async function openPanel(mounted: FailureMount, id: string): Promise<void> {
  usePanelStore(mounted.pinia).open(id);
  await mounted.settle();
}

async function commit(mounted: FailureMount, field: DOMWrapper<Element>, value: string): Promise<void> {
  (field.element as HTMLInputElement).value = value;
  await field.trigger('change');
  await mounted.settle();
}

const reasonSentence: Record<RefusalReason, string> = {
  month_closed: 'Esse mês já está fechado.',
  future_month: 'Esse mês ainda não chegou.',
  pending_reviews: 'Resolva as pendências do mês antes de fechar.',
  stale_change: 'Isso já foi alterado depois e não pode mais ser desfeito.',
  not_found: 'Esse item não existe mais.',
  unknown_line: 'Essa linha não existe no budget desse mês.',
  invalid_budget: 'O budget tem linha sem nome ou repetida.',
  unstable_pot: 'O banco não identifica esse cofrinho de forma estável.',
  split_with_reimbursement: 'Uma compra dividida não pode ter reembolso ligado.',
  proposal_stale: 'Isso mudou depois da proposta. Nada foi alterado.',
};

describe('a write refused for a reason the person can act on', () => {
  it('month_closed: saving the budget of a month closed meanwhile shows the month closed', async () => {
    const state = prototypeState();
    const mounted = await mountBudget(failing('budget.save', 'refused', 'month_closed', state));
    await click(mounted, buttonLabelled(mounted, 'Editar budget')!);
    state.shell.closedMonths.push({ month: MONTH, closedOn: '2026-11-30' });

    const at = await saveTheBudget(mounted);

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.month_closed));
    expect(plain(mounted.wrapper.find('.notice').text())).toBe('Budget de novembro de 2026 · fechado');
    expect(buttonLabelled(mounted, 'Salvar')).toBeUndefined();
  });

  it('invalid_budget: saving a budget the server finds invalid keeps the edit, and the stored budget is unchanged', async () => {
    const api = failing('budget.save', 'refused', 'invalid_budget');
    const stored = await api.budget.document(MONTH);
    const mounted = await mountBudget(api);
    await click(mounted, buttonLabelled(mounted, 'Editar budget')!);

    const at = await saveTheBudget(mounted);

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.invalid_budget));
    expect(buttonLabelled(mounted, 'Salvar')).toBeDefined();
    expect(await api.budget.document(MONTH)).toEqual(stored);
  });

  it('future_month: closing a month that has not come yet leaves it open', async () => {
    await closeTheMonth('future_month');
  });

  it('pending_reviews: closing a month the server still finds pending leaves it open', async () => {
    await closeTheMonth('pending_reviews');
  });

  it('stale_change: undoing a change of line that was changed again keeps the line the stand-in holds', async () => {
    const mounted = await mountList(failing('changes.undo', 'refused', 'stale_change'));
    await changeLineInList(mounted, 'Posto Shell', 'Lazer');
    expect(listRow(mounted, 'Posto Shell').find('.pill .label').text()).toBe('Lazer');
    const undo = useOverlayStore(mounted.pinia).toast!.actions.find((action) => action.label === 'Desfazer')!;
    const at = mark(mounted);

    undo.run();
    await mounted.settle();

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.stale_change));
    expect(listRow(mounted, 'Posto Shell').find('.pill .label').text()).toBe('Lazer');
  });

  it('unknown_line: changing to a line the month\'s budget lacks leaves the line as it was', async () => {
    const mounted = await mountList(failing('transactions.setLine', 'refused', 'unknown_line'));
    const before = listRow(mounted, 'Posto Shell').find('.pill .label').text();
    const at = mark(mounted);

    await changeLineInList(mounted, 'Posto Shell', 'Lazer');

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.unknown_line));
    expect(listRow(mounted, 'Posto Shell').find('.pill .label').text()).toBe(before);
  });

  it('not_found: a note on a transaction the ledger no longer lists shows the stored note again', async () => {
    const mounted = await mountList(failing('transactions.setNote', 'refused', 'not_found'));
    await openPanel(mounted, 't2');
    const at = mark(mounted);

    await commit(mounted, mounted.wrapper.find('.panel .note'), 'abastecer na sexta');

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.not_found));
    expect((mounted.wrapper.find('.panel .note').element as HTMLTextAreaElement).value).toBe('');
  });

  it('unstable_pot: a bucket for a pot the bank does not identify stably keeps the bucket it had', async () => {
    const api = failing('settings.setPotBucket', 'refused', 'unstable_pot');
    const mounted = await mountWithFailures(ConfigScreen, [{ path: '/config/:section?', component: ConfigScreen }], '/config', { api });
    const lastRow = () => mounted.wrapper.find('[data-section="cofrinhos"]').findAll('.row').at(-1)!;
    const bucketBefore = lastRow().find('.bucket').text();
    await click(mounted, lastRow().find('.bucket'));
    const at = mark(mounted);

    await pickFromMenu(mounted, 'Investido');

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.unstable_pot));
    expect(lastRow().find('.bucket').text()).toBe(bucketBefore);
  });

  it('split_with_reimbursement: splitting a purchase with a linked reimbursement leaves it whole', async () => {
    const mounted = await mountList(failing('transactions.setSplit', 'refused', 'split_with_reimbursement'));
    await openPanel(mounted, 't2');
    const at = mark(mounted);

    await click(mounted, mounted.wrapper.find('.panel .toggle'));

    expect(since(mounted, at)).toEqual(shownOnce(reasonSentence.split_with_reimbursement));
    expect(mounted.wrapper.find('.panel .part').exists()).toBe(false);
    expect(mounted.wrapper.find('.panel .toggle').text()).toBe('Dividir em linhas');
  });
});

describe('a write the server did not carry out, or whose answer never arrived', () => {
  it('a change of line that never got an answer says it could not confirm, and fetches again at once', async () => {
    const mounted = await mountList(failing('transactions.setLine', 'network'));
    const at = mark(mounted);

    await changeLineInList(mounted, 'Posto Shell', 'Lazer');

    expect(since(mounted, at)).toEqual(shownOnce(SAVE_UNCONFIRMED_MESSAGE));
  });

  it('a note answered internal gives the stored note back once the refetch has landed', async () => {
    const state = prototypeState();
    state.transactions.find((transaction) => transaction.id === 't2')!.note = 'abastecer na sexta';
    const mounted = await mountList(failing('transactions.setNote', 'internal', undefined, state));
    await openPanel(mounted, 't2');
    const note = () => mounted.wrapper.find('.panel .note').element as HTMLTextAreaElement;
    expect(note().value).toBe('abastecer na sexta');
    const at = mark(mounted);

    await commit(mounted, mounted.wrapper.find('.panel .note'), 'lavar o carro');

    expect(since(mounted, at)).toEqual(shownOnce(SAVE_FAILED_MESSAGE));
    expect(note().value).toBe('abastecer na sexta');
  });

  it('a write answered ledger_unavailable offers "Tentar de novo", which reads again and never sends the write again', async () => {
    const api = failing('transactions.setLine', 'ledger_unavailable');
    const setLine = vi.spyOn(api.transactions, 'setLine');
    const list = vi.spyOn(api.transactions, 'list');
    const mounted = await mountList(api);
    const at = mark(mounted);

    await changeLineInList(mounted, 'Posto Shell', 'Lazer');
    expect(since(mounted, at)).toEqual({ toasts: [LEDGER_UNAVAILABLE_MESSAGE], actions: [RETRY_LABEL], bumps: 0, unclaimed: [] });
    const readsBefore = list.mock.calls.length;

    useOverlayStore(mounted.pinia).toast!.actions[0]!.run();
    await mounted.settle();

    expect(setLine).toHaveBeenCalledTimes(1);
    expect(list.mock.calls.length).toBeGreaterThan(readsBefore);
  });
});
