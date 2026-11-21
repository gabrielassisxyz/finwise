import type { ChangesApi } from './contract/changes';
import type { HomeApi } from './contract/home';
import type { NetWorthApi } from './contract/net-worth';
import type { SavingsPotsApi } from './contract/savings-pots';
import type { SessionApi } from './contract/session';
import type { ShellApi } from './contract/shell';
import type { TransactionsApi } from './contract/transactions';

export type * from './contract/changes';
export type * from './contract/common';
export type * from './contract/home';
export type * from './contract/net-worth';
export type * from './contract/savings-pots';
export type * from './contract/session';
export type * from './contract/shell';
export type * from './contract/transactions';

/** Everything the interface asks of the backend. */
export interface FinwiseApi {
  session: SessionApi;
  shell: ShellApi;
  home: HomeApi;
  transactions: TransactionsApi;
  changes: ChangesApi;
  savingsPots: SavingsPotsApi;
  netWorth: NetWorthApi;
}
