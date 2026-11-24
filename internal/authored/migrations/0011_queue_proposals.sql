-- A line an agent proposes for a transaction that waits for review is the weekly
-- queue's suggestion for it, not a card on the approvals list. The proposal is
-- a row of proposals like any other; this table says which transaction it is
-- for and which line it names, so the queue can show it and settling the review
-- can close it. The table is not journaled, for the reason proposals is not:
-- a proposal is a request and its answer, and what the person does about it in
-- the queue is a change of their own.
--
-- The line is kept by id, as a rule's suggestion is, so a rename of the line
-- does not lose it.

-- A proposal the queue closes because something else settled the review has no
-- refusal reason to give: nobody was refused and nothing was applied. The
-- check of 0010 demanded one for every stale proposal, and SQLite cannot change
-- a check in place, so the table is rebuilt without that demand. Proposals are
-- never deleted, so copying every id keeps the next one past all of them.
CREATE TABLE proposals_rebuilt (
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
    (outcome = 'waiting'  AND answered_at IS NULL     AND refusal IS NULL AND stale_reason IS NULL AND change_id IS NULL) OR
    (outcome = 'approved' AND answered_at IS NOT NULL AND refusal IS NULL AND stale_reason IS NULL AND change_id IS NOT NULL) OR
    (outcome = 'refused'  AND answered_at IS NOT NULL                     AND stale_reason IS NULL AND change_id IS NULL) OR
    (outcome = 'stale'    AND answered_at IS NOT NULL AND refusal IS NULL                          AND change_id IS NULL)
  )
) STRICT;

INSERT INTO proposals_rebuilt
  (id, submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, refusal, stale_reason, change_id)
SELECT id, submitted_at, proposer, method, arguments, reason, title, based_on, outcome, answered_at, refusal, stale_reason, change_id
FROM proposals;

DROP TABLE proposals;

ALTER TABLE proposals_rebuilt RENAME TO proposals;

CREATE TABLE queue_proposals (
  proposal_id INTEGER PRIMARY KEY NOT NULL REFERENCES proposals (id),
  tx_key      INTEGER NOT NULL REFERENCES tx_index (tx_key),
  line_id     INTEGER NOT NULL REFERENCES lines (id)
) STRICT;

CREATE INDEX queue_proposals_by_transaction ON queue_proposals (tx_key);
