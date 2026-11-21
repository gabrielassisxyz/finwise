export type UnlockResult =
  | { ok: true }
  | { ok: false; reason: 'wrong-pin' }
  | { ok: false; reason: 'locked-out'; retryAfterSeconds: number };

export interface SessionStatus {
  locked: boolean;
  /** Zero unless too many wrong PINs put the keypad on hold. */
  retryAfterSeconds: number;
}

export interface SessionApi {
  status(): Promise<SessionStatus>;
  unlock(pin: string): Promise<UnlockResult>;
  lock(): Promise<void>;
}
