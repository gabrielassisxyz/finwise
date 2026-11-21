import { describe, expect, it } from 'vitest';
import { emptyLedgerState, freshInstallState } from './first-run-data';
import { startingState } from './starting-state';

describe('the state the dev server starts in', () => {
  it('is the fresh install for "fresh" and the empty ledger for "empty"', () => {
    expect(startingState('fresh')).toEqual(freshInstallState());
    expect(startingState('empty')).toEqual(emptyLedgerState());
  });

  it('is the prototype dataset, by leaving the choice to the stand-in, when the variable is unset or unknown', () => {
    expect(startingState(undefined)).toBeUndefined();
    expect(startingState('nonsense')).toBeUndefined();
  });
});
