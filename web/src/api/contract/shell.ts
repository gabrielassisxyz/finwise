import type { IsoDay, YearMonth } from './common';

export type SyncStatus =
  | { state: 'ok'; lastSyncDay: IsoDay; lastSyncTime: string }
  | { state: 'warning'; message: string };

export interface ClosedMonth {
  month: YearMonth;
  closedOn: IsoDay;
}

export interface ShellOverview {
  today: IsoDay;
  pendingCount: number;
  sync: SyncStatus;
  firstMonth: YearMonth;
  lastMonth: YearMonth;
  closedMonths: ClosedMonth[];
}

export interface ShellApi {
  overview(): Promise<ShellOverview>;
}
