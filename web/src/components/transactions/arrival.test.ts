import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { useMonthStore } from '@/stores/month';
import { prototypeOverview } from '@/test/mount';
import { mountRouted } from '@/test/mountRouted';
import TransactionList from './TransactionList.vue';

const Blank = { template: '<div />' };

async function mountAtPath(path: string) {
  const mounted = await mountRouted(TransactionList, [{ path: '/', component: Blank }], path);
  useMonthStore().configure(prototypeOverview());
  await flushPromises();
  return mounted;
}

describe('arriving at the list from another screen', () => {
  it('starts filtered by the establishment the route names', async () => {
    const { wrapper } = await mountAtPath('/?establishment=iFood');
    expect(wrapper.text()).toContain('Estabelecimento: iFood');
    expect(wrapper.text()).toContain('3 transações');
    expect(wrapper.text()).not.toContain('Hortifruti');
  });

  it('shows every transaction when the route names none', async () => {
    const { wrapper } = await mountAtPath('/');
    expect(wrapper.text()).not.toContain('Estabelecimento:');
    expect(wrapper.text()).toContain('Hortifruti');
  });

  it('lets the filter chip be removed, which empties the filter', async () => {
    const { wrapper } = await mountAtPath('/?establishment=iFood');
    const chip = wrapper.findAll('button').find((b) => b.text().includes('×') && b.element.parentElement?.textContent?.includes('iFood'));
    await chip!.trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('Hortifruti');
  });
});
