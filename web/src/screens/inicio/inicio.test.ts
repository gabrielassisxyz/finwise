import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { createFakeApi } from '@/api/fake';
import type { HomeSummary } from '@/api/types';
import { useMonthStore } from '@/stores/month';
import { useOverlayStore } from '@/stores/overlay';
import { mountAt, prototypeOverview } from '@/test/mount';
import InicioScreen from './InicioScreen.vue';
import SummaryStrip from './SummaryStrip.vue';

async function mountStrip(change: (summary: HomeSummary) => void = () => {}) {
  const summary = await createFakeApi().home.summary('2026-11');
  change(summary);
  const mounted = await mountAt(SummaryStrip, '/', { props: { summary, month: '2026-11' } });
  await flushPromises();
  return { ...mounted, overlay: useOverlayStore() };
}

const text = (wrapper: Awaited<ReturnType<typeof mountStrip>>['wrapper']) => wrapper.text().replace(/ /g, ' ');

describe('SummaryStrip', () => {
  it('says what is left of the month limit', async () => {
    const { wrapper } = await mountStrip();
    expect(text(wrapper)).toContain('Restam R$ 1.420,00');
    expect(text(wrapper)).toContain('Gasto R$ 1.460,00 de R$ 2.880,00');
    expect(wrapper.find('.bar-fill').attributes('style')).toContain('width: 50.69');
    expect(wrapper.find('.figure.over').exists()).toBe(false);
  });

  it('turns to "Passou" and fills the bar once the limit is exceeded', async () => {
    const { wrapper } = await mountStrip((summary) => (summary.limit.spent = 3000));
    expect(text(wrapper)).toContain('Passou R$ 120,00');
    expect(wrapper.find('.figure.over').exists()).toBe(true);
    expect(wrapper.find('.bar-fill').attributes('style')).toContain('width: 100%');
  });

  it('adds the three buckets into the net worth', async () => {
    const { wrapper } = await mountStrip();
    expect(text(wrapper)).toContain('R$ 80.558,75');
    expect(text(wrapper)).toContain('Reservado' + 'R$ 34.150,00');
  });

  it('words the pending count', async () => {
    expect(text((await mountStrip()).wrapper)).toContain('7 para revisar ›');
    expect(text((await mountStrip((summary) => (summary.pendingCount = 1))).wrapper)).toContain('1 para revisar ›');
    expect(text((await mountStrip((summary) => (summary.pendingCount = 0))).wrapper)).toContain('Tudo revisado ›');
  });

  it('shows the pay-ahead card only when there is something to pay ahead', async () => {
    expect(text((await mountStrip()).wrapper)).toContain('Valor a adiantar da fatura');
    const without = await mountStrip((summary) => (summary.payAhead = null));
    expect(text(without.wrapper)).not.toContain('Valor a adiantar da fatura');
  });

  it('lists every week with its guide, what was spent and whether it went over', async () => {
    const { wrapper, overlay } = await mountStrip();
    await wrapper.find('button.card').trigger('click');
    expect(overlay.menu?.title).toBe('Semanas de novembro 2026 · guia e gasto');
    expect(overlay.menu?.items.map((item) => `${item.label} | ${item.hint}`.replace(/ /g, ' '))).toEqual([
      '01–07 | R$ 500 · R$ 620 · acima',
      '08–14 | R$ 470 · R$ 410',
      '15–21 | R$ 400 · R$ 220',
      '22–28 · esta | R$ 355 · R$ 210',
      '29–30 | R$ 142 · —',
    ]);
  });
});

describe('InicioScreen after a write', () => {
  const strip = (wrapper: { text(): string }) => wrapper.text().replace(/\u00a0/g, ' ');

  it('moves the month limit without hiding the strip while it refetches', async () => {
    const { wrapper, api } = await mountAt(InicioScreen, '/');
    useMonthStore().configure(prototypeOverview());
    await flushPromises();
    expect(strip(wrapper)).toContain('Restam R$ 1.420,00');

    const summary = api.home.summary.bind(api.home);
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    vi.spyOn(api.home, 'summary').mockImplementation(async (month) => (await held, summary(month)));

    const row = wrapper.findAll('.row').find((candidate) => candidate.find('.name').text() === 'Bar do Zé')!;
    await row.find('.pill-button').trigger('click');
    useOverlayStore().menu!.items.find((item) => item.label === 'Restaurantes e bares')!.onPick!();
    await flushPromises();

    expect(wrapper.find('.bar-fill').exists()).toBe(true);
    expect(strip(wrapper)).toContain('Restam R$ 1.420,00');
    release();
    await flushPromises();
    expect(strip(wrapper)).toContain('Restam R$ 1.356,00');
    expect(strip(wrapper)).toContain('6 para revisar ›');
  });
});
