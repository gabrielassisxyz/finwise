import type {
  BudgetCell,
  BudgetEstablishment,
  BudgetGroupView,
  BudgetLineView,
  BudgetView,
  EditableGroupKey,
  ScenarioImpact,
  YearMonth,
} from '../types';
import { budgetFor } from './budget-versions';
import type { FakeBudgetGroup, FakeBudgetLine, FakeSavingsPot, FakeState, FakeTransaction } from './model';
import { contributions, groupTotalOf, percentOfIncome, plannedInvestmentOf, spentByLine, toContract } from './rules';

// What the budget screen shows, worked out from a budget and the month's
// spending. The server will own all of it; the screen only writes it.

const cents = (amount: number): number => Math.round(amount * 100) / 100;
const sum = (amounts: number[]): number => amounts.reduce((total, amount) => total + amount, 0);
const none: BudgetCell = { type: 'none' };
const money = (amount: number): BudgetCell => ({ type: 'money', amount });
const share = (part: number, whole: number): number => Math.min(100, whole ? (part / whole) * 100 : 0);

type Spent = Record<string, number>;

function lineView(name: string, cells: BudgetLineView['cells'], extra: Partial<BudgetLineView> = {}): BudgetLineView {
  return { name, cells, marks: [], over: false, bar: null, expandable: false, ...extra };
}

function incomeLine(line: FakeBudgetLine, spent: Spent): BudgetLineView {
  const marks = line.flag ? [{ text: line.flag, tone: 'neutral' as const }] : [];
  return lineView(line.name, [money(line.amount), money(spent[line.name] ?? 0), none], { marks, expandable: true });
}

/** A fixed line compares what it was meant to cost with what it cost; a variable one is a cap, drawn as a bar. */
function outgoingLine(line: FakeBudgetLine, spent: Spent, capped: boolean): BudgetLineView {
  const done = spent[line.name] ?? 0;
  const left = cents(line.amount - done);
  const over = left < 0;
  const cells: BudgetLineView['cells'] = [money(line.amount), money(done), { type: 'difference', amount: left }];
  if (!capped) return lineView(line.name, cells, { over, expandable: true });
  const marks = over ? [{ text: 'Estourou', tone: 'bad' as const }] : [];
  return lineView(line.name, cells, { over, marks, bar: share(done, line.amount), expandable: true });
}

const potNamed = (state: FakeState, name: string): FakeSavingsPot | undefined =>
  state.savingsPots.find((pot) => pot.name === name);

function provisionLine(state: FakeState, line: FakeBudgetLine): BudgetLineView {
  const charge = potNamed(state, line.name)?.nextCharge ?? null;
  const cells: BudgetLineView['cells'] = [
    money(line.amount),
    money(potNamed(state, line.name)?.balance ?? 0),
    { type: 'next-charge', charge: charge && { description: charge.description, day: charge.day } },
  ];
  return lineView(line.name, cells);
}

function wishLine(state: FakeState, line: FakeBudgetLine, spent: Spent): BudgetLineView {
  const cells: BudgetLineView['cells'] = [
    money(line.amount),
    money(potNamed(state, line.name)?.balance ?? 0),
    money(spent[line.name] ?? 0),
  ];
  return lineView(line.name, cells, { expandable: true });
}

// A rainy fund is a pot named "RF ..." with a target; the budget lists it
// without the prefix.
const isRainyFund = (pot: FakeSavingsPot): boolean => pot.name.startsWith('RF ') && pot.target !== null;

function rainyFundLine(pot: FakeSavingsPot): BudgetLineView {
  const target = pot.target ?? 0;
  const cells: BudgetLineView['cells'] = [
    money(target),
    money(pot.balance),
    { type: 'rainy-deposit', switchedOn: pot.balance < target },
  ];
  return lineView(pot.name.replace('RF ', ''), cells, { bar: share(pot.balance, target) });
}

function editableGroup(state: FakeState, group: FakeBudgetGroup, spent: Spent): BudgetGroupView {
  const planned = sum(group.lines.map((line) => line.amount));
  const done = cents(sum(group.lines.map((line) => spent[line.name] ?? 0)));
  const build: Record<EditableGroupKey, (line: FakeBudgetLine) => BudgetLineView> = {
    rec: (line) => incomeLine(line, spent),
    fix: (line) => outgoingLine(line, spent, false),
    var: (line) => outgoingLine(line, spent, true),
    prov: (line) => provisionLine(state, line),
    res: (line) => wishLine(state, line, spent),
  };
  const comparesWithSpending = group.key === 'fix' || group.key === 'var';
  return {
    key: group.key,
    name: group.name,
    subtotal: planned,
    subtotalSpent: comparesWithSpending ? done : null,
    over: group.key === 'var' && done > planned,
    lines: group.lines.map(build[group.key]),
  };
}

