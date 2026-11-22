import type { MethodClass } from '../contract/classification';
import { REFUSAL_REASONS, type ApiError, type ApiErrorKind, type RefusalReason } from '../contract/errors';

export interface ApiFailureDetail {
  methodClass?: MethodClass;
  /** Read only for `refused`; anything outside the contract's list is kept as an unknown reason. */
  reason?: unknown;
}

function knownReason(reason: unknown): RefusalReason | null {
  return REFUSAL_REASONS.find((known) => known === reason) ?? null;
}

/** What every failed call rejects with, so a handler can tell the API's failures from a bug. */
export class ApiFailure extends Error implements ApiError {
  readonly kind: ApiErrorKind;
  /** The class of the method that failed; null when whoever raised it did not say, which is shown as a failed read. */
  readonly methodClass: MethodClass | null;
  /** Why a `refused` call was refused; null for every other kind, and for a reason this interface does not know. */
  readonly reason: RefusalReason | null;

  constructor(kind: ApiErrorKind, detail: ApiFailureDetail = {}) {
    super(`API call failed: ${kind}`);
    this.name = 'ApiFailure';
    this.kind = kind;
    this.methodClass = detail.methodClass ?? null;
    this.reason = kind === 'refused' ? knownReason(detail.reason) : null;
  }
}

export function isApiFailure(error: unknown): error is ApiFailure {
  return error instanceof ApiFailure;
}
