import type { DOMWrapper } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Component } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';
import App from '@/App.vue';
import { createFakeApi } from '@/api/fake';
import type { FakeState } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import { SAVE_FAILED_MESSAGE } from '@/api/failures';
import type { ApiErrorKind, RefusalReason } from '@/api/contract/errors';
import AppShell from '@/components/shell/AppShell.vue';
import { createAppRouter } from '@/router';
import { useOverlayStore } from '@/stores/overlay';
import { failingOne, mountWithFailures, type FailureMount } from '@/test/readOnly';
import { prototypeOverview } from '@/test/mount';
import BudgetScreen from './BudgetScreen.vue';
import ScenariosScreen from './ScenariosScreen.vue';

// A save that fails keeps the draft, so what the toast names can be fixed; a month closed meanwhile ends the edit,
// and a lost session goes to the PIN screen as from anywhere.

const MONTH = '2026-11';
const TYPED_NAME = 'Mercado do bairro';
const Blank = { template: '<div />' };
const plain = (text: string) => text.replace(/\s+/g, ' ').trim();

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined);
  document.body.innerHTML = '<div id="float-above-toast"></div>';
});
afterEach(() => vi.restoreAllMocks());

function closeInStandIn(state: FakeState): void {
  state.shell.closedMonths.push({ month: MONTH, closedOn: '2026-11-30' });
}

const buttonLabelled = (mounted: FailureMount, label: string) =>
  mounted.wrapper.findAll('button').find((candidate) => plain(candidate.text()) === label);

async function click(mounted: FailureMount, target: DOMWrapper<Element>): Promise<void> {
  await target.trigger('click');
  await mounted.settle();
}

const firstNameInput = (mounted: FailureMount) => mounted.wrapper.find<HTMLInputElement>('.name-input');

/** Opens the edit, types a name into the first line, and saves; `before` is the revision just before the save. */
async function editAndSave(mounted: FailureMount): Promise<{ before: number; toastsBefore: number }> {
  await click(mounted, buttonLabelled(mounted, 'Editar budget')!);
  firstNameInput(mounted).element.value = TYPED_NAME;
  await firstNameInput(mounted).trigger('change');
  await mounted.settle();
  const before = mounted.revisionNow();
  const toastsBefore = mounted.toasts.length;
  await click(mounted, buttonLabelled(mounted, 'Salvar')!);
  useOverlayStore(mounted.pinia).confirmation!.onConfirm();
  await mounted.settle();
  return { before, toastsBefore };
}

async function budgetFailingSave(kind: ApiErrorKind, reason?: RefusalReason, state = prototypeState()): Promise<FailureMount> {
  const api = failingOne(createFakeApi({ startLocked: false, state }), 'budget.save', kind, reason);
  const routes = [{ path: '/budget', component: BudgetScreen as Component }];
  return mountWithFailures(BudgetScreen, routes, '/budget', { api });
}

function shownAfterSave(mounted: FailureMount, { before, toastsBefore }: { before: number; toastsBefore: number }) {
  return {
    toasts: mounted.toasts.slice(toastsBefore),
    actions: useOverlayStore(mounted.pinia).toast?.actions.map((action) => action.label),
    bumps: mounted.revisionNow() - before,
    unclaimed: mounted.unclaimed,
  };
}

