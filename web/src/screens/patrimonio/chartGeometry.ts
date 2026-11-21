import type { NetWorthPoint } from '@/api/types';

export interface StackShare {
  tone: 'invested' | 'reserved' | 'available';
  /** Percent of the column's height. */
  height: number;
}

/** How tall a column reaches, as a percent of the chart; a chart with no top has nothing to reach. */
export function columnHeight(total: number, max: number): number {
  return max > 0 ? (total / max) * 100 : 0;
}

/** The three slices of a column, bottom to top. */
export function stackShares(point: NetWorthPoint): StackShare[] {
  const share = (amount: number) => (point.total ? (amount / point.total) * 100 : 0);
  return [
    { tone: 'invested', height: share(point.invested) },
    { tone: 'reserved', height: share(point.reserved) },
    { tone: 'available', height: share(point.available) },
  ];
}

/** "100 mil", "75 mil", ..., "0": the labels down the left edge, top first. Only the floor is named when there is no top. */
export function axisLabels(max: number): string[] {
  if (max <= 0) return ['', '', '', '', '0'];
  return [1, 0.75, 0.5, 0.25, 0].map((fraction) => (fraction === 0 ? '0' : `${(max * fraction) / 1000} mil`));
}