function investmentLine(investment: number, rate: number, made: number): BudgetLineView {
  const over = investment < 0;
  const marks = over ? [{ text: 'Budget maior que a receita', tone: 'bad' as const }] : [];
  return lineView('Investimento', [money(investment), money(made), { type: 'percent', value: rate }], { over, marks });
}

/** Whether this month's investment deposit has been made; a closed month is taken as fully done. */
function investmentMade(state: FakeState, closed: boolean, investment: number): number {
  const done = state.plannedContributions.find((item) => item.name === 'Investimento')?.state === 'done';
  return done || closed ? investment : 0;
}

export function buildView(
  state: FakeState,
  original: FakeTransaction[],
  month: YearMonth,
  draft?: FakeBudgetGroup[],
): BudgetView {
  const groups = draft ?? budgetFor(state, month);
  const closed = state.shell.closedMonths.some((entry) => entry.month === month);
  const spent = spentByLine(state, original, month);
  const income = groupTotalOf(groups, 'rec');
  const investment = cents(plannedInvestmentOf(groups));
  const outgoing = sum((['fix', 'var', 'prov', 'res'] as const).map((key) => groupTotalOf(groups, key)));
  const rate = percentOfIncome(income, investment);
  const rainy = state.savingsPots.filter(isRainyFund);
  const fees = cents(spent['Tarifas e juros'] ?? 0);
  const derived: BudgetGroupView[] = [
    { key: 'rf', name: 'Rainy Funds', subtotal: cents(sum(rainy.map((pot) => pot.balance))), subtotalSpent: null, over: false, lines: rainy.map(rainyFundLine) },
    {
      key: 'tar',
      name: 'Tarifas e juros',
      subtotal: fees,
      subtotalSpent: null,
      over: false,
      lines: [lineView('Tarifas e juros', [money(fees), none, none], { expandable: true })],
    },
    {
      key: 'inv',
      name: 'Investimento',
      subtotal: investment,
      subtotalSpent: null,
      over: investment < 0,
      lines: [investmentLine(investment, rate, investmentMade(state, closed, investment))],
    },
  ];
  return {
    month,
    closed,
    income,
    // Everything the income is split into, the investment included.
    allocated: cents(outgoing + investment),
    investmentRate: rate,
    groups: [...groups.map((group) => editableGroup(state, group, spent)), ...derived],
  };
}

/** The establishments that make a line up in a month, the biggest first. */
export function establishmentsOf(
  state: FakeState,
  month: YearMonth,
  line: string,
): BudgetEstablishment[] {
  const byKey = new Map<string, BudgetEstablishment>();
  for (const transaction of state.transactions.filter((candidate) => candidate.day.startsWith(month))) {
    const toLine = contributions(transaction).filter(([name]) => name === line);
    if (toLine.length === 0) continue;
    const found = byKey.get(transaction.establishmentKey);
    const entry = found ?? {
      key: transaction.establishmentKey,
      name: toContract(state, transaction).name,
      amount: 0,
      count: 0,
    };
    entry.amount = cents(entry.amount + sum(toLine.map(([, amount]) => amount)));
    entry.count += 1;
    byKey.set(entry.key, entry);
  }
  return [...byKey.values()].sort((a, b) => b.amount - a.amount);
}

export function scenarioImpact(state: FakeState, month: YearMonth, scenario: FakeBudgetGroup[]): ScenarioImpact {
  const current = budgetFor(state, month);
  const figures = (groups: FakeBudgetGroup[]) => {
    const investment = cents(plannedInvestmentOf(groups));
    return { investment, rate: percentOfIncome(groupTotalOf(groups, 'rec'), investment) };
  };
  const before = figures(current);
  const after = figures(scenario);
  return {
    current: before,
    scenario: after,
    difference: cents(after.investment - before.investment),
    changed: scenario.map((group, groupIndex) =>
      group.lines.map((line, lineIndex) => line.amount !== current[groupIndex]?.lines[lineIndex]?.amount),
    ),
  };
}
