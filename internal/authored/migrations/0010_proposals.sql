-- Proposals submitted by an external agent for the person to approve or refuse
-- on the weekly review. The table is not journaled: answering a proposal is not
-- itself a recorded change, and what an approval applies is journaled as a
-- change of its own under author 'model'.
--
-- Instants are UTC with milliseconds, as the journal writes them.

CREATE TABLE proposals (
  -- AUTOINCREMENT: an id is never reused, so a stale handle never points at a
  -- newer proposal.
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  submitted_at TEXT NOT NULL
               CHECK (submitted_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z'),
  proposer     TEXT NOT NULL CHECK (proposer <> ''),
  method       TEXT NOT NULL CHECK (method <> ''),
  arguments    TEXT NOT NULL CHECK (json_valid(arguments) AND json_type(arguments) = 'array'),
  reason       TEXT NOT NULL CHECK (reason <> ''),
  title        TEXT NOT NULL CHECK (title <> ''),
  based_on     INTEGER NOT NULL CHECK (based_on >= 0),
  outcome      TEXT NOT NULL DEFAULT 'waiting'
               CHECK (outcome IN ('waiting', 'approved', 'refused', 'stale')),
  answered_at  TEXT
               CHECK (answered_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z'),
  refusal      TEXT CHECK (refusal <> ''),
  stale_reason TEXT CHECK (stale_reason <> ''),
  change_id    INTEGER REFERENCES changes (id),
  CHECK (
    (outcome = 'waiting'  AND answered_at IS NULL     AND refusal IS NULL AND stale_reason IS NULL     AND change_id IS NULL) OR
    (outcome = 'approved' AND answered_at IS NOT NULL AND refusal IS NULL AND stale_reason IS NULL     AND change_id IS NOT NULL) OR
    (outcome = 'refused'  AND answered_at IS NOT NULL                     AND stale_reason IS NULL     AND change_id IS NULL) OR
    (outcome = 'stale'    AND answered_at IS NOT NULL AND refusal IS NULL AND stale_reason IS NOT NULL AND change_id IS NULL)
  )
) STRICT;
