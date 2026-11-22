-- The person's five settings: how sure the classifier must be before it files a
-- purchase by itself, how many examples it needs, the card's label, and how the
-- card's bill closes. One row per setting the person has set; a setting with no
-- row is answered with the value the app starts with, which the Go package
-- states and this file does not.
--
-- value is a whole number for the three numeric keys and a text for the other
-- two. The column has no declared type so that each keeps its own, and typeof
-- says which: an integer that arrived as the text '85' is refused, and so is a
-- label that arrived as a number.

CREATE TABLE settings (
  key   TEXT PRIMARY KEY NOT NULL
        CHECK (key IN ('confidence_threshold', 'min_examples', 'card_label', 'closing_mode', 'manual_closing_day')),
  value ANY NOT NULL,
  CHECK (key <> 'confidence_threshold' OR (typeof(value) = 'integer' AND value BETWEEN 50 AND 99)),
  CHECK (key <> 'min_examples'         OR (typeof(value) = 'integer' AND value BETWEEN 1 AND 50)),
  CHECK (key <> 'manual_closing_day'   OR (typeof(value) = 'integer' AND value BETWEEN 1 AND 28)),
  CHECK (key <> 'card_label'           OR typeof(value) = 'text'),
  CHECK (key <> 'closing_mode'         OR (typeof(value) = 'text' AND value IN ('auto', 'manual')))
) STRICT;
