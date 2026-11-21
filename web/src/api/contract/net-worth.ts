import type { YearMonth } from './common';
import type { Bucket } from './savings-pots';

/** One thing that makes up a bucket: an account or a pot. */
export interface NetWorthItem {
  /** Null for a pot nobody named. */
  name: string | null;
  /** The bank, or "conta" for a bank account. */
  detail: string;
  amount: number;
}

export interface NetWorthBucket {
  bucket: Bucket;
  total: number;
  items: NetWorthItem[];
}

/** The net worth recorded at the end of one month. */
export interface NetWorthPoint {
  month: YearMonth;
  available: number;
  reserved: number;
  invested: number;
  total: number;
}

export interface MonthContribution {
  planned: number;
  done: number;
  /** Percent of the plan done, capped at 100. */
  progress: number;
}

export interface InvestmentRate {
  month: YearMonth;
  /** Percent of the income the budget plans to invest; null when the budget has no income. */
  planned: number | null;
  /** Percent of the income actually invested so far; null when the budget has no income. */
  actual: number | null;
}

export interface PotYield {
  name: string | null;
  cdiPercent: number | null;
  /** Null when the pot's rate is not known. */
  amount: number | null;
}

export interface NetWorthOverview {
  total: number;
  buckets: NetWorthBucket[];
  chart: {
    /** The value the top of the chart stands for. */
    max: number;
    /** The months the chart has room for, oldest first. */
    months: YearMonth[];
    /** Only the months that have been recorded. */
    points: NetWorthPoint[];
  };
  historyStart: YearMonth;
  contribution: MonthContribution;
  investmentRate: InvestmentRate;
  /** `total` is null when no pot has a known yield. */
  yields: { pots: PotYield[]; total: number | null };
}

export interface NetWorthApi {
  overview(): Promise<NetWorthOverview>;
}
