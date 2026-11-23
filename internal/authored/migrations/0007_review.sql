-- What the review keeps that only the person marks: the steps of a month's
-- closing ticked by hand, the payday transfers made, and the warnings dismissed
-- for good. Each row is a fact by existing; none holds an amount or names a row
-- of another table, since the transfers and the warnings are derived on every
-- read and only their keys are kept here.
--
-- A month is YYYY-MM and a day YYYY-MM-DD, both in Brazil time.

-- The closing's first step follows the queue and its last is the close itself,
-- so neither is ever ticked by hand.
CREATE TABLE closing_steps (
  month TEXT NOT NULL CHECK (month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  step  TEXT NOT NULL
        CHECK (step IN ('planned-actual', 'leftover', 'contributions', 'card-bill', 'subscriptions', 'report')),
  PRIMARY KEY (month, step)
) STRICT;

CREATE TABLE transfer_marks (
  month        TEXT NOT NULL CHECK (month GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
  transfer_key TEXT NOT NULL CHECK (transfer_key <> ''),
  done_on      TEXT NOT NULL CHECK (done_on GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  PRIMARY KEY (month, transfer_key)
) STRICT;

CREATE TABLE dismissed_warnings (
  id TEXT PRIMARY KEY NOT NULL CHECK (id <> '')
) STRICT;
