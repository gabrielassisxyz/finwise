import { CLOSING_DAY_RANGE, MIN_EXAMPLES_RANGE, THRESHOLD_RANGE } from '../contract/settings';
import type { CalibrationBand, SettingsEstablishment, SettingsPerson } from '../types';
import type { FakeState } from './model';

// The rules behind the settings screen: the bounds a value is held to, what
// the calibration suggests, and which establishments the screen lists.

/** A band of confidence is trusted when the model was corrected this seldom in it. */
const MAX_TRUSTED_CORRECTION_PERCENT = 10;

const BAND_WIDTH = 10;

/** The screen lists this many establishments; the rest are reached from a transaction. */
const MAX_ESTABLISHMENTS_LISTED = 10;

const PIX_PREFIX = /^PIX (ENVIADO|RECEBIDO) /;

const clamp = (value: number, range: { min: number; max: number }): number =>
  Math.min(range.max, Math.max(range.min, Math.round(Number.isFinite(value) ? value : range.min)));

export const clampThreshold = (value: number): number => clamp(value, THRESHOLD_RANGE);
export const clampMinExamples = (value: number): number => clamp(value, MIN_EXAMPLES_RANGE);
export const clampClosingDay = (value: number): number => clamp(value, CLOSING_DAY_RANGE);

export function calibrationBands(percents: (number | null)[]): CalibrationBand[] {
  return percents.map((correctionPercent, index) => {
    const from = THRESHOLD_RANGE.min + index * BAND_WIDTH;
    return { from, to: index === percents.length - 1 ? 100 : from + BAND_WIDTH - 1, correctionPercent };
  });
}

/**
 * The lowest threshold from which the model was corrected seldom in every
 * band above it. When even the top band is corrected too often, nothing is
 * safe to apply by itself and the highest allowed threshold is suggested.
 * A band nobody was classified in has no evidence, so it ends the climb, and
 * with no evidence anywhere there is no suggestion.
 */
export function suggestedThreshold(bands: CalibrationBand[]): number | null {
  if (bands.every((band) => band.correctionPercent === null)) return null;
  let suggestion: number = THRESHOLD_RANGE.max;
  for (let index = bands.length - 1; index >= 0; index -= 1) {
    const band = bands[index];
    if (!band || band.correctionPercent === null || band.correctionPercent > MAX_TRUSTED_CORRECTION_PERCENT) break;
    suggestion = band.from;
  }
  return suggestion;
}

/** Each establishment or person once, in the order they first appear; a movement between own accounts is neither. */
function distinctCounterparties(state: FakeState) {
  const seen = new Set<string>();
  return state.transactions.filter((transaction) => {
    if (transaction.movement || seen.has(transaction.establishmentKey)) return false;
    seen.add(transaction.establishmentKey);
    return true;
  });
}

export function listedEstablishments(state: FakeState): SettingsEstablishment[] {
  return distinctCounterparties(state)
    .filter((transaction) => !transaction.person)
    .slice(0, MAX_ESTABLISHMENTS_LISTED)
    .map((transaction) => {
      const known = state.establishments[transaction.establishmentKey];
      return {
        key: transaction.establishmentKey,
        rawText: transaction.raw.replace(PIX_PREFIX, ''),
        nickname: known?.nickname ?? null,
        defaultLine: known?.line ?? null,
        fixedTags: [...(known?.tags ?? [])],
      };
    });
}

export function listedPeople(state: FakeState): SettingsPerson[] {
  return distinctCounterparties(state)
    .filter((transaction) => transaction.person)
    .map((transaction) => ({
      key: transaction.establishmentKey,
      fullName: transaction.name,
      nickname: state.establishments[transaction.establishmentKey]?.nickname ?? null,
    }));
}
