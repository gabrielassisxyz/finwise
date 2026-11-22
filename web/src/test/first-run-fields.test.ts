import { flushPromises } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { emptyLedgerState, freshInstallState } from '@/api/fake/first-run-data';
import type { FakeState } from '@/api/fake/model';
import { prototypeState } from '@/api/fake/prototype-data';
import TransactionPanel from '@/components/transaction-panel/TransactionPanel.vue';
import TransactionRow from '@/components/transactions/TransactionRow.vue';
import { createFakeApi } from '@/api/fake';
import { useMonthStore } from '@/stores/month';
import { useShellStore } from '@/stores/shell';
import { usePanelStore } from '@/stores/panel';
import CofrinhoScreen from '@/screens/cofrinhos/CofrinhoScreen.vue';
import CofrinhosScreen from '@/screens/cofrinhos/CofrinhosScreen.vue';
import BudgetScreen from '@/screens/budget/BudgetScreen.vue';
import CardScreen from '@/screens/contas/CardScreen.vue';
import ConfigScreen from '@/screens/config/ConfigScreen.vue';
import PatrimonioScreen from '@/screens/patrimonio/PatrimonioScreen.vue';
import PaydayTab from '@/screens/revisao/PaydayTab.vue';
import { mountAt, prototypeOverview } from './mount';
import { mountRouted } from './mountRouted';

// A figure the ledger cannot give is a dash. These tests pin that for the fields the contract made nullable,
// and the opposite for a true zero, which stays "R$ 0,00".

const Blank = { template: '<div />' };
const plain = (text: string) => text.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
const everyRoute = ['/cofrinhos', '/cofrinhos/:id', '/config/:section?', '/budget', '/patrimonio', '/revisao/salario', '/contas/cartao'].map(
  (path) => ({ path, component: Blank }),
);

async function mountScreen(component: object, path: string, state: FakeState, mountMonth = true) {
  const mounted = await mountRouted(component, everyRoute, path, { state });
  useShellStore().overview = prototypeOverview();
  if (mountMonth) {
    const month = useMonthStore();
    month.selected = '2026-11';
    month.first = '2026-10';
    month.last = '2026-12';
  }
  await flushPromises();
  return mounted;
}

