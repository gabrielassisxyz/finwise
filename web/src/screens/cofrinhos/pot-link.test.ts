import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { freshInstallState } from '@/api/fake/first-run-data';
import { mountRouted } from '@/test/mountRouted';
import CofrinhosScreen from './CofrinhosScreen.vue';

const Blank = { template: '<div />' };
const routes = [
  { path: '/cofrinhos', component: Blank },
  { path: '/cofrinhos/:id', component: Blank },
];

// The backend names a pot after its account and its place in it, and the name has a slash.
describe('opening a pot whose id holds a slash', () => {
  it('lands on that pot rather than on a route that does not exist', async () => {
    const state = freshInstallState();
    const id = 'reserve:acc-pots/pot-1';
    state.savingsPots[0]!.id = id;
    const { wrapper, router } = await mountRouted(CofrinhosScreen, routes, '/cofrinhos', { state });
    await flushPromises();
    const row = wrapper.findAll('.row').find((candidate) => candidate.text().includes(state.savingsPots[0]!.name))!;
    await row.trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.params.id).toBe(id);
  });
});
