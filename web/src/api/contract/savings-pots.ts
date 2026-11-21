import type { IsoDay, YearMonth } from './common';

/** Where a pot's money counts in the net worth; a pot with none counts nowhere. */
export type Bucket = 'Disponível' | 'Reservado' | 'Investido';

export interface PotTarget {
  amount: number;
  /** Percent of the target reached, capped at 100. */
  progress: number;
}

/** One pot as the list shows it. A pot nobody named arrives with a null name. */
export interface SavingsPotRow {
  id: string;
  name: string | null;
  bank: string;
  bucket: Bucket | null;
  balance: number;
  cdiPercent: number;
  /** What the pot earned this month; zero for a closed pot. */
  monthlyYield: number;
  maturity: IsoDay | null;
  target: PotTarget | null;
  closed: boolean;
}

export interface SavingsPotGroup {
  bucket: Bucket | null;
  subtotal: number;
  pots: SavingsPotRow[];
}

/**
 * Where the month's planned deposit into the pot stands. `switched-off` is a
 * pot that already reached its target, which turns the deposit off.
 */
export type PotContribution =
  | { state: 'closed' }
  | { state: 'switched-off' }
  | { state: 'none-planned' }
  | { state: 'done'; doneOn: IsoDay }
  | { state: 'pending' };

export interface PotBelowTarget {
  target: number;
  shortfall: number;
}

/** A budget line the pot feeds; `share` is its part of the balance when several lines share the pot. */
export interface PotLinkedLine {
  name: string;
  share: number | null;
}

export interface PotInvestment {
  applied: number;
  gross: number;
  tax: number;
  net: number;
  redeemableOn: IsoDay | null;
}

export interface PotMovement {
  day: IsoDay;
  description: string;
  /** Negative for a withdrawal. */
  amount: number;
}

export interface PotYieldDay {
  day: IsoDay;
  amount: number;
}

export interface SavingsPotDetail {
  id: string;
  name: string | null;
  bank: string;
  bucket: Bucket | null;
  cdiPercent: number;
  balance: number;
  /** Present only while the pot has a target and sits under it. */
  belowTarget: PotBelowTarget | null;
  contribution: PotContribution;
  linkedLines: PotLinkedLine[];
  /** Present for an investment only. */
  investment: PotInvestment | null;
  movements: PotMovement[];
  yield: { month: YearMonth; total: number; days: PotYieldDay[] };
  /** The first month the ledger has data for; nothing earlier can appear. */
  historyStart: YearMonth;
}

export interface SavingsPotsApi {
  list(options: { includeClosed: boolean }): Promise<SavingsPotGroup[]>;
  /** Null when no pot has that id. */
  detail(id: string): Promise<SavingsPotDetail | null>;
}