describe('an unknown figure is a dash and never a zero', () => {
  it('shows no date for a backup that never ran, and the date of a sync that did', async () => {
    const { wrapper } = await mountScreen(ConfigScreen, '/config/dados', freshInstallState());
    const when = wrapper.find('[data-section="dados"] .when').text();
    expect(plain(when)).toContain('Última sincronização: hoje às 02:00');
    expect(plain(when)).toContain('Último backup: —');
  });

  it('shows no sync date before the first synchronisation', async () => {
    const { wrapper } = await mountScreen(ConfigScreen, '/config/dados', emptyLedgerState());
    expect(plain(wrapper.find('[data-section="dados"] .when').text())).toContain('Última sincronização: —');
  });

  it('shows a dash for the yield and the rate of a pot whose rate is unknown, on its row and on its page', async () => {
    const list = await mountScreen(CofrinhosScreen, '/cofrinhos', freshInstallState());
    const row = list.wrapper.findAll('.row').find((candidate) => candidate.text().includes('Caixinha da casa'))!;
    expect(row.find('[data-label="% CDI"]').text()).toBe('—');
    expect(row.find('[data-label="Rendimento no mês"]').text()).toBe('—');

    const page = await mountScreen(CofrinhoScreen, '/cofrinhos/p1', freshInstallState());
    expect(page.wrapper.find('.subtitle').text()).toBe('Mercado Pago · Reservado');
    expect(plain(page.wrapper.find('[class*="month"]').text())).toContain('—');
    expect(plain(page.wrapper.text())).not.toContain('R$ 0,00 O histórico');
  });

  it('keeps R$ 0,00 for the yield of a pot that is closed, which truly earned nothing', async () => {
    const page = await mountScreen(CofrinhoScreen, '/cofrinhos/i0', freshInstallState());
    expect(plain(page.wrapper.find('.total').text())).toBe('R$ 0,00');
  });

  it('lists a day whose yield is unknown with a dash, beside the days that are known, and never as a zero', async () => {
    const state = prototypeState();
    state.savingsPots.find((pot) => pot.id === 'rfs')!.unknownYieldDays = [25];
    const { wrapper } = await mountScreen(CofrinhoScreen, '/cofrinhos/rfs', state);
    expect(plain(wrapper.find('.total').text())).toBe('—');
    await wrapper.find('.month').trigger('click');
    const days = wrapper.findAll('.yield-day').map((day) => plain(day.text()));
    expect(days).toHaveLength(18);
    expect(days.filter((day) => day.endsWith('—'))).toHaveLength(1);
    expect(days.find((day) => day.endsWith('—'))).toContain('25');
    expect(days.filter((day) => day.includes('R$'))).toHaveLength(17);
    expect(days.join(' ')).not.toContain('R$ 0,00');
  });

  it('shows a dash for the total yield when no pot has a known one', async () => {
    const state = freshInstallState();
    state.savingsPots = state.savingsPots.filter((pot) => pot.cdiPercent === null);
    const { wrapper } = await mountScreen(PatrimonioScreen, '/patrimonio', state);
    const card = plain(wrapper.text());
    expect(card).toMatch(/Rendimento do mês.*Caixinha da casa —/);
    expect(card).toMatch(/Total\s*—/);
  });

  it('shows no investment rate when the budget has no income, and the rate when it has one', async () => {
    const without = await mountScreen(BudgetScreen, '/budget', freshInstallState());
    expect(plain(without.wrapper.find('.figures').text())).toMatch(/Taxa de investimento\s*—/);
    const withIncome = await mountScreen(BudgetScreen, '/budget', prototypeState());
    expect(plain(withIncome.wrapper.find('.figures').text())).toMatch(/Taxa de investimento\s*32%/);
  });

  it('says there is no budget yet, once, on a budget with no lines', async () => {
    const { wrapper } = await mountScreen(BudgetScreen, '/budget', freshInstallState());
    expect(wrapper.findAll('.notice').map((notice) => plain(notice.text()))).toEqual([
      'Nenhuma linha no budget ainda. Use Editar budget para montar o seu.',
    ]);
    const filled = await mountScreen(BudgetScreen, '/budget', prototypeState());
    expect(filled.wrapper.find('.notice').exists()).toBe(false);
  });

  it('shows no balance and no verdict when no account receives the salary', async () => {
    const state = emptyLedgerState();
    const { wrapper } = await mountScreen(PaydayTab, '/revisao/salario', state);
    // Flex children carry no whitespace between them, so each is read on its own.
    const cells = wrapper.findAll('.cell').map((cell) => plain(Array.from(cell.element.children, (child) => child.textContent).join(' ')));
    expect(cells).toEqual([
      'Colchão necessário R$ 0,00 Fixos e essenciais + variáveis do mês',
      'Saldo em conta — Nenhuma conta recebe o salário',
      'Diferença para o colchão — Sem saldo para comparar',
    ]);
  });

  it('offers no suggested threshold while no band has corrections behind it', async () => {
    const fresh = await mountScreen(ConfigScreen, '/config/categorizacao', freshInstallState());
    const section = fresh.wrapper.find('[data-section="categorizacao"]');
    expect(plain(section.find('.legend').text())).toContain('Sugerido: —');
    expect(section.find('.use').exists()).toBe(false);
    expect(section.findAll('.band .value').map((value) => value.text())).toEqual(['—', '—', '—', '—', '—']);

    const prototype = await mountScreen(ConfigScreen, '/config/categorizacao', prototypeState());
    expect(prototype.wrapper.find('[data-section="categorizacao"] .use').exists()).toBe(true);
  });
});

