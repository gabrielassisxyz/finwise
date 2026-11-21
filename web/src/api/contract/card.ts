import type { IsoDay, YearMonth } from './common';

/** A card purchase listed under the pay-ahead figure or the ones that may still move. */
export interface CardPurchase {
  transactionId: string;
  /** The nickname when the establishment has one. */
  name: string;
  day: IsoDay;
  /** Signed, as the transaction carries it. */
  amount: number;
}

export interface CardBillItem {
  name: string;
  /** Null for a line that sums up several purchases. */
  day: IsoDay | null;
  amount: number;
}

export interface CardBill {
  /** The month the bill fell due in. */
  month: YearMonth;
  closedOn: IsoDay;
  dueOn: IsoDay;
  total: number;
  items: CardBillItem[];
}

export type ClosingMode = 'auto' | 'manual';

export interface CardOverview {
  title: string;
  openBill: number;
  closesOn: IsoDay;
  dueOn: IsoDay;
  /** The first day whose purchases land on the bill after the open one. */
  bestPurchaseDay: IsoDay;
  limit: number;
  availableLimit: number;
  /** What the purchases since the closing add up to; zero before the month's closing day. */
  payAhead: { amount: number; purchases: CardPurchase[] };
  /** Purchases of the two days before the closing, which the bank may still post to either bill. */
  mayFallOnNextBill: CardPurchase[];
  /** Newest first. */
  bills: CardBill[];
}

export interface CardClosing {
  mode: ClosingMode;
  /** The day the bank's own rule closes the bill on; null when the ledger holds no card. */
  bankDay: number | null;
  /** The day used when the mode is manual. */
  manualDay: number;
}

export interface CardApi {
  /** Null when the ledger holds no card. */
  overview(): Promise<CardOverview | null>;
}
