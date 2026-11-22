/** Every way a call can fail. `network` is raised by the client; the others travel on the wire. */
export const API_ERROR_KINDS = [
  'locked',
  'read_only',
  'ledger_unavailable',
  'bad_request',
  'unknown_method',
  'internal',
  'refused',
  'network',
] as const;

export type ApiErrorKind = (typeof API_ERROR_KINDS)[number];

/** The kinds a server can answer with: `network` means no answer arrived. */
export type WireErrorKind = Exclude<ApiErrorKind, 'network'>;

/** Why the server said no to a call it understood; the person reads a sentence the interface keeps for each. */
export const REFUSAL_REASONS = [
  'month_closed',
  'future_month',
  'pending_reviews',
  'stale_change',
  'not_found',
  'unknown_line',
  'invalid_budget',
  'unstable_pot',
  'split_with_reimbursement',
] as const;

export type RefusalReason = (typeof REFUSAL_REASONS)[number];

export interface ApiError {
  kind: ApiErrorKind;
}

/** A wire kind that carries nothing but itself. */
export type PlainWireErrorKind = Exclude<ApiErrorKind, 'network' | 'refused'>;

export interface PlainErrorBody {
  kind: PlainWireErrorKind;
}

export interface RefusedErrorBody {
  kind: 'refused';
  reason: RefusalReason;
}

/** The body of every failed call: `{"error":{"kind":"read_only"}}`, or `{"error":{"kind":"refused","reason":"month_closed"}}`. */
export interface ApiErrorEnvelope {
  error: PlainErrorBody | RefusedErrorBody;
}
