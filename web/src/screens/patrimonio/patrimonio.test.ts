import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { createFakeApi } from '@/api/fake';
import type { NetWorthOverview } from '@/api/types';
import { mountAt } from '@/test/mount';
import { axisLabels, columnHeight, stackShares } from './chartGeometry';
import NetWorthChart from './NetWorthChart.vue';
import PatrimonioScreen from './PatrimonioScreen.vue';
import PlanBlocks from './PlanBlocks.vue';

const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

async function mountScreen() {
  const mounted = await mountAt(PatrimonioScreen, '/patrimonio');
  await flushPromises();
  return mounted;
}

describe('Patrimônio screen', () => {
  it('shows the three buckets and their total', async () => {
    const { wrapper } = await mountScreen();
    const cells = wrapper.findAll('.cell').map((cell) => plain(cell.text()));
    expect(cells).toEqual([
      'Disponível ▸R$ 8.608,75',
      'Reservado ▸R$ 34.150,00',
      'Investido ▸R$ 37.800,00',
      'Patrimônio totalR$ 80.558,75',
    ]);
  });

  it('opens a bucket onto what makes it up, and closes it on a second click', async () => {
    const { wrapper } = await mountScreen();
    expect(wrapper.text()).not.toContain('O que está em');

    const reserved = wrapper.findAll('button.bucket')[1]!;
    await reserved.trigger('click');
    expect(reserved.attributes('aria-expanded')).toBe('true');
    expect(plain(reserved.text())).toContain('Reservado ▾');
    expect(wrapper.find('.head').exists()).toBe(true);
    expect(wrapper.text()).toContain('O que está em Reservado');
    expect(wrapper.findAll('.item')).toHaveLength(10);

    await reserved.trigger('click');
    expect(wrapper.text()).not.toContain('O que está em');
  });

  it('moves the open list from one bucket to another', async () => {
    const { wrapper } = await mountScreen();
    const [available, , invested] = wrapper.findAll('button.bucket');
    await available!.trigger('click');
    expect(wrapper.findAll('.item').map((item) => plain(item.text()))).toEqual([
      'Itaú · Conta corrente · contaR$ 7.020,00',
      'Mercado Pago · Conta · contaR$ 312,40',
      'C6 · Conta corrente · contaR$ 1.180,25',
      'Nubank · Conta · contaR$ 96,10',
    ]);
    await invested!.trigger('click');
    expect(wrapper.text()).toContain('O que está em Investido');
    expect(wrapper.findAll('.item')).toHaveLength(2);
    expect(wrapper.text()).not.toContain('O que está em Disponível');
  });

  it('draws one point for the one month recorded, and swaps it for a stacked column', async () => {
    const { wrapper } = await mountScreen();
    expect(wrapper.findAll('.plot .dot')).toHaveLength(1);
    expect(wrapper.find('.stack').exists()).toBe(false);
    expect(wrapper.find('.plot .dot').attributes('style')).toContain('bottom: 80.5588%');

    const toggle = wrapper.find('.chart-card button.toggle');
    expect(toggle.attributes('aria-pressed')).toBe('false');
    await toggle.trigger('click');

    expect(toggle.attributes('aria-pressed')).toBe('true');
    expect(wrapper.find('.plot .dot').exists()).toBe(false);
    expect(wrapper.findAll('.stack')).toHaveLength(1);
    expect(wrapper.findAll('.slice').map((slice) => slice.classes()[1])).toEqual(['invested', 'reserved', 'available']);
  });

  it('labels the axis and the six months, and says why there is no line yet', async () => {
    const { wrapper } = await mountScreen();
    expect(wrapper.find('.axis').findAll('span').map((label) => label.text())).toEqual([
      '100 mil',
      '75 mil',
      '50 mil',
      '25 mil',
      '0',
    ]);
    expect(wrapper.find('.months').findAll('span').map((label) => label.text())).toEqual([
      'out',
      'nov',
      'dez',
      'jan',
      'fev',
      'mar',
    ]);
    expect(wrapper.find('.note').text()).toBe(
      'O histórico começa em novembro de 2026. A linha aparece quando houver dois meses registrados.',
    );
  });

  it('puts the planned and the done contribution side by side', async () => {
    const { wrapper } = await mountScreen();
    const block = wrapper.findAll('.block')[0]!;
    expect(plain(block.text())).toBe('Aporte do mêsPlanejadoR$ 5.220,00FeitoR$ 320,00');
    const widths = block.findAll('.fill').map((fill) => fill.attributes('style'));
    expect(widths[0]).toContain('width: 100%');
    expect(widths[1]).toContain('width: 6.13');
  });

  it('puts the planned and the real investment rate side by side', async () => {
    const { wrapper } = await mountScreen();
    const block = wrapper.findAll('.block')[1]!;
    expect(plain(block.text())).toContain('Taxa de investimentonovembro 2026');
    expect(plain(block.text())).toContain('Planejada32%');
    expect(plain(block.text())).toContain('Real até agora0%');
    expect(block.findAll('.fill').map((fill) => fill.attributes('style'))).toEqual(['width: 32%;', 'width: 0%;']);
  });

  it('lists the yield of every open pot, with the share of the CDI, and the total', async () => {
    const { wrapper } = await mountScreen();
    const rows = wrapper.findAll('.row').map((row) => plain(row.text()));
    expect(rows).toHaveLength(13);
    expect(rows[0]).toBe('RF Saúde · 120% do CDI+ R$ 25,20');
    expect(rows.at(-1)).toBe('Sem nome · 100% do CDI+ R$ 5,25');
    expect(plain(wrapper.find('.sum').text())).toBe('Total+ R$ 854,18');
  });

  it('keeps the chart and the open bucket where they were when the screen is left and opened again', async () => {
    const first = await mountScreen();
    await first.wrapper.findAll('button.bucket')[2]!.trigger('click');
    await first.wrapper.find('.chart-card button.toggle').trigger('click');
    first.wrapper.unmount();

    const again = await mountAt(PatrimonioScreen, '/patrimonio');
    await flushPromises();
    expect(again.wrapper.text()).not.toContain('O que está em');
    expect(again.wrapper.findAll('.stack')).toHaveLength(0);
  });
});

