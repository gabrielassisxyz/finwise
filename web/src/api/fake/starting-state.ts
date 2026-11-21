import { emptyLedgerState, freshInstallState } from './first-run-data';
import type { FakeState } from './model';

/**
 * The state the dev server's stand-in starts in. The caller reads the variable only under
 * `import.meta.env.DEV`, which a production build replaces with `false`: the call and the data
 * behind it fall out of the bundle, and so does the variable's value.
 */
export function startingState(wanted: string | undefined): FakeState | undefined {
  if (wanted === 'fresh') return freshInstallState();
  if (wanted === 'empty') return emptyLedgerState();
  return undefined;
}
