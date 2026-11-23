export type IsLatest = () => boolean;

export interface LatestLoad {
  /** Starts a load and returns a check that stays true only while nothing newer has started or cancelled it. */
  start(): IsLatest;
  /** Advances the order without starting a load, so any answer still in flight is dropped. */
  cancel(): void;
}

/**
 * Orders overlapping loads for one place. Each `start` supersedes every earlier one;
 * `cancel` supersedes what is in flight when a key is cleared and no new load starts.
 */
export function createLatestLoad(): LatestLoad {
  let latest = 0;

  return {
    start() {
      const request = ++latest;
      return () => request === latest;
    },
    cancel() {
      latest += 1;
    },
  };
}
