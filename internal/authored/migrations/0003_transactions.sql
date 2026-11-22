-- The transactions: what the database has seen of the ledger's transactions,
-- each under a key of its own, and what the person, a rule or the model said
-- about each one.
--
-- A day is the Brazil day, YYYY-MM-DD. An amount is integer cents. A budget
-- line is named by its id in lines. An establishment key is doc:, name: or
-- text: followed by the rest of the identity.

-- One row per transaction the database has met: an observation of the ledger,
-- not a decision, so it is not journaled, an undo never removes a row, and the
-- table only grows. Its facts are the ledger row's as last seen, kept for
-- identity, guards and the rules' examples, never for a figure shown or summed.
-- AUTOINCREMENT: a key is never reused, so what was said about one transaction
-- never lands on another.
CREATE TABLE tx_index (
  tx_key            INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id        TEXT NOT NULL CHECK (account_id <> ''),
  day               TEXT NOT NULL
                    CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  cents             INTEGER NOT NULL,
  establishment_key TEXT NOT NULL
                    CHECK (establishment_key GLOB 'doc:*' OR establishment_key GLOB 'name:*' OR establishment_key GLOB 'text:*'),
  -- 1 while the bank still reports the row pending.
  bank_pending      INTEGER NOT NULL CHECK (bank_pending IN (0, 1)),
  -- The collector runs in which the bank listed the row.
  first_run         INTEGER,
  last_run          INTEGER,
  -- 1 once the rules that classify new transactions have met the row, so they
  -- run once per transaction.
  evaluated         INTEGER NOT NULL DEFAULT 0 CHECK (evaluated IN (0, 1))
) STRICT;

-- The bank's ids of a transaction. A pending row and the posted row that
-- replaces it carry two ids and share one key, so what was said about the
-- pending row holds for the posted one; one id names one transaction only.
CREATE TABLE tx_ids (
  provider_id TEXT PRIMARY KEY NOT NULL CHECK (provider_id <> ''),
  tx_key      INTEGER NOT NULL REFERENCES tx_index (tx_key)
) STRICT;

-- What was decided about a transaction, one row of optional facts per key: a
-- row whose every column is at its default is the same as no row.
CREATE TABLE tx_state (
  tx_key             INTEGER PRIMARY KEY NOT NULL REFERENCES tx_index (tx_key),
  line_id            INTEGER REFERENCES lines (id),
  -- Who put the transaction on its line; null for nobody.
  classified_by      TEXT CHECK (classified_by IN ('person', 'rule', 'model')),
  -- The model's, a whole percent.
  confidence         INTEGER CHECK (confidence BETWEEN 0 AND 100),
  -- Set while the transaction waits for a review. The suggestion may be absent.
  pending_reason     TEXT CHECK (pending_reason <> ''),
  pending_line_id    INTEGER REFERENCES lines (id),
  pending_confidence INTEGER CHECK (pending_confidence BETWEEN 0 AND 100),
  -- 1 while the panel asks why the person corrected the line.
  ask_reason         INTEGER NOT NULL DEFAULT 0 CHECK (ask_reason IN (0, 1)),
  note               TEXT NOT NULL DEFAULT '',
  -- Null while the transaction counts; empty while it is switched off without a
  -- reason yet, which does not take effect.
  exclusion          TEXT,
  -- Null while the ledger's rule decides whether it is a refund.
  refund             INTEGER CHECK (refund IN (0, 1)),
  refund_line_id     INTEGER REFERENCES lines (id),
  reimbursable       INTEGER NOT NULL DEFAULT 0 CHECK (reimbursable IN (0, 1)),
  -- Null when the person cleared it.
  expected_cents     INTEGER CHECK (expected_cents >= 0),
  -- The credit that paid the purchase back: one credit pays one purchase.
  credit_tx_key      INTEGER UNIQUE REFERENCES tx_index (tx_key),
  credit_cents       INTEGER CHECK (credit_cents >= 0)
) STRICT;

-- A transaction's own tags. position is the order a tag was added in and is
-- never renumbered: removing a tag leaves the others where they were, and an
-- undo puts it back at its own. Two tags can share one only after the last was
-- removed and another added; the tag itself then orders them.
CREATE TABLE tx_tags (
  tx_key   INTEGER NOT NULL REFERENCES tx_index (tx_key),
  tag      TEXT NOT NULL CHECK (tag <> ''),
  position INTEGER NOT NULL CHECK (position >= 0),
  PRIMARY KEY (tx_key, tag)
) STRICT;

-- The parts of a split purchase, by their place. A part keeps its place when it
-- loses its line, as when a month's budget no longer has the line.
CREATE TABLE tx_split_parts (
  tx_key   INTEGER NOT NULL REFERENCES tx_index (tx_key),
  position INTEGER NOT NULL CHECK (position >= 0),
  line_id  INTEGER REFERENCES lines (id),
  cents    INTEGER NOT NULL CHECK (cents >= 0),
  PRIMARY KEY (tx_key, position)
) STRICT;

-- An establishment or a person, by its key, as one row of optional facts. A
-- blank nickname is no nickname. The default line is stored by id and belongs
-- to no month.
CREATE TABLE establishments (
  key             TEXT PRIMARY KEY NOT NULL
                  CHECK (key GLOB 'doc:*' OR key GLOB 'name:*' OR key GLOB 'text:*'),
  nickname        TEXT CHECK (nickname <> ''),
  default_line_id INTEGER REFERENCES lines (id)
) STRICT;

-- The tags every transaction of an establishment carries, numbered as a
-- transaction's own. No foreign key to establishments: a row there at its
-- defaults is not kept, and the tags outlive it.
CREATE TABLE establishment_tags (
  key      TEXT NOT NULL CHECK (key GLOB 'doc:*' OR key GLOB 'name:*' OR key GLOB 'text:*'),
  tag      TEXT NOT NULL CHECK (tag <> ''),
  position INTEGER NOT NULL CHECK (position >= 0),
  PRIMARY KEY (key, tag)
) STRICT;

-- A transaction's history. A step records what its change did to the
-- transaction, a note that something happened beside it. day is the change's
-- Brazil day and text the sentence written at the time. AUTOINCREMENT: an id is
-- never reused, and a step an undo brings back keeps its own.
CREATE TABLE history_steps (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  tx_key    INTEGER NOT NULL REFERENCES tx_index (tx_key),
  change_id INTEGER NOT NULL REFERENCES changes (id),
  kind      TEXT NOT NULL CHECK (kind IN ('step', 'note')),
  text      TEXT NOT NULL CHECK (text <> ''),
  day       TEXT NOT NULL
            CHECK (day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]')
) STRICT;

CREATE INDEX history_steps_by_tx ON history_steps (tx_key);

-- The reason the person gave for a step, kept beside it so that giving one
-- changes nothing the step's change wrote, which can then still be taken back
-- or extended. It is owned by its step and removed with it by the recorder: a
-- cascading foreign key would remove rows the journal never saw.
CREATE TABLE history_step_reasons (
  step_id INTEGER PRIMARY KEY NOT NULL REFERENCES history_steps (id),
  reason  TEXT NOT NULL CHECK (reason <> '')
) STRICT;