describe('saving the budget', () => {
  it('refused with invalid_budget says why once and keeps the edit with what was typed', async () => {
    const mounted = await budgetFailingSave('refused', 'invalid_budget');

    const saved = await editAndSave(mounted);

    expect(shownAfterSave(mounted, saved)).toEqual({
      toasts: ['O budget tem linha sem nome ou repetida.'],
      actions: [],
      bumps: 1,
      unclaimed: [],
    });
    expect(buttonLabelled(mounted, 'Salvar')).toBeDefined();
    expect(firstNameInput(mounted).element.value).toBe(TYPED_NAME);
  });

  it('answered internal says it was not saved once, with no action, and keeps the edit with what was typed', async () => {
    const mounted = await budgetFailingSave('internal');

    const saved = await editAndSave(mounted);

    expect(shownAfterSave(mounted, saved)).toEqual({ toasts: [SAVE_FAILED_MESSAGE], actions: [], bumps: 1, unclaimed: [] });
    expect(buttonLabelled(mounted, 'Salvar')).toBeDefined();
    expect(firstNameInput(mounted).element.value).toBe(TYPED_NAME);
  });

  it('refused with month_closed ends the edit and draws the month as closed', async () => {
    const state = prototypeState();
    const mounted = await budgetFailingSave('refused', 'month_closed', state);
    await click(mounted, buttonLabelled(mounted, 'Editar budget')!);
    closeInStandIn(state);
    const before = mounted.revisionNow();
    const toastsBefore = mounted.toasts.length;

    await click(mounted, buttonLabelled(mounted, 'Salvar')!);
    useOverlayStore(mounted.pinia).confirmation!.onConfirm();
    await mounted.settle();

    expect(shownAfterSave(mounted, { before, toastsBefore })).toEqual({
      toasts: ['Esse mês já está fechado.'],
      actions: [],
      bumps: 1,
      unclaimed: [],
    });
    expect(plain(mounted.wrapper.find('.notice').text())).toBe('Budget de novembro de 2026 · fechado');
    expect(buttonLabelled(mounted, 'Salvar')).toBeUndefined();
    expect(buttonLabelled(mounted, 'Editar budget')).toBeUndefined();
    expect(mounted.wrapper.find('.name-input').exists()).toBe(false);
  });

  it('answered locked brings the PIN screen, with no toast and nothing unclaimed', async () => {
    const api = failingOne(createFakeApi({ startLocked: false }), 'budget.save', 'locked');
    const mounted = await mountWithFailures(App, [], '/budget', { api, router: createAppRouter(), frame: false });
    expect(mounted.wrapper.find('[data-screen-label="Budget"]').exists()).toBe(true);

    await editAndSave(mounted);

    expect(mounted.wrapper.find('[data-screen-label="PIN"]').exists()).toBe(true);
    expect(mounted.wrapper.find('[data-screen-label="Budget"]').exists()).toBe(false);
    expect(mounted.toasts).toEqual([]);
    expect(mounted.unclaimed).toEqual([]);
  });
});

describe('applying a scenario to a month found closed', () => {
  // The frame clears the toast when the scenarios are left. The return is made to take a timer, as a navigation that
  // waits on anything does, so the toast survives only if it is shown after the return has completed.
  const SLOW_RETURN_MS = 5;

  it('returns to the budget with the sentence still showing, with the frame around the scenarios', async () => {
    const state = prototypeState();
    const api = failingOne(createFakeApi({ startLocked: false, state }), 'budget.applyScenario', 'refused', 'month_closed');
    const slowReturn = () => new Promise<void>((done) => setTimeout(done, SLOW_RETURN_MS));
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/budget', component: Blank, meta: { nav: 'budget' }, beforeEnter: slowReturn },
        { path: '/budget/cenarios', component: ScenariosScreen as Component, meta: { nav: 'budget' } },
      ],
    });
    const mounted = await mountWithFailures(AppShell, [], '/budget', {
      api,
      router,
      mounting: { props: { overview: prototypeOverview() } },
    });
    await router.push('/budget/cenarios');
    await mounted.settle();
    expect(router.currentRoute.value.fullPath).toBe('/budget/cenarios');
    closeInStandIn(state);

    await click(mounted, buttonLabelled(mounted, 'Aplicar ao budget')!);
    useOverlayStore(mounted.pinia).confirmation!.onConfirm();
    await new Promise((done) => setTimeout(done, SLOW_RETURN_MS * 4));
    await mounted.settle();

    expect(router.currentRoute.value.fullPath).toBe('/budget');
    expect(useOverlayStore(mounted.pinia).toast?.message).toBe('Esse mês já está fechado.');
    expect(mounted.toasts).toEqual(['Esse mês já está fechado.']);
    expect(mounted.unclaimed).toEqual([]);
  });
});
