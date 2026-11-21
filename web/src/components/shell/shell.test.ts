import { describe, expect, it } from 'vitest';
import { PROTOTYPE_SHELL } from '@/api/fake/prototype-data';
import type { ShellOverview } from '@/api/types';
import { useMonthStore } from '@/stores/month';
import { useSessionStore } from '@/stores/session';
import { mountAt } from '@/test/mount';
import AppHeader from './AppHeader.vue';
import PhoneNav from './PhoneNav.vue';
import SidebarNav from './SidebarNav.vue';

const overview: ShellOverview = PROTOTYPE_SHELL;

describe('SidebarNav', () => {
  it('marks the destination of the current route', async () => {
    const { wrapper } = await mountAt(SidebarNav, '/revisao', { props: { pendingCount: 0 } });
    expect(wrapper.find('.item.active').text()).toBe('Revisão');
  });

  it('shows the pending count on Revisão and hides it at zero', async () => {
    const withPending = await mountAt(SidebarNav, '/', { props: { pendingCount: 7 } });
    expect(withPending.wrapper.find('.count').text()).toBe('7');

    const without = await mountAt(SidebarNav, '/', { props: { pendingCount: 0 } });
    expect(without.wrapper.find('.count').exists()).toBe(false);
  });
});

describe('AppHeader', () => {
  async function mountHeader(sync = overview.sync) {
    const mounted = await mountAt(AppHeader, '/', { props: { sync, today: overview.today } });
    const month = useMonthStore();
    month.configure(overview);
    await mounted.wrapper.vm.$nextTick();
    return { ...mounted, month };
  }

  it('opens on the month of today and stops at the last month', async () => {
    const { wrapper, month } = await mountHeader();
    expect(wrapper.find('.month').text()).toBe('novembro 2026');

    const [, next] = wrapper.findAll('.arrow');
    await next!.trigger('click');
    expect(month.selected).toBe('2026-12');
    expect(next!.attributes('disabled')).toBeDefined();

    await next!.trigger('click');
    expect(month.selected).toBe('2026-12');
  });

  it('shows the seal only on a closed month', async () => {
    const { wrapper } = await mountHeader();
    expect(wrapper.find('.closed-seal').exists()).toBe(false);

    await wrapper.findAll('.arrow')[0]!.trigger('click');
    expect(wrapper.find('.month').text()).toBe('outubro 2026');
    expect(wrapper.find('.closed-seal').text()).toBe('Fechado');
  });

  it('says when the last sync was, relative to today', async () => {
    const today = await mountHeader();
    expect(today.wrapper.find('.sync').text()).toContain('Atualizado hoje às 02:00');

    const earlier = await mountHeader({ state: 'ok', lastSyncDay: '2026-11-26', lastSyncTime: '02:03' });
    expect(earlier.wrapper.find('.sync').text()).toContain('Atualizado em 26/11 às 02:03');
  });

  it('replaces the sync line with a link to the data settings when a bank needs attention', async () => {
    const { wrapper } = await mountHeader({ state: 'warning', message: 'Itaú: consentimento expirado' });
    const warning = wrapper.find('.sync-warning');
    expect(warning.text()).toBe('Itaú: consentimento expirado');
    expect(warning.attributes('href')).toBe('/config/dados');
    expect(wrapper.find('.sync').exists()).toBe(false);
  });

  it('locks the app from the padlock', async () => {
    const { wrapper } = await mountHeader();
    const session = useSessionStore();
    session.locked = false;
    await wrapper.find('.lock').trigger('click');
    await new Promise((done) => setTimeout(done));
    expect(session.locked).toBe(true);
  });
});

describe('PhoneNav', () => {
  it('keeps five destinations on the bar and the rest behind Mais', async () => {
    const { wrapper } = await mountAt(PhoneNav, '/', { props: { pendingCount: 0 } });
    expect(wrapper.findAll('.tab').map((tab) => tab.text())).toEqual([
      'Início',
      'Revisão',
      'Budget',
      'Cofrinhos',
      'Assinaturas',
      'Mais',
    ]);
    expect(wrapper.find('.sheet').exists()).toBe(false);

    await wrapper.find('button.tab').trigger('click');
    expect(wrapper.findAll('.sheet-item').map((item) => item.text())).toEqual(['Patrimônio', 'Contas', 'Configuração']);
  });

  it('marks Mais when the current screen lives behind it', async () => {
    const { wrapper } = await mountAt(PhoneNav, '/patrimonio', { props: { pendingCount: 0 } });
    expect(wrapper.find('button.tab').classes()).toContain('active');
  });

  it('closes the sheet on navigation and on a tap outside', async () => {
    const { wrapper, router } = await mountAt(PhoneNav, '/', { props: { pendingCount: 0 } });
    await wrapper.find('button.tab').trigger('click');
    await router.push('/patrimonio');
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.sheet').exists()).toBe(false);

    await wrapper.find('button.tab').trigger('click');
    await wrapper.find('.backdrop').trigger('click');
    expect(wrapper.find('.sheet').exists()).toBe(false);
  });
});
