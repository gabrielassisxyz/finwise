import type { Bucket } from './savings-pots';

export type AccountKind = 'Conta' | 'Cartão';

/** One account as the list shows it. A card shows its open bill where an account shows its balance. */
export interface AccountRow {
  id: string;
  bank: string;
  name: string;
  kind: AccountKind;
  /** The balance of an account, the open bill of a card. */
  amount: number;
  /** True when the bank has sent nothing for the account in a while. */
  idle: boolean;
}

export interface AccountDetail {
  id: string;
  bank: string;
  name: string;
  /** What `Transaction.account` carries for this account's movements. */
  ledgerAccount: string;
  bucket: Bucket;
  balance: number;
}

export interface AccountsApi {
  list(): Promise<AccountRow[]>;
  /** Null when no account has that id, or when it is the card, which has a page of its own. */
  detail(id: string): Promise<AccountDetail | null>;
}
