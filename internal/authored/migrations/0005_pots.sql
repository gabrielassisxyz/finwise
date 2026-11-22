-- What the person decided about a pot: the bucket it counts in, and the budget
-- lines it is funded from. A pot lives in the ledger, so its id is the text the
-- API answers it with and no foreign key can name it.
--
-- A "slot:" id names a different pot from one day to the next, so it is never
-- stored: a choice made about it would silently move to another pot.

-- One row per pot the person gave a bucket. A pot with no row has none, and a
-- row never holds a null bucket in practice: the recorder drops a row back at
-- its default, and the column allows the null only because that is its default.
CREATE TABLE pot_settings (
  pot_id TEXT PRIMARY KEY NOT NULL
         CHECK (pot_id <> '' AND pot_id NOT GLOB 'slot:*'),
  bucket TEXT CHECK (bucket IN ('Disponível', 'Reservado', 'Investido'))
) STRICT;

-- A line is linked to at most one pot: the line is the key, so a second link for
-- it is refused by the table and moving a line is one column changing. line_id
-- is INT and not INTEGER on purpose: a column declared INTEGER PRIMARY KEY is
-- the rowid itself, and a null there is not refused but read as the next free
-- rowid, which can be the id of a real line. A line's link outlives anything
-- but the person's own toggle: a closed pot, a pot gone from the ledger and a
-- line out of the current budget leave their rows alone.
CREATE TABLE pot_lines (
  line_id INT NOT NULL REFERENCES lines (id),
  pot_id  TEXT NOT NULL
          CHECK (pot_id <> '' AND pot_id NOT GLOB 'slot:*'),
  PRIMARY KEY (line_id)
) STRICT;
