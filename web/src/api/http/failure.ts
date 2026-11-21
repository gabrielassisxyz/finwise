import type { ApiError, ApiErrorKind } from '../contract/errors';

/** What every failed call rejects with, so a handler can tell the API's failures from a bug. */
export class ApiFailure extends Error implements ApiError {
  readonly kind: ApiErrorKind;

  constructor(kind: ApiErrorKind) {
    super(`API call failed: ${kind}`);
    this.name = 'ApiFailure';
    this.kind = kind;
  }
}

export function isApiFailure(error: unknown): error is ApiFailure {
  return error instanceof ApiFailure;
}
