/** What every write answers with: the handle that takes it back. */
export interface ChangeResult {
  changeId: string;
}

export interface ChangesApi {
  /** Restores what was there before the change, including any change that extended it. */
  undo(changeId: string): Promise<void>;
}
