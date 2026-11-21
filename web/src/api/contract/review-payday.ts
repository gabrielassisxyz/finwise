import type { ChangeResult } from './changes';
import type { IsoDay, YearMonth } from './common';

export type TransferState = 'on-target' | 'done' | 'pending';

export interface PaydayTransfer {
  name: string;
  amount: number;
  state: TransferState;
  doneOn: IsoDay | null;
  /** Marked by the person, not found by the app. */
  markedByHand: boolean;
}

export interface PaydayView {
  /** Fixed and essential lines plus the variable ones: what the account must hold before the salary is set aside. */
  cushion: number;
  /** Null when no account receives the salary. */
  balance: number | null;
  balanceLabel: string | null;
  /** Balance minus cushion; null with no balance. */
  difference: number | null;
  covered: boolean | null;
  transfers: PaydayTransfer[];
}

export interface PaydayApi {
  payday(month: YearMonth): Promise<PaydayView>;
  /** Null when the transfer is unknown or already done. */
  markTransferDone(name: string): Promise<ChangeResult | null>;
}
