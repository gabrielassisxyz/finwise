/** Every way a call can fail. `network` is raised by the client; the others travel on the wire. */
export const API_ERROR_KINDS = [
  'locked',
  'read_only',
  'ledger_unavailable',
  'bad_request',
  'unknown_method',
  'internal',
  'network',
] as const;

export type ApiErrorKind = (typeof API_ERROR_KINDS)[number];

/** The kinds a server can answer with: `network` means no answer arrived. */
export type WireErrorKind = Exclude<ApiErrorKind, 'network'>;

export interface ApiError {
  kind: ApiErrorKind;
}

/** The body of every failed call: `{"error":{"kind":"read_only"}}`. */
export interface ApiErrorEnvelope {
  error: { kind: WireErrorKind };
}