describe('what a fresh install and an empty ledger have no honest value for', () => {
  it('renders the card page with no card as a sentence, not as a blank page', async () => {
    const { wrapper } = await mountScreen(CardScreen, '/contas/cartao', emptyLedgerState());
    expect(plain(wrapper.text())).toContain('Nenhum cartão encontrado');
    expect(wrapper.find('h1').exists()).toBe(false);
  });

  it('shows no card where the settings would have named one', async () => {
    const { wrapper } = await mountScreen(ConfigScreen, '/config/cartao', emptyLedgerState());
    expect(plain(wrapper.find('[data-section="cartao"]').text())).toContain('Nenhum cartão encontrado');
    expect(plain(wrapper.find('[data-section="apelidos"]').text())).not.toContain('C6 Carbon');
  });

  it('counts a pot in the net worth from the first day: a reserve as reserved, an investment as invested', async () => {
    const summary = await createFakeApi({ state: freshInstallState() }).home.summary('2026-11');
    expect(summary.netWorth).toEqual({ available: 6171.55, reserved: 14250, invested: 25000 });
  });

  it('lists a pot with no bucket, and opens its page', async () => {
    const withoutBucket = (): FakeState => {
      const state = freshInstallState();
      state.savingsPots = state.savingsPots.map((pot) => (pot.id === 'p3' ? { ...pot, bucket: null } : pot));
      return state;
    };
    const list = await mountScreen(CofrinhosScreen, '/cofrinhos', withoutBucket());
    expect(list.wrapper.findAll('.group-name').map((name) => name.text())).toContain('Sem balde');
    const page = await mountScreen(CofrinhoScreen, '/cofrinhos/p3', withoutBucket());
    expect(page.wrapper.find('h1').text()).toBe('Reserva');
    expect(page.wrapper.find('.subtitle').text()).toBe('Nubank · Sem balde · 100% do CDI');
  });

  it('offers no account to filter by where the ledger has none, card included', async () => {
    expect((await createFakeApi({ state: emptyLedgerState() }).transactions.filterOptions('2026-11')).accounts).toEqual([]);
    const fresh = (await createFakeApi({ state: freshInstallState() }).transactions.filterOptions('2026-11')).accounts;
    expect(fresh.map((account) => account.value)).toEqual(['Itaú', 'Mercado Pago', 'C6', 'C6 Carbon']);
  });

  it('says a list is empty where there is nothing to list', async () => {
    const { wrapper } = await mountScreen(CofrinhosScreen, '/cofrinhos', emptyLedgerState());
    expect(plain(wrapper.find('.empty').text())).toContain('Nenhum cofrinho ainda');
  });
});

describe('a transaction nobody has put on a line', () => {
  const state = freshInstallState();
  const unclassified = state.transactions.find((transaction) => transaction.id === 'f4')!;

  it('renders its row, with a placeholder where the line would be', async () => {
    const api = createFakeApi({ state });
    const transaction = (await api.transactions.list({ month: '2026-11', period: 'month' })).find((row) => row.id === 'f4')!;
    expect(transaction.line).toBeNull();
    expect(transaction.classifiedBy).toBeNull();
    const { wrapper } = await mountAt(TransactionRow, '/', {
      props: { transaction, tab: 'todas', selected: false, iofOpen: false },
    });
    expect(wrapper.text()).toContain(unclassified.name);
  });

  it('opens its panel without naming who classified it', async () => {
    const { wrapper } = await mountAt(TransactionPanel, '/', {}, { state, startLocked: false });
    useShellStore().overview = prototypeOverview();
    usePanelStore().open('f4');
    await flushPromises();
    const section = wrapper.find('.line-button');
    expect(section.find('.line').text()).toBe('Sem linha');
    expect(section.find('.origin').exists()).toBe(false);
    expect(plain(wrapper.text())).not.toMatch(/definida por|null/);
  });
});
