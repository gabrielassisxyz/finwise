-- The subscriptions the person keeps, each made by hand from a card purchase.
-- Only what the person decided is here: the cycle, the amounts and the next
-- charge are worked out from the ledger's charges on every read, never stored.
--
-- A day is the Brazil day, YYYY-MM-DD. An amount is integer cents. A budget
-- line is named by its id in lines.

-- AUTOINCREMENT: an id is never reused, so a subscription made again after its
-- creation was undone is a new one, and an id a client holds never names it.
-- One subscription per establishment: the key is what ties the card's charges
-- to it, and two would claim the same charges.
CREATE TABLE subscriptions (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  establishment_key   TEXT NOT NULL UNIQUE
                      CHECK (establishment_key GLOB 'doc:*' OR establishment_key GLOB 'name:*' OR establishment_key GLOB 'text:*'),
  created_from_tx_key INTEGER NOT NULL REFERENCES tx_index (tx_key),
  -- The purchase it was made from, as it was then; they stand in for a charge
  -- once the ledger lists none.
  created_day         TEXT NOT NULL
                      CHECK (created_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  created_cents       INTEGER NOT NULL CHECK (created_cents >= 0),
  -- The name it was created under; it does not follow a later nickname.
  name                TEXT NOT NULL,
  -- A line belongs to no month: saving a budget never clears it.
  line_id             INTEGER REFERENCES lines (id),
  status              TEXT NOT NULL CHECK (status IN ('Manter', 'Analisar', 'Backup e cancelar', 'Cancelar')),
  -- The day the status last changed, which a charge made after cancelling is
  -- told by.
  status_day          TEXT NOT NULL
                      CHECK (status_day GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  review_on           TEXT NOT NULL
                      CHECK (review_on GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
  note                TEXT NOT NULL DEFAULT ''
) STRICT;
