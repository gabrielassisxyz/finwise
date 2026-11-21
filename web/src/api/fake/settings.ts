import type { Bucket, ChangeResult, ClosingMode, SettingsApi, SettingsOverview } from '../types';
import { PROTOTYPE_CARD_FULL_LABEL } from './prototype-data';
import type { ChangeLog } from './changes';
import type { FakeEstablishment, FakeState } from './model';
import {
  calibrationBands,
  clampClosingDay,
  clampMinExamples,
  clampThreshold,
  listedEstablishments,
  listedPeople,
  suggestedThreshold,
} from './settings-rules';

function overview(state: FakeState): SettingsOverview {
  const { settings } = state;
  const bands = calibrationBands(settings.calibration);
  return {
    pots: state.savingsPots
      .filter((pot) => !pot.closed)
      .map((pot) => ({
        id: pot.id,
        name: pot.name || null,
        bank: pot.bank,
        cdiPercent: pot.cdiPercent,
        bucket: pot.bucket,
        linkedLines: [...pot.linkedLines],
      })),
    establishments: listedEstablishments(state),
    people: listedPeople(state),
    cardTitle: PROTOTYPE_CARD_FULL_LABEL,
    cardLabel: settings.cardLabel,
    categorization: {
      threshold: state.confidenceThreshold,
      minExamples: settings.minExamples,
      calibration: bands,
      suggestedThreshold: suggestedThreshold(bands),
    },
    closing: { mode: settings.closingMode, bankDay: state.card.bankClosingDay, manualDay: settings.manualClosingDay },
    data: {
      connections: settings.connections.map(({ bank, consentExpired }) => ({
        bank,
        status: consentExpired ? 'consent-expired' : 'ok',
      })),
      lastSync: { ...settings.lastSync },
      lastBackup: { ...settings.lastBackup },
    },
  };
}

export function createFakeSettings(state: FakeState, log: ChangeLog): SettingsApi {
  /** One snapshot, then the change. */
  function change(apply: () => void): ChangeResult {
    const changeId = log.begin();
    apply();
    return { changeId };
  }

  function updateEstablishment(key: string, patch: Partial<FakeEstablishment>): void {
    const merged = { ...state.establishments[key], ...patch };
    for (const field of Object.keys(merged) as (keyof FakeEstablishment)[]) {
      if (merged[field] === undefined) delete merged[field];
    }
    state.establishments[key] = merged;
  }

  return {
    async overview() {
      return overview(state);
    },
    async setPotBucket(potId: string, bucket: Bucket) {
      return change(() => {
        const pot = state.savingsPots.find((candidate) => candidate.id === potId);
        if (pot) pot.bucket = bucket;
      });
    },
    async toggleLinkedLine(potId: string, line: string) {
      return change(() => {
        const pot = state.savingsPots.find((candidate) => candidate.id === potId);
        if (!pot) return;
        pot.linkedLines = pot.linkedLines.includes(line)
          ? pot.linkedLines.filter((linked) => linked !== line)
          : [...pot.linkedLines, line];
        // How the balance was split between the old lines no longer describes the new ones.
        pot.lineShares = null;
      });
    },
    async setNickname(key: string, nickname: string) {
      return change(() => updateEstablishment(key, { nickname: nickname.trim() || undefined }));
    },
    async setDefaultLine(key: string, line: string) {
      return change(() => updateEstablishment(key, { line }));
    },
    async setCardLabel(label: string) {
      return change(() => {
        state.settings.cardLabel = label.trim();
      });
    },
    async setThreshold(percent: number) {
      return change(() => {
        state.confidenceThreshold = clampThreshold(percent);
      });
    },
    async setMinExamples(count: number) {
      return change(() => {
        state.settings.minExamples = clampMinExamples(count);
      });
    },
    async setClosing(mode: ClosingMode, manualDay: number) {
      return change(() => {
        state.settings.closingMode = mode;
        state.settings.manualClosingDay = clampClosingDay(manualDay);
      });
    },
  };
}
