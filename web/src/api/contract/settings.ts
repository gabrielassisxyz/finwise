import type { ChangeResult } from './changes';
import type { IsoDay } from './common';
import type { CardClosing, ClosingMode } from './card';
import type { Bucket } from './savings-pots';

export interface SettingsPot {
  id: string;
  name: string | null;
  bank: string;
  cdiPercent: number;
  bucket: Bucket | null;
  linkedLines: string[];
}

export interface SettingsEstablishment {
  key: string;
  /** The bank's text without the PIX prefix. */
  rawText: string;
  nickname: string | null;
  defaultLine: string | null;
  fixedTags: string[];
}

export interface SettingsPerson {
  key: string;
  fullName: string;
  nickname: string | null;
}

/** How often the user corrected the model among classifications whose confidence fell in a band. */
export interface CalibrationBand {
  from: number;
  to: number;
  correctionPercent: number;
}

export interface CategorizationSettings {
  threshold: number;
  minExamples: number;
  calibration: CalibrationBand[];
  suggestedThreshold: number;
}

export interface Connection {
  bank: string;
  status: 'ok' | 'consent-expired';
}

export interface Moment {
  day: IsoDay;
  time: string;
}

export interface DataSettings {
  connections: Connection[];
  lastSync: Moment;
  lastBackup: Moment;
}

export interface SettingsOverview {
  pots: SettingsPot[];
  establishments: SettingsEstablishment[];
  people: SettingsPerson[];
  cardTitle: string;
  cardLabel: string;
  categorization: CategorizationSettings;
  closing: CardClosing;
  data: DataSettings;
}

/** The bounds the backend holds the settings to. */
export const THRESHOLD_RANGE = { min: 50, max: 99 } as const;
export const MIN_EXAMPLES_RANGE = { min: 1, max: 50 } as const;
export const CLOSING_DAY_RANGE = { min: 1, max: 28 } as const;

export interface SettingsApi {
  overview(): Promise<SettingsOverview>;
  setPotBucket(potId: string, bucket: Bucket): Promise<ChangeResult>;
  /** Links the line to the pot, or unlinks it when it was linked. */
  toggleLinkedLine(potId: string, line: string): Promise<ChangeResult>;
  /** An empty nickname removes it. Works for an establishment and for a person. */
  setNickname(key: string, nickname: string): Promise<ChangeResult>;
  setDefaultLine(key: string, line: string): Promise<ChangeResult>;
  setCardLabel(label: string): Promise<ChangeResult>;
  setThreshold(percent: number): Promise<ChangeResult>;
  setMinExamples(count: number): Promise<ChangeResult>;
  setClosing(mode: ClosingMode, manualDay: number): Promise<ChangeResult>;
}