describe('PlanBlocks', () => {
  it('draws nothing for a negative planned rate instead of a bar running backwards', async () => {
    const contribution = { planned: 100, done: 0, progress: 0 };
    const rate = { month: '2026-11', planned: -8, actual: 0 };
    const { wrapper } = await mountAt(PlanBlocks, '/', { props: { contribution, rate } });
    const fills = wrapper.findAll('.block')[1]!.findAll('.fill');
    expect(fills[0]!.attributes('style')).toBe('width: 0%;');
    expect(wrapper.findAll('.block')[1]!.text()).toContain('-8%');
  });
});

describe('NetWorthChart', () => {
  async function overview(): Promise<NetWorthOverview> {
    return createFakeApi().netWorth.overview();
  }

  it('leaves out the note about a missing line once two months are recorded', async () => {
    const { chart, historyStart } = await overview();
    const point = chart.points[0]!;
    const two = { ...chart, points: [{ ...point, month: '2026-10' }, point] };
    const { wrapper } = await mountAt(NetWorthChart, '/', { props: { chart: two, historyStart, stacked: false } });
    expect(wrapper.find('.note').text()).toBe('O histórico começa em novembro de 2026.');
    expect(wrapper.findAll('.plot .dot')).toHaveLength(2);
  });

  it('asks its parent to flip the stacking', async () => {
    const { chart, historyStart } = await overview();
    const { wrapper } = await mountAt(NetWorthChart, '/', { props: { chart, historyStart, stacked: false } });
    await wrapper.find('button.toggle').trigger('click');
    expect(wrapper.emitted('update:stacked')).toEqual([[true]]);
  });
});

describe('chart geometry', () => {
  it('stands a column at its share of the top of the chart', () => {
    expect(columnHeight(25000, 100000)).toBe(25);
    expect(columnHeight(80558.75, 100000)).toBeCloseTo(80.55875, 5);
  });

  it('cuts a column into the three buckets, invested at the bottom', () => {
    const slices = stackShares({ month: '2026-11', available: 10, reserved: 30, invested: 60, total: 100 });
    expect(slices).toEqual([
      { tone: 'invested', height: 60 },
      { tone: 'reserved', height: 30 },
      { tone: 'available', height: 10 },
    ]);
  });

  it('draws an empty column when there is nothing to cut', () => {
    const slices = stackShares({ month: '2026-11', available: 0, reserved: 0, invested: 0, total: 0 });
    expect(slices.map((slice) => slice.height)).toEqual([0, 0, 0]);
  });

  it('labels the axis from the chart top', () => {
    expect(axisLabels(100000)).toEqual(['100 mil', '75 mil', '50 mil', '25 mil', '0']);
    expect(axisLabels(200000)[1]).toBe('150 mil');
  });
});
