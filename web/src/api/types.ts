/** A calendar month as `YYYY-MM`. */
export type YearMonth = string;

/** A calendar day as `YYYY-MM-DD`, in Brazil time. */
export type IsoDay = string;

export type UnlockResult =
  | { ok: true }
  | { ok: false; reason: 'wrong-pin' }
  | { ok: false; reason: 'locked-out'; retryAfterSeconds: number };

export interface SessionStatus {
  locked: boolean;
  /** Zero unless too many wrong PINs put the keypad on hold. */
  retryAfterSeconds: number;
}

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

export interface SessionApi {
  status(): Promise<SessionStatus>;
  unlock(pin: string): Promise<UnlockResult>;
  lock(): Promise<void>;
}

export interface ShellApi {
  overview(): Promise<ShellOverview>;
}

export interface FinwiseApi {
  session: SessionApi;
  shell: ShellApi;
}
