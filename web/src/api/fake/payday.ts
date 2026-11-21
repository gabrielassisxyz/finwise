import type { PaydayApi, PaydayView, YearMonth } from '../types';
import { budgetFor } from './budget-versions';
import type { ChangeLog } from './changes';
import type { FakeState } from './model';
import { groupTotalOf } from './rules';

const cents = (amount: number): number => Math.round(amount * 100) / 100;

/** The account the salary arrives in. */
const SALARY_ACCOUNT_ID = 'itau';

export function createPayday(state: FakeState, log: ChangeLog): PaydayApi {
  return {
    async payday(month: YearMonth): Promise<PaydayView> {
      const groups = budgetFor(state, month);
      const cushion = groupTotalOf(groups, 'fix') + groupTotalOf(groups, 'var');
      const account = state.accounts.find((candidate) => candidate.id === SALARY_ACCOUNT_ID);
      const balance = account?.balance ?? 0;
      const difference = cents(balance - cushion);
      return {
        cushion,
        balance,
        balanceLabel: account ? `${account.bank} · ${account.name}` : '',
        difference,
        covered: difference >= 0,
        transfers: state.plannedContributions.map(({ name, amount, state: transferState, doneOn, markedByHand }) => ({
          name,
          amount,
          state: transferState,
          doneOn,
          markedByHand: !!markedByHand,
        })),
      };
    },

    async markTransferDone(name) {
      const transfer = state.plannedContributions.find((candidate) => candidate.name === name);
      if (!transfer || transfer.state !== 'pending') return null;
      const changeId = log.begin();
      Object.assign(transfer, { state: 'done', doneOn: state.shell.today, markedByHand: true });
      return { changeId };
    },
  };
}
