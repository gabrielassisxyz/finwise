import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { useRevisionStore } from '@/stores/revision';
import { mountBudgetAt, plain, typeInto } from './budgetTestSupport';
import ScenariosScreen from './ScenariosScreen.vue';

const mountScenarios = () => mountBudgetAt('/budget/cenarios', ScenariosScreen);
type Mounted = Awaited<ReturnType<typeof mountScenarios>>;

const field = (m: Mounted, label: string) => m.wrapper.find(`input[aria-label="${label}"]`);
const button = (m: Mounted, label: string) => m.wrapper.findAll('button').find((b) => b.text() === label)!;

describe('Scenarios screen', () => {
  it('puts the current budget, read only, beside an editable copy with a name', async () => {
    const m = await mountScenarios();
    const left = m.wrapper.findAll('section')[0]!;
    expect(left.find('input').exists()).toBe(false);
    expect(plain(left.find('.summary').text())).toBe('Investimento R$ 3.800 · Taxa de investimento 32%');
    expect((field(m, 'Nome do cenário').element as HTMLInputElement).value).toBe('Mês sem renda extra');
    expect((field(m, 'Renda extra').element as HTMLInputElement).value).toBe('1500');
    expect(plain(m.wrapper.find('.difference').text())).toBe('Investimento R$ 3.800 → R$ 3.800 (sem diferença)');
    expect(m.wrapper.find('.amount.changed').exists()).toBe(false);
  });

  it('shows the impact of a change as it is made, and marks the value that changed', async () => {
    const m = await mountScenarios();
    await typeInto(field(m, 'Renda extra'), '0');
    expect(plain(m.wrapper.find('.difference').text())).toBe('Investimento R$ 3.800 → R$ 2.300 (− R$ 1.500)');
    expect(plain(m.wrapper.find('.rate').text())).toBe('Taxa de investimento 32% → 22%');
    expect(m.wrapper.find('.difference').classes()).toContain('bad');
    expect(m.wrapper.findAll('.amount.changed')).toHaveLength(1);
    expect(field(m, 'Renda extra').classes()).toContain('changed');
    expect(plain(m.wrapper.find('.total').text())).toContain('Investimento');
  });

  it('shows a gain in green with a plus sign', async () => {
    const m = await mountScenarios();
    await typeInto(field(m, 'Salário'), '11000');
    expect(plain(m.wrapper.find('.difference').text())).toBe('Investimento R$ 3.800 → R$ 4.800 (+ R$ 1.000)');
    expect(m.wrapper.find('.difference').classes()).toContain('positive');
  });

  it('writes a negative investment with its minus sign', async () => {
    const m = await mountScenarios();
    await typeInto(field(m, 'Salário'), '1000');
    expect(plain(m.wrapper.find('.difference').text())).toContain('→ − R$ 5.200');
  });

  it('saves a scenario under "Salvos" and loads it again', async () => {
    const m = await mountScenarios();
    expect(m.wrapper.find('.saved').exists()).toBe(false);
    await typeInto(field(m, 'Renda extra'), '0');
    await typeInto(field(m, 'Nome do cenário'), 'Sem extra');
    await button(m, 'Salvar cenário').trigger('click');
    await flushPromises();
    expect(m.overlay.toast?.message).toBe('Cenário "Sem extra" salvo.');
    expect(m.wrapper.findAll('.chip').map((chip) => chip.text())).toEqual(['Sem extra']);

    await typeInto(field(m, 'Renda extra'), '700');
    await m.wrapper.find('.chip').trigger('click');
    await flushPromises();
    expect((field(m, 'Renda extra').element as HTMLInputElement).value).toBe('0');
    expect((field(m, 'Nome do cenário').element as HTMLInputElement).value).toBe('Sem extra');
  });

  it('gives a scenario without a name one', async () => {
    const m = await mountScenarios();
    await typeInto(field(m, 'Nome do cenário'), '');
    await button(m, 'Salvar cenário').trigger('click');
    await flushPromises();
    expect(m.overlay.toast?.message).toBe('Cenário "Cenário sem nome" salvo.');
  });

  it('asks before applying, then makes it the budget from the month on and returns to the budget', async () => {
    const m = await mountScenarios();
    const revision = useRevisionStore();
    const before = revision.current;
    await typeInto(field(m, 'Renda extra'), '0');
    await button(m, 'Aplicar ao budget').trigger('click');
    expect(m.overlay.confirmation?.message).toBe(
      'Aplicar "Mês sem renda extra" ao budget? Vale a partir de novembro de 2026. Meses fechados não mudam.',
    );
    expect(m.overlay.confirmation?.confirmLabel).toBe('Aplicar ao budget');
    expect((await m.api.budget.document('2026-11'))[0]!.lines[1]!.amount).toBe(1500);
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect((await m.api.budget.document('2026-11'))[0]!.lines[1]!.amount).toBe(0);
    expect((await m.api.budget.document('2026-10'))[0]!.lines[1]!.amount).toBe(1500);
    expect(revision.current).toBe(before + 1);
    expect(m.router.currentRoute.value.path).toBe('/budget');
  });

  it('goes back to the budget from "‹ Budget"', async () => {
    const m = await mountScenarios();
    await button(m, '‹ Budget').trigger('click');
    await flushPromises();
    expect(m.router.currentRoute.value.path).toBe('/budget');
  });

  it('is left when the month changes', async () => {
    const m = await mountScenarios();
    m.month.shift(1);
    await flushPromises();
    expect(m.router.currentRoute.value.path).toBe('/budget');
  });

  it('is not open on a closed month', async () => {
    const m = await mountBudgetAt('/budget', ScenariosScreen);
    m.month.shift(-1);
    await m.router.push('/budget/cenarios');
    await flushPromises();
    expect(m.router.currentRoute.value.path).toBe('/budget');
  });
});
