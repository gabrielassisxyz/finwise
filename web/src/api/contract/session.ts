export type UnlockResult =
  | { ok: true }
  | { ok: false; reason: 'wrong-pin' }
  | { ok: false; reason: 'locked-out'; retryAfterSeconds: number };

export interface SessionStatus {
  locked: boolean;
  /** Zero unless too many wrong PINs put the keypad on hold. */
  retryAfterSeconds: number;
}

export type ChangePinResult =
  | { ok: true }
  | { ok: false; reason: 'wrong-current' | 'invalid-new' | 'mismatch' };

export interface SessionApi {
  status(): Promise<SessionStatus>;
  unlock(pin: string): Promise<UnlockResult>;
  lock(): Promise<void>;
  /** After a change the old PIN no longer opens the app. */
  changePin(current: string, next: string, repeat: string): Promise<ChangePinResult>;
}
