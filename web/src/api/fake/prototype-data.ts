import type { ShellOverview } from '../types';

// The dataset of the design prototype. Every figure and name in it is
// invented. The parity check renders these exact values against the
// prototype's screenshots, so a change here is a change to that baseline.

export const PROTOTYPE_PIN = '123456';

export const PROTOTYPE_TODAY = '2026-11-28';

export const PROTOTYPE_SHELL: ShellOverview = {
  today: PROTOTYPE_TODAY,
  pendingCount: 7,
  sync: { state: 'ok', lastSyncDay: PROTOTYPE_TODAY, lastSyncTime: '02:00' },
  firstMonth: '2026-10',
  lastMonth: '2026-12',
  closedMonths: [{ month: '2026-10', closedOn: '2026-11-02' }],
};
