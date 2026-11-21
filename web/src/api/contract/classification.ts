import type { FinwiseApi } from '../types';

/**
 * - `session`: the PIN gate itself.
 * - `read`: returns data and changes nothing.
 * - `evaluate`: takes a draft, computes a result and stores nothing.
 * - `write`: changes state.
 */
export type MethodClass = 'session' | 'read' | 'evaluate' | 'write';

/** One class for every method of every domain; a missing or surplus entry is a type error. */
export type ApiClassification = {
  [Domain in keyof FinwiseApi]: { [Method in keyof FinwiseApi[Domain]]: MethodClass };
};

/** What each method does to state. Read by the manifest script; the type makes a forgotten method a compile error. */
export const METHOD_CLASSES: ApiClassification = {
  session: {
    status: 'session',
    unlock: 'session',
    lock: 'session',
    changePin: 'write',
  },
  shell: {
    overview: 'read',
  },
  home: {
    summary: 'read',
  },
  transactions: {
    list: 'read',
    filterOptions: 'read',
    setLine: 'write',
    applyLineToEstablishment: 'write',
    setLineForMany: 'write',
    addTagToMany: 'write',
    excludeMany: 'write',
    detail: 'read',
    confirmLine: 'write',
    setReason: 'write',
    setNickname: 'write',
    setDefaultLine: 'write',
    addFixedTag: 'write',
    removeFixedTag: 'write',
    addTag: 'write',
    removeTag: 'write',
    setNote: 'write',
    setSplit: 'write',
    setReimbursable: 'write',
    setExpectedReimbursement: 'write',
    linkReimbursement: 'write',
    unlinkReimbursement: 'write',
    setExcluded: 'write',
    setExclusionReason: 'write',
    setRefund: 'write',
    setRefundLine: 'write',
    undoHistoryStep: 'write',
  },
  changes: {
    undo: 'write',
  },
  savingsPots: {
    list: 'read',
    detail: 'read',
  },
  netWorth: {
    overview: 'read',
  },
  budget: {
    document: 'read',
    view: 'read',
    establishments: 'read',
    save: 'write',
    scenarios: 'read',
    evaluateScenario: 'evaluate',
    saveScenario: 'write',
    applyScenario: 'write',
  },
  subscriptions: {
    list: 'read',
    detail: 'read',
    candidates: 'read',
    recurring: 'read',
    create: 'write',
    acceptSuggestion: 'write',
    rejectSuggestion: 'write',
    setStatus: 'write',
    setLine: 'write',
    setReviewDate: 'write',
    setNote: 'write',
  },
  review: {
    weekly: 'read',
    acceptPending: 'write',
    approve: 'write',
    refuse: 'write',
    propose: 'write',
    dismissWarning: 'write',
    runWarningAction: 'write',
    closing: 'read',
    markStepDone: 'write',
    keepSubscription: 'write',
    cancelSubscription: 'write',
    closeMonth: 'write',
    payday: 'read',
    markTransferDone: 'write',
  },
  accounts: {
    list: 'read',
    detail: 'read',
  },
  card: {
    overview: 'read',
  },
  settings: {
    overview: 'read',
    setPotBucket: 'write',
    toggleLinkedLine: 'write',
    setNickname: 'write',
    setDefaultLine: 'write',
    setCardLabel: 'write',
    setThreshold: 'write',
    setMinExamples: 'write',
    setClosing: 'write',
  },
};
