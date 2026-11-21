import type { AccountsApi } from './contract/accounts';
import type { BudgetApi } from './contract/budget';
import type { CardApi } from './contract/card';
import type { ChangesApi } from './contract/changes';
import type { HomeApi } from './contract/home';
import type { NetWorthApi } from './contract/net-worth';
import type { SavingsPotsApi } from './contract/savings-pots';
import type { SessionApi } from './contract/session';
import type { SettingsApi } from './contract/settings';
import type { ShellApi } from './contract/shell';
import type { SubscriptionsApi } from './contract/subscriptions';
import type { TransactionsApi } from './contract/transactions';

export type * from './contract/accounts';
export type * from './contract/budget';
export type * from './contract/card';
export type * from './contract/changes';
export type * from './contract/common';
export type * from './contract/home';
export type * from './contract/net-worth';
export type * from './contract/savings-pots';
export type * from './contract/session';
export type * from './contract/settings';
export type * from './contract/shell';
export type * from './contract/subscriptions';
export type * from './contract/transaction-detail';
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
  budget: BudgetApi;
  subscriptions: SubscriptionsApi;
  accounts: AccountsApi;
  card: CardApi;
  settings: SettingsApi;
}
