import type { YearMonth } from './common';

export interface MonthLimit {
  /** Sum of the caps of the variable lines. */
  cap: number;
  /** What was spent on those lines. */
  spent: number;
}

export interface WeekGuide {
  label: string;
  guide: number;
  /** Null for a week that has not started. */
  spent: number | null;
  current: boolean;
}

export interface NetWorthSummary {
  available: number;
  reserved: number;
  invested: number;
}

export interface PayAhead {
  amount: number;
  cardLabel: string;
}

export interface HomeSummary {
  limit: MonthLimit;
  weeks: WeekGuide[];
  netWorth: NetWorthSummary;
  pendingCount: number;
  /** Present near the end of the month, when card purchases already fall on the next bill. */
  payAhead: PayAhead | null;
}

export interface HomeApi {
  summary(month: YearMonth): Promise<HomeSummary>;
}
