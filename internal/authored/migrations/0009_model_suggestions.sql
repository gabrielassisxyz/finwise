-- What a configured language model suggested for a pending purchase, and how
-- the person answered it in the weekly queue. Kept apart from tx_state and out
-- of the journal: a model's suggestion is not something the person authored,
-- does not count in any figure until accepted, and stays behind when the
-- person settles the transaction so calibration can compare what the model
-- suggested against what the person chose.
--
-- One row per transaction key: a transaction with a row is never asked again.
-- Line, confidence and justification are all set for a usable suggestion, or
-- all null when the transaction was asked and the model gave no usable answer.
-- asked_at is an instant in UTC with milliseconds, as the journal writes them.

CREATE TABLE model_suggestions (
  tx_key           INTEGER PRIMARY KEY NOT NULL REFERENCES tx_index (tx_key),
  model            TEXT NOT NULL CHECK (model <> ''),
  asked_at         TEXT NOT NULL
                   CHECK (asked_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9]Z'),
  line_id          INTEGER REFERENCES lines (id),
  confidence       INTEGER CHECK (confidence BETWEEN 0 AND 100),
  justification    TEXT CHECK (justification <> ''),
  answer           TEXT CHECK (answer IN ('accepted', 'corrected')),
  answer_change_id INTEGER REFERENCES changes (id),
  CHECK (
    (line_id IS NULL AND confidence IS NULL AND justification IS NULL) OR
    (line_id IS NOT NULL AND confidence IS NOT NULL AND justification IS NOT NULL)
  ),
  CHECK ((answer IS NULL) = (answer_change_id IS NULL)),
  CHECK (answer IS NULL OR line_id IS NOT NULL)
) STRICT;
