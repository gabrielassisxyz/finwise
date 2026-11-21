import type { ChangeResult } from './changes';
import type { IsoDay, YearMonth } from './common';

/** The groups of the budget a person edits; the others on the screen are derived from them. */
export type EditableGroupKey = 'rec' | 'fix' | 'var' | 'prov' | 'res';

export type BudgetGroupKey = EditableGroupKey | 'rf' | 'tar' | 'inv';

/** One line of the budget as it is stored and edited. */
export interface BudgetLineDoc {
  name: string;
  amount: number;
  /** A marker the line carries, such as "Não garantida". */
  flag?: string;
}

export interface BudgetGroupDoc {
  key: EditableGroupKey;
  name: string;
  lines: BudgetLineDoc[];
}

/** The budget of a month: what is edited, saved and sent back to be evaluated. */
export type BudgetDoc = BudgetGroupDoc[];

/** One value of a row, already worked out; the screen only writes it. */
export type BudgetCell =
  | { type: 'none' }
  | { type: 'money'; amount: number }
  /** Money with a sign: what is left of a cap, or what a line went over. */
  | { type: 'difference'; amount: number }
  | { type: 'percent'; value: number }
  | { type: 'next-charge'; charge: { description: string; day: IsoDay } | null }
  /** A rainy fund's deposit: switched off at its target, switched on again below it. */
  | { type: 'rainy-deposit'; switchedOn: boolean };

export interface BudgetMark {
  text: string;
  tone: 'neutral' | 'bad';
}

/**
 * `over` is a line past its cap, or a budget past its income. `bar` is the
 * share of the amount reached, 0 to 100, for a row that draws one.
 */
export interface BudgetLineView {
  name: string;
  cells: [BudgetCell, BudgetCell, BudgetCell];
  marks: BudgetMark[];
  over: boolean;
  bar: number | null;
  /** The line can be opened into the establishments that make it up. */
  expandable: boolean;
}

export interface BudgetGroupView {
  key: BudgetGroupKey;
  name: string;
  subtotal: number;
  /** What the group's lines spent, for a group whose subtotal is a cap to compare it with. */
  subtotalSpent: number | null;
  /** The spending went past the cap. */
  over: boolean;
  lines: BudgetLineView[];
}

export interface BudgetView {
  month: YearMonth;
  closed: boolean;
  income: number;
  allocated: number;
  /** Planned investment, as a percent of the income. Negative when the budget outgrows the income. */
  investmentRate: number;
  groups: BudgetGroupView[];
}

/** What one establishment adds to a line in a month. */
export interface BudgetEstablishment {
  key: string;
  name: string;
  amount: number;
  count: number;
}

export interface SavedScenario {
  name: string;
  budget: BudgetDoc;
}

export interface InvestmentFigures {
  investment: number;
  rate: number;
}

/** A scenario weighed against the budget of the month. */
export interface ScenarioImpact {
  current: InvestmentFigures;
  scenario: InvestmentFigures;
  /** Scenario minus current, in money. */
  difference: number;
  /** Per group and per line of the scenario: whether its amount is not the current budget's. */
  changed: boolean[][];
}

export interface ScenarioSaved extends ChangeResult {
  /** The name it was kept under; a scenario without a name gets one. */
  name: string;
}

export interface BudgetApi {
  /** The budget in force in the month, to edit or to start a scenario from. */
  document(month: YearMonth): Promise<BudgetDoc>;
  /** The screen's figures; with a draft, as they would be were the draft the month's budget. */
  view(month: YearMonth, draft?: BudgetDoc): Promise<BudgetView>;
  establishments(month: YearMonth, line: string): Promise<BudgetEstablishment[]>;
  /** The budget holds from the month on. Refused for a closed month. */
  save(month: YearMonth, draft: BudgetDoc): Promise<ChangeResult>;
  scenarios(): Promise<SavedScenario[]>;
  evaluateScenario(month: YearMonth, draft: BudgetDoc): Promise<ScenarioImpact>;
  saveScenario(name: string, draft: BudgetDoc): Promise<ScenarioSaved>;
  /** Makes the scenario the budget from the month on. Refused for a closed month. */
  applyScenario(month: YearMonth, draft: BudgetDoc): Promise<ChangeResult>;
}
