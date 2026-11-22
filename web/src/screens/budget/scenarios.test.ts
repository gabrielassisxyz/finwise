import { flushPromises } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { createFakeApi } from '@/api/fake';
import type { BudgetDoc, FinwiseApi } from '@/api/types';
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
    const m = await mountBudgetAt('/budget/cenarios', ScenariosScreen, { earlier: true });
    expect(m.month.isClosed).toBe(true);
    expect(m.router.currentRoute.value.path).toBe('/budget');
  });
});

describe('Scenarios keep each line its id', () => {
  const NOV = '2026-11';
  const plainCopy = (doc: BudgetDoc): BudgetDoc => JSON.parse(JSON.stringify(doc)) as BudgetDoc;
  const idsOf = (doc: BudgetDoc) => doc.map((group) => group.lines.map((line) => [line.id, line.name]));

  /**
   * A stand-in whose budget of the month answers an id on every line, with a scenario kept under other ids,
   * that records the drafts it is given.
   */
  async function withIds() {
    const api: FinwiseApi = createFakeApi();
    const doc = await api.budget.document(NOV);
    let next = 100;
    for (const group of doc) for (const line of group.lines) line.id = String(next++);
    await api.budget.save(NOV, doc);
    const kept = plainCopy(doc);
    for (const group of kept) for (const line of group.lines) line.id = `9${line.id}`;
    await api.budget.saveScenario('Guardado', kept);
    const sent: Record<'evaluated' | 'saved' | 'applied', BudgetDoc[]> = { evaluated: [], saved: [], applied: [] };
    const { evaluateScenario, saveScenario, applyScenario } = api.budget;
    api.budget.evaluateScenario = (month, draft) => (sent.evaluated.push(plainCopy(draft)), evaluateScenario(month, draft));
    api.budget.saveScenario = (name, draft) => (sent.saved.push(plainCopy(draft)), saveScenario(name, draft));
    api.budget.applyScenario = (month, draft) => (sent.applied.push(plainCopy(draft)), applyScenario(month, draft));
    return { api, doc, kept, sent };
  }

  it('keeps the ids of the budget it copied through a change of amount, and sends them on', async () => {
    const { api, doc, sent } = await withIds();
    const m = await mountBudgetAt('/budget/cenarios', ScenariosScreen, { api });
    await typeInto(field(m, 'Renda extra'), '0');
    expect(sent.evaluated.at(-1)![0]!.lines[1]!.amount).toBe(0);
    expect(idsOf(sent.evaluated.at(-1)!)).toEqual(idsOf(doc));

    await button(m, 'Salvar cenário').trigger('click');
    await flushPromises();
    expect(idsOf(sent.saved[0]!)).toEqual(idsOf(doc));

    await button(m, 'Aplicar ao budget').trigger('click');
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect(sent.applied[0]![0]!.lines[1]!.amount).toBe(0);
    expect(idsOf(sent.applied[0]!)).toEqual(idsOf(doc));
  });

  it('keeps the ids of a saved scenario it loaded through a change of amount, and sends them on', async () => {
    const { api, kept, sent } = await withIds();
    const m = await mountBudgetAt('/budget/cenarios', ScenariosScreen, { api });
    await m.wrapper.findAll('.chip').find((chip) => chip.text() === 'Guardado')!.trigger('click');
    await flushPromises();
    await typeInto(field(m, 'Salário'), '11000');
    expect(sent.evaluated.at(-1)![0]!.lines[0]!.amount).toBe(11000);
    expect(idsOf(sent.evaluated.at(-1)!)).toEqual(idsOf(kept));

    await button(m, 'Salvar cenário').trigger('click');
    await flushPromises();
    expect(idsOf(sent.saved[0]!)).toEqual(idsOf(kept));

    await button(m, 'Aplicar ao budget').trigger('click');
    m.overlay.confirmation!.onConfirm();
    await flushPromises();
    expect(idsOf(sent.applied[0]!)).toEqual(idsOf(kept));
  });
});

describe('Scenario amounts', () => {
  async function amountsAfterTyping(value: string) {
    const m = await mountScenarios();
    const evaluate = vi.spyOn(m.api.budget, 'evaluateScenario');
    await typeInto(field(m, 'Renda extra'), value);
    return evaluate.mock.calls.map(([, draft]) => draft.flatMap((group) => group.lines.map((line) => line.amount)));
  }

  it('turns a negative into 0 before the scenario is weighed', async () => {
    const sent = await amountsAfterTyping('-50');
    expect(sent.length).toBeGreaterThan(0);
    expect(sent.flat().every((value) => value >= 0)).toBe(true);
    expect(sent.at(-1)).toContain(0);
  });

  it('sends a positive amount as typed', async () => {
    const sent = await amountsAfterTyping('150');
    expect(sent.at(-1)).toContain(150);
  });
});
